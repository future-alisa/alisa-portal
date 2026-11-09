# 架构与实施边界

## 技术选择

- 前端：Next.js App Router、React、TypeScript、Lucide 图标、响应式 CSS。通过同源代理调用 Go，避免浏览器直接访问内网 API。
- 后端：Go 1.24 标准库路由与 JSON API，明确的服务/租户/部署领域模型。MVP 使用本地 JSON 持久化，不引入不必要的运行依赖。
- 当前统一登录：[Alisa Cloud OAuth + PKCE](alisa-cloud-auth.md)，服务端会话及实时内省 / 权限判断，仅显式授予门户实例管理权限的用户进入完整控制台。
- 后续演进：PostgreSQL + sqlc/pgx 作为事实数据源，OIDC（Keycloak / 现有身份提供商）+ 服务端会话，RBAC 授权、租户访问过滤和数据库审计记录。
- 集群接入：后续使用 client-go、限定命名空间的 ServiceAccount；租户映射 Namespace、ResourceQuota、LimitRange、NetworkPolicy。
- 交付：当前仅输出 Deployment/Service YAML。后续对接 Git 仓库提交审批，经 Argo CD 同步，不将高权限 kubeconfig 暴露给浏览器。
- 可观测性：已通过 Go 端查询固定 PromQL，配置 PROMETHEUS_URL 后可用；Grafana 提供仪表盘，Loki 提供日志，Alertmanager 提供告警。Grafana、Loki 与 Alertmanager 连接器尚未实现。

## 开源参考

- [Backstage 软件目录](https://backstage.io/docs/overview/technical-overview/)：服务归属、统一资源目录和插件化集成，借鉴设计思路，没有引入整个 Backstage 运行时。
- [Backstage Kubernetes](https://backstage.io/docs/features/kubernetes/installation/)：按服务聚合关联集群资源，作为后续详情页方案。
- [Argo CD 架构](https://argo-cd.readthedocs.io/en/stable/operator-manual/architecture/)：Git 中的期望状态与集群实际状态协调。
- [Next.js App Router](https://nextjs.org/docs/app)：应用路由与前后端边界。

## 对现有环境的适配

入口链路保持：公网入口 → Caddy → Traefik → Portal Web → Go API。建议门户独立域名或根路径部署；当前 Next.js 不配置子路径 basePath，不能直接剥离任意门户路径前缀使用。

前端构建时 API_URL 指向 Go ClusterIP 服务，API 不直接暴露公网。Kubernetes 部署先创建 Namespace 和独立 PVC，再配置 API_TOKEN Secret、容器探针与资源限额。JSON 文件后端只运行一个副本。真实 Kubernetes 清单需结合已有 Gateway、存储类、域名与证书配置；本项目未猜测或覆盖这些资源。

Registry、SeaweedFS、数据库等连接凭据应置于 Secret 或外部凭据管理系统。门户仅登记其访问入口；登记操作不自动验证协议连通性，也不等同于获得服务管理权限。

## 后续实施顺序

1. PostgreSQL 持久化、迁移脚本、备份与恢复。
2. 在现有 Alisa Cloud 统一登录上继续完善细粒度角色和身份租户数据隔离；当前 console:manage 是整个门户实例的管理权限，不等同于租户级访问过滤。
3. client-go 只读资源发现、状态同步、namespace 配额同步。
4. Git 仓库变更申请与 Argo CD 只读发布状态，然后增补审批与回滚。
5. 扩展 Prometheus 历史趋势、日志查询与告警确认。

当前为管理员单租户权限域中的“租户登记”，尚不提供多租户安全隔离；部署是清单草稿，无真实发布状态、回滚或日志流。
