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

### 3.1 Kubernetes 集群中的实际架构

以下是在 Kubernetes 集群中的实际部署架构图：

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Kubernetes Cluster                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Node 1 (k8s-master)                                                │   │
│  │  ┌───────────────────────────────────────────────────────────────┐ │   │
│  │  │  /var/log/pods/                                               │ │   │
│  │  │  ├── default_my-app-xxx_abc123/                              │ │   │
│  │  │  │   └── my-app/0.log  ← Go App JSON logs                    │ │   │
│  │  │  ├── kube-system_coredns-xxx/                                 │ │   │
│  │  │  └── ...                                                      │ │   │
│  │  └───────────────────────────────────────────────────────────────┘ │   │
│  │                          ▲                                           │   │
│  │                          │                                           │   │
│  │  ┌───────────────────────────────────────────────────────────────┐ │   │
│  │  │  Vector Agent (DaemonSet)                                     │ │   │
│  │  │  Pod: vector-xxxxx                                            │ │   │
│  │  │  - 监控 /var/log/pods/                                        │ │   │
│  │  │  - 解析 JSON 日志                                             │ │   │
│  │  │  - 添加 K8s 元数据                                            │ │   │
│  │  └───────────────────────────────────────────────────────────────┘ │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                          │                                                   │
│  ┌───────────────────────┼───────────────────────────────────────────────┐ │
│  │  Node 2 (k8s-node1)  │                                               │ │
│  │  ┌───────────────────────────────────────────────────────────────┐ │ │
│  │  │  /var/log/pods/                                               │ │ │
│  │  │  ├── backend_payment-api-xxx/                                │ │ │
│  │  │  └── ...                                                      │ │ │
│  │  └───────────────────────────────────────────────────────────────┘ │ │
│  │                          ▲                                           │ │
│  │                          │                                           │ │
│  │  ┌───────────────────────────────────────────────────────────────┐ │ │
│  │  │  Vector Agent (DaemonSet)                                     │ │ │
│  │  │  Pod: vector-yyyyy                                            │ │ │
│  │  └───────────────────────────────────────────────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                          │                                                   │
│                          │ HTTP POST (批量)                                  │
│                          ▼                                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Namespace: openobserve                                            │   │
│  │  ┌───────────────────────────────────────────────────────────────┐ │   │
│  │  │  OpenObserve StatefulSet                                       │ │   │
│  │  │  Pod: openobserve-0                                            │ │   │
│  │  │  Service: openobserve (ClusterIP: None)                       │ │   │
│  │  │  PVC: openobserve-data (5Gi)                                   │ │   │
│  │  │  Port: 5080                                                    │ │   │
│  │  └───────────────────────────────────────────────────────────────┘ │   │
│  │                          │                                           │   │
│  │                          │                                           │   │
│  │  ┌───────────────────────────────────────────────────────────────┐ │   │
│  │  │  Ingress: openobserve-ingress                                 │ │   │
│  │  │  Host: openobserve.iceymoss.com                               │ │   │
│  │  │  → Traefik → openobserve:5080                                 │ │   │
│  │  └───────────────────────────────────────────────────────────────┘ │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                          │                                                   │
│                          │ HTTP/HTTPS                                        │
│                          ▼                                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  External Access (Browser)                                         │   │
│  │  http://openobserve.iceymoss.com:10000                             │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                               │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.2 工作原理图（数据流和组件交互）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          日志流水线工作原理                                    │
└─────────────────────────────────────────────────────────────────────────────┘

步骤 1: 应用输出日志
┌──────────────┐
│ Go Application│
│  (Pod)        │
│              │
│ log.Info()   │──┐
│ log.Error()  │  │ 写入 Stdout/Stderr
└──────────────┘  │
                  │
                  ▼
步骤 2: Kubernetes 容器运行时捕获
┌─────────────────────────────────────┐
│  Container Runtime (containerd)     │
│  - 捕获 Stdout/Stderr               │
│  - 写入节点文件系统                 │
└─────────────────────────────────────┘
                  │
                  ▼
步骤 3: 日志文件存储
┌─────────────────────────────────────────────────────────────┐
│ /var/log/pods/<ns>_<pod>_<uid>/<container>/<instance>.log  │
│                                                              │
│ {"level":"info","time":"...","message":"Application started"}│
│ {"level":"error","time":"...","message":"Database error"}   │
└─────────────────────────────────────────────────────────────┘
                  │
                  │ inotify 监控
                  ▼
步骤 4: Vector Agent 发现并读取
┌─────────────────────────────────────────────────────────────┐
│  Vector Agent (DaemonSet)                                   │
│  ┌───────────────────────────────────────────────────────┐ │
│  │  Source: kubernetes_logs                               │ │
│  │  - 自动发现新日志文件                                   │ │
│  │  - 读取日志内容                                        │ │
│  │  - 添加元数据:                                        │ │
│  │    * kubernetes.pod_name                             │ │
│  │    * kubernetes.namespace                             │ │
│  │    * kubernetes.node_name                             │ │
│  │    * kubernetes.container_name                        │ │
│  └───────────────────────────────────────────────────────┘ │
│                          │                                   │
│                          ▼                                   │
│  ┌───────────────────────────────────────────────────────┐ │
│  │  Transform: parse_json                                 │ │
│  │  - 尝试解析 JSON 格式                                  │ │
│  │  - 成功: 展开 JSON 字段                                │ │
│  │  - 失败: 保持原样 (message 字段)                       │ │
│  └───────────────────────────────────────────────────────┘ │
│                          │                                   │
│                          ▼                                   │
│  ┌───────────────────────────────────────────────────────┐ │
│  │  Buffer (内存缓冲)                                     │ │
│  │  - 收集日志事件                                        │ │
│  │  - 等待批量发送条件                                    │ │
│  └───────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                  │
                  │ 批量发送 (max_events: 1000 或 timeout: 1s)
                  ▼
步骤 5: 发送到 OpenObserve
┌─────────────────────────────────────────────────────────────┐
│  HTTP POST                                                   │
│  URI: http://openobserve.openobserve.svc.cluster.local:5080 │
│       /api/<org-id>/_json                                   │
│  Headers:                                                   │
│    Authorization: Basic <base64(user:pass)>                 │
│    Content-Type: application/json                           │
│  Body: [                                                    │
│    {                                                        │
│      "level": "info",                                      │
│      "time": "2025-12-29T10:00:00Z",                       │
│      "service": "my-app",                                 │
│      "kubernetes.pod_name": "my-app-xxx",                  │
│      "kubernetes.namespace": "default",                    │
│      "message": "Application started"                      │
│    },                                                       │
│    ... (最多 1000 条)                                       │
│  ]                                                          │
└─────────────────────────────────────────────────────────────┘
                  │
                  ▼
步骤 6: OpenObserve 处理
┌─────────────────────────────────────────────────────────────┐
│  OpenObserve                                                 │
│  ┌───────────────────────────────────────────────────────┐ │
│  │  1. 接收 HTTP 请求                                    │ │
│  │  2. 验证认证信息                                       │ │
│  │  3. 解析 JSON 数组                                     │ │
│  │  4. 索引和存储                                         │ │
│  │  5. 返回 200 OK                                       │ │
│  └───────────────────────────────────────────────────────┘ │
│                          │                                   │
│                          ▼                                   │
│  ┌───────────────────────────────────────────────────────┐ │
│  │  持久化存储 (PVC)                                      │ │
│  │  - 压缩存储                                            │ │
│  │  - 建立索引                                            │ │
│  └───────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                  │
                  │ 查询请求
                  ▼
步骤 7: 用户查询
┌─────────────────────────────────────────────────────────────┐
│  Web Browser                                                │
│  ┌───────────────────────────────────────────────────────┐ │
│  │  OpenObserve UI                                       │ │
│  │  - 选择 Stream (kubernetes_logs)                       │ │
│  │  - 输入查询条件                                        │ │
│  │  - 查看日志结果                                        │ │
│  └───────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### 3.3 组件交互时序图

```
应用 Pod         容器运行时      Vector Agent          OpenObserve
  │                 │                │                    │
  │ log.Info()      │                │                    │
  │────────────────>│                │                    │
  │                 │                │                    │
  │                 │ 写入文件       │                    │
  │                 │────────────────>│                    │
  │                 │                │                    │
  │                 │                │ inotify 触发        │
  │                 │                │<───────────────────│
  │                 │                │                    │
  │                 │                │ 读取日志            │
  │                 │                │───────────────────>│
  │                 │                │                    │
  │                 │                │ 解析 JSON          │
  │                 │                │ 添加元数据          │
  │                 │                │                    │
  │                 │                │ 缓冲 (等待批量)     │
  │                 │                │                    │
  │                 │                │ 批量发送 (1000条)   │
  │                 │                │───────────────────>│
  │                 │                │                    │
  │                 │                │                    │ 存储和索引
  │                 │                │                    │
  │                 │                │<───────────────────│ 200 OK
  │                 │                │                    │
```

### 3.4 整体架构图（逻辑视图）

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

### 3.5 数据流向

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

## 6. 详细部署步骤

### 6.1 前置条件检查

在执行部署前，请确认以下条件：

#### 6.1.1 检查 Kubernetes 集群

```bash
# 1. 检查集群版本（需要 1.20+）
kubectl version --short

# 预期输出示例：
# Client Version: v1.28.0
# Server Version: v1.28.0

# 2. 检查集群连接
kubectl cluster-info

# 预期输出：
# Kubernetes control plane is running at https://...
```

#### 6.1.2 检查 Helm

```bash
# 检查 Helm 版本（需要 3.x）
helm version

# 预期输出：
# version.BuildInfo{Version:"v3.12.0", ...}

# 如果没有安装 Helm，请先安装：
# curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
```

#### 6.1.3 检查存储

```bash
# 检查可用的 StorageClass
kubectl get storageclass

# 预期输出示例：
# NAME                 PROVISIONER       RECLAIMPOLICY
# local-path           rancher.io/local-path   Delete
```

#### 6.1.4 检查节点资源

```bash
# 检查节点资源使用情况
kubectl top nodes

# 确保有足够的 CPU 和内存资源
```

### 6.2 部署 OpenObserve

#### 6.2.1 步骤 1: 准备配置文件

首先，编辑 OpenObserve 配置文件：

```bash
# 进入配置文件目录
cd apps/openobserver/base

# 编辑配置文件
vim openobserve.yaml
```

**关键配置项**：
- `ZO_ROOT_USER_EMAIL`: 设置管理员邮箱
- `ZO_ROOT_USER_PASSWORD`: 设置管理员密码（**重要：生产环境请使用 Secret**）
- `storage`: 根据需求调整存储大小（默认 5Gi）

#### 6.2.2 步骤 2: 部署 OpenObserve

```bash
# 应用配置文件
kubectl apply -f openobserve.yaml

# 预期输出：
# namespace/openobserve created
# persistentvolumeclaim/openobserve-data created
# statefulset.apps/openobserve created
# service/openobserve created
# ingress.networking.k8s.io/openobserve-ingress created
```

#### 6.2.3 步骤 3: 等待 Pod 就绪

```bash
# 等待 StatefulSet 就绪（最多等待 5 分钟）
kubectl wait --for=condition=ready pod -l app=openobserve -n openobserve --timeout=300s

# 预期输出：
# pod/openobserve-0 condition met
```

**如果等待超时，检查 Pod 状态**：

```bash
# 查看 Pod 状态
kubectl get pods -n openobserve

# 预期输出：
# NAME            READY   STATUS    RESTARTS   AGE
# openobserve-0   1/1     Running   0          2m

# 如果状态不是 Running，查看日志排查问题
kubectl logs -n openobserve openobserve-0
```

#### 6.2.4 步骤 4: 验证 OpenObserve 服务

```bash
# 检查 Service
kubectl get svc -n openobserve

# 预期输出：
# NAME          TYPE        CLUSTER-IP   EXTERNAL-IP   PORT(S)    AGE
# openobserve   ClusterIP   None         <none>        5080/TCP   2m

# 检查 Ingress
kubectl get ingress -n openobserve

# 预期输出：
# NAME                  CLASS    HOSTS                          ADDRESS   PORTS   AGE
# openobserve-ingress   traefik  openobserve.iceymoss.com                 80      2m
```

#### 6.2.5 步骤 5: 获取组织 ID（重要）

这是配置 Vector 的关键步骤：

1. **访问 OpenObserve Web UI**
   ```bash
   # 如果使用 Ingress，访问配置的域名
   # 例如：http://openobserve.iceymoss.com:10000
   
   # 或者使用 port-forward（临时访问）
   kubectl port-forward -n openobserve svc/openobserve 5080:5080
   # 然后访问 http://localhost:5080
   ```

2. **登录 OpenObserve**
   - 使用步骤 1 中配置的邮箱和密码登录

3. **获取组织 ID**
   - 点击左侧菜单 `Ingestion` → `Custom` → `JSON`
   - 查看生成的 URL，格式如下：
     ```
     http://.../api/<org-id>/_json
     ```
   - 记录 `<org-id>` 的值（例如：`iceymoss` 或 `ic2_deepcodify_com`）

### 6.3 部署 Vector

#### 6.3.1 步骤 1: 添加 Helm 仓库

```bash
# 添加 Vector 官方 Helm 仓库
helm repo add vector https://helm.vector.dev

# 预期输出：
# "vector" has been added to your repositories

# 更新仓库
helm repo update

# 预期输出：
# Hang tight while we grab the latest from the chart repositories...
# ...Successfully got an update from the "vector" chart repository
# Update Complete.
```

#### 6.3.2 步骤 2: 配置 vector-values.yaml

编辑 Vector 配置文件：

```bash
# 编辑配置文件
vim apps/openobserver/base/vector-values.yaml
```

**关键配置项**：

1. **更新 OpenObserve URI**
   ```yaml
   uri: "http://openobserve.openobserve.svc.cluster.local:5080/api/<org-id>/_json"
   ```
   将 `<org-id>` 替换为步骤 6.2.5 中获取的组织 ID

2. **更新认证信息**
   ```yaml
   auth:
     strategy: basic
     user: "your-email@example.com"  # 替换为 OpenObserve 管理员邮箱
     password: "your-password"        # 替换为 OpenObserve 管理员密码
   ```

3. **确认配置**
   ```yaml
   role: Agent
   service:
     enabled: false  # Agent 模式不需要 Service
   ```

#### 6.3.3 步骤 3: 系统资源调优（重要）

Vector 需要监控大量日志文件，需要提高系统限制：

```bash
# 临时生效（立即生效）
sysctl -w fs.inotify.max_user_watches=524288
sysctl -w fs.inotify.max_user_instances=512

# 预期输出：
# fs.inotify.max_user_watches = 524288
# fs.inotify.max_user_instances = 512

# 永久生效（重启后仍然有效）
echo "fs.inotify.max_user_watches = 524288" | sudo tee -a /etc/sysctl.conf
echo "fs.inotify.max_user_instances = 512" | sudo tee -a /etc/sysctl.conf

# 验证设置
sysctl fs.inotify.max_user_watches fs.inotify.max_user_instances

# 预期输出：
# fs.inotify.max_user_watches = 524288
# fs.inotify.max_user_instances = 512
```

**注意**：如果是在 Kubernetes 节点上执行，需要在**每个节点**上都执行此操作。

#### 6.3.4 步骤 4: 安装 Vector

```bash
# 使用 helm upgrade --install（如果已存在则升级，不存在则安装）
helm upgrade --install vector vector/vector \
  --namespace openobserve \
  -f apps/openobserver/base/vector-values.yaml

# 预期输出：
# Release "vector" does not exist. Installing it now.
# NAME: vector
# LAST DEPLOYED: Mon Dec 29 16:14:27 2025
# NAMESPACE: openobserve
# STATUS: deployed
# REVISION: 1
# ...
```

**如果遇到错误 "cannot re-use a name that is still in use"**：

```bash
# 先卸载旧的安装
helm uninstall vector --namespace openobserve

# 然后重新安装
helm upgrade --install vector vector/vector \
  --namespace openobserve \
  -f apps/openobserver/base/vector-values.yaml
```

#### 6.3.5 步骤 5: 验证 Vector 部署

```bash
# 检查 Vector DaemonSet
kubectl get daemonset -n openobserve

# 预期输出：
# NAME     DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
# vector   2         2         2       2            2           <none>           1m

# 检查 Vector Pods（应该在每个节点上都有一个）
kubectl get pods -n openobserve -l app.kubernetes.io/name=vector

# 预期输出（假设有 2 个节点）：
# NAME           READY   STATUS    RESTARTS   AGE
# vector-2fl57   1/1     Running   0          1m
# vector-js4md   1/1     Running   0          1m
```

**如果 Pod 状态不是 Running**：

```bash
# 查看 Pod 详细信息
kubectl describe pod -n openobserve -l app.kubernetes.io/name=vector

# 查看 Pod 日志
kubectl logs -n openobserve -l app.kubernetes.io/name=vector
```

---

## 7. 详细验证和测试

### 7.1 验证 Vector 运行状态

#### 7.1.1 检查 Pod 状态

```bash
# 查看 Vector Pod 状态
kubectl get pods -n openobserve -l app.kubernetes.io/name=vector

# 预期输出（假设有 2 个节点）：
# NAME           READY   STATUS    RESTARTS   AGE
# vector-2fl57   1/1     Running   0          5m
# vector-js4md   1/1     Running   0          5m

# 如果状态不是 Running，查看详细信息
kubectl describe pod -n openobserve <pod-name>
```

#### 7.1.2 检查 Vector 日志

```bash
# 查看 Vector 日志（应该没有 ERROR）
kubectl logs -f -n openobserve -l app.kubernetes.io/name=vector

# 成功标志：
# ✅ Healthcheck passed
# ✅ Vector has started
# ✅ API server running
# ✅ 没有 404 Not Found 错误
# ✅ 没有 Broken pipe 错误
# ✅ 没有 Connection refused 错误

# 预期输出示例：
# 2025-12-29T10:11:54.152897Z  INFO vector::app: Log level is enabled. level="info"
# 2025-12-29T10:11:54.169365Z  INFO vector::app: Loading configs. paths=["/etc/vector"]
# 2025-12-29T10:11:54.281078Z  INFO source{...}: Obtained Kubernetes Node name...
# 2025-12-29T10:11:54.369481Z  INFO vector::topology::running: Running healthchecks.
# 2025-12-29T10:11:54.369717Z  INFO vector::topology::builder: Healthcheck passed.
# 2025-12-29T10:11:54.370648Z  INFO vector: Vector has started. version="0.52.0"
# 2025-12-29T10:11:54.389295Z  INFO vector::internal_events::api: API server running.
```

**如果看到错误**：

- `404 Not Found`: 检查 OpenObserve URI 中的组织 ID 是否正确
- `401 Unauthorized`: 检查用户名和密码是否正确
- `Connection refused`: 检查 OpenObserve 是否正常运行
- `too many open files`: 执行系统资源调优（见 6.3.3）

#### 7.1.3 检查 Vector 配置

```bash
# 进入 Vector Pod 查看配置
kubectl exec -n openobserve -it <vector-pod-name> -- vector validate --config-dir /etc/vector

# 预期输出：
# ✓ Configuration is valid
```

#### 7.1.4 检查 Vector API（可选）

```bash
# 使用 port-forward 访问 Vector API
kubectl port-forward -n openobserve <vector-pod-name> 8686:8686

# 在另一个终端访问 API
curl http://localhost:8686/health

# 预期输出：
# {"status":"ok"}
```

### 7.2 验证 OpenObserve 运行状态

#### 7.2.1 检查 OpenObserve Pod

```bash
# 查看 OpenObserve Pod 状态
kubectl get pods -n openobserve -l app=openobserve

# 预期输出：
# NAME            READY   STATUS    RESTARTS   AGE
# openobserve-0   1/1     Running   0          10m

# 查看 Pod 日志
kubectl logs -n openobserve openobserve-0 --tail=50
```

#### 7.2.2 检查 OpenObserve 服务

```bash
# 检查 Service
kubectl get svc -n openobserve

# 预期输出：
# NAME          TYPE        CLUSTER-IP   EXTERNAL-IP   PORT(S)    AGE
# openobserve   ClusterIP   None         <none>        5080/TCP   10m

# 测试内部连接（从集群内部测试）
kubectl run -it --rm test-curl --image=curlimages/curl --restart=Never -- \
  curl -u "your-email@example.com:your-password" \
  http://openobserve.openobserve.svc.cluster.local:5080/api/<org-id>/_json

# 预期输出：
# {"code":200,"message":"Success"}
```

#### 7.2.3 访问 Web UI

```bash
# 方法 1: 使用 Ingress（如果已配置）
# 访问 http://openobserve.iceymoss.com:10000

# 方法 2: 使用 port-forward（临时访问）
kubectl port-forward -n openobserve svc/openobserve 5080:5080

# 然后访问 http://localhost:5080
```

**验证步骤**：
1. 使用管理员账号登录
2. 检查是否能正常访问界面
3. 进入 `Ingestion` → `Custom` → `JSON`，确认组织 ID

### 7.3 验证日志采集流程

#### 7.3.1 部署测试应用

创建一个测试应用来验证日志采集：

```bash
# 创建测试应用部署文件 test-app.yaml
cat <<EOF > test-app.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test-app
  template:
    metadata:
      labels:
        app: test-app
    spec:
      containers:
      - name: test-app
        image: busybox:latest
        command: 
        - sh
        - -c
        - |
          while true; do
            echo '{"level":"info","time":"'$(date -Iseconds)'","service":"test-app","env":"prod","message":"Test log message"}'
            sleep 5
          done
        env:
        - name: APP_ENV
          value: "prod"
EOF

# 部署测试应用
kubectl apply -f test-app.yaml

# 等待 Pod 就绪
kubectl wait --for=condition=ready pod -l app=test-app --timeout=60s
```

#### 7.3.2 验证应用日志输出

```bash
# 查看测试应用 Pod 日志（应该是 JSON 格式）
kubectl logs -f deployment/test-app

# 预期输出：
# {"level":"info","time":"2025-12-29T10:30:00Z","service":"test-app","env":"prod","message":"Test log message"}
# {"level":"info","time":"2025-12-29T10:30:05Z","service":"test-app","env":"prod","message":"Test log message"}
```

#### 7.3.3 验证 Vector 是否采集到日志

```bash
# 查看 Vector 日志，应该能看到文件发现和读取的日志
kubectl logs -n openobserve -l app.kubernetes.io/name=vector --tail=100 | grep test-app

# 预期输出示例：
# INFO source{...}: Found new file to watch. file=/var/log/pods/default_test-app-xxx/...
```

#### 7.3.4 在 OpenObserve 中查询日志

1. **访问 OpenObserve Web UI**
   ```bash
   # 使用 port-forward 或 Ingress 访问
   kubectl port-forward -n openobserve svc/openobserve 5080:5080
   # 访问 http://localhost:5080
   ```

2. **查询日志**
   - 点击左侧菜单 `Logs`
   - 在 Stream 下拉框中选择 `kubernetes_logs` 或 `default`
   - 在查询框中输入：
     ```
     service="test-app"
     ```
   - 点击 `Run Query`

3. **验证结果**
   - 应该能看到测试应用产生的日志
   - 日志应该包含以下字段：
     - `level`: "info"
     - `service`: "test-app"
     - `env`: "prod"
     - `message`: "Test log message"
     - `kubernetes.pod_name`: Pod 名称
     - `kubernetes.namespace`: "default"
     - `kubernetes.node_name`: 节点名称

#### 7.3.5 验证元数据

在 OpenObserve 中查看日志详情，确认包含以下 Kubernetes 元数据：

**必需的元数据字段**：
- `kubernetes.pod_name`: Pod 名称（例如：`test-app-xxx-xxx`）
- `kubernetes.namespace`: 命名空间（例如：`default`）
- `kubernetes.node_name`: 节点名称（例如：`k8s-master`）
- `kubernetes.container_name`: 容器名称（例如：`test-app`）

**应用自定义字段**：
- `service`: 应用服务名（来自 Go 应用配置）
- `env`: 环境标识（来自 Go 应用配置）
- `level`: 日志级别
- `time`: 时间戳
- `message`: 日志消息

**验证命令**（使用 OpenObserve API）：

```bash
# 查询最近的日志
curl -u "your-email@example.com:your-password" \
  "http://localhost:5080/api/<org-id>/_search" \
  -H "Content-Type: application/json" \
  -d '{
    "query": {
      "sql": "SELECT * FROM kubernetes_logs WHERE service='test-app' LIMIT 10"
    }
  }'
```

### 7.4 端到端测试

#### 7.4.1 测试完整流程

1. **部署真实 Go 应用**
   ```bash
   # 使用包含 logger 包的 Go 应用
   # 确保应用设置了 APP_ENV=prod
   ```

2. **生成测试日志**
   ```go
   // 在应用中执行
   log.Info().Msg("Application started")
   log.Warn().Int("latency", 500).Msg("Database slow")
   log.Error().Err(err).Msg("File not found")
   ```

3. **验证日志流转**
   - ✅ 应用输出 JSON 到 Stdout
   - ✅ Kubernetes 写入日志文件
   - ✅ Vector 发现并读取日志
   - ✅ Vector 解析 JSON 并添加元数据
   - ✅ Vector 批量发送到 OpenObserve
   - ✅ OpenObserve 存储和索引日志
   - ✅ 在 Web UI 中能查询到日志

#### 7.4.2 性能测试

```bash
# 生成大量日志测试性能
kubectl run -it --rm perf-test --image=busybox --restart=Never -- \
  sh -c 'for i in $(seq 1 1000); do 
    echo "{\"level\":\"info\",\"time\":\"'$(date -Iseconds)'\",\"message\":\"Log message $i\"}"
  done'

# 在 OpenObserve 中验证是否能查询到所有日志
```

### 7.5 清理测试资源

```bash
# 删除测试应用
kubectl delete deployment test-app

# 删除测试文件
rm test-app.yaml
```

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

