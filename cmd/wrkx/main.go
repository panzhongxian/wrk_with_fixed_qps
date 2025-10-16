package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/panzhongxian/wrkx/internal/gen"
	"github.com/panzhongxian/wrkx/internal/worker"
)

// isIPAvailable 检查指定的IP地址是否可用
func isIPAvailable(ip string) bool {
	// 解析IP地址
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// 获取所有网络接口
	interfaces, err := net.Interfaces()
	if err != nil {
		return false
	}

	// 检查IP是否属于任何网络接口
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ifaceIP net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ifaceIP = v.IP
			case *net.IPAddr:
				ifaceIP = v.IP
			}
			if ifaceIP != nil && ifaceIP.Equal(parsedIP) {
				return true
			}
		}
	}

	return false
}

func main() {
	var (
		url               string
		concurrency       int
		duration          int
		timeout           float64
		qps               int
		maxWorkers        int
		enableSecondStats bool
		file              string
		reqTemplate       string
		request           string
		method            string
		headers           string
		srcIP             string
	)

	flag.StringVar(&url, "url", "http://localhost:8080/delay", "测试目标URL")
	flag.IntVar(&concurrency, "concurrency", 0, "并发数（与qps互斥）")
	flag.IntVar(&duration, "duration", 30, "测试持续时间(秒)")
	flag.Float64Var(&timeout, "timeout", 5, "请求超时时间(秒)")
	flag.IntVar(&qps, "qps", 0, "每秒请求数（与concurrency互斥）")
	flag.IntVar(&maxWorkers, "max-workers", 2000, "QPS模式下的最大并发数")
	flag.BoolVar(&enableSecondStats, "enable-second-stats", false, "是否记录每秒的统计信息（不需要指定值，使用该参数即表示启用）")
	flag.StringVar(&file, "file", "", "输入文件路径，如果指定则使用文件内容作为请求体")
	flag.StringVar(&reqTemplate, "req-template", "", "请求模板，用于从CSV文件生成请求体")
	flag.StringVar(&request, "request", "", "请求体字符串，如果指定则file和req-template必须为空")
	flag.StringVar(&method, "method", "POST", "HTTP请求方法")
	flag.StringVar(&headers, "header", "", "额外的HTTP头部，格式为key1:value1,key2:value2")
	flag.StringVar(&srcIP, "src-ip", "", "指定源IP地址，用于绑定网络连接")

	// 检查是否有未定义的参数
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: %s [选项]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "选项:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n注意：布尔类型参数（如 --enable-second-stats）不需要指定值，直接使用参数名即可\n")
	}

	// 检查是否有未定义的参数
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			// 检查是否是布尔参数
			if arg == "--enable-second-stats" {
				continue
			}
			// 检查下一个参数是否是值
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++ // 跳过值
				continue
			}
			// 检查是否是等号形式
			if strings.Contains(arg, "=") {
				continue
			}
			fmt.Printf("错误：未知参数 %s\n", arg)
			flag.Usage()
			os.Exit(1)
		}
	}

	flag.Parse()

	// 打印所有参数值，帮助调试
	fmt.Printf("参数值:\n")
	fmt.Printf("  URL: %s\n", url)
	fmt.Printf("  请求方法: %s\n", method)
	if headers != "" {
		fmt.Printf("  额外头部: %s\n", headers)
	}
	if concurrency > 0 {
		fmt.Printf("  模式: 并发模式, 并发数: %d\n", concurrency)
	} else {
		fmt.Printf("  模式: QPS模式, QPS: %d\n", qps)
	}
	fmt.Printf("  持续时间: %d秒\n", duration)
	fmt.Printf("  超时时间: %.3f秒\n", timeout)
	fmt.Printf("  最大并发数: %d\n", maxWorkers)
	fmt.Printf("  每秒统计: %v\n", enableSecondStats)
	if srcIP != "" {
		fmt.Printf("  源IP地址: %s\n", srcIP)
	}

	if request != "" {
		fmt.Printf("  请求体: %s\n", request)
	} else if reqTemplate != "" {
		fmt.Printf("  请求模板: %s\n", reqTemplate)
		fmt.Printf("  文件路径: %s\n", file)
	} else if file != "" {
		fmt.Printf("  文件路径: %s\n", file)
	}
	fmt.Println()

	// 验证参数
	if concurrency > 0 && qps > 0 {
		fmt.Println("错误：concurrency 和 qps 参数不能同时使用")
		return
	}
	if concurrency == 0 && qps == 0 {
		fmt.Println("错误：必须指定 concurrency 或 qps 参数")
		return
	}
	if qps > 0 && maxWorkers <= 0 {
		fmt.Println("错误：QPS模式下必须指定大于0的max-workers参数")
		return
	}

	// 验证request参数
	if request != "" && (file != "" || reqTemplate != "") {
		fmt.Println("错误：使用 --request 参数时，--file 和 --req-template 必须为空")
		return
	}

	// 验证src-ip参数
	if srcIP != "" {
		if net.ParseIP(srcIP) == nil {
			fmt.Printf("错误：无效的IP地址 %s\n", srcIP)
			return
		}
		// 检查IP地址是否可用
		if !isIPAvailable(srcIP) {
			fmt.Printf("错误：IP地址 %s 不可用或不存在于本机\n", srcIP)
			return
		}
	}

	// 验证文件相关参数
	if reqTemplate != "" && file == "" {
		fmt.Println("错误：使用 --req-template 时必须指定 --file 参数")
		return
	}
	if file != "" {
		// Split files by comma and validate each one
		files := strings.Split(file, ",")
		for _, f := range files {
			f = strings.TrimSpace(f)
			if _, err := os.Stat(f); os.IsNotExist(err) {
				fmt.Printf("错误：文件 %s 不存在\n", f)
				return
			}
			// If template is specified, validate CSV format for each file
			if reqTemplate != "" {
				ext := strings.ToLower(filepath.Ext(f))
				if ext != ".csv" {
					fmt.Printf("错误：使用 --req-template 时，文件 %s 必须是CSV格式\n", f)
					return
				}
			}
		}
	}

	// 初始化随机数生成器
	rand.Seed(time.Now().UnixNano())

	// 创建请求生成器
	var reqGenerator gen.RequestGenerator
	var err error
	if file != "" {
		if reqTemplate != "" {
			// 使用模板生成器
			reqGenerator, err = gen.NewTplGenerator(file, reqTemplate)
			if err != nil {
				fmt.Printf("创建模板生成器失败: %v\n", err)
				return
			}
		} else {
			// 使用文件生成器
			reqGenerator, err = gen.NewFileGenerator(file)
			if err != nil {
				fmt.Printf("创建文件生成器失败: %v\n", err)
				return
			}
		}
	} else if request != "" {
		reqGenerator = gen.NewSimpleRequestGenerator(request)
	} else {
		reqGenerator = gen.NewCustomRequestGenerator()
	}

	w := worker.NewWorker(url, concurrency, time.Duration(duration)*time.Second, time.Duration(timeout*1000)*time.Millisecond, qps, reqGenerator, enableSecondStats, method, headers, srcIP)
	w.SetMaxWorkers(int32(maxWorkers))

	fmt.Printf("开始压测...\n")

	w.Start()
	w.GetStats().PrintStats()
}
