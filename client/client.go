package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// StartConcurrentRequests starts concurrent HTTP requests with the specified concurrency level
func StartConcurrentRequests(url string, concurrency int) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a single HTTP client with proper configuration
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 1000,
		},
	}
	//http.Transport{}.RoundTrip()

	// Start concurrent requests
	for i := 0; i < concurrency; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte("{\"delay\": 1}")))
					if err != nil {
						fmt.Printf("Error creating request: %v\n", err)
						continue
					}
					req.Header.Set("Content-Type", "application/json")

					resp, err := client.Do(req)
					if err != nil {
						fmt.Printf("Error making request: %v\n", err)
						continue
					}

					body, err := io.ReadAll(resp.Body)
					resp.Body.Close()
					if err != nil {
						fmt.Printf("Error reading response body: %v\n", err)
						continue
					}

					if resp.StatusCode != http.StatusOK {
						fmt.Printf("\nRequest failed with status: %d, body: %s\n", resp.StatusCode, string(body))
					}
				}
			}
		}()
	}

	// Wait for interrupt signal
	select {
	case <-ctx.Done():
		return
	}
}

func main() {
	// 设置目标URL和并发数
	url := "http://wrk-test-server.shcdpdsp-in.woa.com/delay" // 替换为你的目标URL
	concurrency := 10

	fmt.Printf("Starting %d concurrent requests to %s\n", concurrency, url)
	fmt.Println("Press Ctrl+C to stop")

	// 创建一个通道来接收系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动并发请求
	go StartConcurrentRequests(url, concurrency)

	// 等待中断信号
	<-sigChan
	fmt.Println("\nShutting down...")
}
