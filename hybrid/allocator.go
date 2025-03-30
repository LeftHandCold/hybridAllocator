// Package hybrid provides disk space allocation management
package hybrid

import (
	"sync"
	"unsafe"
)

// NewAllocator creates a new memory hybrid instance
func NewAllocator() *Allocator {
	Debug("Creating new hybrid")
	buddy := NewBuddyAllocator()

	// Initialize buddy hybrid with max block
	maxBlock := &Block{
		start:  0,
		size:   MaxBlockSize,
		isFree: true,
	}
	buddy.blocks[MaxOrder] = append(buddy.blocks[MaxOrder], maxBlock)

	slab := &SlabAllocator{
		buddy:  buddy,
		slabs:  make([]*Slab, 0),
		mutex:  sync.RWMutex{},
		cache:  make(map[uint64][]*Slab),
		counts: make(map[uint64]int),
	}

	return &Allocator{
		buddy: buddy,
		slab:  slab,
	}
}

// Allocate allocates memory of specified size
func (a *Allocator) Allocate(size uint64) (uint64, error) {
	Debug("Allocating %d bytes", size)
	if size > MaxBlockSize {
		Error("Requested size %d exceeds MaxBlockSize %d", size, MaxBlockSize)
		return 0, ErrSizeTooLarge
	}

	if size <= SlabMaxSize {
		start, err := a.slab.Allocate(size)
		if err == ErrSlabFull {
			Debug("Slab is full, trying buddy hybrid")
			return a.buddy.Allocate(size)
		}
		if err != nil {
			Error("Slab allocation failed: %v", err)
			return 0, err
		}
		Debug("Allocated %d bytes from slab at address %d", size, start)
		return start, nil
	}

	start, err := a.buddy.Allocate(size)
	if err != nil {
		Error("Buddy allocation failed: %v", err)
		return 0, err
	}
	Debug("Allocated %d bytes from buddy at address %d", size, start)
	return start, nil
}

// Free releases allocated memory at specified address
func (a *Allocator) Free(start uint64, size uint64) error {
	Debug("Freeing %d bytes at address %d", size, start)
	if size <= SlabMaxSize {
		err := a.slab.Free(start)
		if err == ErrSlabNotFound {
			Debug("Address not found in slab, trying buddy hybrid")
			return a.buddy.Free(start)
		}
		if err != nil {
			Error("Slab free failed: %v", err)
			return err
		}
		Debug("Freed %d bytes from slab at address %d", size, start)
		return nil
	}

	err := a.buddy.Free(start)
	if err != nil {
		Error("Buddy free failed: %v", err)
		return err
	}
	Debug("Freed %d bytes from buddy at address %d", size, start)
	return nil
}

// GetUsedSize returns the total size of allocated memory
func (a *Allocator) GetUsedSize() uint64 {
	used := a.buddy.GetUsedSize()
	Debug("Total used size: %d bytes", used)
	return used
}

// GetMemoryUsage returns the memory overhead of the hybrid
func (a *Allocator) GetMemoryUsage() uint64 {
	var size uint64
	// Calculate buddy hybrid memory usage
	size += uint64(unsafe.Sizeof([]*Block{})) * uint64(len(a.buddy.blocks))
	for _, blocks := range a.buddy.blocks {
		size += uint64(unsafe.Sizeof(&Block{})) * uint64(len(blocks))
	}

	// Calculate slab hybrid memory usage
	size += uint64(unsafe.Sizeof(&Slab{})) * uint64(len(a.slab.cache))
	for _, slabs := range a.slab.cache {
		for _, slab := range slabs {
			size += uint64(unsafe.Sizeof(slab))
		}
	}

	Debug("Memory overhead: %d bytes", size)
	return size
}
