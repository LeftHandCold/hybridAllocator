# HybridAllocator

HybridAllocator 是一款高性能混合磁盘分配器，通过融合伙伴系统（Buddy System）和 Slab 分配器，实现 4kB-4MB 范围的高效磁盘空间管理。提供高达 96% 的磁盘利用率。

## 设计原理

### 1. 三层架构

HybridAllocator 采用三层设计，每层针对不同大小的磁盘空间请求进行优化：

#### 1.1 底层：伙伴系统（Buddy System）
- 负责大块磁盘空间（1MB-4MB）的管理
- 核心功能：
  - 支持磁盘块的合并和分裂
  - 保证磁盘分配的连续性
  - 减少外部碎片
- 工作方式：
  - 将大块磁盘空间按2的幂次方进行划分
  - 维护空闲块链表
  - 支持相邻空闲块的合并

#### 1.2 上层：Slab 分配器
- 负责中小块磁盘空间（4kB-1MB）的管理
- 核心功能：
  - 针对 4kB 对齐的大小进行优化
  - 减少内部碎片
  - 提高中小对象分配效率
- 工作方式：
  - 预分配固定大小的磁盘块（4MB）
  - 维护对象缓存
  - 支持对象的快速分配和释放
  - 自动合并空闲块

#### 1.3 顶层：内存池缓存层
- 作为磁盘分配的缓存层
- 核心功能：
   - 预分配常用大小的磁盘块
   - 减少真实分配操作
   - 提高分配效率
- 工作方式：
   - 维护空闲块链表
   - 支持磁盘块的复用

### 2. 算法原理

#### 2.1 Slab 分配器算法

1. **分配流程**：
   - 计算请求大小的对齐值（向上取整到4kB的倍数）
   - 在对应大小的Slab缓存中查找可用空间
   - 如果找到可用空间，直接分配
   - 如果没有可用空间，从伙伴系统申请新的Slab
   - 在新Slab中分配空间并更新空闲列表

2. **释放流程**：
   - 定位包含释放地址的Slab
   - 将释放的空间加入Slab的空闲列表
   - 检查Slab是否完全空闲
   - 如果完全空闲且来自伙伴系统，则归还给伙伴系统

3. **空间管理**：
   - 维护空闲列表加速分配
   - 支持空间合并减少碎片

#### 2.2 伙伴系统算法

1. **分配流程**：
   - 计算请求大小的阶数（order）
   - 在对应阶数的空闲链表中查找可用块
   - 如果找到合适大小的块，直接分配
   - 如果没有合适大小的块，从更高阶分裂
   - 分裂后的块加入对应阶数的空闲链表

2. **释放流程**：
   - 定位要释放的块
   - 计算伙伴块的地址
   - 检查伙伴块是否空闲
   - 如果伙伴块空闲，合并两个块
   - 递归检查更高阶的合并可能性

3. **块管理**：
   - 使用双向链表维护空闲块
   - 通过位运算快速计算伙伴块地址
   - 支持多阶块的高效管理

### 3. 协作机制

#### 3.1 分配流程
1. 中小空间分配（4KB-1MB）：
   ```
   请求 -> 内存池 -> Slab分配器 -> 伙伴系统
   ```
   - 首先检查内存池是否有可用块
   - 如果池中无可用块，则从 Slab 分配器分配
   - 如果 Slab 分配器无法满足，则会从伙伴系统分配

2. 大空间分配（1MB-4MB）：
   ```
   请求 -> 内存池 -> 伙伴系统
   ```
   - 首先检查内存池是否有可用块
   - 如果池中无可用块，则直接从伙伴系统分配

#### 3.2 释放流程
1. 中小空间释放（4KB-1MB）：
   ```
    释放 -> 内存池 -> Slab分配器
   ```
   - 优先尝试归还到内存池
   - 如果池已满，则归还给 Slab 分配器

2. 大空间释放（1MB-4MB）：
   ```
   释放 -> 内存池 -> 伙伴系统
   ```
   - 优先尝试归还到内存池
   - 如果池已满，则归还给伙伴系统
   - 尝试与相邻的空闲块合并

### 3. RPC 接口

HybridAllocator 提供了 RPC 接口，支持远程磁盘空间分配：

```go
// 分配磁盘空间
func (c *Client) Allocate(size uint64) (uint64, error)

// 释放磁盘空间
func (c *Client) Free(start uint64, size uint64) error

// 获取已使用空间
func (c *Client) GetUsedSize() uint64

// 获取内存使用情况
func (c *Client) GetMemoryUsage() uint64
```

## 测试结果

### 1. 10TB 压力测试

| 测试阶段 | 内存使用 | 平均速度         | 磁盘使用率 | 运行时间 |
|---------|---------|--------------|-----------|---------|
| 初始阶段(0-1TB) | 124 MB | 2.8 TB/s     | 96.16% | 376ms |
| 中期阶段(1-5TB) | 130-167 MB | 2.6-2.7 TB/s | 95.91-96.04% | 约1.5s |
| 后期阶段(5-10TB) | 167-238 MB | 2.6 TB/s     | 95.50-95.57% | 约2.1s |

总运行时间：约4秒

### 2. 100TB 压力测试

| 测试阶段 | 内存使用 | 平均速度         | 磁盘使用率 | 运行时间 |
|---------|---------|--------------|-----------|------|
| 初始阶段(0-10TB) | 133-242 MB | 2.1-2.2 TB/s | 95.50-96.15% | 约13s |
| 中期阶段(10-50TB) | 242-269 MB | 2.1 TB/s     | 95.49-95.57% | 约14s |
| 后期阶段(50-100TB) | 269-300 MB | 2.1 TB/s     | 95.46-95.50% | 约15s |

总运行时间：约2分30秒

### 3. 性能特点

1. **高并发支持**：
   - 支持多线程并发分配和释放
   - 32个goroutine并发操作
   - 分配和释放操作保持平衡

2. **内存效率**：
   - 内存使用量随分配量线性增长
   - 10TB测试：124-238 MB
   - 100TB测试：133-300 MB
   - 无内存泄漏迹象

3. **空间利用率**：
   - 平均使用率：95.5%-96.2%
   - 最大使用率：96.16%
   - 最小使用率：95.46%
   - 使用率波动极小

4. **性能稳定**：
   - 在各种负载下保持稳定的性能表现
   - 写入速度稳定在2.1-2.8 TB/s
   - 性能随数据量增长保持稳定

## 使用方法

```go
// 创建分配器
allocator := hybrid.NewAllocator()

// 分配磁盘空间
start, err := allocator.Allocate(size)
if err != nil {
    // 处理错误
}

// 释放磁盘空间
err = allocator.Free(start, size)
if err != nil {
    // 处理错误
}

// 获取使用统计
used := allocator.GetUsedSize()
total := allocator.GetTotalSize()
```

## 配置参数

- `MinBlockSize`: 最小分配大小（4KB）
- `MaxBlockSize`: 最大分配大小（4MB）
- `SlabMaxSize`: Slab分配器最大分配大小（1MB）
- `BuddyStartSize`: 伙伴系统起始大小（1MB）
