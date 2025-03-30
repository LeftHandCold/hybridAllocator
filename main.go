package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/shenjiangwei/hsAllocator/hsAllocator"
)

const (
	TotalSize     = 1024 * 1024 * 1024 * 1024 // 1TB
	MinBlockSize  = 4 * 1024                  // 4KB
	MaxBlockSize  = 4 * 1024 * 1024           // 4MB
	TestIteration = 3
)

// TestResult stores test iteration results
type TestResult struct {
	Iteration     int
	TotalWrites   uint64
	TotalFrees    uint64
	MaxUsage      float64
	FinalUsage    float64
	MemoryUsage   uint64
	TotalDuration time.Duration
}

func runTest(iteration int) TestResult {
	allocator := hsAllocator.NewAllocator()
	allocated := make(map[uint64]uint64) // start -> size
	var mutex sync.Mutex
	var wg sync.WaitGroup

	startTime := time.Now()
	ops := 0
	maxOps := 1000000

	// Start multiple goroutines for concurrent operations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mutex.Lock()
				if ops >= maxOps {
					mutex.Unlock()
					return
				}
				ops++
				mutex.Unlock()

				// Randomly decide whether to allocate or free
				if rand.Float64() < 0.7 { // 70% chance to allocate
					size := uint64(rand.Int63n(MaxBlockSize-MinBlockSize+1) + int64(MinBlockSize))
					start, err := allocator.Allocate(size)
					if err == nil {
						mutex.Lock()
						allocated[start] = size
						mutex.Unlock()
					}
				} else { // 30% chance to free
					mutex.Lock()
					if len(allocated) > 0 {
						// Randomly select an allocated space to free
						keys := make([]uint64, 0, len(allocated))
						for k := range allocated {
							keys = append(keys, k)
						}
						idx := rand.Intn(len(keys))
						start := keys[idx]
						size := allocated[start]
						delete(allocated, start)
						mutex.Unlock()
						allocator.Free(start, size)
					} else {
						mutex.Unlock()
					}
				}
			}
		}()
	}

	wg.Wait()
	duration := time.Since(startTime)

	// Calculate usage statistics
	used := allocator.GetUsedSize()
	memoryUsage := allocator.GetMemoryUsage()

	return TestResult{
		Iteration:     iteration,
		TotalWrites:   uint64(len(allocated)),
		TotalFrees:    uint64(maxOps - len(allocated)),
		MaxUsage:      float64(used) / float64(TotalSize) * 100,
		FinalUsage:    float64(used) / float64(TotalSize) * 100,
		MemoryUsage:   memoryUsage,
		TotalDuration: duration,
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())

	fmt.Printf("Starting disk allocation test with %d iterations\n", TestIteration)
	fmt.Println("Total disk size:", TotalSize/1024/1024/1024, "GB")
	fmt.Println("Min block size:", MinBlockSize/1024, "KB")
	fmt.Println("Max block size:", MaxBlockSize/1024/1024, "MB")
	fmt.Println()

	var results []TestResult
	for i := 0; i < TestIteration; i++ {
		fmt.Printf("Running iteration %d...\n", i+1)
		result := runTest(i + 1)
		results = append(results, result)

		fmt.Printf("Iteration %d results:\n", i+1)
		fmt.Printf("  Total writes: %d\n", result.TotalWrites)
		fmt.Printf("  Total frees: %d\n", result.TotalFrees)
		fmt.Printf("  Max usage: %.2f%%\n", result.MaxUsage)
		fmt.Printf("  Final usage: %.2f%%\n", result.FinalUsage)
		fmt.Printf("  Memory usage: %d bytes\n", result.MemoryUsage)
		fmt.Printf("  Duration: %v\n", result.TotalDuration)
		fmt.Println()
	}

	// Calculate averages
	var avgUsage, avgMemory, avgDuration float64
	for _, r := range results {
		avgUsage += r.FinalUsage
		avgMemory += float64(r.MemoryUsage)
		avgDuration += r.TotalDuration.Seconds()
	}
	avgUsage /= float64(len(results))
	avgMemory /= float64(len(results))
	avgDuration /= float64(len(results))

	fmt.Println("Average results:")
	fmt.Printf("  Average usage: %.2f%%\n", avgUsage)
	fmt.Printf("  Average memory usage: %.2f bytes\n", avgMemory)
	fmt.Printf("  Average duration: %.2f seconds\n", avgDuration)
}
