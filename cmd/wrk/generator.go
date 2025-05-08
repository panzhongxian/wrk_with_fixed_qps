package main

import (
	"encoding/json"
	"sync/atomic"
)

// RequestGenerator 请求生成器接口
type RequestGenerator interface {
	Generate() ([]byte, error)
}

// SimpleRequestGenerator 简单请求生成器
type SimpleRequestGenerator struct{}

// NewSimpleRequestGenerator 创建简单请求生成器
func NewSimpleRequestGenerator() *SimpleRequestGenerator {
	return &SimpleRequestGenerator{}
}

// Generate 生成请求
func (g *SimpleRequestGenerator) Generate() ([]byte, error) {
	body := map[string]interface{}{"message": "test", "delay": 1}
	return json.Marshal(body)
}

// CustomRequestGenerator 自定义请求生成器
type CustomRequestGenerator struct {
	requests [][]byte
	index    int64
}

// NewCustomRequestGenerator 创建自定义请求生成器
func NewCustomRequestGenerator(requests [][]byte) *CustomRequestGenerator {
	return &CustomRequestGenerator{
		requests: requests,
	}
}

// Generate 按顺序生成请求
func (g *CustomRequestGenerator) Generate() ([]byte, error) {
	index := atomic.AddInt64(&g.index, 1) % int64(len(g.requests))
	return g.requests[index], nil
}
