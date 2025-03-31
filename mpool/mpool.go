package mpool

import (
	"fmt"
	"hybridAllocator/hybrid"
	"math/rand"
	"sync"
)

const (
	MB = 1024 * 1024
	KB = 1024

	SmallPoolSize  = 20000 // Small pool size (4KB-64KB)
	MediumPoolSize = 10000 // Medium pool size (64KB-1MB)
	LargePoolSize  = 5000  // Large pool size (1MB-4MB)
)

// PoolStats represents memory pool statistics
type PoolStats struct {
	TotalAllocations uint64
	PoolHits         uint64
	PoolMisses       uint64
	TotalFrees       uint64
	PoolFreeHits     uint64
	PoolFreeMisses   uint64
}

// MemoryPool represents a memory pool structure
type MemoryPool struct {
	smallBlocks  []uint64 // 4KB-64KB blocks
	mediumBlocks []uint64 // 64KB-1MB blocks
	largeBlocks  []uint64 // 1MB-4MB blocks
	smallSizes   []uint64
	mediumSizes  []uint64
	largeSizes   []uint64
	smallUsed    []bool
	mediumUsed   []bool
	largeUsed    []bool
	mu           sync.Mutex
	allocator    *hybrid.Allocator
	stats        PoolStats
}

// NewMemoryPool creates a new memory pool
func NewMemoryPool(allocator *hybrid.Allocator) (*MemoryPool, error) {
	pool := &MemoryPool{
		smallBlocks:  make([]uint64, SmallPoolSize),
		mediumBlocks: make([]uint64, MediumPoolSize),
		largeBlocks:  make([]uint64, LargePoolSize),
		smallSizes:   make([]uint64, SmallPoolSize),
		mediumSizes:  make([]uint64, MediumPoolSize),
		largeSizes:   make([]uint64, LargePoolSize),
		smallUsed:    make([]bool, SmallPoolSize),
		mediumUsed:   make([]bool, MediumPoolSize),
		largeUsed:    make([]bool, LargePoolSize),
		allocator:    allocator,
	}
	// Pre-allocate small memory blocks (4KB-64KB)
	for i := 0; i < SmallPoolSize; i++ {
		size := uint64(rand.Intn(60*KB) + 4*KB) // 4KB-64KB
		addr, err := allocator.Allocate(size)
		if err != nil {
			return nil, fmt.Errorf("failed to pre-allocate small memory block: %v", err)
		}
		pool.smallBlocks[i] = addr
		pool.smallSizes[i] = size
	}

	// Pre-allocate medium memory blocks (64KB-1MB)
	for i := 0; i < MediumPoolSize; i++ {
		size := uint64(rand.Intn(936*KB) + 64*KB) // 64KB-1MB
		addr, err := allocator.Allocate(size)
		if err != nil {
			return nil, fmt.Errorf("failed to pre-allocate medium memory block: %v", err)
		}
		pool.mediumBlocks[i] = addr
		pool.mediumSizes[i] = size
	}

	// Pre-allocate large memory blocks (1MB-4MB)
	for i := 0; i < LargePoolSize; i++ {
		size := uint64(rand.Intn(3*MB) + 1*MB) // 1MB-4MB
		addr, err := allocator.Allocate(size)
		if err != nil {
			return nil, fmt.Errorf("failed to pre-allocate large memory block: %v", err)
		}
		pool.largeBlocks[i] = addr
		pool.largeSizes[i] = size
	}

	return pool, nil
}

// Allocate allocates memory from the memory pool
func (p *MemoryPool) Allocate(size uint64) (uint64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stats.TotalAllocations++

	// Select appropriate pool based on size
	switch {
	case size <= 64*KB:
		// Search in small pool
		for i := range p.smallBlocks {
			if !p.smallUsed[i] && p.smallSizes[i] >= size {
				p.smallUsed[i] = true
				p.stats.PoolHits++
				return p.smallBlocks[i], nil
			}
		}
	case size <= 1*MB:
		// Search in medium pool
		for i := range p.mediumBlocks {
			if !p.mediumUsed[i] && p.mediumSizes[i] >= size {
				p.mediumUsed[i] = true
				p.stats.PoolHits++
				return p.mediumBlocks[i], nil
			}
		}
	case size <= 4*MB:
		// Search in large pool
		for i := range p.largeBlocks {
			if !p.largeUsed[i] && p.largeSizes[i] >= size {
				p.largeUsed[i] = true
				p.stats.PoolHits++
				return p.largeBlocks[i], nil
			}
		}
	}

	p.stats.PoolMisses++
	// If no suitable free block found, allocate directly from allocator
	return p.allocator.Allocate(size)
}

// Free releases memory back to the memory pool
func (p *MemoryPool) Free(addr uint64, size uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stats.TotalFrees++

	// Find corresponding pool based on size
	switch {
	case size <= 64*KB:
		for i := range p.smallBlocks {
			if p.smallBlocks[i] == addr {
				p.smallUsed[i] = false
				p.stats.PoolFreeHits++
				return nil
			}
		}
	case size <= 1*MB:
		for i := range p.mediumBlocks {
			if p.mediumBlocks[i] == addr {
				p.mediumUsed[i] = false
				p.stats.PoolFreeHits++
				return nil
			}
		}
	case size <= 4*MB:
		for i := range p.largeBlocks {
			if p.largeBlocks[i] == addr {
				p.largeUsed[i] = false
				p.stats.PoolFreeHits++
				return nil
			}
		}
	}

	p.stats.PoolFreeMisses++
	// If block not found in pool, free directly through allocator
	return p.allocator.Free(addr, size)
}

// Close closes the memory pool and releases all pre-allocated memory
func (p *MemoryPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Free small memory blocks
	for i := range p.smallBlocks {
		if err := p.allocator.Free(p.smallBlocks[i], p.smallSizes[i]); err != nil {
			return fmt.Errorf("failed to free small memory block: %v", err)
		}
	}

	// Free medium memory blocks
	for i := range p.mediumBlocks {
		if err := p.allocator.Free(p.mediumBlocks[i], p.mediumSizes[i]); err != nil {
			return fmt.Errorf("failed to free medium memory block: %v", err)
		}
	}

	// Free large memory blocks
	for i := range p.largeBlocks {
		if err := p.allocator.Free(p.largeBlocks[i], p.largeSizes[i]); err != nil {
			return fmt.Errorf("failed to free large memory block: %v", err)
		}
	}

	// Print statistics
	fmt.Printf("\nMemory Pool Statistics:\n")
	fmt.Printf("Total Allocations: %d\n", p.stats.TotalAllocations)
	fmt.Printf("Pool Hits: %d (%.2f%%)\n", p.stats.PoolHits, float64(p.stats.PoolHits)/float64(p.stats.TotalAllocations)*100)
	fmt.Printf("Pool Misses: %d (%.2f%%)\n", p.stats.PoolMisses, float64(p.stats.PoolMisses)/float64(p.stats.TotalAllocations)*100)
	fmt.Printf("Total Frees: %d\n", p.stats.TotalFrees)
	fmt.Printf("Pool Free Hits: %d (%.2f%%)\n", p.stats.PoolFreeHits, float64(p.stats.PoolFreeHits)/float64(p.stats.TotalFrees)*100)
	fmt.Printf("Pool Free Misses: %d (%.2f%%)\n", p.stats.PoolFreeMisses, float64(p.stats.PoolFreeMisses)/float64(p.stats.TotalFrees)*100)

	return nil
}
