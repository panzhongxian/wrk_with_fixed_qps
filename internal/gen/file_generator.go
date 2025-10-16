package gen

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

// FileGenerator 从文件中循环读取内容的生成器
type FileGenerator struct {
	filePaths []string
	lines     []string
	index     int32
}

// NewFileGenerator 创建一个新的文件生成器
func NewFileGenerator(filePath string) (*FileGenerator, error) {
	// Split file paths by comma
	filePaths := strings.Split(filePath, ",")
	var allLines []string

	// Read all files
	for _, path := range filePaths {
		path = strings.TrimSpace(path)
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open file %s: %v", path, err)
		}
		defer file.Close()

		// Read all lines from this file
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			allLines = append(allLines, scanner.Text())
		}

		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("error reading file %s: %v", path, err)
		}
	}

	if len(allLines) == 0 {
		return nil, fmt.Errorf("all files are empty")
	}

	return &FileGenerator{
		filePaths: filePaths,
		lines:     allLines,
		index:     0,
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
