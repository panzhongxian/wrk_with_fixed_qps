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
	filePath string
	template string
	headers  []string
	records  [][]string
	index    int32
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
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %v", filePath, err)
	}
	defer file.Close()

	// 读取CSV文件
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("error reading CSV file %s: %v", filePath, err)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("CSV file %s is empty", filePath)
	}

	// 第一行作为表头
	headers := records[0]
	if len(headers) > 0 && len(headers[0]) >= 3 && headers[0][0] == 0xEF && headers[0][1] == 0xBB && headers[0][2] == 0xBF {
		headers[0] = headers[0][3:]
	}
	// 剩余行作为数据
	dataRecords := records[1:]

	if len(dataRecords) == 0 {
		return nil, fmt.Errorf("CSV file %s has no data rows", filePath)
	}

	// 校验1: 确保每一行的列数与表头数量相同
	for i, record := range dataRecords {
		if len(record) != len(headers) {
			return nil, fmt.Errorf("row %d has %d columns, but header has %d columns", i+2, len(record), len(headers))
		}
	}

	// 校验2: 确保模板中的所有变量都在CSV表头中存在
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
		filePath: filePath,
		template: template,
		headers:  headers,
		records:  dataRecords,
		index:    0,
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
