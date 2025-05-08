package main

import (
	"encoding/json"
	"math/rand"
	"sync/atomic"
)

// RequestGenerator 请求生成器接口
type RequestGenerator interface {
	Generate() ([]byte, error)
}

// RandomDelayGenerator 随机延迟请求生成器
type RandomDelayGenerator struct {
	minDelay int64
	maxDelay int64
}

// NewRandomDelayGenerator 创建随机延迟请求生成器
func NewRandomDelayGenerator(minDelay, maxDelay int64) *RandomDelayGenerator {
	return &RandomDelayGenerator{
		minDelay: minDelay,
		maxDelay: maxDelay,
	}
}

// Generate 生成随机延迟的请求
func (g *RandomDelayGenerator) Generate() ([]byte, error) {
	delay := g.minDelay + rand.Int63n(g.maxDelay-g.minDelay+1)
	body := map[string]int64{"delay_ms": delay}
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