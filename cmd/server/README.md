# Mock HTTP Server

这是一个模拟HTTP服务器，提供延迟响应功能并统计请求数量和活跃连接数。

## 功能特点

1. 提供HTTP服务，监听8080端口
2. 处理JSON格式的POST请求
3. 支持延迟响应（通过delay_ms参数指定延迟时间）
4. 统计请求数量
5. 统计活跃连接数
6. 将每秒的请求数量和活跃连接数存储到Redis中

## 前置要求

- Go 1.16+
- Redis服务器

## 安装和运行

1. 确保Redis服务器正在运行（默认地址：localhost:6379）

2. 克隆并编译项目：
```bash
git clone <repository-url>
cd mock_http_server
go build -o server cmd/server/main.go
```

3. 运行服务器：
```bash
./server
```

## API使用

### 延迟响应接口

发送POST请求到 `/delay` 端点：

```bash
curl -X POST http://localhost:8080/delay \
  -H "Content-Type: application/json" \
  -d '{"delay_ms": 122}'
```

响应示例：
```json
{
  "status": "OK"
}
```

### 统计信息接口

获取当前活跃连接数：

```bash
curl http://localhost:8080/stats
```

响应示例：
```json
{
  "active_connections": 5
}
```

## Redis数据

统计信息存储在Redis的hash中：
- Key: `request_counts`
- Fields:
  - `{timestamp}_requests`: 该秒内的请求数量
  - `{timestamp}_connections`: 该秒内的活跃连接数

可以使用以下Redis命令查看数据：
```bash
HGETALL request_counts
``` 