package main

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// DNSCache 是一个简单的DNS缓存实现
type DNSCache struct {
	cache    map[string][]string
	mu       sync.RWMutex
	ttl      time.Duration
	resolver *net.Resolver
}

// NewDNSCache 创建一个新的DNS缓存实例
func NewDNSCache(ttl time.Duration) *DNSCache {
	return &DNSCache{
		cache:    make(map[string][]string),
		ttl:      ttl,
		resolver: &net.Resolver{},
	}
}

// LookupHost 查找主机名对应的IP地址，优先使用缓存
func (d *DNSCache) LookupHost(ctx context.Context, host string) ([]string, error) {
	// 先检查缓存
	d.mu.RLock()
	if ips, ok := d.cache[host]; ok {
		d.mu.RUnlock()
		return ips, nil
	}
	d.mu.RUnlock()

	// 缓存未命中，进行DNS查询
	ips, err := d.resolver.LookupHost(ctx, host)
	if err != nil {
		return nil, err
	}

	// 更新缓存
	d.mu.Lock()
	d.cache[host] = ips
	d.mu.Unlock()

	// 设置缓存过期
	time.AfterFunc(d.ttl, func() {
		d.mu.Lock()
		delete(d.cache, host)
		d.mu.Unlock()
	})

	return ips, nil
}

// 创建DNS缓存实例
var dnsCache = NewDNSCache(5 * time.Minute)

// DialWithCache 自定义的DialContext函数，使用DNS缓存
func DialWithCache(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	// 使用DNS缓存查找IP
	ips, err := dnsCache.LookupHost(ctx, host)
	if err != nil {
		return nil, err
	}

	// 尝试连接所有IP地址，最多重试3次
	maxRetries := 3
	var lastErr error
	for retry := 0; retry < maxRetries; retry++ {
		for _, ip := range ips {
			conn, err := (&net.Dialer{
				Timeout:   5 * time.Second,    // 增加超时时间到5秒
				KeepAlive: 30 * time.Second,
			}).DialContext(ctx, network, net.JoinHostPort(ip, port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
			// 如果还有重试机会，等待一段时间再重试
			if retry < maxRetries-1 {
				time.Sleep(time.Duration(retry+1) * 500 * time.Millisecond)
			}
		}
	}

	return nil, fmt.Errorf("failed after %d retries, last error: %v", maxRetries, lastErr)
} 