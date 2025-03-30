package main

import (
	"flag"
	"fmt"
	"hybridAllocator/hybrid"
	"hybridAllocator/mpool"
	"hybridAllocator/rpc"
	"log"
	"math/rand"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
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
	TestIteration = 2

	ServerAddress = "localhost:1234"
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

type StressTest struct {
	allocator *hybrid.Allocator
	pool      *mpool.MemoryPool
	allocated map[uint64]uint64 // start -> size
	mu        sync.Mutex
}

func NewStressTest() *StressTest {
	allocator := hybrid.NewAllocator()
	mp, _ := mpool.NewMemoryPool(allocator)
	return &StressTest{
		allocator: allocator,
		pool:      mp,
		allocated: make(map[uint64]uint64),
	}
}

func (st *StressTest) runStressTest(targetSize uint64) {
	log.Printf("Starting stress test with target size: %d TB", targetSize/(1024*1024*1024*1024))

	startTime := time.Now()
	totalWritten := uint64(0)
	iteration := 0

	for totalWritten < targetSize {
		start := time.Now()
		iteration++
		log.Printf("Iteration %d: Starting allocation phase", iteration)
		used := uint64(0)
		for {
			size := generateRandomSize()
			start, err := st.allocator.Allocate(size)
			if err != nil {
				if strings.Contains(err.Error(), "no space available") {
					err = nil
					used = st.allocator.GetUsedSize()
					break
				}
				panic(fmt.Sprintf("Failed to Allocate. err: %v", err))
			}

			st.mu.Lock()
			st.allocated[start] = size
			st.mu.Unlock()
			totalWritten += size
		}

		releaseRatio := 0.3 + rand.Float64()*0.2 // 30%-50%
		st.mu.Lock()
		toRelease := make([]uint64, 0, len(st.allocated))
		for start := range st.allocated {
			toRelease = append(toRelease, start)
		}
		st.mu.Unlock()

		releaseCount := int(float64(len(toRelease)) * releaseRatio)
		for i := 0; i < releaseCount; i++ {
			idx := rand.Intn(len(toRelease))
			start := toRelease[idx]

			st.mu.Lock()
			size := st.allocated[start]
			delete(st.allocated, start)
			st.mu.Unlock()

			st.allocator.Free(start, size)
			toRelease[idx] = toRelease[len(toRelease)-1]
			toRelease = toRelease[:len(toRelease)-1]
		}

		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		duration := time.Since(start)
		usage := float64(used) / float64(st.allocator.GetTotalSize()) * 100
		log.Printf("Iteration %d completed:", iteration)
		log.Printf("  Total written: %d TB", totalWritten/(1024*1024*1024*1024))
		log.Printf("  usage: %.5f%%\n", usage)
		log.Printf("  Current memory usage: %d MB", m.Alloc/1024/1024)
		log.Printf("  Duration: %v", duration)
		log.Printf("  Average write speed: %.2f MB/s", float64(totalWritten)/time.Since(startTime).Seconds()/1024/1024)
	}
	log.Printf("  Total Duration: %v", time.Since(startTime))
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
	var allocator *hybrid.Allocator
	var memoryPool *mpool.MemoryPool
	var err error

	var Allocate func(uint64) (uint64, error)
	var Free func(uint64, uint64) error
	var GetUsedSize func() uint64
	var GetMemoryUsage func() uint64

	if iteration == 0 {
		allocator = hybrid.NewAllocator()
		memoryPool, err = mpool.NewMemoryPool(allocator)
		Allocate = memoryPool.Allocate
		Free = memoryPool.Free
		GetUsedSize = allocator.GetUsedSize
		GetMemoryUsage = allocator.GetMemoryUsage
		defer memoryPool.Close()
	} else {
		server, err := rpc.NewServer()
		if err != nil {
			log.Fatalf("Failed to create server: %v", err)
		}
		defer server.Close()

		go func() {
			if err := server.Start(ServerAddress); err != nil {
				log.Printf("Server error: %v", err)
			}
		}()

		time.Sleep(time.Second)

		client, err := rpc.NewClient(1, ServerAddress)
		if err != nil {
			log.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		Allocate = client.Allocate
		Free = client.Free
		GetUsedSize = server.GetUsedSize
		GetMemoryUsage = server.GetMemoryUsage
	}

	if err != nil {
		log.Fatalf("Failed to create memory pool: %v", err)
	}

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
					start, err := Allocate(size)
					if err == nil {
						mutex.Lock()
						allocated[start] = size
						totalWritten += size
						totalAllocated += size
						writeCount++
						if totalAllocated >= printThreshold {
							used := GetUsedSize()
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
						if strings.Contains(err.Error(), "no space available") {
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
						err := Free(start, size)
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
	used := GetUsedSize()
	hybrid.Info("used: %v", used)
	memoryUsage := GetMemoryUsage()

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
	testMode := flag.String("mode", "basic", "Test mode: basic, stress10t, stress100t")
	flag.Parse()

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

	switch *testMode {
	case "basic":
		runBasicTest()
	case "stress10t":
		runStressTest10T()
	case "stress100t":
		runStressTest100T()
	default:
		fmt.Printf("Unknown test mode: %s\n", *testMode)
		fmt.Println("Available modes: basic, stress10t, stress100t")
		os.Exit(1)
	}

	if err := pprof.WriteHeapProfile(memProfile); err != nil {
		log.Fatal("could not write memory profile: ", err)
	}
}

func runBasicTest() {
	fmt.Printf("Starting basic disk allocation test with %d iterations\n", TestIteration)
	fmt.Println("Min block size:", MinBlockSize/1024, "KB")
	fmt.Println("Max block size:", MaxBlockSize/1024/1024, "MB")
	fmt.Println()

	var results []TestResult
	for i := 0; i < TestIteration; i++ {
		fmt.Printf("Running iteration %d...\n", i+1)
		result := runTest(i)
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

func runStressTest10T() {
	log.Println("Starting 10TB stress test...")
	st := NewStressTest()
	st.runStressTest(10 * TB)
}

func runStressTest100T() {
	log.Println("Starting 100TB stress test...")
	st := NewStressTest()
	st.runStressTest(100 * TB)
}
