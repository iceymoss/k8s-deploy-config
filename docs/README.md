# Kubernetes 部署配置文档

本目录包含 Kubernetes 集群的架构和配置文档。

## 📚 文档列表

### [架构与配置流向深度解析](./architecture-and-configuration-flow.md) | [中文版](./架构与配置流向深度解析.md)

**完整系统架构文档**，包含：

- ✅ 集群架构图（Mermaid 图表）
- ✅ 配置流向全景图
- ✅ 网络流量路径图
- ✅ 层级详细拆解与操作命令
- ✅ 核心知识点与防坑指南
- ✅ 实际项目配置说明
- ✅ 故障排查指南

**适用对象**: DevOps 工程师、Kubernetes 管理员、系统架构师

---

### [ArgoCD 安装指南](./argocd-installation-guide.md) | [中文版](./argocd-安装指南.md)

**ArgoCD 完整安装文档**，包含：

- ✅ 标准安装步骤（推荐）
- ✅ 资源受限环境安装（可选，适用于 1GB 内存节点）
- ✅ 多种访问方式配置（NodePort/Port Forward/Ingress）
- ✅ 安装验证和故障排查
- ✅ 资源限制参考表

**适用对象**: DevOps 工程师、Kubernetes 管理员

---

### [Traefik 链路排查验证指南](./traefik-link-verification-guide.md) | [中文版](./traefik-链路排查验证指南.md)

**Traefik Ingress Controller 链路验证文档**，包含：

- ✅ Traefik 工作原理深度解析
- ✅ 项目部署架构分析
- ✅ 5 种链路验证方法（日志、Header、Dashboard、拔线测试、端口验证）
- ✅ 日志解读与分析（成功、404、502 等场景）
- ✅ 故障排查指南
- ✅ 最佳实践

**适用对象**: DevOps 工程师、Kubernetes 管理员、系统架构师

---

### [Kustomize TCP 服务配置指南](./kustomize-tcp-service-guide.md) | [中文版](./kustomize-tcp-服务配置指南.md)

**Kustomize TCP 服务配置文档**，包含：

- ✅ 项目结构标准化（Base/Overlay 模式）
- ✅ Base 层配置详解（Deployment、Service、IngressRouteTCP）
- ✅ Overlay 层配置详解（资源限制、路由补丁、镜像替换）
- ✅ 多 TCP 服务架构方案（多端口策略 vs TLS SNI 多路复用）
- ✅ 最佳实践和端口分配表

**适用对象**: DevOps 工程师、Kubernetes 管理员

---

### [Kustomize UDP 服务配置指南](./kustomize-udp-service-guide.md) | [中文版](./kustomize-udp-服务配置指南.md)

**Kustomize UDP 服务配置文档**，包含：

- ✅ UDP vs TCP 核心区别对比
- ✅ Traefik UDP 架构图、原理图、数据流图
- ✅ Base 层配置详解（Deployment、Service、IngressRouteUDP）
- ✅ Overlay 层配置详解（资源限制、路由补丁、镜像替换）
- ✅ Go UDP 应用开发示例
- ✅ 测试验证和故障排查

**适用对象**: DevOps 工程师、Kubernetes 管理员

---

### [k3d 端口映射解决方案](./k3d-port-mapping-solution.md) | [中文版](./k3d-端口映射解决方案.md)

**k3d 集群端口映射解决方案文档**，包含：

- ✅ 问题背景和原因分析
- ✅ 方案一：kubectl port-forward（仅 TCP）
- ✅ 方案二：Docker 外挂网关（TCP + UDP，推荐）
- ✅ 架构图和方案对比
- ✅ 故障排查和最佳实践

**适用对象**: DevOps 工程师、本地开发人员

## 🚀 快速开始

1. 查看 [架构与配置流向文档](./architecture-and-configuration-flow.md) 了解整体架构
2. 根据文档中的命令进行日常操作
3. 遇到问题时参考故障排查指南

## 📊 当前集群状态

### 节点信息

- **k8s-master** (10.4.4.15): control-plane 节点
- **k8s-node1** (10.4.0.17): worker 节点

### 主要组件

- **ArgoCD**: GitOps 持续部署
- **Traefik**: Ingress Controller (NodePort: 30080/30443)
- **Calico**: 网络插件
- **Kubernetes Dashboard**: 集群管理 UI

### 业务应用

- **backend/admin-api**: 管理后台 API (`dev.admin.test.com`)
- **web/web-em**: 前端应用

## 🔗 相关链接

- [项目仓库](https://github.com/iceymoss/k8s-deploy-config)
- [ArgoCD 官方文档](https://argo-cd.readthedocs.io/)
- [Traefik 官方文档](https://doc.traefik.io/traefik/)

