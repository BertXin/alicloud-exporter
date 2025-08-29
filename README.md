# Alicloud Exporter

一个标准化的 Prometheus Exporter，用于收集阿里云服务的监控指标。

## 特性

- **标准化架构**: 遵循 Prometheus Exporter 最佳实践
- **多服务支持**: 支持 SLB、Redis、RDS 等阿里云服务
- **配置驱动**: 通过 YAML 配置文件灵活配置
- **速率限制**: 内置 API 调用速率限制，避免触发阿里云限制
- **错误处理**: 完善的错误处理和重试机制
- **可观测性**: 内置指标监控 exporter 自身状态
- **优雅关闭**: 支持优雅关闭和资源清理

## 支持的服务

### SLB (Server Load Balancer)
- 连接数指标 (ActiveConnection, NewConnection, etc.)
- 流量指标 (TrafficRXNew, TrafficTXNew)
- 状态码指标 (StatusCode2xx, StatusCode4xx, etc.)
- 健康检查指标 (HeathyServerCount, UnhealthyServerCount)

### Redis (KVStore)
- 资源使用率 (CpuUsage, MemoryUsage, ConnectionUsage)
- 性能指标 (UsedQPS, HitRate)
- 网络指标 (IntranetIn, IntranetOut)
- 错误指标 (FailedCount)

### RDS (Relational Database Service)
- 资源使用率 (CpuUsage, MemoryUsage, DiskUsage)
- 数据库性能 (MySQL_QPS, MySQL_TPS)
- 连接指标 (MySQL_ActiveSessions, ConnectionUsage)
- InnoDB 指标 (MySQL_InnoDBDataRead, MySQL_InnoDBDataWritten)

## 快速开始

### 安装

```bash
# 克隆项目
git clone <repository-url>
cd alicloud-exporter

# 构建
make build

# 或者使用 Go 直接构建
go build -o alicloud-exporter cmd/alicloud-exporter/main.go
```

### 配置

1. 复制示例配置文件：
```bash
cp config/config.yaml config/my-config.yaml
```

2. 编辑配置文件，设置阿里云凭据：
```yaml
alicloud:
  access_key_id: "your-access-key-id"
  access_key_secret: "your-access-key-secret"
  region: "cn-hangzhou"
```

3. 或者使用环境变量：
```bash
export ALICLOUD_ACCESS_KEY_ID="your-access-key-id"
export ALICLOUD_ACCESS_KEY_SECRET="your-access-key-secret"
```

### 运行

```bash
# 使用配置文件运行
./alicloud-exporter --config config/my-config.yaml

# 或者使用环境变量
./alicloud-exporter

# 查看帮助
./alicloud-exporter --help
```

### 验证

```bash
# 检查健康状态
curl http://localhost:9100/health

# 查看指标
curl http://localhost:9100/metrics

# 验证配置文件
./alicloud-exporter validate --config config/my-config.yaml

# 查看可用指标列表
./alicloud-exporter metrics
```

## 配置说明

### 服务器配置
```yaml
server:
  listen_address: ":9100"     # 监听地址
  metrics_path: "/metrics"     # 指标路径
  log_level: "info"           # 日志级别
  log_format: "json"          # 日志格式
```

### 阿里云配置
```yaml
alicloud:
  access_key_id: "${ALICLOUD_ACCESS_KEY_ID}"
  access_key_secret: "${ALICLOUD_ACCESS_KEY_SECRET}"
  region: "cn-hangzhou"
  rate_limit:
    requests_per_second: 10   # 每秒请求数限制
    burst: 20                 # 突发请求数
```

### 服务配置
```yaml
services:
  slb:
    enabled: true
    namespace: "acs_slb_dashboard"
    scrape_interval: 60s
    metrics:
      - "ActiveConnection"
      - "NewConnection"
      # ... 更多指标
```

## Docker 使用

```bash
# 构建镜像
docker build -t alicloud-exporter .

# 运行容器
docker run -d \
  --name alicloud-exporter \
  -p 9100:9100 \
  -e ALICLOUD_ACCESS_KEY_ID="your-key" \
  -e ALICLOUD_ACCESS_KEY_SECRET="your-secret" \
  alicloud-exporter
```

## Prometheus 配置

在 Prometheus 配置文件中添加：

```yaml
scrape_configs:
  - job_name: 'alicloud-exporter'
    static_configs:
      - targets: ['localhost:9100']
    scrape_interval: 60s
    scrape_timeout: 30s
```

## 监控指标

### 内置指标
- `alicloud_up`: Exporter 是否正常运行
- `alicloud_scrapes_total`: 总抓取次数
- `alicloud_scrape_errors_total`: 抓取错误次数
- `alicloud_scrape_duration_seconds`: 抓取耗时
- `alicloud_last_scrape_timestamp_seconds`: 最后抓取时间

### 服务指标
所有服务指标都带有以下标签：
- `instance_id`: 实例 ID
- `exporter`: 固定值 "alicloud-exporter"

SLB 指标额外包含：
- `protocol`: 协议类型
- `port`: 端口号
- `vip`: 虚拟 IP

## 开发

### 项目结构
```
├── cmd/alicloud-exporter/     # 主程序入口
├── internal/
│   ├── client/               # 阿里云客户端
│   ├── collector/            # 指标收集器
│   ├── config/              # 配置管理
│   ├── exporter/            # 导出器主逻辑
│   └── logger/              # 日志管理
├── config/                  # 配置文件
├── Makefile                # 构建脚本
└── README.md               # 项目文档
```

### 添加新服务

1. 在 `internal/collector/` 中创建新的收集器
2. 实现 `ServiceCollector` 接口
3. 在 `internal/exporter/exporter.go` 中注册新收集器
4. 更新配置文件结构

### 构建和测试

```bash
# 运行测试
make test

# 代码检查
make lint

# 构建
make build

# 清理
make clean
```

## 故障排除

### 常见问题

1. **认证失败**
   - 检查 AccessKey 和 AccessKeySecret 是否正确
   - 确认账号有相应服务的监控权限

2. **指标为空**
   - 检查实例是否存在
   - 确认时间范围内有数据
   - 查看日志中的错误信息

3. **速率限制**
   - 调整 `rate_limit` 配置
   - 减少监控的指标数量
   - 增加 `scrape_interval`

### 日志分析

```bash
# 查看详细日志
./alicloud-exporter --log-level debug

# JSON 格式日志便于分析
./alicloud-exporter --log-format json | jq .
```

## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request！

## 相关链接

- [Prometheus](https://prometheus.io/)
- [阿里云监控 API](https://help.aliyun.com/document_detail/51939.html)
- [Prometheus Exporter 最佳实践](https://prometheus.io/docs/instrumenting/writing_exporters/)