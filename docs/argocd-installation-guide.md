# ArgoCD 安装指南

**版本**: 1.0  
**日期**: 2025-12-25  
**适用对象**: DevOps 工程师、Kubernetes 管理员

---

## 目录

1. [安装前准备](#1-安装前准备)
2. [标准安装（推荐）](#2-标准安装推荐)
3. [资源受限环境安装（可选）](#3-资源受限环境安装可选)
4. [配置访问](#4-配置访问)
5. [验证安装](#5-验证安装)
6. [故障排查](#6-故障排查)

---

## 1. 安装前准备

### 1.1 系统要求

**最低要求**:
- Kubernetes 集群版本: v1.19+
- 至少 2 个 CPU 核心
- 至少 2GB 可用内存（推荐 4GB+）
- 网络连接到 GitHub/Docker Hub

**推荐配置**:
- Kubernetes 集群版本: v1.24+
- 4+ CPU 核心
- 8GB+ 可用内存
- 稳定的网络连接

### 1.2 检查集群状态

在开始安装前，请确认集群状态正常：

```bash
# 检查节点状态
kubectl get nodes -o wide

# 检查集群版本
kubectl version --short

# 检查可用资源
kubectl top nodes
```

---

## 2. 标准安装（推荐）

### 2.1 创建命名空间

```bash
kubectl create namespace argocd
```

### 2.2 安装 ArgoCD

使用官方提供的安装清单：

```bash
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
```

**说明**: 这会安装 ArgoCD 的非高可用（Non-HA）版本，包含以下组件：
- `argocd-application-controller`: 应用控制器
- `argocd-repo-server`: 仓库服务器
- `argocd-server`: API 服务器和 UI
- `argocd-redis`: Redis 缓存
- `argocd-dex-server`: 认证服务器
- `argocd-notifications-controller`: 通知控制器（可选）
- `argocd-applicationset-controller`: ApplicationSet 控制器

### 2.3 等待所有 Pod 就绪

```bash
# 查看 Pod 状态
kubectl get pods -n argocd -w

# 等待所有 Pod 状态为 Running
kubectl wait --for=condition=ready pod --all -n argocd --timeout=300s
```

**预期输出**: 所有 Pod 状态应为 `Running`，READY 为 `1/1`。

---

## 3. 资源受限环境安装（可选）

> **⚠️ 注意**: 本节适用于资源受限的环境（如工作节点只有 1GB 内存）。如果你的集群资源充足，可以跳过本节。

### 3.1 核心难点

**场景**: 工作节点只有 1GB 内存

**现状**: 
- K8s 系统组件 + Traefik 已经占用了一部分内存
- ArgoCD 是一套组件（Repo Server, Application Controller, API Server, Redis, Dex, Server），全套跑起来非常吃内存
- 风险：很容易导致节点 OOM（内存溢出）死机

**解决方案**: 
- 使用非高可用（Non-HA）版本
- 手动调低 ArgoCD 各组件的资源请求和限制

### 3.2 安装 ArgoCD（标准步骤）

首先按照 [标准安装](#2-标准安装推荐) 的步骤安装 ArgoCD。

### 3.3 给 ArgoCD 瘦身

默认配置可能会撑爆你的内存，我们需要限制它的资源使用。请依次执行以下 Patch 命令：

```bash
# 降低 Redis 资源
kubectl patch deployment argocd-redis -n argocd --type='json' \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/resources", "value": {"requests": {"memory": "32Mi", "cpu": "10m"}, "limits": {"memory": "64Mi"}}}]'

# 降低 Repo Server 资源
kubectl patch deployment argocd-repo-server -n argocd --type='json' \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/resources", "value": {"requests": {"memory": "32Mi", "cpu": "10m"}, "limits": {"memory": "128Mi"}}}]'

# 降低 Application Controller 资源
kubectl patch statefulset argocd-application-controller -n argocd --type='json' \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/resources", "value": {"requests": {"memory": "32Mi", "cpu": "10m"}, "limits": {"memory": "128Mi"}}}]'

# 降低 API Server 资源
kubectl patch deployment argocd-server -n argocd --type='json' \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/resources", "value": {"requests": {"memory": "32Mi", "cpu": "10m"}, "limits": {"memory": "128Mi"}}}]'

# 降低 Dex (认证组件) 资源
kubectl patch deployment argocd-dex-server -n argocd --type='json' \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/resources", "value": {"requests": {"memory": "16Mi", "cpu": "10m"}, "limits": {"memory": "64Mi"}}}]'

# 降低 Notifications Controller 资源（如果存在）
kubectl patch deployment argocd-notifications-controller -n argocd --type='json' \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/resources", "value": {"requests": {"memory": "16Mi", "cpu": "10m"}, "limits": {"memory": "64Mi"}}}]' 2>/dev/null || echo "Notifications Controller not found, skipping..."

# 降低 ApplicationSet Controller 资源（如果存在）
kubectl patch deployment argocd-applicationset-controller -n argocd --type='json' \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/resources", "value": {"requests": {"memory": "16Mi", "cpu": "10m"}, "limits": {"memory": "64Mi"}}}]' 2>/dev/null || echo "ApplicationSet Controller not found, skipping..."
```

### 3.4 验证资源限制

检查资源限制是否生效：

```bash
# 查看所有 Pod 的资源限制
kubectl get pods -n argocd -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.containers[0].resources}{"\n"}{end}'

# 或者使用更友好的方式
kubectl get pods -n argocd -o custom-columns=NAME:.metadata.name,REQUESTS:.spec.containers[0].resources.requests,LIMITS:.spec.containers[0].resources.limits
```

### 3.5 监控资源使用

持续监控资源使用情况，确保不会 OOM：

```bash
# 查看 Pod 资源使用情况
kubectl top pods -n argocd

# 查看节点资源使用情况
kubectl top nodes

# 查看 Pod 事件（关注是否有 OOM 相关事件）
kubectl get events -n argocd --sort-by='.lastTimestamp' | grep -i oom
```

### 3.6 资源限制参考表

| 组件 | 内存请求 | 内存限制 | CPU 请求 | CPU 限制 |
|------|---------|---------|---------|---------|
| Redis | 32Mi | 64Mi | 10m | - |
| Repo Server | 32Mi | 128Mi | 10m | - |
| Application Controller | 32Mi | 128Mi | 10m | - |
| API Server | 32Mi | 128Mi | 10m | - |
| Dex Server | 16Mi | 64Mi | 10m | - |
| Notifications Controller | 16Mi | 64Mi | 10m | - |
| ApplicationSet Controller | 16Mi | 64Mi | 10m | - |

**总计**: 约 200-300Mi 内存请求，500-600Mi 内存限制

---

## 4. 配置访问

### 4.1 方式一：使用 NodePort（推荐用于开发/测试）

为了方便访问，我们把 ArgoCD Server 改为 NodePort：

```bash
kubectl patch svc argocd-server -n argocd -p '{"spec": {"type": "NodePort", "ports": [{"port": 80, "targetPort": 8080, "nodePort": 30088}, {"port": 443, "targetPort": 8080, "name": "https", "nodePort": 30444}]}}'
```

**访问地址**:
- HTTP: `http://<NodeIP>:30088`
- HTTPS: `https://<NodeIP>:30444`

**注意**: ArgoCD 默认强制 HTTPS，即便访问 HTTP 也会重定向到 HTTPS。

### 4.2 方式二：使用 Port Forward（临时访问）

```bash
kubectl port-forward svc/argocd-server -n argocd 8080:443
```

然后访问: `https://localhost:8080`

### 4.3 方式三：使用 Ingress（生产环境推荐）

如果你已经安装了 Ingress Controller（如 Traefik），可以创建 Ingress 资源：

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: argocd-server-ingress
  namespace: argocd
  annotations:
    # Traefik 相关注解
    traefik.ingress.kubernetes.io/router.entrypoints: web,websecure
spec:
  ingressClassName: traefik
  rules:
  - host: argocd.yourdomain.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: argocd-server
            port:
              number: 80
```

---

## 5. 验证安装

### 5.1 获取初始管理员密码

ArgoCD 安装后会生成一个初始管理员密码，存储在 Secret 中：

```bash
# 获取初始密码
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d; echo
```

**默认用户名**: `admin`  
**默认密码**: 上面命令输出的内容

### 5.2 登录 ArgoCD UI

1. 打开浏览器访问 ArgoCD UI（根据你选择的访问方式）
2. 使用 `admin` 和上面获取的密码登录
3. 首次登录会提示修改密码（建议修改）

### 5.3 使用 ArgoCD CLI（可选）

#### 安装 ArgoCD CLI

**Linux/macOS**:
```bash
# 下载最新版本
curl -sSL -o /usr/local/bin/argocd https://github.com/argoproj/argo-cd/releases/latest/download/argocd-linux-amd64
chmod +x /usr/local/bin/argocd
```

**Windows**:
```powershell
# 使用 Chocolatey
choco install argocd

# 或使用 Scoop
scoop install argocd
```

#### 登录 ArgoCD CLI

```bash
# 如果使用 NodePort
argocd login <NodeIP>:30444 --insecure

# 如果使用 Port Forward
argocd login localhost:8080 --insecure

# 如果使用 Ingress
argocd login argocd.yourdomain.com
```

### 5.4 验证组件状态

```bash
# 检查所有 Pod 状态
kubectl get pods -n argocd

# 检查 Service
kubectl get svc -n argocd

# 检查 ArgoCD 版本
argocd version

# 检查集群连接
argocd cluster list
```

**预期输出**: 所有组件状态正常，可以正常访问 UI 和 CLI。

---

## 6. 故障排查

### 6.1 Pod 无法启动

**症状**: Pod 状态为 `Pending` 或 `CrashLoopBackOff`

**排查步骤**:

```bash
# 1. 查看 Pod 详细信息
kubectl describe pod <pod-name> -n argocd

# 2. 查看 Pod 日志
kubectl logs <pod-name> -n argocd

# 3. 查看事件
kubectl get events -n argocd --sort-by='.lastTimestamp'

# 4. 检查资源配额
kubectl describe nodes
```

**常见原因**:
- 资源不足（内存/CPU）
- 镜像拉取失败
- 配置错误

### 6.2 内存不足（OOM）

**症状**: Pod 被 OOMKilled

**排查步骤**:

```bash
# 1. 查看 Pod 状态
kubectl get pods -n argocd

# 2. 查看 Pod 事件
kubectl describe pod <pod-name> -n argocd | grep -A 10 Events

# 3. 查看资源使用
kubectl top pods -n argocd

# 4. 如果使用资源受限安装，检查资源限制是否生效
kubectl get pod <pod-name> -n argocd -o jsonpath='{.spec.containers[0].resources}'
```

**解决方案**:
- 如果使用资源受限安装，进一步降低资源限制
- 考虑增加节点内存
- 禁用不必要的组件（如 Notifications Controller）

### 6.3 无法访问 UI

**症状**: 浏览器无法访问 ArgoCD UI

**排查步骤**:

```bash
# 1. 检查 Service 类型和端口
kubectl get svc argocd-server -n argocd

# 2. 检查 Pod 是否运行
kubectl get pods -n argocd -l app.kubernetes.io/name=argocd-server

# 3. 检查 Pod 日志
kubectl logs -n argocd -l app.kubernetes.io/name=argocd-server

# 4. 测试端口连通性（从节点上）
curl -k https://localhost:30444

# 5. 如果使用 NodePort，检查防火墙规则
```

**常见原因**:
- Service 类型配置错误
- 防火墙阻止端口
- Pod 未正常运行

### 6.4 无法获取初始密码

**症状**: Secret 不存在或密码为空

**排查步骤**:

```bash
# 1. 检查 Secret 是否存在
kubectl get secret argocd-initial-admin-secret -n argocd

# 2. 如果不存在，等待 ArgoCD 初始化完成
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=argocd-server -n argocd --timeout=300s

# 3. 如果仍然不存在，可以手动重置密码
# 删除 Secret，ArgoCD 会重新生成
kubectl delete secret argocd-initial-admin-secret -n argocd
# 等待几秒钟后再次获取
```

### 6.5 同步失败

**症状**: Application 无法同步，状态为 `Unknown` 或 `Degraded`

**排查步骤**:

```bash
# 1. 查看 Application 状态
kubectl -n argocd get applications

# 2. 查看 Application Controller 日志
kubectl logs -n argocd -l app.kubernetes.io/name=argocd-application-controller --tail=100

# 3. 查看 Repo Server 日志
kubectl logs -n argocd -l app.kubernetes.io/name=argocd-repo-server --tail=100

# 4. 检查 Git 仓库连接
argocd repo list
```

**常见原因**:
- Git 仓库无法访问
- 认证信息错误
- 资源定义错误

---

## 附录

### A. 卸载 ArgoCD

如果需要完全卸载 ArgoCD：

```bash
# 删除命名空间（会删除所有资源）
kubectl delete namespace argocd

# 或者只删除安装的资源
kubectl delete -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
```

### B. 升级 ArgoCD

```bash
# 1. 备份当前配置
kubectl get applications -n argocd -o yaml > argocd-applications-backup.yaml

# 2. 更新安装清单
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# 3. 等待升级完成
kubectl rollout status deployment/argocd-server -n argocd
kubectl rollout status deployment/argocd-repo-server -n argocd
kubectl rollout status statefulset/argocd-application-controller -n argocd
```

### C. 常用命令速查

| 操作 | 命令 |
|------|------|
| 查看所有 Pod | `kubectl get pods -n argocd` |
| 查看所有 Service | `kubectl get svc -n argocd` |
| 查看资源使用 | `kubectl top pods -n argocd` |
| 获取初始密码 | `kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" \| base64 -d; echo` |
| 查看 ArgoCD 版本 | `argocd version` |
| 查看集群列表 | `argocd cluster list` |

### D. 参考资源

- [ArgoCD 官方文档](https://argo-cd.readthedocs.io/)
- [ArgoCD GitHub 仓库](https://github.com/argoproj/argo-cd)
- [ArgoCD 最佳实践](https://argo-cd.readthedocs.io/en/stable/user-guide/best_practices/)
- [ArgoCD 故障排查](https://argo-cd.readthedocs.io/en/stable/operator-manual/troubleshooting/)

---

**文档维护**: 本文档应随 ArgoCD 版本更新及时更新。  
**最后更新**: 2025-12-25

