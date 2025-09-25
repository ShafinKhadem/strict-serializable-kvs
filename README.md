## Core Features

### 1. Two-Phase Locking (2PL)

**How it works:**
- Multiple readers can read the same data at once.
- Only one writer can update a piece of data at a time.
- Locks are taken during the transaction and released only when it ends.

**Conflicts:**
- Write vs. Read → writer fails.
- Read vs. Write → reader fails.
- No waiting — failed transactions just restart.

**Why it matters:** Keeps data consistent and avoids "dirty" reads/writes.

### 2. Two-Phase Commit (2PC)

**Phase 1 (Prepare):**
- Servers lock data and prepare changes but don't apply them yet.
- If any server can't lock → whole transaction is aborted.

**Phase 2 (Commit/Abort):**
- If all are ready → changes are applied everywhere.
- If one fails → nothing is applied.

**Why it matters:** Guarantees atomicity — either all servers commit or none do.

### 3. Transaction Management

**Start:** Create unique transaction ID, track reads/writes, mark as active.

**Read:**
- Check your own pending writes first (read-your-own-writes).
- Otherwise, lock the key on the server.

**Write:**
- Save changes locally first.
- Lock the key on the server.
- Real changes only happen at commit.

**Commit:** Apply all writes and release locks.

**Abort:** Discard all writes and release locks.

**Why it matters:** Ensures ACID (Atomicity, Consistency, Isolation, Durability).

### 4. Conflict Detection & Resolution

- **Write-Write:** Two writers → second one fails, first succeeds.
- **Read-Write:** If someone read it first → writer fails, reader succeeds.
- **Retry:** Failed transactions automatically restart.

**Why it matters:** Keeps strict serializability (as if transactions happened one at a time in order).

## Usage

### Prerequisites

This project requires Go 1.21 or later. If Go is not installed:

```bash
# Download and install Go 1.21.5 (or later)
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
tar -C $HOME -xzf go1.21.5.linux-amd64.tar.gz

# Set up environment variables
export PATH=$PATH:$HOME/go/bin
export GOROOT=$HOME/go
export GOPATH=$HOME/go-workspace
export GOMODCACHE=$HOME/go-workspace/pkg/mod
export GOTOOLCHAIN=local
```

### Building

The project has been configured to work with Go 1.21. Build the binaries:

```bash
# Build server
go build -v -o bin/kvsserver ./kvs/server

# Build client  
go build -v -o bin/kvsclient ./kvs/client
```

### Running Tests

```bash
# Start server in background
./bin/kvsserver -port 8080 &

# Run unit tests
go test ./kvs/client -v

# Stop server when done
pkill -f kvsserver
```

### Running Workloads

Use the `run-cluster.sh` script to run distributed workloads:

```bash
# YCSB-B workload (95% reads, 5% writes) - 2 servers, 1 client, 30 seconds
./run-cluster.sh 2 1 "" "-workload YCSB-B -secs 30"

# YCSB-A workload (50% reads, 50% writes) - 2 servers, 1 client, 30 seconds  
./run-cluster.sh 2 1 "" "-workload YCSB-A -secs 30"

# YCSB-C workload (100% reads) - 2 servers, 1 client, 30 seconds
./run-cluster.sh 2 1 "" "-workload YCSB-C -secs 30"

# Payment workload (strict serializability testing) - 2 servers, 1 client, 30 seconds
./run-cluster.sh 2 1 "" "-workload xfer -secs 30"
```

### Script Parameters

The `run-cluster.sh` script takes 4 parameters:
- **Server count**: Number of server instances to run
- **Client count**: Number of client instances to run  
- **Server args**: Additional arguments for servers (use `""` for none)
- **Client args**: Workload configuration for clients

### Expected Output

The script will:
1. Start the specified number of servers on ports 8080, 8081, etc.
2. Start the specified number of clients with the given workload
3. Run for the specified duration
4. Display throughput statistics (ops/s) for each server and total
5. Clean up all processes when finished

Example output:
```
Using 3 of 4 available nodes (node0 to node3)
Server nodes: localhost localhost
Client nodes: localhost
Server args: 
Client args: -workload YCSB-B -secs 30

Starting server 0...
Starting server 1...
Starting client 0...
Waiting for clients to finish...
All clients finished.

0 median 422 op/s
1 median 382 op/s
total 803 op/s
```

## Architecture

- **Client**: Handles transaction logic, conflict detection, and retry
- **Server**: Manages locking, data storage, and 2PC coordination
- **Sharding**: Hash-based key distribution across multiple servers
- **Statistics**: Real-time monitoring of commits/aborts per second

---
*Last updated: September 25, 2025*
*Branch: feature/updated-readme*


