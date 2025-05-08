package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type RequestStats struct {
	TotalRequests    int64
	FailedRequests   int64
	TotalLatency     time.Duration
	MinLatency       time.Duration
	MaxLatency       time.Duration
	RequestsPerSec   float64
	TotalBytes       int64
}

// 请求生成器接口
type RequestGenerator interface {
	Generate() ([]byte, error)
}

// 随机延迟请求生成器
type RandomDelayGenerator struct {
	minDelay int64
	maxDelay int64
}

func NewRandomDelayGenerator(minDelay, maxDelay int64) *RandomDelayGenerator {
	return &RandomDelayGenerator{
		minDelay: minDelay,
		maxDelay: maxDelay,
	}
}

func (g *RandomDelayGenerator) Generate() ([]byte, error) {
	delay := g.minDelay + rand.Int63n(g.maxDelay-g.minDelay+1)
	body := map[string]int64{"delay_ms": delay}
	return json.Marshal(body)
}

// 自定义请求生成器
type CustomRequestGenerator struct {
	requests [][]byte
	index    int64
}

func NewCustomRequestGenerator(requests [][]byte) *CustomRequestGenerator {
	return &CustomRequestGenerator{
		requests: requests,
	}
}

func (g *CustomRequestGenerator) Generate() ([]byte, error) {
	index := atomic.AddInt64(&g.index, 1) % int64(len(g.requests))
	return g.requests[index], nil
}

type Worker struct {
	client      *http.Client
	url         string
	concurrency int
	duration    time.Duration
	stats       *RequestStats
	wg          *sync.WaitGroup
	stopChan    chan struct{}
	generator   RequestGenerator
}

func NewWorker(url string, concurrency int, duration time.Duration, generator RequestGenerator) *Worker {
	return &Worker{
		client: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        concurrency,
				MaxIdleConnsPerHost: concurrency,
				IdleConnTimeout:     30 * time.Second,
			},
		},
		url:         url,
		concurrency: concurrency,
		duration:    duration,
		stats:       &RequestStats{MinLatency: time.Hour},
		wg:          &sync.WaitGroup{},
		stopChan:    make(chan struct{}),
		generator:   generator,
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
		atomic.AddInt64(&w.stats.FailedRequests, 1)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := w.client.Do(req)
	if err != nil {
		atomic.AddInt64(&w.stats.FailedRequests, 1)
		return
	}
	defer resp.Body.Close()

	latency := time.Since(start)
	
	// 更新统计信息
	atomic.AddInt64(&w.stats.TotalRequests, 1)
	atomic.AddInt64(&w.stats.TotalBytes, resp.ContentLength)
	
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

func (w *Worker) Start() {
	// 启动工作协程
	for i := 0; i < w.concurrency; i++ {
		w.wg.Add(1)
		go w.worker()
	}

	// 设置测试时间
	time.AfterFunc(w.duration, func() {
		close(w.stopChan)
	})

	// 等待所有工作协程完成
	w.wg.Wait()

	// 计算每秒请求数
	w.stats.RequestsPerSec = float64(w.stats.TotalRequests) / w.duration.Seconds()
}

func (w *Worker) PrintStats() {
	fmt.Printf("\n压测结果:\n")
	fmt.Printf("总请求数: %d\n", w.stats.TotalRequests)
	fmt.Printf("失败请求数: %d\n", w.stats.FailedRequests)
	fmt.Printf("每秒请求数: %.2f\n", w.stats.RequestsPerSec)
	fmt.Printf("最小延迟: %v\n", w.stats.MinLatency)
	fmt.Printf("最大延迟: %v\n", w.stats.MaxLatency)
	fmt.Printf("平均延迟: %v\n", time.Duration(int64(w.stats.TotalLatency)/w.stats.TotalRequests))
	fmt.Printf("总传输字节: %d\n", w.stats.TotalBytes)
}

func main() {
	var (
		url         string
		concurrency int
		duration    int
		minDelay    int64
		maxDelay    int64
	)

	flag.StringVar(&url, "url", "http://localhost:8080/delay", "测试目标URL")
	flag.IntVar(&concurrency, "c", 100, "并发数")
	flag.IntVar(&duration, "d", 30, "测试持续时间(秒)")
	flag.Int64Var(&minDelay, "min", 50, "最小延迟(毫秒)")
	flag.Int64Var(&maxDelay, "max", 200, "最大延迟(毫秒)")
	flag.Parse()

	// 初始化随机数生成器
	rand.Seed(time.Now().UnixNano())

	// 创建请求生成器
	generator := NewRandomDelayGenerator(minDelay, maxDelay)

	worker := NewWorker(url, concurrency, time.Duration(duration)*time.Second, generator)
	
	fmt.Printf("开始压测 %s\n", url)
	fmt.Printf("并发数: %d, 持续时间: %d秒\n", concurrency, duration)
	fmt.Printf("延迟范围: %d-%d毫秒\n", minDelay, maxDelay)
	
	worker.Start()
	worker.PrintStats()
} 