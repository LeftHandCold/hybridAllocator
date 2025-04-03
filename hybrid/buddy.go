package hybrid

import (
	"fmt"
	"math/bits"
	"unsafe"
)

// NewBuddyAllocator creates a new buddy allocator
func NewBuddyAllocator() *BuddyAllocator {
	b := &BuddyAllocator{
		blockMap:  [MaxOrder + 1]map[uint64]*Block{},
		allocated: make(map[uint64]*Block),
		startAddr: 0,
		endAddr:   MaxBlockSize,
	}

	// Initialize blockMap for each order
	for j := 0; j <= MaxOrder; j++ {
		b.blockMap[j] = make(map[uint64]*Block)
	}

	// Initialize the largest block
	maxBlock := &Block{
		start:  0,
		size:   MaxBlockSize,
		isFree: true,
	}
	order := getOrder(maxBlock.size)
	b.blocks[order] = maxBlock
	b.blockMap[order][maxBlock.start] = maxBlock

	return b
}

// getOrder calculates the order value for a given size
func getOrder(size uint64) int {
	if size < BuddyStartSize {
		return 0
	}
	size = (size + BuddyStartSize - 1) & ^uint64(BuddyStartSize-1) // Round up to nearest MinBlockSize
	order := bits.TrailingZeros64(size / BuddyStartSize)
	return order
}

// Allocate allocates memory of specified size
func (b *BuddyAllocator) Allocate(size uint64) (uint64, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	order := getOrder(size)
	if order > MaxOrder {
		return 0, ErrSizeTooLarge
	}

	// Find available block from current order up
	for i := order; i <= MaxOrder; i++ {
		if b.blocks[i] != nil {
			block := b.blocks[i]
			// Remove from linked list
			if block.prev != nil {
				block.prev.next = block.next
			} else {
				b.blocks[i] = block.next
			}
			if block.next != nil {
				block.next.prev = block.prev
			}
			delete(b.blockMap[i], block.start)

			if _, exists := b.allocated[block.start]; exists {
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
					if b.blocks[j] != nil {
						newBlock.next = b.blocks[j]
						b.blocks[j].prev = newBlock
					}
					b.blocks[j] = newBlock
					b.blockMap[j][newBlock.start] = newBlock
				}
			}

			block.isFree = false
			b.allocated[block.start] = block
			b.used += block.size
			return block.start, nil
		}
	}
	return 0, ErrNoSpaceAvailable
}

// mergeBlockLocked performs the actual merge operation
func (b *BuddyAllocator) mergeBlockLocked(start, size uint64) error {
	order := getOrder(size)
	currentStart := start

	// Starting from the current order, try to merge
	for {
		buddyStart := currentStart ^ (1 << uint(order) * BuddyStartSize)

		// Use blockMap to find buddy block
		buddyBlock, exists := b.blockMap[order][buddyStart]
		if !exists {
			// No buddy block was found to merge with, so the current block is added to the free list
			newBlock := &Block{
				start:  currentStart,
				size:   (1 << uint(order)) * BuddyStartSize,
				isFree: true,
			}

			// Add to linked list
			if b.blocks[order] != nil {
				newBlock.next = b.blocks[order]
				b.blocks[order].prev = newBlock
			}
			b.blocks[order] = newBlock
			b.blockMap[order][newBlock.start] = newBlock
			break
		}

		// Remove buddy block from linked list
		if buddyBlock.prev != nil {
			buddyBlock.prev.next = buddyBlock.next
		} else {
			b.blocks[order] = buddyBlock.next
		}
		if buddyBlock.next != nil {
			buddyBlock.next.prev = buddyBlock.prev
		}
		delete(b.blockMap[order], buddyStart)

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
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// Find the block in allocated blocks
	block, exists := b.allocated[start]
	if !exists {
		return ErrBlockNotFound
	}

	// Remove from allocated blocks
	delete(b.allocated, start)
	b.used -= block.size
	if err := b.mergeBlockLocked(start, block.size); err != nil {
		return err
	}

	return nil
}

// GetUsedSize returns the total size of allocated memory
func (b *BuddyAllocator) GetUsedSize() uint64 {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.used
}

// GetMemoryUsage returns the memory usage of the allocator
func (b *BuddyAllocator) GetMemoryUsage() uint64 {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return uint64(unsafe.Sizeof([]*Block{})) * uint64(len(b.blocks))
}

// Close closes the buddy allocator
func (b *BuddyAllocator) Close() error {
	b.allocated = nil
	return nil
}
