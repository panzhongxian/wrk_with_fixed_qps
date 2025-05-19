package main

import (
	"net/http"
	"time"
)

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
