package main

import (
	"fmt"
	"hybridAllocator/hybrid"
	"hybridAllocator/mpool"
	"log"
	"math/rand"
	"os"
	"runtime/pprof"
	"sync"
	"time"
)

const (
	TB = 1024 * 1024 * 1024 * 1024
	GB = 1024 * 1024 * 1024
	MB = 1024 * 1024
	KB = 1024

	MinBlockSize  = 4 * KB // 4KB
	MaxBlockSize  = 4 * MB // 4MB
	TestIteration = 1
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

func p2roundup(x uint64, align uint64) uint64 {
	return -(-x & -align)
}

func generateRandomSize() uint64 {
	maxBlocks := MaxBlockSize / 512
	numBlocks := uint64(rand.Intn(maxBlocks)) + 1
	size := numBlocks * 512
	if size < 4096 {
		p2roundup(numBlocks*512, 4096)
	}
	return size
}

func runTest(iteration int) TestResult {
	allocator := hybrid.NewAllocator()

	memoryPool, err := mpool.NewMemoryPool(allocator)
	if err != nil {
		log.Fatalf("Failed to create memory pool: %v", err)
	}
	defer memoryPool.Close()

	allocated := make(map[uint64]uint64) // start -> size
	diskSize := allocator.GetTotalSize()

	var totalWritten, totalAllocated uint64
	var writeCount, deleteCount int
	var printThreshold uint64 = 10 * GB
	var mutex sync.Mutex
	var wg sync.WaitGroup

	startTime := time.Now()
	ops := 0
	maxOps := 2000000
	startPrint := time.Now()
	// Start multiple goroutines for concurrent operations
	for i := 0; i < 32; i++ {
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
					size := generateRandomSize()
					start, err := memoryPool.Allocate(size)
					if err == nil {
						mutex.Lock()
						allocated[start] = size
						totalWritten += size
						totalAllocated += size
						writeCount++
						if totalAllocated >= printThreshold {
							used := allocator.GetUsedSize()
							use := float64(used) / float64(diskSize) * 100
							elapsed := time.Since(startPrint)
							hybrid.Info(
								"%d MB allocated, cumulative writes: %d, cumulative frees: %d\n"+
									"Duration: %v, Space usage: %.5f%%\n"+
									"-----------------------------------------",
								totalAllocated/MB,
								writeCount,
								deleteCount,
								elapsed.Round(time.Millisecond),
								use)

							printThreshold += 10 * GB
							startPrint = time.Now()
						}
						mutex.Unlock()
					} else {
						if err == hybrid.ErrNoSpaceAvailable {
							err = nil
							break
						}
						panic(fmt.Sprintf("Failed to Allocate. err: %v", err))
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
						totalWritten -= size
						deleteCount++
						mutex.Unlock()
						err := memoryPool.Free(start, size)
						if err != nil {
							panic(fmt.Sprintf("Failed to Free. offset: %d, err: %v", start, err))
						}
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
	hybrid.Info("used is %v", used)
	memoryUsage := allocator.GetMemoryUsage()

	return TestResult{
		Iteration:     iteration,
		TotalWrites:   uint64(writeCount),
		TotalFrees:    uint64(deleteCount),
		MaxUsage:      float64(used) / float64(diskSize) * 100,
		FinalUsage:    float64(used) / float64(diskSize) * 100,
		MemoryUsage:   memoryUsage,
		TotalDuration: duration,
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())
	cpuProfile, err := os.Create("cpu.prof")
	if err != nil {
		log.Fatal("could not create CPU profile: ", err)
	}
	defer cpuProfile.Close()

	if err := pprof.StartCPUProfile(cpuProfile); err != nil {
		log.Fatal("could not start CPU profile: ", err)
	}
	defer pprof.StopCPUProfile()

	memProfile, err := os.Create("mem.prof")
	if err != nil {
		log.Fatal("could not create memory profile: ", err)
	}
	defer memProfile.Close()

	fmt.Printf("Starting disk allocation test with %d iterations\n", TestIteration)
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
		fmt.Printf("  Max usage: %.5f%%\n", result.MaxUsage)
		fmt.Printf("  Final usage: %.5f%%\n", result.FinalUsage)
		fmt.Printf("  Memory usage: %d bytes\n", result.MemoryUsage)
		fmt.Printf("  Duration: %v\n", result.TotalDuration)
		fmt.Println()
	}
	if err := pprof.WriteHeapProfile(memProfile); err != nil {
		log.Fatal("could not write memory profile: ", err)
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
	fmt.Printf("  Average usage: %.5f%%\n", avgUsage)
	fmt.Printf("  Average memory usage: %.2f bytes\n", avgMemory)
	fmt.Printf("  Average duration: %.2f seconds\n", avgDuration)
}
