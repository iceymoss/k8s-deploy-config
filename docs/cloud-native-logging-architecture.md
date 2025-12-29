# 云原生日志系统架构设计文档

## 1. 概述

### 1.1 设计目标

本文档描述了一套**云原生（Cloud Native）**的日志系统架构，旨在替代传统的 ELK（Elasticsearch + Logstash + Kibana）方案。该架构遵循"**应用只管业务，日志交给基建**"的设计理念，实现应用与日志基础设施的完全解耦。

### 1.2 核心原则

- **本地开发**：日志仅输出到控制台，便于开发调试
- **生产环境**：应用输出标准 JSON 到 Stdout，由基础设施层统一采集
- **零侵入**：应用代码不包含任何日志发送逻辑，性能最优
- **高可靠**：日志采集失败不影响业务运行
- **自动化**：自动发现 Pod，自动添加 Kubernetes 元数据

### 1.3 架构对比

| 特性 | 传统方案 (Direct HTTP) | 本架构 (Vector + Stdout) | ELK (Filebeat + ES) |
|------|----------------------|-------------------------|-------------------|
| 应用性能 | 差（业务线程被 HTTP 阻塞） | 极致（只写内存/文件句柄） | 极致 |
| 可靠性 | 低（日志服务挂了，应用可能崩溃） | 高（日志服务挂了，Vector 会重试，应用无感） | 高 |
| 元数据 | 无（不知道是哪个 Pod 发的） | 全（自动带上 Pod IP, Node, Namespace） | 全 |
| 网络开销 | 高（每条日志一个请求） | 低（Vector 自动压缩、合并请求） | 低 |
| 运维难度 | 高（每个 App 都要配账号密码） | 低（App 零配置，Vector 统一配置） | 中（Java 维护重） |

---

## 2. 为什么选择 OpenObserve

### 2.1 OpenObserve 的优势

OpenObserve 是一个现代化的可观测性平台，相比 ELK 具有以下优势：

1. **性能卓越**
   - 使用 Rust 编写，性能比 Elasticsearch 高 10-140 倍
   - 存储压缩比高，相同数据量占用空间更小
   - 查询速度快，支持实时日志检索

2. **资源占用低**
   - 相比 Elasticsearch 需要大量内存和 CPU，OpenObserve 资源占用极低
   - 适合中小型团队和资源受限环境

3. **部署简单**
   - 单二进制文件，无需复杂的集群配置
   - 支持 Kubernetes StatefulSet 部署，运维成本低

4. **功能完整**
   - 支持日志（Logs）、指标（Metrics）、链路追踪（Traces）
   - 提供类似 Kibana 的查询界面
   - 支持多种数据源接入

5. **开源免费**
   - Apache 2.0 许可证
   - 社区活跃，持续更新

### 2.2 与 Vector 的完美配合

- Vector 和 OpenObserve 都使用 Rust 编写，性能匹配
- Vector 作为采集器，OpenObserve 作为存储和查询平台
- 两者结合形成完整的日志流水线

---

## 3. 架构设计

### 3.1 整体架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                        应用层 (Application Layer)                │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│  ┌──────────────────┐              ┌──────────────────┐          │
│  │   Go Application │              │   Go Application │          │
│  │                  │              │                  │          │
│  │  Local Mode:     │              │  Prod Mode:      │          │
│  │  Console Output  │              │  JSON to Stdout   │          │
│  │  (彩色、可读)     │              │  (压缩、结构化)   │          │
│  └──────────────────┘              └──────────────────┘          │
│           │                                  │                    │
│           └──────────────┬───────────────────┘                    │
│                          │                                        │
│                    Kubernetes                                    │
│                    Container Logs                                │
│                    (/var/log/pods/...)                           │
└──────────────────────────┼──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                  基础设施层 (Infrastructure Layer)                │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │              Vector (DaemonSet)                         │    │
│  │  ┌──────────────────────────────────────────────────┐   │    │
│  │  │  Source: kubernetes_logs                         │   │    │
│  │  │  - 自动发现所有 Pod 的日志文件                     │   │    │
│  │  │  - 自动添加 K8s 元数据 (Pod, Namespace, Node)     │   │    │
│  │  └──────────────────────────────────────────────────┘   │    │
│  │                          │                              │    │
│  │                          ▼                              │    │
│  │  ┌──────────────────────────────────────────────────┐   │    │
│  │  │  Transform: parse_json                           │   │    │
│  │  │  - 解析 Go 应用输出的 JSON 日志                    │   │    │
│  │  │  - 非 JSON 日志保持原样                           │   │    │
│  │  └──────────────────────────────────────────────────┘   │    │
│  │                          │                              │    │
│  │                          ▼                              │    │
│  │  ┌──────────────────────────────────────────────────┐   │    │
│  │  │  Sink: openobserve                               │   │    │
│  │  │  - 批量压缩上传 (max_events: 1000)                │   │    │
│  │  │  - 自动重试机制                                   │   │    │
│  │  │  - Basic Auth 认证                                │   │    │
│  │  └──────────────────────────────────────────────────┘   │    │
│  └─────────────────────────────────────────────────────────┘    │
│                          │                                        │
│                          ▼                                        │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │              OpenObserve (StatefulSet)                  │    │
│  │  - 接收日志数据                                          │    │
│  │  - 存储和索引                                            │    │
│  │  - 提供查询界面                                          │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
```

### 3.2 数据流向

1. **应用层**：Go 应用根据环境变量输出日志
   - 本地环境：彩色文本输出到控制台
   - 生产环境：JSON 格式输出到 Stdout

2. **Kubernetes**：容器运行时捕获 Stdout/Stderr，写入节点文件系统
   - 路径：`/var/log/pods/<namespace>_<pod-name>_<uid>/<container-name>/<instance>.log`

3. **Vector Agent**：以 DaemonSet 方式运行在每个节点
   - 自动发现并监控所有 Pod 的日志文件
   - 解析 JSON 日志，添加 Kubernetes 元数据
   - 批量压缩后发送到 OpenObserve

4. **OpenObserve**：接收、存储、索引日志
   - 提供 Web UI 进行日志查询和分析
   - 支持流式查询和实时监控

---

## 4. 应用层设计

### 4.1 Go 日志库设计

应用层使用 `zerolog` 作为日志库，通过环境变量控制输出格式。

#### 4.1.1 Logger 配置

文件位置：`pkg/logger/logger.go`

```go
package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
)

type Config struct {
	Env         string // "local" or "prod"
	ServiceName string // e.g. "payment-service"
	LogLevel    string // "debug", "info", "error"
}

// Setup 初始化日志系统
func Setup(cfg *Config) {
	// 1. 基础设置
	zerolog.TimeFieldFormat = time.RFC3339
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	// 2. 解析日志级别
	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// 3. 关键分支：决定输出格式
	var output io.Writer

	if cfg.Env == "local" {
		// === 本地开发模式 ===
		// 使用 ConsoleWriter，带颜色，人性化
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",
		}
	} else {
		// === 线上生产模式 (ELK/OpenObserve 模式) ===
		// 1. 纯 JSON: 方便 Vector/FluentBit 解析
		// 2. Stdout: 写入标准输出，这是 K8s 日志的标准源
		// 3. 高性能: 无锁，无 HTTP 请求，不阻塞业务
		output = os.Stdout
	}

	// 4. 构建 Logger
	// 自动注入 ServiceName, Environment, Caller(代码行号)
	log.Logger = zerolog.New(output).
		Level(level).
		With().
		Timestamp().
		Caller(). // 生产环境定位 bug 神器
		Str("service", cfg.ServiceName).
		Str("env", cfg.Env).
		Logger()
}
```

#### 4.1.2 使用示例

```go
package main

import (
	"os"

	"github.com/rs/zerolog/log"
	"your-project/pkg/logger"
)

func main() {
	// 只有这一行配置
	logger.Setup(&logger.Config{
		Env:         os.Getenv("APP_ENV"), // "local" or "prod"
		ServiceName: "my-go-app",
		LogLevel:    "info",
	})

	// 业务代码完全解耦，根本不知道 OpenObserve 的存在
	log.Info().Msg("Application started")
	log.Warn().Int("latency", 500).Msg("Database slow")
	log.Error().Err(err).Msg("File not found")
}
```

### 4.2 日志输出格式

#### 4.2.1 本地环境输出

```
15:04:05 INF Application started service=my-go-app env=local caller=main.go:12
15:04:05 WRN Database slow latency=500 service=my-go-app env=local caller=main.go:13
15:04:05 ERR File not found error="file not found" service=my-go-app env=local caller=main.go:14
```

#### 4.2.2 生产环境输出（JSON）

```json
{"level":"info","time":"2025-12-29T10:00:00Z","service":"my-go-app","env":"prod","caller":"main.go:12","message":"Application started"}
{"level":"warn","time":"2025-12-29T10:00:01Z","service":"my-go-app","env":"prod","latency":500,"caller":"main.go:13","message":"Database slow"}
{"level":"error","time":"2025-12-29T10:00:02Z","service":"my-go-app","env":"prod","error":"file not found","caller":"main.go:14","message":"File not found"}
```

### 4.3 设计优势

1. **零网络依赖**：应用不发送 HTTP 请求，性能最优
2. **环境自适应**：根据 `APP_ENV` 自动切换输出格式
3. **结构化日志**：生产环境使用 JSON，便于解析和查询
4. **自动元数据**：自动注入服务名、环境、调用位置等信息

---

## 5. 基础设施层设计

### 5.1 Vector 配置

Vector 作为日志采集器，以 DaemonSet 方式部署在 Kubernetes 集群中。

#### 5.1.1 Helm Values 配置

文件位置：`apps/openobserver/base/vector-values.yaml`

```yaml
role: Agent

# Agent 模式不需要对外提供服务
service:
  enabled: false

customConfig:
  data_dir: /srv/logs/vector-data-dir
  api:
    enabled: true
    address: 0.0.0.0:8686

  sources:
    kubernetes_logs:
      type: kubernetes_logs

  transforms:
    # 解析 JSON 日志 (这对你的 Go App 至关重要)
    parse_json:
      type: remap
      inputs: ["kubernetes_logs"]
      source: |
        . = parse_json(.message) ?? .

  sinks:
    openobserve:
      type: http
      inputs: ["parse_json"]
      # 使用 Kubernetes 内部 DNS
      uri: "http://openobserve.openobserve.svc.cluster.local:5080/api/<org-id>/_json"
      encoding:
        codec: json
      auth:
        strategy: basic
        user: "your-email@example.com"
        password: "your-password"
      batch:
        max_events: 1000  # 攒够 1000 条发一次
        timeout_secs: 1   # 最多等 1 秒
```

#### 5.1.2 Vector 组件说明

1. **Source: kubernetes_logs**
   - 自动发现所有 Pod 的日志文件
   - 自动添加 Kubernetes 元数据（Pod 名、Namespace、Node 等）
   - 监控 `/var/log/pods/` 目录

2. **Transform: parse_json**
   - 解析 Go 应用输出的 JSON 日志
   - 非 JSON 日志（如系统日志）保持原样
   - 使用 `remap` 类型进行数据转换

3. **Sink: openobserve**
   - HTTP 类型，批量发送到 OpenObserve
   - 支持 Basic Auth 认证
   - 自动重试机制，确保可靠性

### 5.2 OpenObserve 配置

OpenObserve 作为日志存储和查询平台，使用 StatefulSet 部署。

#### 5.2.1 Kubernetes 部署配置

文件位置：`apps/openobserver/base/openobserve.yaml`

主要组件：

1. **Namespace**: `openobserve`
2. **PersistentVolumeClaim**: 持久化存储
3. **StatefulSet**: 主应用
4. **Service**: 集群内服务发现
5. **Ingress**: 对外暴露 Web UI

#### 5.2.2 环境变量配置

```yaml
env:
  # 初始账号配置
  - name: ZO_ROOT_USER_EMAIL
    value: "your-email@example.com"
  - name: ZO_ROOT_USER_PASSWORD
    value: "your-password"
  # 数据目录
  - name: ZO_DATA_DIR
    value: "/srv/openobserver/data"
  # 遥测选项
  - name: ZO_TELEMETRY
    value: "false"
```

---

## 6. 部署步骤

### 6.1 前置条件

1. Kubernetes 集群（1.20+）
2. Helm 3.x
3. kubectl 配置正确
4. 足够的存储空间（建议至少 5Gi）

### 6.2 部署 OpenObserve

```bash
# 1. 创建命名空间和部署 OpenObserve
kubectl apply -f apps/openobserver/base/openobserve.yaml

# 2. 等待 Pod 就绪
kubectl wait --for=condition=ready pod -l app=openobserve -n openobserve --timeout=300s

# 3. 验证部署
kubectl get pods -n openobserve
```

### 6.3 部署 Vector

```bash
# 1. 添加 Vector Helm 仓库
helm repo add vector https://helm.vector.dev
helm repo update

# 2. 修改 vector-values.yaml 中的配置
# - 更新 OpenObserve URI（包含正确的组织 ID）
# - 更新认证信息

# 3. 安装 Vector
helm upgrade --install vector vector/vector \
  --namespace openobserve \
  -f apps/openobserver/base/vector-values.yaml

# 4. 验证部署
kubectl get pods -n openobserve -l app.kubernetes.io/name=vector
```

### 6.4 系统资源调优（重要）

Vector 需要监控大量日志文件，可能需要提高系统限制：

```bash
# 临时生效
sysctl -w fs.inotify.max_user_watches=524288
sysctl -w fs.inotify.max_user_instances=512

# 永久生效
echo "fs.inotify.max_user_watches = 524288" >> /etc/sysctl.conf
echo "fs.inotify.max_user_instances = 512" >> /etc/sysctl.conf
```

---

## 7. 验证和测试

### 7.1 验证 Vector 运行状态

```bash
# 查看 Vector Pod 状态
kubectl get pods -n openobserve -l app.kubernetes.io/name=vector

# 查看 Vector 日志（应该没有 ERROR）
kubectl logs -f -n openobserve -l app.kubernetes.io/name=vector

# 成功标志：
# - Healthcheck passed
# - Vector has started
# - 没有 404/Broken pipe 错误
```

### 7.2 验证日志采集

1. **部署测试应用**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: test-app
        image: your-go-app:latest
        env:
        - name: APP_ENV
          value: "prod"
```

2. **查看应用日志**

```bash
# 查看应用 Pod 日志（应该是 JSON 格式）
kubectl logs -f deployment/test-app
```

3. **在 OpenObserve 中查询**

- 打开 OpenObserve Web UI
- 进入 Logs 页面
- 选择对应的 Stream（通常是 `kubernetes_logs` 或 `default`）
- 运行查询，应该能看到测试应用的日志

### 7.3 验证元数据

在 OpenObserve 中查看日志，应该包含以下 Kubernetes 元数据：

- `kubernetes.pod_name`: Pod 名称
- `kubernetes.namespace`: 命名空间
- `kubernetes.node_name`: 节点名称
- `kubernetes.container_name`: 容器名称
- `service`: 应用服务名（来自 Go 应用）
- `env`: 环境标识（来自 Go 应用）

---

## 8. 常见问题排查

### 8.1 Vector 连接失败

**问题**：日志显示 `404 Not Found` 或 `Connection refused`

**原因**：
1. OpenObserve URI 中的组织 ID 不正确
2. OpenObserve 和 Vector 不在同一个集群/命名空间

**解决**：
1. 在 OpenObserve Web UI 中确认正确的组织 ID（Ingestion -> Custom -> JSON）
2. 更新 `vector-values.yaml` 中的 URI
3. 执行 `helm upgrade` 应用更改

### 8.2 认证失败

**问题**：日志显示 `401 Unauthorized`

**原因**：用户名或密码错误

**解决**：
1. 确认 OpenObserve 的账号密码
2. 更新 `vector-values.yaml` 中的认证信息
3. 执行 `helm upgrade` 应用更改

### 8.3 文件句柄不足

**问题**：日志显示 `too many open files`

**原因**：系统 inotify 限制太低

**解决**：
```bash
sysctl -w fs.inotify.max_user_watches=524288
sysctl -w fs.inotify.max_user_instances=512
```

### 8.4 日志未出现在 OpenObserve

**可能原因**：
1. Vector 未正常运行
2. 应用未输出 JSON 格式日志
3. OpenObserve 组织 ID 配置错误

**排查步骤**：
1. 检查 Vector Pod 状态和日志
2. 检查应用 Pod 日志格式
3. 在 OpenObserve 中检查 Stream 配置

---

## 9. 最佳实践

### 9.1 日志级别管理

- **开发环境**：使用 `debug` 级别，便于调试
- **生产环境**：使用 `info` 级别，减少日志量
- **关键错误**：使用 `error` 级别，确保被监控

### 9.2 日志字段设计

建议在日志中包含以下字段：

- `service`: 服务名称
- `env`: 环境标识
- `request_id`: 请求 ID（用于链路追踪）
- `user_id`: 用户 ID（如适用）
- `latency`: 延迟时间（如适用）

### 9.3 性能优化

1. **批量发送**：Vector 已配置批量发送（1000 条/批）
2. **压缩传输**：Vector 自动压缩数据
3. **异步处理**：应用日志输出不阻塞业务

### 9.4 安全考虑

1. **认证**：使用 Basic Auth 保护 OpenObserve
2. **网络隔离**：Vector 和 OpenObserve 使用集群内部网络通信
3. **敏感信息**：避免在日志中输出密码、Token 等敏感信息

---

## 10. 架构优势总结

### 10.1 性能优势

- **应用层**：零网络开销，只写内存/文件句柄
- **采集层**：Vector 使用 Rust 编写，性能极高
- **存储层**：OpenObserve 压缩比高，查询速度快

### 10.2 可靠性优势

- **解耦设计**：日志采集失败不影响业务
- **自动重试**：Vector 自动重试失败的请求
- **数据持久化**：OpenObserve 使用 PVC 持久化数据

### 10.3 运维优势

- **零配置应用**：应用无需配置日志发送
- **自动发现**：Vector 自动发现所有 Pod
- **统一管理**：所有日志配置集中在 Vector

### 10.4 成本优势

- **资源占用低**：相比 ELK，资源占用大幅降低
- **开源免费**：OpenObserve 和 Vector 都是开源项目
- **易于扩展**：支持水平扩展

---

## 11. 总结

本架构设计实现了一套完整的云原生日志系统，具有以下特点：

1. **应用与基础设施解耦**：应用只负责输出日志，不关心日志的去向
2. **环境自适应**：本地开发友好，生产环境高效
3. **自动化程度高**：自动发现、自动采集、自动添加元数据
4. **性能优异**：使用 Rust 编写的高性能组件
5. **运维简单**：统一的配置管理，易于维护

这套架构完全符合云原生最佳实践，可以作为 ELK 的替代方案，特别适合中小型团队和资源受限的环境。

---

## 附录

### A. 相关文件位置

- Go Logger 实现：`test/pkg/logger/logger.go`
- Vector 配置：`apps/openobserver/base/vector-values.yaml`
- OpenObserve 配置：`apps/openobserver/base/openobserve.yaml`

### B. 参考资源

- [OpenObserve 官方文档](https://openobserve.ai/docs/)
- [Vector 官方文档](https://vector.dev/docs/)
- [Zerolog 文档](https://github.com/rs/zerolog)

### C. 版本信息

- OpenObserve: latest
- Vector: 0.52.0
- Kubernetes: 1.20+
- Helm: 3.x

---

**文档版本**: 1.0  
**最后更新**: 2025-12-29  
**维护者**: DevOps Team

