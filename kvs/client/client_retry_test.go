package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

/*
Expected Output (just an example, the real timing may vary):

=== Testing Retry Logic with Forced Conflicts ===
[C2-Attempt-1] Put failed: lock failed (EXPECTED)
[C2-Attempt-2] Put failed: lock failed (EXPECTED)
[C1] SUCCESS after 1 attempts
[C2-Attempt-3] Put failed: lock failed (EXPECTED)
[C2] SUCCESS after 4 attempts

=== Retry Statistics ===
C1 retries: 0
C2 retries: 3

Final value: c2_final

==============================================================================

Corresponding Timeline:

T0 (0ms): initialization
    - shared_key = "initial"
    - start C1 and C2 goroutines

T1 (1ms): C1 attempt 1
    - C1: Begin() → txid: "c1-tx1"
    - C1: Put("shared_key", "c1_value") → success, acquired write lock
    - C1: sent signal to c1Started channel
    - C1: waiting for c2Started channel (blocked)

T2 (2ms): C2 attempt 1
    - C2: received c1Started signal
    - C2: Begin() → txid: "c2-tx1"
    - C2: Put("shared_key", "c2_value") → failed! C1 holds write lock
    - C2: printed "[C2-Attempt-1] Put failed: lock failed (EXPECTED)"
    - C2: sent signal to c2Started channel
    - C2: Abort() → released all locks (even though there were none)
    - C2: c2Retries++
    - C2: Sleep(10ms)

T3 (3ms): C1 continue
    - C1: received c2Started signal
    - C1: Sleep(20ms) ← intentionally holding the lock

T12 (12ms): C2 attempt 2
    - C2: Begin() → txid: "c2-tx2"
    - C2: Put("shared_key", "c2_value") → failed! C1 still holds the lock
    - C2: printed "[C2-Attempt-2] Put failed: lock failed (EXPECTED)"
    - C2: Abort()
    - C2: c2Retries++
    - C2: Sleep(10ms)

T22 (22ms): C2 attempt 3
    - C2: Begin() → txid: "c2-tx3"
    - C2: Put("shared_key", "c2_value") → failed! C1 still holds the lock
    - C2: printed "[C2-Attempt-3] Put failed: lock failed (EXPECTED)"
    - C2: Abort()
    - C2: c2Retries++
    - C2: Sleep(10ms)

T23 (23ms): C1 complete first transaction
    - C1: Get("shared_key") → success, read its own write "c1_value"
    - C1: Put("shared_key", "c1_final") → success, updated value
    - C1: Commit() → success, released all locks
    - C1: printed "[C1] SUCCESS after 1 attempts"
    - C1: exited

T32 (32ms): C2 attempt 4
    - C2: Begin() → txid: "c2-tx4"
    - C2: Put("shared_key", "c2_value") → success! No locks held
    - C2: Get("shared_key") → success, read its own write "c2_value"
    - C2: Put("shared_key", "c2_final") → success
    - C2: Commit() → success
    - C2: printed "[C2] SUCCESS after 4 attempts"
    - C2: exited

T35 (35ms): Verification
    - C3: Read shared_key
    - Final value: "c2_final" (C2 last committed)
*/

func TestRetryLogicWithForcedConflict(t *testing.T) {
    fmt.Println("\n=== Testing Retry Logic with Forced Conflicts ===")
    
    c1 := NewClient([]string{"localhost:8080"})
    c2 := NewClient([]string{"localhost:8080"})
    
    // Init
    c1.Begin()
    c1.Put("shared_key", "initial")
    c1.Commit()
    
    // Channels to coordinate start
    c1Started := make(chan bool)
    c2Started := make(chan bool)
    c1Retries := int32(0)
    c2Retries := int32(0)
    
    var wg sync.WaitGroup
    wg.Add(2)
    
    // Client 1
    go func() {
        defer wg.Done()
        
        attempts := 0
        for attempts < 100 { // max 100 attempts
            attempts++
            
            err := c1.Begin()
            if err != nil {
                atomic.AddInt32(&c1Retries, 1)
                continue
            }
            
            // try to get a write lock on the same key
            err = c1.Put("shared_key", "c1_value")

            // Notify C2 that we have started
            if attempts == 1 {
                c1Started <- true
                // Wait for C2 to start
                <-c2Started
                // Hold the lock for a while to let C2 collide
                time.Sleep(20 * time.Millisecond)
            }
            
            if err != nil {
                fmt.Printf("[C1-Attempt-%d] Put failed: %v\n", attempts, err)
                c1.Abort()
                atomic.AddInt32(&c1Retries, 1)
                time.Sleep(5 * time.Millisecond)
                continue
            }
            
            // 2nd operation
            _, err = c1.Get("shared_key")
            if err != nil {
                fmt.Printf("[C1-Attempt-%d] Get failed: %v\n", attempts, err)
                c1.Abort()
                atomic.AddInt32(&c1Retries, 1)
                time.Sleep(5 * time.Millisecond)
                continue
            }
            
            // 3rd operation
            err = c1.Put("shared_key", "c1_final")
            if err != nil {
                fmt.Printf("[C1-Attempt-%d] Put2 failed: %v\n", attempts, err)
                c1.Abort()
                atomic.AddInt32(&c1Retries, 1)
                time.Sleep(5 * time.Millisecond)
                continue
            }
            
            err = c1.Commit()
            if err != nil {
                fmt.Printf("[C1-Attempt-%d] Commit failed: %v\n", attempts, err)
                atomic.AddInt32(&c1Retries, 1)
                time.Sleep(5 * time.Millisecond)
                continue
            }
            
            fmt.Printf("[C1] SUCCESS after %d attempts\n", attempts)
            break
        }
    }()
    
    // Client 2
    go func() {
        defer wg.Done()
        
        // Wait for C1 to start
        <-c1Started
        
        attempts := 0
        for attempts < 100 {
            attempts++
            
            err := c2.Begin()
            if err != nil {
                atomic.AddInt32(&c2Retries, 1)
                continue
            }
            
            // try to get a write lock on the same key
            err = c2.Put("shared_key", "c2_value")
            
            // Notify C1 that we have started
            if attempts == 1 {
                c2Started <- true
            }
            
            if err != nil {
                fmt.Printf("[C2-Attempt-%d] Put failed: %v (EXPECTED)\n", attempts, err)
                c2.Abort()
                atomic.AddInt32(&c2Retries, 1)
                time.Sleep(10 * time.Millisecond) // Wait longer to let C1 finish
                continue
            }
            
            _, err = c2.Get("shared_key")
            if err != nil {
                fmt.Printf("[C2-Attempt-%d] Get failed: %v\n", attempts, err)
                c2.Abort()
                atomic.AddInt32(&c2Retries, 1)
                time.Sleep(10 * time.Millisecond)
                continue
            }
            
            err = c2.Put("shared_key", "c2_final")
            if err != nil {
                fmt.Printf("[C2-Attempt-%d] Put2 failed: %v\n", attempts, err)
                c2.Abort()
                atomic.AddInt32(&c2Retries, 1)
                time.Sleep(10 * time.Millisecond)
                continue
            }
            
            err = c2.Commit()
            if err != nil {
                fmt.Printf("[C2-Attempt-%d] Commit failed: %v\n", attempts, err)
                atomic.AddInt32(&c2Retries, 1)
                time.Sleep(10 * time.Millisecond)
                continue
            }
            
            fmt.Printf("[C2] SUCCESS after %d attempts\n", attempts)
            break
        }
    }()
    
    wg.Wait()
    
    finalC1Retries := atomic.LoadInt32(&c1Retries)
    finalC2Retries := atomic.LoadInt32(&c2Retries)
    
    fmt.Printf("\n=== Retry Statistics ===\n")
    fmt.Printf("C1 retries: %d\n", finalC1Retries)
    fmt.Printf("C2 retries: %d\n", finalC2Retries)
    
    // at least one should have retried due to lock conflicts
    assert.True(t, finalC1Retries > 0 || finalC2Retries > 0, 
                 "At least one client should have retried due to lock conflict")

    // Verify final value
    c3 := NewClient([]string{"localhost:8080"})
    c3.Begin()
    finalVal, _ := c3.Get("shared_key")
    c3.Commit()
    fmt.Printf("Final value: %s\n", finalVal)
}