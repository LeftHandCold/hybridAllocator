package rpc

import (
	"fmt"
	"hybridAllocator/hybrid"
	"hybridAllocator/mpool"
	"net"
	"net/rpc"
	"sync"
)

// Server represents the memory pool server
type Server struct {
	pool      *mpool.MemoryPool
	allocator *hybrid.Allocator
	mu        sync.Mutex
}

// AllocRequest represents a memory allocation request
type AllocRequest struct {
	Size uint64
}

// AllocResponse represents a memory allocation response
type AllocResponse struct {
	Start uint64
	Error string
}

// FreeRequest represents a memory free request
type FreeRequest struct {
	Start uint64
	Size  uint64
}

// FreeResponse represents a memory free response
type FreeResponse struct {
	Error string
}

// NewServer creates a new memory pool server
func NewServer() (*Server, error) {
	allocator := hybrid.NewAllocator()
	pool, err := mpool.NewMemoryPool(allocator)
	if err != nil {
		return nil, fmt.Errorf("failed to create memory pool: %v", err)
	}

	server := &Server{
		pool:      pool,
		allocator: allocator,
	}

	// Register RPC methods
	rpc.Register(server)
	return server, nil
}

// Start starts the server on the specified address
func (s *Server) Start(address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to start server: %v", err)
	}
	defer listener.Close()

	fmt.Printf("Server listening on %s\n", address)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Failed to accept connection: %v\n", err)
			continue
		}
		go rpc.ServeConn(conn)
	}
}

func (s *Server) Allocate(req *AllocRequest, resp *AllocResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	start, err := s.pool.Allocate(req.Size)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}

	resp.Start = start
	return nil
}

func (s *Server) GetUsedSize() uint64 {
	return s.allocator.GetUsedSize()
}

func (s *Server) GetMemoryUsage() uint64 {
	return s.allocator.GetMemoryUsage()
}

func (s *Server) Free(req *FreeRequest, resp *FreeResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.pool.Free(req.Start, req.Size)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}

	return nil
}

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.pool.Close()
}
