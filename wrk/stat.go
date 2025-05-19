package main

import (
	"fmt"
	"os"
	"sort"
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

// SecondStatsCollector 负责收集和记录每秒的统计信息
type SecondStatsCollector struct {
	enabled     bool
	statsFile   *os.File
	statsTicker *time.Ticker
	stats       *RequestStats
	stopChan    chan struct{}
}

// NewSecondStatsCollector 创建一个新的每秒统计收集器
func NewSecondStatsCollector(stats *RequestStats, enabled bool) (*SecondStatsCollector, error) {
	collector := &SecondStatsCollector{
		enabled:  enabled,
		stats:    stats,
		stopChan: make(chan struct{}),
	}

	if enabled {
		var err error
		collector.statsFile, err = os.Create("stats.csv")
		if err != nil {
			return nil, fmt.Errorf("创建统计文件失败: %v", err)
		}

		// 写入CSV头
		fmt.Fprintf(collector.statsFile, "时间点,当秒请求数,错误数量,平均延迟,p75_latency,p90_latency,p99_latency\n")
		collector.statsTicker = time.NewTicker(time.Second)
	}

	return collector, nil
}

// Start 启动统计收集
func (c *SecondStatsCollector) Start() {
	if !c.enabled {
		return
	}

	go func() {
		for {
			select {
			case <-c.stopChan:
				return
			case <-c.statsTicker.C:
				if stats := c.collectStats(); stats != nil {
					fmt.Fprintf(c.statsFile, "%s,%d,%d,%d,%d,%d,%d\n",
						stats.Timestamp.Format("2006-01-02 15:04:05"),
						stats.RequestCount,
						stats.ErrorCount,
						stats.AvgLatency.Milliseconds(),
						stats.P75Latency.Milliseconds(),
						stats.P90Latency.Milliseconds(),
						stats.P99Latency.Milliseconds())
				}
			}
		}
	}()
}

// Stop 停止统计收集
func (c *SecondStatsCollector) Stop() {
	if !c.enabled {
		return
	}

	close(c.stopChan)
	c.statsTicker.Stop()
	if c.statsFile != nil {
		c.statsFile.Close()
	}
}

// RecordLatency 记录请求延迟
func (c *SecondStatsCollector) RecordLatency(latency time.Duration) {
	if !c.enabled {
		return
	}

	c.stats.mu.Lock()
	c.stats.Latencies = append(c.stats.Latencies, latency)
	c.stats.mu.Unlock()
}

// collectStats 收集当前秒的统计信息
func (c *SecondStatsCollector) collectStats() *SecondStats {
	c.stats.mu.Lock()
	defer c.stats.mu.Unlock()

	if len(c.stats.Latencies) == 0 {
		return nil
	}

	// 计算分位数
	p75 := calculatePercentile(c.stats.Latencies, 0.75)
	p90 := calculatePercentile(c.stats.Latencies, 0.90)
	p99 := calculatePercentile(c.stats.Latencies, 0.99)

	// 计算平均延迟
	var totalLatency time.Duration
	for _, l := range c.stats.Latencies {
		totalLatency += l
	}
	avgLatency := totalLatency / time.Duration(len(c.stats.Latencies))

	stats := &SecondStats{
		Timestamp:    time.Now(),
		RequestCount: int64(len(c.stats.Latencies)),
		ErrorCount:   c.stats.FailedRequests,
		AvgLatency:   avgLatency,
		P75Latency:   p75,
		P90Latency:   p90,
		P99Latency:   p99,
	}

	// 清空延迟数组，准备下一秒的统计
	c.stats.Latencies = c.stats.Latencies[:0]

	return stats
}

// calculatePercentile 计算延迟分位数
func calculatePercentile(latencies []time.Duration, percentile float64) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	index := int(float64(len(latencies)-1) * percentile)
	return latencies[index]
}
