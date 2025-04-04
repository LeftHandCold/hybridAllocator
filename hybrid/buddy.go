package hybrid

import (
	"fmt"
	"sync"
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

	// Initialize block pool
	b.blockPool = &sync.Pool{
		New: func() interface{} {
			return &Block{}
		},
	}

	// Initialize the largest block
	maxBlock := b.getBlock()
	maxBlock.start = 0
	maxBlock.size = MaxBlockSize
	maxBlock.isFree = true
	maxBlock.next = nil
	maxBlock.prev = nil
	maxBlock.slab = nil

	order := getOrder(maxBlock.size)
	b.blocks[order] = maxBlock
	b.blockMap[order][maxBlock.start] = maxBlock

	return b
}

// getBlock gets a Block from the pool
func (b *BuddyAllocator) getBlock() *Block {
	return b.blockPool.Get().(*Block)
}

// putBlock puts a Block back to the pool
func (b *BuddyAllocator) putBlock(block *Block) {
	block.next = nil
	block.prev = nil
	block.slab = nil
	b.blockPool.Put(block)
}

// getOrder calculates the order value for a given size
func getOrder(size uint64) int {
	if size < BuddyStartSize {
		return 0
	}
	size = (size + BuddyStartSize - 1) & ^uint64(BuddyStartSize-1) // Round up to nearest MinBlockSize
	order := 0
	for size > BuddyStartSize {
		size >>= 1
		order++
	}
	return order
}

func getBlockSizeWithSize(size uint64) uint64 {
	order := getOrder(size)
	return (1 << uint(order)) * BuddyStartSize
}

func getBlockSize(order int) uint64 {
	return (1 << uint(order)) * BuddyStartSize
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
			if EnableTrackBlock() {
				if _, exists := b.allocated[block.start]; exists {
					panic(fmt.Sprintf("Address %d is already allocated", block.start))
				}
			}

			// Split block if too large
			if i > order {
				for j := i - 1; j >= order; j-- {
					newBlock := b.getBlock()
					newBlock.start = block.start + getBlockSize(j)
					newBlock.size = getBlockSize(j)
					newBlock.isFree = true
					newBlock.next = nil
					newBlock.prev = nil
					newBlock.slab = nil

					block.size = getBlockSize(j)

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
			if EnableTrackBlock() {
				b.allocated[block.start] = block
			}
			b.used += block.size
			if size > block.size {
				panic(fmt.Sprintf("An invalid address was assigned %d - %d - %d",
					block.start, block.size, size))
			}
			return block.start, nil
		}
	}
	return 0, ErrNoSpaceAvailable
}

// mergeBlockLocked performs the actual merge operation
func (b *BuddyAllocator) mergeBlockLocked(start, size uint64) error {
	order := getOrder(size)
	currentStart := start

	// Try to merge blocks starting from current order
	for order <= MaxOrder {
		buddyStart := currentStart ^ getBlockSize(order)
		buddyBlock, exists := b.blockMap[order][buddyStart]

		if !exists {
			// No buddy found, add current block to free list
			newBlock := b.getBlock()
			newBlock.start = currentStart
			newBlock.size = getBlockSize(order)
			newBlock.isFree = true
			newBlock.next = nil
			newBlock.prev = nil
			newBlock.slab = nil

			// Add to linked list
			if b.blocks[order] != nil {
				newBlock.next = b.blocks[order]
				b.blocks[order].prev = newBlock
			}
			b.blocks[order] = newBlock
			b.blockMap[order][newBlock.start] = newBlock
			break
		}

		// Remove buddy from linked list
		if buddyBlock.prev != nil {
			buddyBlock.prev.next = buddyBlock.next
		} else {
			b.blocks[order] = buddyBlock.next
		}
		if buddyBlock.next != nil {
			buddyBlock.next.prev = buddyBlock.prev
		}
		delete(b.blockMap[order], buddyStart)
		b.putBlock(buddyBlock)

		// Merge with buddy
		if currentStart > buddyStart {
			currentStart = buddyStart
		}
		order++
	}

	return nil
}

// Free releases allocated memory at specified address
func (b *BuddyAllocator) Free(start, size uint64) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	blockSize := size
	if EnableTrackBlock() {
		// Find the block in allocated blocks
		block, exists := b.allocated[start]
		if !exists {
			return ErrBlockNotFound
		}

		// Remove from allocated blocks
		delete(b.allocated, start)
		blockSize = block.size
		if blockSize != getBlockSizeWithSize(size) {
			panic(fmt.Sprintf("Free an invalid block %d, %v", size, block))
		}
	} else {
		blockSize = getBlockSizeWithSize(size)
	}
	b.used -= blockSize
	if err := b.mergeBlockLocked(start, blockSize); err != nil {
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
	return nil
}
