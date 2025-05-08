package main

import (
	"bufio"
	"fmt"
	"os"
	"sync"
)

// FileGenerator 从文件中循环读取内容的生成器
type FileGenerator struct {
	filePath string
	lines    []string
	mu       sync.RWMutex
	index    int
}

// NewFileGenerator 创建一个新的文件生成器
func NewFileGenerator(filePath string) (*FileGenerator, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %v", filePath, err)
	}
	defer file.Close()

	// 读取所有行
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file %s: %v", filePath, err)
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("file %s is empty", filePath)
	}

	return &FileGenerator{
		filePath: filePath,
		lines:    lines,
		index:    0,
	}, nil
}

// Generate 生成下一行内容，如果到达文件末尾则从头开始
func (g *FileGenerator) Generate() ([]byte, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.index >= len(g.lines) {
		g.index = 0
	}

	line := g.lines[g.index]
	g.index++
	return []byte(line), nil
} 