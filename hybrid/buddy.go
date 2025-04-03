package hybrid

import (
	"fmt"
	"math"
	"unsafe"
)

// NewBuddyAllocator creates a new buddy hybrid
func NewBuddyAllocator() *BuddyAllocator {
	b := &BuddyAllocator{
		stopChan: make(chan struct{}),
	}

	// Create buddyRegionCount regions
	regionSize := MaxBlockSize / buddyRegionCount
	for i := 0; i < buddyRegionCount; i++ {
		startAddr := uint64(i) * uint64(regionSize)
		endAddr := startAddr + uint64(regionSize)
		if i == buddyRegionCount-1 {
			endAddr = MaxBlockSize // The last area is processed to the maximum address
		}

		region := &BuddyRegion{
			blockMap:  [MaxOrder + 1]map[uint64]*Block{},
			allocated: make(map[uint64]*Block),
			startAddr: startAddr,
			endAddr:   endAddr,
			mergeChan: make(chan MergeRequest, mergeBatchSize),
			stopChan:  make(chan struct{}),
		}

		// Initialize blockMap for each order
		for j := 0; j <= MaxOrder; j++ {
			region.blockMap[j] = make(map[uint64]*Block)
		}

		// Initialize the largest block in the region
		maxBlock := &Block{
			start:  startAddr,
			size:   endAddr - startAddr,
			isFree: true,
		}
		order := getOrder(maxBlock.size)
		region.blocks[order] = maxBlock
		region.blockMap[order][maxBlock.start] = maxBlock

		b.regions[i] = region
		go region.run()
	}

	return b
}

// getOrder calculates the order value for a given size
func getOrder(size uint64) int {
	if size < BuddyStartSize {
		return 0
	}
	size = (size + BuddyStartSize - 1) & ^uint64(BuddyStartSize-1) // Round up to nearest MinBlockSize
	order := int(math.Log2(float64(size) / float64(BuddyStartSize)))
	Debug("Calculated order %d for size %d", order, size)
	return order
}

// run processes merge requests for a region
func (r *BuddyRegion) run() {
	for {
		select {
		case req := <-r.mergeChan:
			r.mutex.Lock()
			err := r.mergeBlockLocked(req.start, req.size)
			r.mutex.Unlock()
			if err != nil {
				panic(fmt.Sprintf("Failed to merge block: %v", err))
			}
		case <-r.stopChan:
			return
		}
	}
}

// Allocate allocates memory of specified size
func (b *BuddyAllocator) Allocate(size uint64) (uint64, error) {
	// Iterate through all regions and find the first region with enough space
	for _, region := range b.regions {
		region.mutex.Lock()
		addr, err := region.allocate(size)
		region.mutex.Unlock()
		if err == nil {
			return addr, nil
		}
	}
	return 0, ErrNoSpaceAvailable
}

// allocate allocates memory within a region
func (r *BuddyRegion) allocate(size uint64) (uint64, error) {
	order := getOrder(size)
	if order > MaxOrder {
		return 0, ErrSizeTooLarge
	}

	// Find available block from current order up
	for i := order; i <= MaxOrder; i++ {
		if r.blocks[i] != nil {
			block := r.blocks[i]
			// Remove from linked list
			if block.prev != nil {
				block.prev.next = block.next
			} else {
				r.blocks[i] = block.next
			}
			if block.next != nil {
				block.next.prev = block.prev
			}
			delete(r.blockMap[i], block.start)

			if _, exists := r.allocated[block.start]; exists {
				panic(fmt.Sprintf("Address %d is already allocated", block.start))
			}

			// Split block if too large
			if i > order {
				for j := i - 1; j >= order; j-- {
					newBlock := &Block{
						start:  block.start + (1<<uint(j))*BuddyStartSize,
						size:   (1 << uint(j)) * BuddyStartSize,
						isFree: true,
					}
					block.size = (1 << uint(j)) * BuddyStartSize

					// Add to linked list
					if r.blocks[j] != nil {
						newBlock.next = r.blocks[j]
						r.blocks[j].prev = newBlock
					}
					r.blocks[j] = newBlock
					r.blockMap[j][newBlock.start] = newBlock
				}
			}

			block.isFree = false
			r.allocated[block.start] = block
			r.used += block.size
			return block.start, nil
		}
	}
	return 0, ErrNoSpaceAvailable
}

// mergeBlockLocked performs the actual merge operation
func (r *BuddyRegion) mergeBlockLocked(start, size uint64) error {
	order := getOrder(size)
	currentStart := start

	// Starting from the current order, try to merge
	for {
		buddyStart := currentStart ^ (1 << uint(order) * BuddyStartSize)

		// Use blockMap to find buddy block
		buddyBlock, exists := r.blockMap[order][buddyStart]
		if !exists {
			// No buddy block was found to merge with, so the current block is added to the free list
			newBlock := &Block{
				start:  currentStart,
				size:   (1 << uint(order)) * BuddyStartSize,
				isFree: true,
			}

			// Add to linked list
			if r.blocks[order] != nil {
				newBlock.next = r.blocks[order]
				r.blocks[order].prev = newBlock
			}
			r.blocks[order] = newBlock
			r.blockMap[order][newBlock.start] = newBlock
			break
		}

		// Remove buddy block from linked list
		if buddyBlock.prev != nil {
			buddyBlock.prev.next = buddyBlock.next
		} else {
			r.blocks[order] = buddyBlock.next
		}
		if buddyBlock.next != nil {
			buddyBlock.next.prev = buddyBlock.prev
		}
		delete(r.blockMap[order], buddyStart)

		// Merge
		if currentStart > buddyStart {
			currentStart = buddyStart
		}
		order++
		if order > MaxOrder {
			break
		}
	}

	return nil
}

// Free releases allocated memory at specified address
func (b *BuddyAllocator) Free(start uint64) error {
	// Find the corresponding region
	regionSize := MaxBlockSize / buddyRegionCount
	regionIndex := int(start) / regionSize
	if regionIndex >= buddyRegionCount {
		regionIndex = buddyRegionCount - 1
	}
	region := b.regions[regionIndex]

	region.mutex.Lock()
	defer region.mutex.Unlock()

	// Find the block in allocated blocks
	block, exists := region.allocated[start]
	if !exists {
		return ErrBlockNotFound
	}

	// Remove from allocated blocks
	delete(region.allocated, start)
	region.used -= block.size
	// Send a merge request
	select {
	case region.mergeChan <- MergeRequest{start: start, size: block.size}:
	default:
		if err := region.mergeBlockLocked(start, block.size); err != nil {
			return err
		}
	}

	return nil
}

// GetUsedSize returns the total size of allocated memory
func (b *BuddyAllocator) GetUsedSize() uint64 {
	var totalUsed uint64
	for _, region := range b.regions {
		region.mutex.RLock()
		totalUsed += region.used
		region.mutex.RUnlock()
	}
	return totalUsed
}

// GetUsedSize returns the total size of allocated memory
func (b *BuddyAllocator) GetMemoryUsage() uint64 {
	var size uint64
	// Calculate buddy hybrid memory usage
	for _, region := range b.regions {
		region.mutex.RLock()
		size += uint64(unsafe.Sizeof([]*Block{})) * uint64(len(region.blocks))
		region.mutex.RUnlock()
	}
	return size
}

// Close closes the buddy allocator and stops all regions
func (b *BuddyAllocator) Close() error {
	for _, region := range b.regions {
		region.allocated = nil
		close(region.stopChan)
	}
	return nil
}
