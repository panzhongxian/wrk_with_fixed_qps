package gen

import (
	"bufio"
	"fmt"
	"os"
	"sync/atomic"
)

// FileGenerator 从文件中循环读取内容的生成器
type FileGenerator struct {
	filePath string
	lines    []string
	index    int32
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
	// 获取当前索引并递增
	currentIndex := atomic.AddInt32((*int32)(&g.index), 1) - 1

	// 如果索引超出范围，重置为0
	if int(currentIndex) >= len(g.lines) {
		atomic.StoreInt32(&g.index, 0)
		currentIndex = 0
	}

	line := g.lines[currentIndex]
	return []byte(line), nil
}
