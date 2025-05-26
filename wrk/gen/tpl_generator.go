package gen

import (
	"encoding/csv"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
)

// TplGenerator 从CSV文件中读取数据并填充到模板中的生成器
type TplGenerator struct {
	filePaths []string
	template  string
	headers   []string
	records   [][]string
	index     int32
}

// extractTemplateVars 从模板中提取所有变量名
func extractTemplateVars(template string) []string {
	re := regexp.MustCompile(`\${([^}]+)}`)
	matches := re.FindAllStringSubmatch(template, -1)
	vars := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			vars = append(vars, match[1])
		}
	}
	return vars
}

// NewTplGenerator 创建一个新的模板生成器
func NewTplGenerator(filePath string, template string) (*TplGenerator, error) {
	// Split file paths by comma
	filePaths := strings.Split(filePath, ",")
	var allRecords [][]string
	var headers []string

	// Read all CSV files
	for _, path := range filePaths {
		path = strings.TrimSpace(path)
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open file %s: %v", path, err)
		}
		defer file.Close()

		// Read CSV file
		reader := csv.NewReader(file)
		records, err := reader.ReadAll()
		if err != nil {
			return nil, fmt.Errorf("error reading CSV file %s: %v", path, err)
		}

		if len(records) == 0 {
			return nil, fmt.Errorf("CSV file %s is empty", path)
		}

		// First file sets the headers
		if headers == nil {
			headers = records[0]
			if len(headers) > 0 && len(headers[0]) >= 3 && headers[0][0] == 0xEF && headers[0][1] == 0xBB && headers[0][2] == 0xBF {
				headers[0] = headers[0][3:]
			}
		} else {
			// For subsequent files, verify headers match
			if len(records[0]) != len(headers) {
				return nil, fmt.Errorf("file %s has different number of columns than first file", path)
			}
			for i, header := range records[0] {
				if header != headers[i] {
					return nil, fmt.Errorf("file %s has different header '%s' at position %d", path, header, i)
				}
			}
		}

		// Add data records (skip header row)
		allRecords = append(allRecords, records[1:]...)
	}

	if len(allRecords) == 0 {
		return nil, fmt.Errorf("all CSV files have no data rows")
	}

	// Validate template variables against headers
	templateVars := extractTemplateVars(template)
	headerMap := make(map[string]bool)
	for _, header := range headers {
		headerMap[header] = true
	}

	var missingVars []string
	for _, varName := range templateVars {
		if !headerMap[varName] {
			missingVars = append(missingVars, varName)
		}
	}

	if len(missingVars) > 0 {
		return nil, fmt.Errorf("template variables not found in CSV headers: %v", missingVars)
	}

	return &TplGenerator{
		filePaths: filePaths,
		template:  template,
		headers:   headers,
		records:   allRecords,
		index:     0,
	}, nil
}

func (g *TplGenerator) Generate() ([]byte, error) {
	// 获取当前索引并递增
	currentIndex := atomic.AddInt32((*int32)(&g.index), 1) - 1

	// 如果索引超出范围，重置为0
	if int(currentIndex) >= len(g.records) {
		atomic.StoreInt32(&g.index, 0)
		currentIndex = 0
	}

	record := g.records[currentIndex]

	// 将模板中的占位符替换为CSV中的值
	result := g.template
	for i, header := range g.headers {
		if i < len(record) {
			placeholder := fmt.Sprintf("${%s}", header)
			result = strings.ReplaceAll(result, placeholder, record[i])
		}
	}

	return []byte(result), nil
}
