# HTTP 压测工具

这是一个简单的 HTTP 压测工具，用于于 HTTP POST 请求的测试。
它支持两种压测模式：**固定并发模式**和 **固定QPS 模式**。支持按照秒级统计请求延迟和 QPS。

![effect](images/effect.png)

## 特性

- **并发模式**和**QPS模式**两种压测方式
- 支持所有HTTP方法（GET、POST、PUT、DELETE等）
- 自定义HTTP头部和源IP绑定
- 实时延迟统计（最小、最大、平均、分位数）
- 秒级统计和CSV报告生成
- 失败请求和超时请求计数
- 从文件或CSV模板生成请求体
- Web UI界面和Redis统计存储
- DNS缓存和连接池优化

## 项目结构

项目采用标准的Go项目布局，分为以下主要目录：

```
wrkx/
├── cmd/                    # 可执行程序入口
│   ├── server/            # HTTP测试服务器
│   │   ├── main.go        # 服务器程序入口，提供延迟测试接口
│   │   └── README.md      # 服务器说明文档
│   └── wrkx/              # 压测工具
│       └── main.go        # 压测工具程序入口，包含命令行参数处理和压测启动逻辑
├── internal/              # 内部包（不对外暴露）
│   ├── counter/           # 计数器逻辑，支持Redis统计和连接数监控
│   │   └── counter.go     # 请求计数和Redis统计
│   ├── gen/               # 请求生成器
│   │   ├── generator.go   # 请求生成器接口和基础实现，定义请求生成器接口和基础实现
│   │   ├── file_generator.go    # 从文件循环读取内容的生成器
│   │   └── tpl_generator.go     # 从CSV文件生成请求体的模板生成器
│   └── worker/            # 压测工作器
│       ├── worker.go      # 主要的压测逻辑和HTTP客户端管理
│       ├── stat.go       # 统计信息收集和报告，支持秒级统计
│       └── dns_cache.go  # DNS缓存实现，提高连接性能
├── ui/                    # Web UI界面
├── images/                # 项目图片资源
└── README.md              # 项目说明文档
```

## 使用方法

### 安装

```bash
go install github.com/panzhongxian/wrkx/cmd/wrkx@latest
```

从源码安装：

```bash
git clone <repository-url>
go build -o wrkx ./cmd/wrkx
```

### 使用压测工具

```bash
./wrkx [选项参数]
```

#### 压测模式参数（二选一）

- `--concurrency`: 并发数（与 qps 互斥），适合测试**固定并发**下的性能
- `--qps`: 每秒请求数（与 concurrency 互斥），适合测试**固定请求频率**下的性能
- `--max-workers`: QPS 模式下的最大并发数（默认：2000）

#### 通用参数

- `--url`: 测试目标URL（默认：http://localhost:8080/delay）
- `--method`: HTTP请求方法（默认：POST），支持所有标准HTTP方法：GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS等
- `--header`: 额外的HTTP头部，格式为key1:value1,key2:value2（可选）
- `--duration`: 测试持续时间，单位秒（默认：30）
- `--timeout`: 请求超时时间，单位秒（默认：5）
- `--src-ip`: 指定源IP地址，用于绑定网络连接（可选）
- `--enable-second-stats`: 是否记录每秒的统计信息（不需要指定值，使用该参数即表示启用）

#### 请求来源参数（三选一）

- `--request`: 直接指定请求体字符串。使用此选项时，`--file` 和 `--req-template` 必须为空
- `--file`: 输入文件路径，如果指定则使用文件内容作为请求体
- `--req-template`: 请求模板，用于从CSV文件生成请求体。使用此选项时必须同时指定 `--file` 参数，且文件必须是CSV格式

### 参数使用详细说明

#### 压测模式选择

1. 并发模式：

```bash
./wrkx --url http://localhost:8080/api --concurrency 100 --duration 30
```

2. QPS模式：

```bash
./wrkx --url http://localhost:8080/api --qps 100 --duration 30
```

#### HTTP方法和头部设置

1. 使用GET方法：

```bash
./wrkx --url http://localhost:8080/api --method GET --qps 100
```

2. 添加自定义头部：

```bash
./wrkx --url http://localhost:8080/api --header "Authorization:Bearer token123,X-Custom-Header:value" --qps 100
```

3. 组合使用不同方法和头部：

```bash
./wrkx --url http://localhost:8080/api --method PUT --header "Content-Type:application/xml,Authorization:Bearer token" --qps 100
```

4. 使用指定的源IP地址：

```bash
./wrkx --url http://localhost:8080/api --src-ip 192.168.1.100 --qps 100
```

#### 请求来源选择

1. 使用固定请求体：

```bash
./wrkx --url http://localhost:8080/api --request '{"key": "value"}' --qps 100
```

2. 使用文件内容：

```bash
./wrkx --url http://localhost:8080/api --file requests.txt --qps 100
```

3. 使用CSV文件和模板：

```bash
./wrkx --url http://localhost:8080/api \
      --file data.csv \
      --req-template '{"name": "${name}", "age": "${age}", "city": "${city}"}' \
      --qps 100
```

### 输出说明

工具会输出以下统计信息：

- 总请求数
- 失败请求数
- 超时请求数
- 每秒请求数（QPS）
- 最小延迟
- 最大延迟
- 平均延迟
- 总传输字节

当启用 `--enable-second-stats` 时，会生成 stats.csv 文件，包含以下信息：

- 时间点
- 当秒请求数
- 错误数量
- 平均延迟
- P75 延迟
- P90 延迟
- P99 延迟

### 示例输出

```
开始压测 http://localhost:8080/delay
QPS: 1000, 持续时间: 30秒
最大并发数: 200
请求超时: 5秒
源IP地址: 192.168.1.100

压测结果:
总请求数: 30000
失败请求数: 0
超时请求数: 0
每秒请求数: 1000.00
最小延迟: 50ms
最大延迟: 200ms
平均延迟: 125ms
总传输字节: 1200000
``` 

## 被压测服务器

本项目包含一个简单的HTTP测试服务器，提供延迟测试接口和统计信息接口，方便进行压测测试。

### 编译测试服务器

```bash
go build -o server ./cmd/server
```

### 启动测试服务器

首先启动测试服务器（可选，用于测试压测工具）：

```bash
# 启动服务器
./server

# 或者使用go run
go run ./cmd/server
```

服务器将在 `http://localhost:8080` 启动，提供以下接口：

- `POST /delay`: 延迟测试接口
- `GET /stats`: 查看当前统计信息

## UI 的使用

可以直接启动 Web UI 服务，通过页面进行压力测试，能够清晰的展示压测结果。

启动方法：

```bash
cd ui && python app.py
```

浏览器访问<http://127.0.0.1:8081/> 即可开始压测：

![web-ui](images/webui.png)

