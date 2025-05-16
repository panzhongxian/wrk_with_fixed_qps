package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"
)

func main() {
	var (
		url               string
		concurrency       int
		duration          int
		timeout           int
		qps               int
		maxWorkers        int
		enableSecondStats bool
		file              string
	)

	flag.StringVar(&url, "url", "http://localhost:8080/delay", "测试目标URL")
	flag.IntVar(&concurrency, "concurrency", 0, "并发数（与qps互斥）")
	flag.IntVar(&duration, "duration", 30, "测试持续时间(秒)")
	flag.IntVar(&timeout, "timeout", 5, "请求超时时间(秒)")
	flag.IntVar(&qps, "qps", 0, "每秒请求数（与concurrency互斥）")
	flag.IntVar(&maxWorkers, "max-workers", 2000, "QPS模式下的最大并发数")
	flag.BoolVar(&enableSecondStats, "enable-second-stats", false, "是否记录每秒的统计信息（不需要指定值，使用该参数即表示启用）")
	flag.StringVar(&file, "file", "", "输入文件路径，如果指定则使用文件内容作为请求体")

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
	if concurrency > 0 {
		fmt.Printf("  模式: 并发模式, 并发数: %d\n", concurrency)
	} else {
		fmt.Printf("  模式: QPS模式, QPS: %d\n", qps)
	}
	fmt.Printf("  持续时间: %d秒\n", duration)
	fmt.Printf("  超时时间: %d秒\n", timeout)
	fmt.Printf("  最大并发数: %d\n", maxWorkers)
	fmt.Printf("  每秒统计: %v\n", enableSecondStats)
	fmt.Printf("  文件路径: %s\n", file)
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
	if file != "" {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			fmt.Printf("错误：文件 %s 不存在\n", file)
			return
		}
	}

	// 初始化随机数生成器
	rand.Seed(time.Now().UnixNano())

	// 创建请求生成器
	var generator RequestGenerator
	var err error
	if file != "" {
		// 检查文件是否存在
		if _, err := os.Stat(file); os.IsNotExist(err) {
			fmt.Printf("错误：文件 %s 不存在\n", file)
			return
		}
		generator, err = NewFileGenerator(file)
		if err != nil {
			fmt.Printf("创建文件生成器失败: %v\n", err)
			return
		}
	} else {
		generator = NewSimpleRequestGenerator()
	}

	worker := NewWorker(url, concurrency, time.Duration(duration)*time.Second, time.Duration(timeout)*time.Second, qps, generator, enableSecondStats)
	worker.maxWorkers = int32(maxWorkers)

	fmt.Printf("开始压测...\n")

	worker.Start()
	worker.stats.PrintStats()
}
