package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
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
	// 是否记录每秒统计
	enableSecondStats bool
	// 可复用的请求对象
	reusableReq *http.Request
	reqMu       sync.Mutex
}

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

	reusableReq, err := http.NewRequest("POST", url, nil)
	if err != nil {
		fmt.Printf("创建可复用请求对象失败: %v\n", err)
		return nil
	}
	reusableReq.Header.Set("Content-Type", "application/json")

	return &Worker{
		url:               url,
		concurrency:       concurrency,
		duration:          duration,
		timeout:           timeout,
		qps:               qps,
		stats:             &RequestStats{MinLatency: time.Hour, MaxLatency: 0},
		wg:                &sync.WaitGroup{},
		stopChan:          make(chan struct{}),
		generator:         generator,
		maxWorkers:        maxWorkers,
		workerChan:        make(chan struct{}, maxWorkers),
		enableSecondStats: enableSecondStats,
		reusableReq:       reusableReq,
	}
}

func (w *Worker) makeRequest() {
	jsonBody, err := w.generator.Generate()
	if err != nil {
		atomic.AddInt64(&w.stats.FailedRequests, 1)
		return
	}

	// 复用请求对象
	w.reqMu.Lock()
	// 创建一个新的请求，而不是复用旧的
	req, err := http.NewRequest("POST", w.url, bytes.NewBuffer(jsonBody))
	if err != nil {
		w.reqMu.Unlock()
		fmt.Printf("创建请求失败: %v\n", err)
		atomic.AddInt64(&w.stats.FailedRequests, 1)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	w.reusableReq = req
	w.reqMu.Unlock()

	// 从客户端池获取HTTP客户端
	client := clientPool.GetClient()

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
	defer resp.Body.Close()

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

	// 只在启用详细统计时记录延迟数组
	if w.enableSecondStats {
		w.stats.mu.Lock()
		w.stats.Latencies = append(w.stats.Latencies, latency)
		w.stats.mu.Unlock()
	}
}

func (w *Worker) calculatePercentile(latencies []time.Duration, percentile float64) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	index := int(float64(len(latencies)-1) * percentile)
	return latencies[index]
}

func (w *Worker) collectSecondStats() *SecondStats {
	w.stats.mu.Lock()
	defer w.stats.mu.Unlock()

	if len(w.stats.Latencies) == 0 {
		return nil
	}

	// 计算分位数
	p75 := w.calculatePercentile(w.stats.Latencies, 0.75)
	p90 := w.calculatePercentile(w.stats.Latencies, 0.90)
	p99 := w.calculatePercentile(w.stats.Latencies, 0.99)

	// 计算平均延迟
	var totalLatency time.Duration
	for _, l := range w.stats.Latencies {
		totalLatency += l
	}
	avgLatency := totalLatency / time.Duration(len(w.stats.Latencies))

	stats := &SecondStats{
		Timestamp:    time.Now(),
		RequestCount: int64(len(w.stats.Latencies)),
		ErrorCount:   w.stats.FailedRequests,
		AvgLatency:   avgLatency,
		P75Latency:   p75,
		P90Latency:   p90,
		P99Latency:   p99,
	}

	// 清空延迟数组，准备下一秒的统计
	w.stats.Latencies = w.stats.Latencies[:0]

	return stats
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
	fmt.Println("requestsPerInterval:", requestsPerInterval)

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
	// 创建统计文件
	var statsFile *os.File
	var err error
	if w.enableSecondStats {
		statsFile, err = os.Create("stats.csv")
		if err != nil {
			fmt.Printf("创建统计文件失败: %v\n", err)
			return
		}
		defer statsFile.Close()

		// 写入CSV头
		fmt.Fprintf(statsFile, "时间点,当秒请求数,错误数量,平均延迟,p75_latency,p90_latency,p99_latency\n")
	}

	// 启动统计收集器
	var statsTicker *time.Ticker
	if w.enableSecondStats {
		statsTicker = time.NewTicker(time.Second)
		defer statsTicker.Stop()
	}

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

	// 启动统计收集
	if w.enableSecondStats {
		go func() {
			for {
				select {
				case <-w.stopChan:
					return
				case <-statsTicker.C:
					if stats := w.collectSecondStats(); stats != nil {
						fmt.Fprintf(statsFile, "%s,%d,%d,%d,%d,%d,%d\n",
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

	// 等待所有工作协程完成
	w.wg.Wait()

	// 计算每秒请求数
	w.stats.RequestsPerSec = float64(w.stats.TotalRequests) / w.duration.Seconds()

	// 写入最后一秒的统计
	if w.enableSecondStats {
		if stats := w.collectSecondStats(); stats != nil {
			fmt.Fprintf(statsFile, "%s,%d,%d,%d,%d,%d,%d\n",
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

func (w *Worker) PrintStats() {
	fmt.Printf("\n压测结果:\n")
	fmt.Printf("总请求数: %d\n", w.stats.TotalRequests)
	fmt.Printf("失败请求数: %d\n", w.stats.FailedRequests)
	fmt.Printf("超时请求数: %d\n", w.stats.TimeoutRequests)
	fmt.Printf("每秒请求数: %.2f\n", w.stats.RequestsPerSec)

	if w.stats.TotalRequests > 0 {
		fmt.Printf("最小延迟: %v\n", w.stats.MinLatency)
		fmt.Printf("最大延迟: %v\n", w.stats.MaxLatency)
		fmt.Printf("平均延迟: %v\n", time.Duration(int64(w.stats.TotalLatency)/w.stats.TotalRequests))
		fmt.Printf("总传输字节: %d\n", w.stats.TotalBytes)
	} else {
		fmt.Println("没有成功的请求，无法计算延迟统计")
	}
}

func main() {
	// 解析命令行参数
	url := flag.String("url", "", "目标URL")
	duration := flag.Int("duration", 60, "测试持续时间（秒）")
	concurrency := flag.Int("concurrency", 1, "并发数")
	filePath := flag.String("file", "", "输入文件路径，如果指定则使用文件内容作为请求体")
	flag.Parse()

	if *url == "" {
		fmt.Println("请指定目标URL")
		flag.Usage()
		return
	}

	// 创建请求生成器
	var generator RequestGenerator
	var err error
	if *filePath != "" {
		generator, err = NewFileGenerator(*filePath)
		if err != nil {
			fmt.Printf("创建文件生成器失败: %v\n", err)
			return
		}
	} else {
		generator = NewSimpleRequestGenerator()
	}

	// 初始化随机数生成器
	rand.Seed(time.Now().UnixNano())

	// 创建请求生成器
	generator = NewSimpleRequestGenerator()

	worker := NewWorker(*url, *concurrency, time.Duration(*duration)*time.Second, 5*time.Second, 0, generator, false)

	fmt.Printf("开始压测 %s\n", *url)
	fmt.Printf("并发数: %d, 持续时间: %d秒\n", *concurrency, *duration)
	fmt.Printf("请求超时: 5秒\n")
	fmt.Printf("每秒统计: %v\n", false)

	worker.Start()
	worker.PrintStats()
}
