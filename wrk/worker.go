package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type Worker struct {
	url         string
	concurrency int
	duration    time.Duration
	timeout     time.Duration
	qps         int
	stats       *RequestStats
	wg          *sync.WaitGroup
	stopChan    chan struct{}
	generator   RequestGenerator
	// QPS模式下的并发控制
	activeWorkers int32
	maxWorkers    int32
	workerChan    chan struct{}
	// 每秒统计收集器
	statsCollector *SecondStatsCollector
}

// createClient 创建一个新的HTTP客户端
func createClient() *http.Client {
	transport := &http.Transport{
		MaxIdleConns:           10000,            // 增加最大空闲连接数
		MaxIdleConnsPerHost:    10000,            // 增加每个主机的最大空闲连接数
		MaxConnsPerHost:        10000,            // 增加每个主机的最大连接数
		IdleConnTimeout:        60 * time.Second, // 空闲连接超时时间
		DisableCompression:     true,             // 禁用压缩
		ResponseHeaderTimeout:  20 * time.Second, // 响应头超时时间
		ExpectContinueTimeout:  2 * time.Second,  // 100-continue超时时间
		DialContext:            DialWithCache,    // 使用DNS缓存
		TLSHandshakeTimeout:    10 * time.Second, // TLS握手超时时间
		MaxResponseHeaderBytes: 4096,             // 限制响应头大小
		WriteBufferSize:        4096,             // 写缓冲区大小
		ReadBufferSize:         4096,             // 读缓冲区大小
	}

	return &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}
}

var client = createClient()

func NewWorker(url string, concurrency int, duration time.Duration, timeout time.Duration, qps int, generator RequestGenerator, enableSecondStats bool) *Worker {
	// 根据QPS动态调整最大并发数
	maxWorkers := int32(1000)
	if qps > 0 {
		estimatedConcurrency := int32(float64(qps) * 0.1 * 2)
		if estimatedConcurrency > maxWorkers {
			maxWorkers = estimatedConcurrency
		}
	} else if concurrency > 0 {
		maxWorkers = int32(concurrency)
	}

	stats := &RequestStats{MinLatency: time.Hour, MaxLatency: 0}
	statsCollector, err := NewSecondStatsCollector(stats, enableSecondStats)
	if err != nil {
		fmt.Printf("创建统计收集器失败: %v\n", err)
		return nil
	}

	return &Worker{
		url:            url,
		concurrency:    concurrency,
		duration:       duration,
		timeout:        timeout,
		qps:            qps,
		stats:          stats,
		wg:             &sync.WaitGroup{},
		stopChan:       make(chan struct{}),
		generator:      generator,
		maxWorkers:     maxWorkers,
		workerChan:     make(chan struct{}, maxWorkers),
		statsCollector: statsCollector,
	}
}

func (w *Worker) makeRequest() {
	jsonBody, err := w.generator.Generate()
	if err != nil {
		atomic.AddInt64(&w.stats.FailedRequests, 1)
		return
	}

	req, err := http.NewRequest("POST", w.url, bytes.NewBuffer(jsonBody))
	if err != nil {
		fmt.Printf("创建请求失败: %v\n", err)
		atomic.AddInt64(&w.stats.FailedRequests, 1)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			atomic.AddInt64(&w.stats.TimeoutRequests, 1)
		}
		fmt.Printf("请求失败: %v\n", err)
		atomic.AddInt64(&w.stats.FailedRequests, 1)
		return
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	_ = body

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("请求返回非200状态码: %d, 请求体: %s\n", resp.StatusCode, string(jsonBody))
		atomic.AddInt64(&w.stats.FailedRequests, 1)
		return
	}

	// 更新基本请求计数
	atomic.AddInt64(&w.stats.TotalRequests, 1)
	atomic.AddInt64(&w.stats.TotalBytes, resp.ContentLength)

	// 计算延迟
	latency := time.Since(start)

	// 更新延迟统计
	for {
		oldMin := w.stats.MinLatency
		if latency >= oldMin {
			break
		}
		if atomic.CompareAndSwapInt64((*int64)(&w.stats.MinLatency), int64(oldMin), int64(latency)) {
			break
		}
	}

	for {
		oldMax := w.stats.MaxLatency
		if latency <= oldMax {
			break
		}
		if atomic.CompareAndSwapInt64((*int64)(&w.stats.MaxLatency), int64(oldMax), int64(latency)) {
			break
		}
	}

	atomic.AddInt64((*int64)(&w.stats.TotalLatency), int64(latency))

	// 记录延迟到统计收集器
	w.statsCollector.RecordLatency(latency)
}

func (w *Worker) worker() {
	defer w.wg.Done()

	for {
		select {
		case <-w.stopChan:
			return
		default:
			w.makeRequest()
		}
	}
}

func (w *Worker) qpsWorker() {
	defer w.wg.Done()

	// 计算每个10ms间隔需要发送的请求数
	intervalCount := 50 // 1秒分成100个10ms的间隔
	baseRequests := w.qps / intervalCount
	remainder := w.qps % intervalCount
	tickerInterval := 1000 * time.Millisecond / time.Duration(intervalCount)

	// 预计算每个间隔的请求数
	requestsPerInterval := make([]int, intervalCount)
	for i := 0; i < intervalCount; i++ {
		requestsPerInterval[i] = baseRequests
		if i < remainder {
			requestsPerInterval[i]++
		}
	}

	// 创建请求发送通道
	requestChan := make(chan struct{}, w.qps*2) // 增加通道大小

	// 启动请求发送器
	activeWorkers := int32(0)
	for i := 0; i < int(w.maxWorkers); i++ {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			atomic.AddInt32(&activeWorkers, 1)
			defer atomic.AddInt32(&activeWorkers, -1)

			for {
				select {
				case <-w.stopChan:
					return
				case <-requestChan:
					w.makeRequest()
				}
			}
		}()
	}

	// 使用10ms的ticker
	ticker := time.NewTicker(tickerInterval)
	defer ticker.Stop()

	// 当前间隔的索引
	intervalIndex := 0

	// 等待所有worker启动
	time.Sleep(100 * time.Millisecond)
	fmt.Printf("Active workers: %d\n", atomic.LoadInt32(&activeWorkers))

	for {
		select {
		case <-w.stopChan:
			fmt.Println("Stopping QPS worker...")
			return
		case <-ticker.C:
			// 获取当前间隔需要发送的请求数
			requestsToSend := requestsPerInterval[intervalIndex]

			// 更新间隔索引
			intervalIndex = (intervalIndex + 1) % intervalCount

			// 发送请求
			for i := 0; i < requestsToSend; i++ {
				select {
				case requestChan <- struct{}{}:
					// 请求已发送到通道
				case <-w.stopChan:
					fmt.Println("Stopping QPS worker...")
					return
				default:
					// 通道已满，跳过这个请求
					fmt.Printf("Channel is full (len: %d), skip this request\n", len(requestChan))
					atomic.AddInt64(&w.stats.FailedRequests, 1)
				}
			}
		}
	}
}

func (w *Worker) Start() {
	// 启动统计收集器
	w.statsCollector.Start()

	// 启动工作协程
	if w.qps > 0 {
		// QPS模式：使用一个goroutine，通过ticker控制请求频率
		w.wg.Add(1)
		go w.qpsWorker()
	} else {
		// 并发模式：启动多个goroutine
		for i := 0; i < w.concurrency; i++ {
			w.wg.Add(1)
			go w.worker()
		}
	}

	// 设置测试时间
	time.AfterFunc(w.duration, func() {
		close(w.stopChan)
	})

	// 等待所有工作协程完成
	w.wg.Wait()

	// 计算每秒请求数
	w.stats.RequestsPerSec = float64(w.stats.TotalRequests) / w.duration.Seconds()

	// 停止统计收集器
	w.statsCollector.Stop()
}
