package main

import (
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ClientPool 管理多个HTTP客户端
type ClientPool struct {
	clients []*http.Client
	mu      sync.RWMutex
	// 用于轮询选择客户端的计数器
	counter uint64
}

// NewClientPool 创建一个新的客户端池
func NewClientPool() *ClientPool {
	numCPU := runtime.NumCPU()
	pool := &ClientPool{
		clients: make([]*http.Client, numCPU),
	}

	// 为每个CPU核心创建一个独立的HTTP客户端
	for i := 0; i < numCPU; i++ {
		pool.clients[i] = createClient()
	}

	return pool
}

// createClient 创建一个新的HTTP客户端
func createClient() *http.Client {
	transport := &http.Transport{
		MaxIdleConns:          100,                // 每个客户端的最大空闲连接数
		MaxIdleConnsPerHost:   50,                 // 每个主机的最大空闲连接数
		MaxConnsPerHost:       100,                // 每个主机的最大连接数
		IdleConnTimeout:       60 * time.Second,   // 空闲连接超时时间
		DisableKeepAlives:     false,              // 启用连接复用
		DisableCompression:    true,               // 禁用压缩
		ResponseHeaderTimeout: 20 * time.Second,   // 响应头超时时间
		ExpectContinueTimeout: 2 * time.Second,    // 100-continue超时时间
		ForceAttemptHTTP2:     true,               // 启用HTTP/2
		DialContext:           DialWithCache,      // 使用DNS缓存
		TLSHandshakeTimeout:   10 * time.Second,   // TLS握手超时时间
		// 优化连接池参数
		MaxResponseHeaderBytes: 4096,              // 限制响应头大小
		WriteBufferSize:        4096,              // 写缓冲区大小
		ReadBufferSize:         4096,              // 读缓冲区大小
	}

	return &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}
}

// GetClient 获取一个HTTP客户端
func (p *ClientPool) GetClient() *http.Client {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	// 使用原子操作递增计数器，确保线程安全
	// 使用取模运算确保在客户端数量范围内循环
	id := atomic.AddUint64(&p.counter, 1) % uint64(len(p.clients))
	return p.clients[id]
}

// 全局客户端池实例
var clientPool = NewClientPool() 