// Package hybrid provides disk space allocation management
package hybrid

import (
	"sync"
)

const (
	MaxBlockSize   = 1024 * 1024 * 1024 * 1024 // 1TB
	BuddyStartSize = 1024 * 1024               // 1MB
	SlabMaxSize    = 1024 * 1024               // 1MB
	MaxOrder       = 20                        // Maximum order value, supports up to 1TB

	EnableTrackAllocatedBlocks = 0
)

// Slab represents a memory slab
type Slab struct {
	start     uint64
	size      uint64
	used      uint64
	allocator *SlabAllocator
	allocated map[uint64]uint64 // start -> size
	freeList  []uint64
	fromBuddy bool
}

// Block represents a memory block
type Block struct {
	start  uint64
	size   uint64
	isFree bool
	next   *Block
	prev   *Block
	slab   *Slab
}

// Allocator is the main hybrid combining buddy and slab systems
type Allocator struct {
	buddy *BuddyAllocator
	slab  *SlabAllocator
	mutex sync.RWMutex
}

// SlabAllocator represents the slab allocator
type SlabAllocator struct {
	buddy  *BuddyAllocator
	slabs  map[uint64]*Slab
	mutex  sync.RWMutex
	cache  map[uint64][]*Slab
	counts map[uint64]int
}

// BuddyAllocator represents the buddy system allocator
type BuddyAllocator struct {
	blocks    [MaxOrder + 1]*Block            // MaxOrder + 1 = 21, head of linked list for each order
	blockMap  [MaxOrder + 1]map[uint64]*Block // Maps block start address to block pointer
	mutex     sync.RWMutex
	allocated map[uint64]*Block // track allocated blocks
	used      uint64
	startAddr uint64
	endAddr   uint64
	blockPool *sync.Pool // Pool for Block objects
}

func EnableTrackBlock() bool {
	return EnableTrackAllocatedBlocks == 1
}
