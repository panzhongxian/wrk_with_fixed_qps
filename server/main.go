package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/panzhongxian/wrkx/pkg/counter"
)

type DelayRequest struct {
	DelayMs int64 `json:"delay_ms"`
}

func main() {
	// 创建计数器实例
	requestCounter, err := counter.NewCounter()
	if err != nil {
		log.Fatalf("Failed to create counter: %v", err)
	}

	// 启动计数器报告协程
	ctx := context.Background()
	go requestCounter.StartReporting(ctx)

	// 创建自定义的HTTP服务器
	server := &http.Server{
		Addr: ":8080",
		// 设置读写超时
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		// 设置空闲超时
		IdleTimeout: 300 * time.Second,
		ConnState: func(conn net.Conn, state http.ConnState) {
			switch state {
			case http.StateNew:
				// 新连接建立
				requestCounter.IncrementConnections()
			case http.StateClosed, http.StateHijacked:
				// 连接关闭或被劫持
				requestCounter.DecrementConnections()
			}
		},
	}

	http.HandleFunc("/delay", func(w http.ResponseWriter, r *http.Request) {
		// 增加并发请求计数
		requestCounter.IncrementConcurrent()
		// 确保在函数返回时减少并发请求计数
		defer requestCounter.DecrementConcurrent()

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// 确保请求体被关闭
		defer r.Body.Close()

		var req DelayRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("Failed to decode request body: %v", err)
			http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
			return
		}

		// 增加请求计数器
		requestCounter.Increment()

		// 执行延迟
		time.Sleep(time.Duration(req.DelayMs) * time.Millisecond)

		// 返回响应
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "OK"}); err != nil {
			log.Printf("Failed to encode response: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	})

	// 添加一个端点来查看当前统计信息
	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"active_connections":  requestCounter.GetActiveConnections(),
			"concurrent_requests": requestCounter.GetConcurrentRequests(),
		})
	})

	// 启动服务器
	fmt.Printf("Server starting on port 8080...\n")
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
