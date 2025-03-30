// Package hybrid provides disk space allocation management
package hybrid

import (
	"sync"
)

const (
	MinBlockSize   = 4 * 1024                  // 4KB
	MaxBlockSize   = 1024 * 1024 * 1024 * 1024 // 1TB
	BuddyStartSize = 1024 * 1024               // 1MB
	SlabMaxSize    = 1024 * 1024               // 1MB
	MaxOrder       = 16                        // Maximum order value, supports up to 1TB
	SlabCacheSize  = 32                        // Size of each slab cache
)

// Block represents a memory block
type Block struct {
	start  uint64
	size   uint64
	isFree bool
	next   *Block
	prev   *Block
	slab   *Slab
}

// BuddyAllocator manages memory blocks using buddy system
type BuddyAllocator struct {
	blocks    [MaxOrder + 1][]*Block // MaxOrder + 1 = 21
	mutex     sync.RWMutex
	allocated map[uint64]*Block // track allocated blocks
}

// Slab represents a slab cache
type Slab struct {
	start     uint64
	size      uint64
	used      uint64
	freeList  *Block
	allocator *SlabAllocator
}

// SlabAllocator manages small memory blocks using slab allocation
type SlabAllocator struct {
	buddy  *BuddyAllocator
	slabs  []*Slab
	mutex  sync.RWMutex
	cache  map[uint64][]*Slab
	counts map[uint64]int
}

// Allocator is the main hybrid combining buddy and slab systems
type Allocator struct {
	buddy *BuddyAllocator
	slab  *SlabAllocator
}
