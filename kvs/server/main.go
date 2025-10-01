package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"sync"
	"time"

	"github.com/rstutsman/cs6450-labs/kvs"
)

type Stats struct {
	puts    uint64
	gets    uint64
	commits uint64
	aborts  uint64
}

func (s *Stats) Sub(prev *Stats) Stats {
	r := Stats{}
	r.puts = s.puts - prev.puts
	r.gets = s.gets - prev.gets
	r.commits = s.commits - prev.commits
	r.aborts = s.aborts - prev.aborts
	return r
}

type Transaction struct {
	ID       string
	ReadSet  map[string]bool
	WriteSet map[string]string
	Status   string // "active", "committed", "aborted"
}

type LockInfo struct {
	Readers map[string]bool // transaction IDs holding read locks
	Writer  string          // transaction ID holding write lock
}

type KVService struct {
	sync.Mutex
	mp           map[string]string
	stats        Stats
	prevStats    Stats
	lastPrint    time.Time
	transactions map[string]*Transaction
	locks        map[string]*LockInfo
}

func NewKVService() *KVService {
	kvs := &KVService{}
	kvs.mp = make(map[string]string)
	kvs.lastPrint = time.Now()
	kvs.transactions = make(map[string]*Transaction)
	kvs.locks = make(map[string]*LockInfo)
	return kvs
}

func (kv *KVService) Get(request *kvs.GetRequest, response *kvs.GetResponse) error {
	kv.Lock()
	defer kv.Unlock()

	kv.stats.gets++

	// Get or create transaction
	tx, exists := kv.transactions[request.TransactionID]
	if !exists {
		tx = &Transaction{
			ID:       request.TransactionID,
			ReadSet:  make(map[string]bool),
			WriteSet: make(map[string]string),
			Status:   "active",
		}
		kv.transactions[request.TransactionID] = tx
	}

	// Try to acquire read lock
	if !kv.acquireReadLock(request.Key, request.TransactionID) {
		response.LockFail = true
		return nil
	}

	// Add to read set
	tx.ReadSet[request.Key] = true

	// Check if we have a pending write for this key
	if value, exists := tx.WriteSet[request.Key]; exists {
		response.Value = value
	} else if value, found := kv.mp[request.Key]; found {
		response.Value = value
	}

	response.Success = true
	return nil
}

func (kv *KVService) Put(request *kvs.PutRequest, response *kvs.PutResponse) error {
	kv.Lock()
	defer kv.Unlock()

	kv.stats.puts++

	// Get or create transaction
	tx, exists := kv.transactions[request.TransactionID]
	if !exists {
		tx = &Transaction{
			ID:       request.TransactionID,
			ReadSet:  make(map[string]bool),
			WriteSet: make(map[string]string),
			Status:   "active",
		}
		kv.transactions[request.TransactionID] = tx
	}

	// Try to acquire write lock
	if !kv.acquireWriteLock(request.Key, request.TransactionID) {
		response.LockFail = true
		return nil
	}

	// Add to write set
	tx.WriteSet[request.Key] = request.Value

	response.Success = true
	return nil
}

// Helper method to acquire read lock
func (kv *KVService) acquireReadLock(key, txID string) bool {
	lock, exists := kv.locks[key]
	if !exists {
		lock = &LockInfo{
			Readers: make(map[string]bool),
			Writer:  "",
		}
		kv.locks[key] = lock
	}

	// Already have read lock
	if lock.Readers[txID] {
		return true
	}

	// If we have write lock, keep it (don't downgrade)
	if lock.Writer == txID {
		return true // Read is allowed when holding write lock
	}

	// Can acquire read lock if no writer or if we already have read lock
	if lock.Writer == "" || lock.Readers[txID] {
		lock.Readers[txID] = true
		return true
	}

	return false
}

// Helper method to acquire write lock
func (kv *KVService) acquireWriteLock(key, txID string) bool {
	lock, exists := kv.locks[key]
	if !exists {
		lock = &LockInfo{
			Readers: make(map[string]bool),
			Writer:  "",
		}
		kv.locks[key] = lock
	}

	// Can acquire write lock if no other readers and no writer
	if len(lock.Readers) == 0 && lock.Writer == "" {
		lock.Writer = txID
		return true
	}

	// Can upgrade if we already have write lock
	if lock.Writer == txID {
		return true
	}

	// Try to upgrade from read lock to write lock
	if lock.Readers[txID] {
		// Can only upgrade if we're the ONLY reader
		if len(lock.Readers) == 1 {
			delete(lock.Readers, txID)
			lock.Writer = txID
			return true
		}
		// Cannot upgrade with other readers present
		return false
	}

	return false
}

// Helper method to release all locks for a transaction
func (kv *KVService) releaseLocks(txID string) {
	for key, lock := range kv.locks {
		// Remove from readers
		delete(lock.Readers, txID)

		// Remove write lock if we have it
		if lock.Writer == txID {
			lock.Writer = ""
		}

		// Clean up empty lock info
		if len(lock.Readers) == 0 && lock.Writer == "" {
			delete(kv.locks, key)
		}
	}
}

func (kv *KVService) Commit(req *kvs.CommitRequest, resp *kvs.CommitResponse) error {
	kv.Lock()
	defer kv.Unlock()

	tx, exists := kv.transactions[req.TransactionID]
	if !exists {
		resp.Success = false
		return nil
	}

	// Apply all pending writes
	for key, value := range tx.WriteSet {
		kv.mp[key] = value
	}

	// Release all locks
	kv.releaseLocks(req.TransactionID)

	// Update transaction status
	tx.Status = "committed"

	// Update stats (only count if this is the lead participant)
	if req.Lead {
		kv.stats.commits++
	}

	resp.Success = true
	return nil
}

func (kv *KVService) Abort(req *kvs.AbortRequest, resp *kvs.AbortResponse) error {
	kv.Lock()
	defer kv.Unlock()

	tx, exists := kv.transactions[req.TransactionID]
	if !exists {
		resp.Success = false
		return nil
	}

	// Discard all pending writes (they're already in write set, not applied)
	// Just release locks
	kv.releaseLocks(req.TransactionID)

	// Update transaction status
	tx.Status = "aborted"

	// Update stats (only count if this is the lead participant)
	if req.Lead {
		kv.stats.aborts++
	}

	resp.Success = true
	return nil
}

func (kv *KVService) printStats() {
	kv.Lock()
	stats := kv.stats
	prevStats := kv.prevStats
	kv.prevStats = stats
	now := time.Now()
	lastPrint := kv.lastPrint
	kv.lastPrint = now
	kv.Unlock()

	diff := stats.Sub(&prevStats)
	deltaS := now.Sub(lastPrint).Seconds()

	fmt.Printf("get/s %0.2f\nput/s %0.2f\nops/s %0.2f\ncommit/s %0.2f\nabort/s %0.2f\n\n",
		float64(diff.gets)/deltaS,
		float64(diff.puts)/deltaS,
		float64(diff.gets+diff.puts)/deltaS,
		float64(diff.commits)/deltaS,
		float64(diff.aborts)/deltaS)
}

func main() {
	port := flag.String("port", "8080", "Port to run the server on")
	flag.Parse()

	kvs := NewKVService()
	rpc.Register(kvs)
	rpc.HandleHTTP()

	l, e := net.Listen("tcp", fmt.Sprintf(":%v", *port))
	if e != nil {
		log.Fatal("listen error:", e)
	}

	fmt.Printf("Starting KVS server on :%s\n", *port)

	go func() {
		for {
			kvs.printStats()
			time.Sleep(1 * time.Second)
		}
	}()

	http.Serve(l, nil)
}
