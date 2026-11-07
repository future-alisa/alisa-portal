# Alisa Portal

面向个人云与小型团队的基础设施门户。Next.js App Router + React + TypeScript 前端，Go 标准库 HTTP API 后端。当前为可运行的本地 MVP；没有连接或更改真实基础设施。

## 启动

要求 Node.js 22.9+、npm、Go 1.24+。

```sh
cd frontend
npm ci
npm run dev
```

另开终端：

```sh
cd backend
go run ./cmd/server
```

打开 http://localhost:3000。API 默认监听 `127.0.0.1:8080`；Next.js 代理同源 `/api/*`，无需浏览器跨域配置。前端 `.env.local` 可配置 `API_URL`（修改后重启；生产构建时也需传入）。

可选：设置后端 `API_TOKEN` 后，在页面「连接设置」输入同一个管理员令牌。令牌仅保存在页面内存。未设置令牌仅适用于可信的本地开发环境。此令牌代表单一管理员，并非多用户认证或租户访问隔离。

## 已实现

- 工作台：资源汇总、服务分类/搜索、最近操作记录。
- 服务资源：登记、归属租户、HTTP(S) 入口、移除目录记录；状态均为待核验。
- 租户管理：创建租户、命名空间唯一性校验、CPU/内存规划配额。
- 部署中心：校验应用/镜像/端口/副本，检查门户预留配额，生成并下载 Deployment + Service YAML。每副本 limits 为 250m CPU / 256Mi。不会直接部署。
- 资源监控：可配置 Prometheus 即时查询 CPU、内存、存储及运行中 Pod，未连接/无数据时明确标注。
- Go API：JSON 文件原子写入、并发保护、管理员令牌、请求体限制、输入校验、操作记录、优雅关闭。

数据保存在 `backend/data/portal.json`，已忽略。适用于单实例 MVP；多个后端进程不能共享写入此文件。后续迁移 PostgreSQL 后再扩容副本。

## 容器运行

支持 Docker Compose / Podman Compose（需安装对应 compose provider）：

```sh
cp .env.example .env
# 编辑 .env，替换 API_TOKEN
podman compose up --build -d
# 或 docker compose up --build -d
```

打开 http://localhost:3000，在连接设置中填写令牌。Go API 不映射宿主端口，持久化卷为 `portal-data`。现有 Traefik/Caddy 入口接入请参见 [架构说明](docs/architecture.md)。

## 验证

```sh
make test
make build
```

Go 测试覆盖令牌认证、租户唯一性、部署清单生成、配额校验、URL 校验、持久化，以及 Prometheus 查询和错误处理。

## API

除健康检查外，若设置 `API_TOKEN`，需携带 `Authorization: Bearer <token>`。

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | /api/health | 健康检查 |
| GET | /api/state | 目录、租户、部署申请、操作记录 |
| POST | /api/services | 登记服务（name/category/tenant/url/description） |
| DELETE | /api/services/{id} | 删除目录记录 |
| POST | /api/tenants | 创建租户（name/namespace/cpu/memory） |
| POST | /api/deployments | 生成清单（name/tenant/image/replicas/port） |
| GET | /api/metrics | Prometheus 指标或 unconfigured 状态 |

## 私有基础设施文件

根目录 `基础设施.md` 及同名前缀文件已加入 `.gitignore`。项目不会读取该文件用于构建、不会包含其凭据或私有地址。预置目录仅包含通用服务名称。基础设施原文不纳入版本控制。

## Prometheus 监控

在 Go 服务环境中配置 `PROMETHEUS_URL=http://prometheus:9090`，可选 `PROMETHEUS_TOKEN`。刷新门户后读取汇总指标。需要 node-exporter 与 kube-state-metrics；空查询结果显示暂无数据。地址和令牌由服务端配置，不通过浏览器提交。当前只有即时指标，没有历史趋势、日志和告警连接器。Compose 可在 api.environment 中增加这些配置。

## Kubernetes / Argo CD

生产清单位于 `deploy/overlays/production`，Application 位于 `deploy/argocd/application.yaml`。私有 Registry 镜像构建、Secret 注入、根域名入口和回滚说明参见 [Argo CD 部署](docs/deployment.md)。
