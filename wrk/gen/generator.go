package gen

import (
	"encoding/json"
	"time"
)

// RequestGenerator 请求生成器接口
type RequestGenerator interface {
	Generate() ([]byte, error)
}

// SimpleRequestGenerator 简单请求生成器
type SimpleRequestGenerator struct {
	req string
}

// NewSimpleRequestGenerator 创建简单请求生成器
func NewSimpleRequestGenerator(req string) *SimpleRequestGenerator {
	return &SimpleRequestGenerator{
		req: req,
	}
}

// Generate 生成请求
func (g *SimpleRequestGenerator) Generate() ([]byte, error) {
	return []byte(g.req), nil

}

// CustomRequestGenerator 自定义请求生成器
type CustomRequestGenerator struct {
	index int64
}

// NewCustomRequestGenerator 创建自定义请求生成器
func NewCustomRequestGenerator() *CustomRequestGenerator {
	return &CustomRequestGenerator{}
}

// Generate 按顺序生成请求
func (g *CustomRequestGenerator) Generate() ([]byte, error) {
	// 获取当前时间的秒数
	now := time.Now()
	minute := now.Minute()

	// 计算delay
	delay := 0
	flag := minute % 4
	if flag < 2 {
		delay = 25
	}

	body := map[string]interface{}{"delay_ms": int64(delay)}
	return json.Marshal(body)
}
