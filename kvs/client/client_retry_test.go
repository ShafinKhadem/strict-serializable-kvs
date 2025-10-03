package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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

    assert.True(t, finalC2Retries > 0, "C2 should have retried due to lock conflict with C1")

    // Verify final value
    c3 := NewClient([]string{"localhost:8080"})
    c3.Begin()
    finalVal, _ := c3.Get("shared_key")
    c3.Commit()
    fmt.Printf("Final value: %s\n", finalVal)
}