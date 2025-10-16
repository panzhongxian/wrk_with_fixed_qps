package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"git.woa.com/jasonzxpan/wrk_server/wrkx/gen"
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
	generator   gen.RequestGenerator
	// QPS模式下的并发控制
	activeWorkers int32
	maxWorkers    int32
	workerChan    chan struct{}
	// 每秒统计收集器
	statsCollector *SecondStatsCollector
	// HTTP请求相关
	method  string
	headers map[string]string
	srcIP   string
	client  *http.Client
}

// createDialContext 创建一个支持指定source IP和DNS缓存的DialContext
func createDialContext(srcIP string) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
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
				dialer := &net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}

				// 如果指定了source IP，则使用它
				if srcIP != "" {
					localAddr, err := net.ResolveIPAddr("ip", srcIP)
					if err != nil {
						return nil, fmt.Errorf("无法解析source IP %s: %v", srcIP, err)
					}
					dialer.LocalAddr = localAddr
				}

				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip, port))
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
}

// createClient 创建一个新的HTTP客户端
func createClient(srcIP string) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:           10000,                    // 增加最大空闲连接数
		MaxIdleConnsPerHost:    10000,                    // 增加每个主机的最大空闲连接数
		MaxConnsPerHost:        10000,                    // 增加每个主机的最大连接数
		IdleConnTimeout:        60 * time.Second,         // 空闲连接超时时间
		DisableCompression:     true,                     // 禁用压缩
		ResponseHeaderTimeout:  30 * time.Second,         // 响应头超时时间
		ExpectContinueTimeout:  2 * time.Second,          // 100-continue超时时间
		DialContext:            createDialContext(srcIP), // 使用指定的source IP
		TLSHandshakeTimeout:    10 * time.Second,         // TLS握手超时时间
		MaxResponseHeaderBytes: 4096,                     // 限制响应头大小
		WriteBufferSize:        4096,                     // 写缓冲区大小
		ReadBufferSize:         4096,                     // 读缓冲区大小
	}

	return &http.Client{
		Transport: transport,
		// 移除全局超时，使用 context 控制单个请求超时
	}
}

func NewWorker(url string, concurrency int, duration time.Duration, timeout time.Duration, qps int, generator gen.RequestGenerator, enableSecondStats bool, method string, headers string, srcIP string) *Worker {
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

	// 解析headers字符串
	headersMap := make(map[string]string)
	if headers != "" {
		headerPairs := strings.Split(headers, ",")
		for _, pair := range headerPairs {
			pair = strings.TrimSpace(pair)
			if pair != "" {
				parts := strings.SplitN(pair, ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					headersMap[key] = value
				}
			}
		}
	}

	// 创建HTTP客户端
	client := createClient(srcIP)

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
		method:         method,
		headers:        headersMap,
		srcIP:          srcIP,
		client:         client,
	}
}

func (w *Worker) makeRequest() {
	jsonBody, err := w.generator.Generate()
	if err != nil {
		w.stats.RecordError()
		return
	}

	req, err := http.NewRequest(w.method, w.url, bytes.NewBuffer(jsonBody))
	if err != nil {
		fmt.Printf("创建请求失败: %v\n", err)
		w.stats.RecordError()
		return
	}

	// 设置默认的Content-Type头部
	req.Header.Set("Content-Type", "application/json")

	// 设置用户指定的额外头部
	for key, value := range w.headers {
		req.Header.Set(key, value)
	}

	start := time.Now()

	// 使用 context 控制单个请求的超时
	ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()

	req = req.WithContext(ctx)
	resp, err := w.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			atomic.AddInt64(&w.stats.TimeoutRequests, 1)
		}
		fmt.Printf("请求失败: %v\n", err)
		w.stats.RecordError()
		return
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	_ = body

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("请求返回非200状态码: %d, 请求体: %s\n", resp.StatusCode, string(jsonBody))
		w.stats.RecordError()
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
					w.stats.RecordError()
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
