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
	counter  int64
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
	var redisCli *redis.Client = nil
	if redisAddr != "" && redisPassword != "" {
		redisCli = redis.NewClient(&redis.Options{
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
	}

	// 创建计数器实例
	return &Counter{
		counter:  0,
		redisCli: redisCli,
	}, nil
}

// Increment 增加计数器
func (c *Counter) Increment() {
	// 使用分片计数器
	atomic.AddInt64(&c.counter, 1)
}

// GetCount 获取当前计数
func (c *Counter) GetCount() int64 {
	var total = atomic.LoadInt64(&c.counter)
	return total
}

// GetAndReset 获取当前计数并重置为0
func (c *Counter) GetAndReset() int64 {
	total := atomic.SwapInt64(&c.counter, 0)
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

			// 只在有数据时才写入Redis
			fields := make(map[string]interface{})
			if count > 0 {
				fields[fmt.Sprintf("%d_requests", timestamp)] = count
			}
			if activeConns > 0 {
				fields[fmt.Sprintf("%d_connections", timestamp)] = activeConns
			}
			if concurrentReqs > 0 {
				fields[fmt.Sprintf("%d_concurrent", timestamp)] = concurrentReqs
			}
			fmt.Println(fields)

			// 如果有数据才写入Redis
			if len(fields) > 0 {
				if c.redisCli != nil {
					err := c.redisCli.HSet(ctx, "request_counts", fields).Err()
					if err != nil {
						fmt.Printf("Error writing to Redis: %v\n", err)
					}
				} else {
					fmt.Printf("Second stat info:\n")
					for key, value := range fields {
						fmt.Printf("   %s = %d\n", key, value)
					}
				}
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
