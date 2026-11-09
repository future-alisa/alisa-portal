# 开发与发布流程

日常使用短期任务分支，不额外引入长期 develop 分支。`main` 是经过评审的生产基线，也是 Argo CD 的跟踪分支。

## 开始任务

确认工作区状态，保留他人的未提交修改。从最新 main 创建对应分支：

```sh
git fetch origin
git switch main
git pull --ff-only origin main
git switch -c feature/your-feature
# 修复使用 fix/your-fix，维护使用 chore/your-task
```

每个分支围绕一个可评审目标。完成后提交并推送任务分支，通过 PR 合并到 main，不日常直接向 main 提交。紧急例外需要用户明确授权。

## 本地验证与 CI

- 后端：`cd backend && go test -race ./... && go vet ./... && go build ./cmd/server`。
- 前端：`cd frontend && npm ci && npm run typecheck && npm run test:network && npm run build`。
- CI 在分支 push 和针对 main 的 PR 中执行 backend/frontend 两组检查，仅有仓库读取权限，不访问生产 Secret、构建私有镜像或部署集群。
- PR 描述说明具体问题、结果、验证及发布影响；保留需要评审的截图或运行结果。CI 通过不代表自动获准合并。

## 合并与发布

1. 任务分支通过 CI 和代码评审后合并 main，建议 squash merge。
2. 从明确的代码版本构建镜像并推送 Registry，验证镜像摘要；构建成功本身不会更新生产工作负载。
3. 需要发布时，从最新 main 创建 `release/*` 或 `fix/*` 分支，更新 production kustomization 的镜像标签和 digest，并提交发布 PR。发布 PR 可以与小修复合并，但必须先存在已验证镜像。
4. 发布 PR 经评审合并 main 后，Argo CD 自动同步并 self-heal；自动 prune 保持关闭。
5. 检查 Argo CD Synced/Healthy、Pod、入口、API 和必要的数据兼容性。回滚通过还原生产镜像引用的 PR 完成，不直接修改在线 Deployment 与 Git 对抗。

只修改应用源码的 PR 不会自动产生新镜像。当前镜像构建仍使用 docs/deployment.md 中的构建工具；此 CI 未配置 Registry 推送凭据。

## main 分支保护建议

在 GitHub 仓库设置中，为 main 启用 PR 合并要求、至少一次评审、必须通过 backend/frontend 检查，并禁止强推和删除。仓库内的工作约定不能替代 GitHub 服务器端的分支保护；新增 workflow 合并并首次运行后，管理员再将相应检查设为 required。

## 私有信息

禁止提交基础设施私有原文、.env、kubeconfig、认证头、Registry/Argo CD 凭据、生产数据、备份和包含环境信息的临时构建文件。生产管理员令牌仅在受控环境中使用，不写入 PR、CI 输出或构建产物。

## 公开服务目录

未登录页面仅请求 `GET /api/public/services`，不请求管理状态或监控接口。服务新增 `visibility`（`private` / `public`）、`accessRequirement`（`none` / `account`）和独立 `publicDescription` 字段。已有资源默认仅管理员可见，避免历史内网数据意外公开。管理员编辑服务时可逐项公开应用网站，并标注是否需要目标服务账号；公开目录只返回名称、分类、公开说明、外网链接及访问要求。内部说明、内网入口、租户、网络、部署和事件不进入公开响应。公开资源必须配置外网地址。该标注不改变应用本身的认证和授权。
