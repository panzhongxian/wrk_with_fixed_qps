package counter

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

type Counter struct {
	count    int64
	redisCli *redis.Client
	// 活跃连接计数
	activeConnections int64
	// 并发请求计数
	concurrentRequests int64
}

func NewCounter() (*Counter, error) {
	redisAddr := os.Getenv("REDIS_ADDR")
	redisPassword := os.Getenv("REDIS_PASSWORD")
	client := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
	})

	// 测试Redis连接
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %v", err)
	}

	return &Counter{
		redisCli: client,
	}, nil
}

// Increment 增加请求计数
func (c *Counter) Increment() {
	atomic.AddInt64(&c.count, 1)
}

// GetAndReset 获取当前计数并重置为0
func (c *Counter) GetAndReset() int64 {
	return atomic.SwapInt64(&c.count, 0)
}

// IncrementConnections 增加活跃连接计数
func (c *Counter) IncrementConnections() {
	atomic.AddInt64(&c.activeConnections, 1)
}

// DecrementConnections 减少活跃连接计数
func (c *Counter) DecrementConnections() {
	atomic.AddInt64(&c.activeConnections, -1)
}

// GetActiveConnections 获取当前活跃连接数
func (c *Counter) GetActiveConnections() int64 {
	return atomic.LoadInt64(&c.activeConnections)
}

// IncrementConcurrent 增加并发请求计数
func (c *Counter) IncrementConcurrent() {
	atomic.AddInt64(&c.concurrentRequests, 1)
}

// DecrementConcurrent 减少并发请求计数
func (c *Counter) DecrementConcurrent() {
	atomic.AddInt64(&c.concurrentRequests, -1)
}

// GetConcurrentRequests 获取当前并发请求数
func (c *Counter) GetConcurrentRequests() int64 {
	return atomic.LoadInt64(&c.concurrentRequests)
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
