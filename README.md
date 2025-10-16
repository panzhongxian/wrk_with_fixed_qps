# HTTP 压测工具

这是一个简单的 HTTP 压测工具，用于于 HTTP POST 请求的测试。
它支持两种压测模式：**固定并发模式**和 **固定QPS 模式**。支持按照秒级统计请求延迟和 QPS。

![effect](images/effect.png)

## 特性

- 支持并发模式和 QPS 模式
- 支持自定义HTTP方法（GET, POST, PUT, DELETE等）
- 支持自定义HTTP头部
- 请求延迟统计（最小、最大、平均）
- QPS（每秒查询数、耗时分位数等）按秒统计
- 失败请求计数，落本地日志（TODO）
- 超时请求计数
- 总传输字节统计
- 可自定义测试持续时间
- 可自定义请求超时时间
- QPS 模式下的并发数限制
- 支持从文件读取请求体
- 支持从CSV文件生成请求体（使用模板）
- 支持WEB UI页面

## 项目结构

项目采用标准的Go项目布局，分为以下主要目录：

### 目录结构
```
wrk_with_fixed_qps/
├── cmd/                    # 可执行程序入口
│   ├── server/            # HTTP测试服务器
│   │   ├── main.go        # 服务器程序入口
│   │   └── README.md      # 服务器说明文档
│   └── wrkx/              # 压测工具
│       └── main.go        # 压测工具程序入口
├── internal/              # 内部包（不对外暴露）
│   ├── counter/           # 计数器逻辑
│   │   └── counter.go     # 请求计数和Redis统计
│   ├── gen/               # 请求生成器
│   │   ├── generator.go   # 请求生成器接口和基础实现
│   │   ├── file_generator.go    # 从文件读取请求体
│   │   └── tpl_generator.go     # 从CSV模板生成请求体
│   └── worker/            # 压测工作器
│       ├── worker.go      # 主要的压测逻辑和HTTP客户端管理
│       ├── stat.go       # 统计信息收集和报告
│       └── dns_cache.go  # DNS缓存实现
├── ui/                    # Web UI界面
├── images/                # 项目图片资源
└── README.md              # 项目说明文档
```

### 核心组件

#### 压测工具 (`cmd/wrkx/`)
- `main.go`: 程序入口，包含命令行参数处理和压测启动逻辑

#### 计数器 (`internal/counter/`)
- `counter.go`: 请求计数器，支持Redis统计和连接数监控

#### 请求生成器 (`internal/gen/`)
- `generator.go`: 定义请求生成器接口和基础实现
    - `RequestGenerator` 接口：定义请求生成的标准接口
    - `SimpleRequestGenerator`: 生成简单测试请求
    - `CustomRequestGenerator`: 使用预定义请求列表的生成器
- `file_generator.go`: 从文件循环读取内容的生成器
- `tpl_generator.go`: 从CSV文件生成请求体的模板生成器

#### 压测工作器 (`internal/worker/`)
- `worker.go`: 包含主要的压测逻辑，管理HTTP客户端和请求处理
- `stat.go`: 统计信息收集和报告，支持秒级统计
- `dns_cache.go`: DNS缓存实现，提高连接性能

#### 测试服务器 (`cmd/server/`)
- `main.go`: HTTP测试服务器，提供延迟测试接口

### 架构设计

项目采用分层架构设计，遵循Go语言的最佳实践：

1. **入口层 (`cmd/`)**: 只包含程序入口逻辑，负责参数解析和程序启动
2. **业务逻辑层 (`internal/`)**: 包含所有核心业务逻辑，不对外暴露
3. **资源层**: 包含UI界面、图片等静态资源

**设计优势**:
- 清晰的职责分离，便于维护和测试
- 内部包不对外暴露，可以安全重构
- 符合Go社区标准，便于团队协作
- 支持独立编译和部署

## 编译

### 编译压测工具
```bash
go build -o wrkx ./cmd/wrkx
```

### 编译测试服务器
```bash
go build -o server ./cmd/server
```

### 同时编译所有程序
```bash
# 编译压测工具
go build -o bin/wrkx ./cmd/wrkx

# 编译测试服务器
go build -o bin/server ./cmd/server
```

## 使用方法

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

### 使用压测工具

```bash
./wrkx [选项]
```

### 命令行参数

#### 压测模式参数（二选一）
- `--concurrency`: 并发数（与 qps 互斥）
- `--qps`: 每秒请求数（与 concurrency 互斥）
- `--max-workers`: QPS 模式下的最大并发数（默认：2000）

#### 通用参数
- `--url`: 测试目标URL（默认：http://localhost:8080/delay）
- `--method`: HTTP请求方法（默认：POST）
- `--header`: 额外的HTTP头部，格式为key1:value1,key2:value2（可选）
- `--duration`: 测试持续时间，单位秒（默认：30）
- `--timeout`: 请求超时时间，单位秒（默认：5）
- `--src-ip`: 指定源IP地址，用于绑定网络连接（可选）
- `--enable-second-stats`: 是否记录每秒的统计信息（不需要指定值，使用该参数即表示启用）

#### 请求来源参数（三选一）
- `--request`: 直接指定请求体字符串。使用此选项时，`--file` 和 `--req-template` 必须为空
- `--file`: 输入文件路径，如果指定则使用文件内容作为请求体
- `--req-template`: 请求模板，用于从CSV文件生成请求体。使用此选项时必须同时指定 `--file` 参数，且文件必须是CSV格式

### 参数使用说明

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

### 注意事项

1. 压测模式相关：
   - 并发模式和QPS模式不能同时使用
   - 必须指定并发数或QPS中的一个
   - QPS模式下必须指定大于0的max-workers参数

2. HTTP方法和头部相关：
   - `--method` 支持所有标准HTTP方法：GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS等
   - `--header` 参数格式为 `key1:value1,key2:value2`，多个头部用逗号分隔
   - 工具会自动设置 `Content-Type: application/json` 头部，除非在 `--header` 中重新指定
   - 如果 `--header` 中指定的头部与默认头部冲突，`--header` 中的值会覆盖默认值

3. 请求来源相关：
   - 使用 `--request` 时，`--file` 和 `--req-template` 必须为空
   - 使用 `--req-template` 时：
     - 必须同时指定 `--file` 参数
     - 文件必须是CSV格式（.csv后缀）
     - CSV文件的第一行必须是表头
     - 模板中使用的所有变量名必须在CSV表头中存在
     - CSV文件的每一行数据列数必须与表头列数相同

4. 源IP地址相关：
   - `--src-ip` 参数用于指定HTTP请求的源IP地址
   - 程序会验证IP地址格式是否正确
   - 程序会检查IP地址是否存在于本机网络接口
   - 如果IP地址无效或不存在，程序会报错并退出
   - 适用于多网卡环境或需要模拟特定IP来源的场景

### 压测模式

1. 并发模式
    - 使用 `--concurrency` 参数指定并发数
    - 适合测试服务器在固定并发下的性能
    - 示例：`./wrkx --concurrency 100 --duration 30`

2. QPS 模式
    - 使用 `--qps` 参数指定每秒请求数
    - 适合测试服务器在固定请求频率下的性能
    - 通过 `--max-workers` 参数控制最大并发数，防止协程数量过多
    - 示例：`./wrkx --qps 1000 --duration 30 --max-workers 200`


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


## UI 的使用

可以直接启动 Web UI 服务，通过页面进行压力测试，能够清晰的展示压测结果。

启动方法：

```bash
cd ui && python app.py
```

浏览器访问<http://127.0.0.1:8081/> 即可开始压测：

![web-ui](images/webui.png)

## 开发说明

### 项目结构说明

本项目采用标准的Go项目布局，主要特点：

1. **`cmd/` 目录**: 包含所有可执行程序的入口点
   - `cmd/wrkx/`: 压测工具主程序
   - `cmd/server/`: 测试服务器程序

2. **`internal/` 目录**: 包含项目的内部包，不对外暴露
   - `internal/counter/`: 计数器相关逻辑
   - `internal/gen/`: 请求生成器
   - `internal/worker/`: 压测工作器

3. **`ui/` 目录**: Web UI界面相关文件

### 开发环境设置

```bash
# 克隆项目
git clone <repository-url>
cd wrk_with_fixed_qps

# 安装依赖
go mod tidy

# 运行测试
go test ./...

# 编译所有程序
go build -o bin/wrkx ./cmd/wrkx
go build -o bin/server ./cmd/server
```

### 添加新功能

1. 业务逻辑应添加到 `internal/` 目录下的相应包中
2. 程序入口逻辑应添加到 `cmd/` 目录下
3. 遵循Go的包命名和接口设计规范

### 测试

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./internal/counter
go test ./internal/worker
go test ./internal/gen
```
