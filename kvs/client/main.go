package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/rpc"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rstutsman/cs6450-labs/kvs"
)

type Client struct {
	rpcClient         *rpc.Client
	activeTransaction string            // current active transaction ID
	writeSet          map[string]string // local write set
	participants      []*rpc.Client     // list of participating servers
	clientID          string
	hosts             []string               // list of all server hosts
	connCache         map[string]*rpc.Client // cache of RPC clients by host
}

func Dial(addr string) *Client {
	rpcClient, err := rpc.DialHTTP("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}

	return &Client{
		rpcClient:         rpcClient,
		activeTransaction: "",
		writeSet:          make(map[string]string),
		participants:      nil,
		clientID:          "",
		hosts:             nil,
	}
}

func NewClient(hosts []string) *Client {
	// Connect to the first host initially
	client := Dial(hosts[0])
	client.hosts = hosts
	client.clientID = fmt.Sprintf("%d", rand.Int63())
	client.connCache = make(map[string]*rpc.Client)
	return client
}

func (client *Client) getConnection(addr string) (*rpc.Client, error) {
	if conn, exists := client.connCache[addr]; exists {
		return conn, nil
	}

	conn, err := rpc.DialHTTP("tcp", addr)
	if err != nil {
		return nil, err
	}

	client.connCache[addr] = conn
	return conn, nil
}

func (c *Client) Begin() error {
	if c.activeTransaction != "" {
		return fmt.Errorf("Cannot begin transaction: already in transaction")
	}

	// Generate unique transaction ID
	txID := fmt.Sprintf("%s-%d", c.clientID, time.Now().UnixNano())
	c.activeTransaction = txID

	// Initialize transaction state
	c.writeSet = make(map[string]string)
	c.participants = make([]*rpc.Client, 0)
	return nil
}

func (c *Client) Commit() error {
	if c.activeTransaction == "" {
		panic("Cannot commit: no active transaction")
	}

	// Phase 2 of 2PC: Send commit to all participants
	success := true
	for i, participant := range c.participants {
		req := kvs.CommitRequest{
			TransactionID: c.activeTransaction,
			Lead:          i == 0, // First participant is the lead
		}
		resp := kvs.CommitResponse{}
		err := participant.Call("KVService.Commit", &req, &resp)
		if err != nil || !resp.Success {
			success = false
			break
		}
	}

	// Clear transaction state
	c.activeTransaction = ""
	c.writeSet = nil
	c.participants = nil

	if !success {
		return fmt.Errorf("commit failed")
	}
	return nil
}

func (c *Client) Abort() error {
	if c.activeTransaction == "" {
		fmt.Println("Warning: Abort called with no active transaction")
		return fmt.Errorf("Cannot abort: no active transaction")
	}

	// Phase 2 of 2PC: Send abort to all participants
	for _, participant := range c.participants {
		req := kvs.AbortRequest{
			TransactionID: c.activeTransaction,
		}
		resp := kvs.AbortResponse{}
		participant.Call("KVService.Abort", &req, &resp)
		// Don't check for errors on abort - just try to clean up
	}

	// Clear transaction state
	c.activeTransaction = ""
	c.writeSet = make(map[string]string)
	c.participants = make([]*rpc.Client, 0)

	return nil
}

func (client *Client) Get(key string) (string, error) {
	if client.activeTransaction == "" {
		return "", fmt.Errorf("Cannot get: no active transaction")
	}

	// Check write set first (read own writes)
	if value, exists := client.writeSet[key]; exists {
		return value, nil
	}

	// Determine which server to contact based on key
	serverAddr := client.getServerForKey(key)
	rpcClient, err := client.getConnection(serverAddr)
	if err != nil {
		return "", err
	}

	// Add to participants if not already there
	client.addParticipant(rpcClient)

	request := kvs.GetRequest{
		Key:           key,
		TransactionID: client.activeTransaction,
	}
	response := kvs.GetResponse{}
	err = rpcClient.Call("KVService.Get", &request, &response)
	if err != nil {
		return "", err
	}

	if response.LockFail {
		// Lock failed, abort transaction automatically
		// client.Abort()
		return "", fmt.Errorf("lock failed")
	}

	return response.Value, nil
}

func (client *Client) Put(key string, value string) error {
	if client.activeTransaction == "" {
		return fmt.Errorf("Cannot put: no active transaction")
	}

	// Add to local write set (read own writes)
	client.writeSet[key] = value

	// Determine which server to contact based on key
	serverAddr := client.getServerForKey(key)
	rpcClient, err := client.getConnection(serverAddr)
	if err != nil {
		return err
	}

	// Add to participants if not already there
	client.addParticipant(rpcClient)

	request := kvs.PutRequest{
		Key:           key,
		Value:         value,
		TransactionID: client.activeTransaction,
	}
	response := kvs.PutResponse{}
	err = rpcClient.Call("KVService.Put", &request, &response)
	if err != nil {
		return err
	}

	if response.LockFail {
		// Lock failed, abort transaction automatically
		// client.Abort()
		return fmt.Errorf("lock failed")
	}

	return nil
}

// Helper method to determine which server to contact for a key
func (client *Client) getServerForKey(key string) string {
	// If no hosts configured, use the current connection
	if client.hosts == nil || len(client.hosts) == 0 {
		return "localhost:8080" // Default for single server tests
	}

	// Simple hash-based sharding
	hash := 0
	for _, c := range key {
		hash = hash*31 + int(c)
	}
	if hash < 0 {
		hash = -hash
	}
	return client.hosts[hash%len(client.hosts)]
}

// Helper method to add a participant if not already present
func (client *Client) addParticipant(rpcClient *rpc.Client) {
	// Check if this client is already in participants
	for _, p := range client.participants {
		if p == rpcClient {
			return
		}
	}
	client.participants = append(client.participants, rpcClient)
}

func runClient(id int, hosts []string, done *atomic.Bool, workload *kvs.Workload, resultsCh chan<- uint64) {
	client := NewClient(hosts)
	value := strings.Repeat("x", 128)
	const batchSize = 1024
	const maxRetries = 100
	opsCompleted := uint64(0)

	for !done.Load() {
		for j := 0; j < batchSize; j++ {
			// Pre-generate the 3 operations for this transaction
			ops := make([]kvs.WorkloadOp, 3)
			for k := 0; k < 3; k++ {
				ops[k] = workload.Next()
			}

			// Retry loop for the same transaction
			retryCount := 0
			for {
				retryCount++
				if retryCount > 1 && retryCount <= 10 {
					fmt.Printf("Client %d: Retrying transaction (attempt %d)\n", id, retryCount)
				}

				// start new transaction
				err := client.Begin()
				if err != nil {
					continue
				}

				success := true
				// failedAt := -1
				for k := 0; k < 3; k++ {
					key := fmt.Sprintf("%d", ops[k].Key)
					if ops[k].IsRead {
						fmt.Printf("Client %d: Attempting Get(%s)\n", id, key)
						_, err := client.Get(key)
						if err != nil {
							fmt.Printf("Client %d: Get(%s) failed: %v\n", id, key, err)
							// failedAt = k
							// client.Abort()
							success = false
							break
						}
					} else {
						fmt.Printf("Client %d: Attempting Put(%s)\n", id, key)
						err := client.Put(key, value)
						if err != nil {
							fmt.Printf("Client %d: Put(%s) failed: %v\n", id, key, err)
							// failedAt = k
							// client.Abort()
							success = false
							break
						}
					}
				}

				if success {
					err = client.Commit()
					if err == nil {
						if retryCount > 10 {
							fmt.Printf("Client %d: Transaction finally committed after %d attempts\n",
								id, retryCount)
						}
						break
					}
				} else {
					client.Abort()
					// Exponential backoff, max 100ms
					backoff := time.Duration(1<<uint(min(retryCount, 7))) * time.Millisecond
					if backoff > 100*time.Millisecond {
						backoff = 100 * time.Millisecond
					}
					time.Sleep(backoff)
				}

				if retryCount >= maxRetries {
					// Skip this transaction. Usually not expected to happen
					// unless the system is overloaded or there's a bug.
					fmt.Printf("Client %d: Giving up on transaction after %d retries\n",
						id, maxRetries)

					break
				}
			}

			opsCompleted++
		}
	}

	fmt.Printf("Client %d finished operations.\n", id)
	resultsCh <- opsCompleted
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func runPaymentClient(id int, hosts []string, done *atomic.Bool, resultsCh chan<- uint64) {
	client := NewClient(hosts)

	// Initialize accounts if this is client 0
	if id == 0 {
		err := client.Begin()
		if err == nil {
			for i := 0; i < 10; i++ {
				client.Put(fmt.Sprintf("account_%d", i), "1000")
			}
			client.Put("initialized", "true")
			client.Commit()
		}
	} else {
		// Wait until client 0 finishes initialization
		for {
			err := client.Begin()
			if err != nil {
				continue
			}
			initializedStr, err := client.Get("initialized")
			client.Commit()
			if err == nil && initializedStr == "true" {
				break
			}
		}
	}

	fmt.Printf("Payment client %d starting\n", id)
	opsCompleted := uint64(0)

	for !done.Load() {
		// Transfer transaction
		err := client.Begin()
		if err != nil {
			continue
		}

		src := id
		dst := (id + 1) % 10

		fmt.Printf("Payment client %d: transferring $100 from account_%d to account_%d\n", id, src, dst)

		srcBalStr, err := client.Get(fmt.Sprintf("account_%d", src))
		if err != nil {
			client.Abort()
			continue
		}

		// Convert to int (simplified)
		srcBal := 1000 // Default value for simplicity
		if srcBalStr != "" {
			fmt.Sscanf(srcBalStr, "%d", &srcBal)
		}

		if srcBal < 100 {
			client.Abort()
			continue
		}

		// Update source account balance
		err = client.Put(fmt.Sprintf("account_%d", src), fmt.Sprintf("%d", srcBal-100))
		if err != nil {
			client.Abort()
			continue
		}

		dstBalStr, err := client.Get(fmt.Sprintf("account_%d", dst))
		if err != nil {
			client.Abort()
			continue
		}

		dstBal := 1000 // Default value for simplicity
		if dstBalStr != "" {
			fmt.Sscanf(dstBalStr, "%d", &dstBal)
		}

		err = client.Put(fmt.Sprintf("account_%d", dst), fmt.Sprintf("%d", dstBal+100))
		if err != nil {
			client.Abort()
			continue
		}

		err = client.Commit()
		if err != nil {
			continue
		}

		opsCompleted++

		// Balance check transaction
		err = client.Begin()
		if err != nil {
			continue
		}
		total := 0
		balances := make([]int, 10)

		for i := 0; i < 10; i++ {
			balStr, err := client.Get(fmt.Sprintf("account_%d", i))
			if err != nil {
				client.Abort()
				break
			}

			bal := 1000
			if balStr != "" {
				fmt.Sscanf(balStr, "%d", &bal)
			}
			balances[i] = bal
			total += bal
		}

		fmt.Printf("Balances: %v\n", balances)

		// Check for negative balances
		for i, bal := range balances {
			if bal < 0 {
				fmt.Printf("ERROR: account_%d has negative balance %d\n", i, bal)
			}
		}

		// Check total balance invariant
		if total != 10000 {
			fmt.Printf("ERROR: Total balance is %d, expected 10000\n", total)
		}

		client.Commit()
	}

	fmt.Printf("Payment client %d finished operations.\n", id)
	resultsCh <- opsCompleted
}

type HostList []string

func (h *HostList) String() string {
	return strings.Join(*h, ",")
}

func (h *HostList) Set(value string) error {
	*h = strings.Split(value, ",")
	return nil
}

func main() {
	hosts := HostList{}

	flag.Var(&hosts, "hosts", "Comma-separated list of host:ports to connect to")
	theta := flag.Float64("theta", 0.99, "Zipfian distribution skew parameter")
	workload := flag.String("workload", "YCSB-B", "Workload type (YCSB-A, YCSB-B, YCSB-C)")
	secs := flag.Int("secs", 30, "Duration in seconds for each client to run")
	flag.Parse()

	if len(hosts) == 0 {
		hosts = append(hosts, "localhost:8080")
	}

	fmt.Printf(
		"hosts %v\n"+
			"theta %.2f\n"+
			"workload %s\n"+
			"secs %d\n",
		hosts, *theta, *workload, *secs,
	)

	start := time.Now()

	done := atomic.Bool{}
	resultsCh := make(chan uint64)

	if *workload == "xfer" {
		for clientId := 0; clientId < 10; clientId++ {
			go runPaymentClient(clientId, hosts, &done, resultsCh)
		}
	} else {
		clientId := 0
		go func(clientId int) {
			workload := kvs.NewWorkload(*workload, *theta)
			runClient(clientId, hosts, &done, workload, resultsCh)
		}(clientId)
	}

	time.Sleep(time.Duration(*secs) * time.Second)
	done.Store(true)

	opsCompleted := <-resultsCh

	elapsed := time.Since(start)

	opsPerSec := float64(opsCompleted) / elapsed.Seconds()
	fmt.Printf("throughput %.2f ops/s\n", opsPerSec)
}
