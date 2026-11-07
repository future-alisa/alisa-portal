# Argo CD 部署

## 交付链路

已跟踪的前后端源码 → 集群内 Kaniko 构建 → 私有 Registry → Argo CD 同步 Deployment → Kubernetes 拉取镜像。

- 环境入口：`https://alisa.vip.cpolar.cn/`。
- 应用名：`alisa-portal`；命名空间：`alisa-portal`。
- GitOps 路径：`deploy/overlays/production`。
- 镜像标签为源码包内容哈希，不使用浮动 latest。
- 后端数据：`portal-data` PVC，1Gi，使用集群默认 StorageClass。API 单副本并采用 Recreate 更新策略。
- `portal-auth` 保存管理员 API_TOKEN；`portal-registry-auth` 保存镜像仓库认证。Git 中只有空占位资源，真实值经 Argo CD Resource API 注入 Secret。
- Application 对两份 Secret 的 `/data` 配置 ignoreDifferences，并启用 RespectIgnoreDifferences，后续同步保留实时凭据。
- PVC 和两份 Secret 配置 Prune=false、Delete=false。删除 Application 不会主动删除持久化数据和凭据。

## 首次安装

1. 将 `deploy/argocd/application.yaml` 注册到 Argo CD；首次同步创建命名空间、空 Secret、PVC 和工作负载。
2. 向两份 Secret 注入真实值。不要把真实凭据填进 YAML 或提交 Git。没有凭据时容器不会正常就绪。
3. 构建并推送两个镜像，将不可变标签写入 production kustomization，然后同步。
4. 确认 PVC Bound、Deployment Available、HTTPRoute Accepted/ResolvedRefs。
5. 验证公网入口。若 Windows Caddy 尚未转发根路径，需要在现有站点增加兜底 reverse_proxy；不要替换已有 /argocd、/fs 等路径规则。

## 镜像构建

`scripts/make-build-context.py` 只读取 Git 已跟踪的 backend/frontend 文件，生成压缩源码 ConfigMap 和非特权构建 Job，避免把基础设施原文、.env 或本机配置打进镜像。生成物写到显式指定的临时目录，不提交 Git。

```sh
python3 scripts/make-build-context.py \
  --registry YOUR_REGISTRY_HOST \
  --output /tmp/alisa-portal-build
```

输出 `build-manifests.json` 可通过 Argo CD 本地清单同步执行（每个对象需 JSON 序列化为字符串），也可由有权限的管理员使用 kubectl 逐个创建。构建 Pod 使用 `portal-registry-auth`，不挂载 ServiceAccount 令牌、Docker socket 或宿主机目录。两份 Dockerfile 分别构建 Linux 节点架构对应的镜像。

这是首次部署的构建入口；尚未安装定时构建器或 GitHub Actions runner。新版本按相同步骤构建、推送、更新标签、同步。

## 根路径 Caddy 转发参考

将下面兜底规则加在已有站点的服务路由之后。具体上游地址、SNI 和 CA 路径由服务器管理员从现有配置复用。

```caddyfile
handle {
    reverse_proxy https://TRAEFIK_ADDRESS:443 {
        header_up Host alisa.vip.cpolar.cn
        transport http {
            tls_server_name EXISTING_TRAEFIK_TLS_NAME
            tls_trust_pool file EXISTING_CA_FILE
        }
    }
}
```

不要关闭 TLS 校验。此仓库不会修改 Windows 上的 Caddy 文件；没有该服务器访问权限时，根路径转发必须由管理员完成。

## 验证与回滚

在 Argo CD 查看应用健康、同步结果与 Pod 日志。可用 kubectl 时：

```sh
kubectl -n alisa-portal get pods,pvc,svc,httproute
kubectl -n alisa-portal rollout status deployment/api
kubectl -n alisa-portal rollout status deployment/web
```

回滚应将 production 镜像标签还原到上一已验证版本并提交 Git，然后同步。不要直接回滚到会破坏数据格式兼容性的后端版本，回滚前备份 PVC 中的 portal.json。

## 本地凭据

部署时生成的管理员令牌保存在项目本地 `.env`（权限 0600，已忽略）及集群 `portal-auth` Secret 中。在门户「连接设置」输入该令牌。Registry 凭据来源于用户的私有基础设施文件，不复制进此文档或源码。
