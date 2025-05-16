package main

import (
	"fmt"
	"sync"
	"time"
)

type RequestStats struct {
	TotalRequests   int64
	FailedRequests  int64
	TimeoutRequests int64
	TotalLatency    time.Duration
	MinLatency      time.Duration
	MaxLatency      time.Duration
	RequestsPerSec  float64
	TotalBytes      int64
	// 用于计算分位数的延迟数组
	Latencies []time.Duration
	mu        sync.Mutex
}

type SecondStats struct {
	Timestamp    time.Time
	RequestCount int64
	ErrorCount   int64
	AvgLatency   time.Duration
	P75Latency   time.Duration
	P90Latency   time.Duration
	P99Latency   time.Duration
}

// PrintStats 打印请求统计信息
func (rs *RequestStats) PrintStats() {
	fmt.Printf("\n压测结果:\n")
	fmt.Printf("总请求数: %d\n", rs.TotalRequests)
	fmt.Printf("失败请求数: %d\n", rs.FailedRequests)
	fmt.Printf("超时请求数: %d\n", rs.TimeoutRequests)
	fmt.Printf("每秒请求数: %.2f\n", rs.RequestsPerSec)

	if rs.TotalRequests > 0 {
		fmt.Printf("最小延迟: %v\n", rs.MinLatency)
		fmt.Printf("最大延迟: %v\n", rs.MaxLatency)
		fmt.Printf("平均延迟: %v\n", time.Duration(int64(rs.TotalLatency)/rs.TotalRequests))
		fmt.Printf("总传输字节: %d\n", rs.TotalBytes)
	} else {
		fmt.Println("没有成功的请求，无法计算延迟统计")
	}
}
