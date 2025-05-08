package counter

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// Counter 是一个请求计数器
type Counter struct {
	// 使用分片计数器来提高并发性能
	counters []int64
	redisCli *redis.Client
	// 活跃连接计数
	activeConnections int64
	// 并发请求计数
	concurrentRequests int64
}

// NewCounter 创建一个新的计数器实例
func NewCounter() (*Counter, error) {
	redisAddr := os.Getenv("REDIS_ADDR")
	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisCli := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       0,
		// 优化Redis连接池
		PoolSize:        100,
		MinIdleConns:    10,
		ConnMaxLifetime: 30 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
	})

	// 测试Redis连接
	ctx := context.Background()
	if err := redisCli.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %v", err)
	}

	// 创建计数器实例
	counter := &Counter{
		counters: make([]int64, 32), // 使用32个分片
		redisCli: redisCli,
	}

	return counter, nil
}

// Increment 增加计数器
func (c *Counter) Increment() {
	// 使用分片计数器
	shard := time.Now().UnixNano() % int64(len(c.counters))
	atomic.AddInt64(&c.counters[shard], 1)
}

// GetCount 获取当前计数
func (c *Counter) GetCount() int64 {
	var total int64
	for i := range c.counters {
		total += atomic.LoadInt64(&c.counters[i])
	}
	return total
}

// GetAndReset 获取当前计数并重置为0
func (c *Counter) GetAndReset() int64 {
	var total int64
	for i := range c.counters {
		total += atomic.SwapInt64(&c.counters[i], 0)
	}
	return total
}

// StartReporting 启动一个协程，每秒将统计信息写入Redis
func (c *Counter) StartReporting(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count := c.GetAndReset()
			timestamp := time.Now().Unix()
			activeConns := c.GetActiveConnections()
			concurrentReqs := c.GetConcurrentRequests()

			// 将计数、活跃连接数和并发请求数写入Redis
			err := c.redisCli.HSet(ctx, "request_counts",
				fmt.Sprintf("%d_requests", timestamp), count,
				fmt.Sprintf("%d_connections", timestamp), activeConns,
				fmt.Sprintf("%d_concurrent", timestamp), concurrentReqs,
			).Err()
			if err != nil {
				fmt.Printf("Error writing to Redis: %v\n", err)
			}
		}
	}
}

// IncrementConnections 增加活跃连接数
func (c *Counter) IncrementConnections() {
	atomic.AddInt64(&c.activeConnections, 1)
}

// DecrementConnections 减少活跃连接数
func (c *Counter) DecrementConnections() {
	atomic.AddInt64(&c.activeConnections, -1)
}

// GetActiveConnections 获取当前活跃连接数
func (c *Counter) GetActiveConnections() int64 {
	return atomic.LoadInt64(&c.activeConnections)
}

// IncrementConcurrent 增加并发请求数
func (c *Counter) IncrementConcurrent() {
	atomic.AddInt64(&c.concurrentRequests, 1)
}

// DecrementConcurrent 减少并发请求数
func (c *Counter) DecrementConcurrent() {
	atomic.AddInt64(&c.concurrentRequests, -1)
}

// GetConcurrentRequests 获取当前并发请求数
func (c *Counter) GetConcurrentRequests() int64 {
	return atomic.LoadInt64(&c.concurrentRequests)
}
