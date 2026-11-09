# Alisa Cloud 统一授权接入

门户是 OAuth 保密客户端，也是需要内省权限的资源服务器。协议对应 `alisa-cloud/OAUTH.md` 和 `MULTITENANCY.md`，不依赖未实现的 OIDC userinfo / ID token。

## 权限边界

当前门户是一个基础设施管理实例。`portal:console:manage` 表示管理整个实例，包含全部服务、网络、部署申请、操作记录和本地登记的基础设施租户。**仅授予门户实例管理员**，指定身份租户内的普通成员可以登录门户，使用公开目录。门户的「租户管理」打开 Alisa Admin 的租户界面，「资源配额」保留基础设施命名空间和配额规划。门户中的基础设施租户不是授权中心的身份租户；本次不把二者自动映射，也不声称提供跨身份租户的数据隔离。

每个管理 API 请求实时调用 `/oauth2/introspect`，检查 active、exp、iss、application_id=portal、aud 包含 portal、identity_type=user、userId、tenant_id 和 scope；再以该用户 token 调用 `/api/v1/authorization/check`，要求 applicationId=portal、resourceType=console、resourceId=*、action=manage 返回 `code=200` / `PERMIT`。拒绝其他业务 token、后台应用、错误租户或只有登录而没有权限的用户访问管理 API。网络错误或不确定决策拒绝访问，不缓存允许结果。

`ALISA_OAUTH_TENANT_ID` 为空时仅接受无身份租户的 token。上游开启多租户后，配置门户所属的唯一身份租户 ID，并使用 tenant 授权提供者；其他身份租户即使存在同名角色也不能进入此门户。没有自动创建角色或给所有用户授权。

## 登录与会话

浏览器只持有随机会话 Cookie（生产 `Secure`、`HttpOnly`、`SameSite=Lax`、`__Host-`、Path=/），OAuth token、客户端密钥均在 Go 服务端。登录绑定随机 state、浏览器登录 Cookie 和 PKCE S256，回调只能消费一次；回调地址由配置固定为 `ALISA_PORTAL_ORIGIN/api/auth/callback`，不接受请求 Host / returnTo 决定跳转。

会话最多 8 小时，且不会超过 access token 的到期时间；本次不保存或自动使用 refresh token，过期后重新走统一登录（授权中心原有登录会话可复用）。Go API 单副本，进程重启后需重新登录。每个 Cookie 写请求和注销都验证配置的 Origin 和会话 CSRF 值。注销立即删除门户会话，并尽力撤销上游 access token；不退出其他应用或授权中心会话。

公开目录、健康检查、登录配置、会话检查无需管理员令牌。未登录会话检查返回 200 / authenticated=false，前端不会预先请求管理 API，避免正常访客遇到 401 控制台错误。公开目录不受授权中心故障影响。

## 上游配置步骤

1. 先备份用户服务数据库。按 alisa-cloud 文档安装 OAuth 和资源授权表（开启多租户时还要安装 tenant 表）；不要盲目重跑 SQL，也不要仅改 enabled 就重启。门户不自动修改授权数据库。
2. 在用户服务现有配置中合并 [配置示例](../deploy/examples/alisa-cloud-portal.yaml)，保留其他业务 scope。配置平台管理员的真实内部用户 ID。多租户启用时按上节改用 tenant 提供者。
3. 确定浏览器和门户都可访问的 HTTPS issuer。代理/网关必须路由 `/oauth2/**`、`/oauth/**`、`/login`、`/logout`、`/.well-known/**` 和 `/api/v1/authorization/**` 到正确服务，且按 alisa-cloud 文档允许协议路径通过网关认证。若使用 `/alisa-gateway` 前缀，需要网关和 servlet 的受信任 forwarded-prefix 设置，确保登录重定向、表单 action 和 Cookie 均保留前缀；不能只放开 API 健康检查。
4. 用真实管理员账号在授权中心登记 `applicationId=portal` 的保密委托应用，回调精确填写 `https://alisa.vip.cpolar.cn/api/auth/callback`，只请求 `portal:console:manage`。保存 clientId/clientSecret 到私有 Secret。多租户模式需完成应用审批。
5. 同一业务登记资源服务器凭据，用于 token 内省。客户端凭据与内省凭据必须分开；内省凭据不授予业务用户权限。
6. 单租户模式用授权管理 API 创建门户管理员角色，grant `{resourceType:"console",resourceId:"*",action:"manage"}`，bind `{subjectType:"USER",subjectId:"实际用户ID"}`。多租户模式使用对应租户 permissions API，且仅向门户实例管理员授予该权限。OAuth 同意页面仍只同意已批准的最小范围。
7. 先验证授权码、内省、权限判断和撤销行为，再启用门户配置。

## 门户服务端配置

```dotenv
ALISA_OAUTH_ISSUER=https://alisa.vip.cpolar.cn/admin/auth
ALISA_PORTAL_ORIGIN=https://alisa.vip.cpolar.cn
ALISA_OAUTH_CLIENT_ID=<保密客户端 ID>
ALISA_OAUTH_CLIENT_SECRET=<客户端密钥>
ALISA_OAUTH_RESOURCE_CLIENT_ID=<资源服务器 ID>
ALISA_OAUTH_RESOURCE_CLIENT_SECRET=<内省密钥>
ALISA_OAUTH_TENANT_ID=<Alisa Admin 中指定的租户 ID>
ALISA_OAUTH_PERMISSION_URL=https://alisa.vip.cpolar.cn/admin/api/v1/authorization/check
ALISA_OAUTH_ALLOW_API_TOKEN=false
```

默认协议地址由 issuer 加上述协议路径生成。如果实际网关使用不同路径，可单独设置 `ALISA_OAUTH_AUTHORIZATION_URL`、`ALISA_OAUTH_TOKEN_URL`、`ALISA_OAUTH_INTROSPECTION_URL`、`ALISA_OAUTH_PERMISSION_URL` 和 `ALISA_OAUTH_REVOCATION_URL`。所有地址要求 HTTPS，不跳过证书验证，不跟随后端协议请求的重定向。自签 CA 通过 Go 的 `SSL_CERT_FILE` 或系统信任注入。HTTP 仅允许显式 `ALISA_OAUTH_ALLOW_LOOPBACK=true` 的本机开发。

Kubernetes 从既有 `portal-auth` Secret 导入配置（Argo 忽略 /data，不将私密值提交 Git）。Secret 环境变量在 Pod 启动时加载，修改后通过发布流程重建 Pod。Compose 显式读取 .env 中对应字段；直接运行 Go 进程时需 export，不会自动加载 .env。

未设置 issuer 时沿用旧管理员令牌模式。设置部分 OAuth 凭据却没有 issuer、或 issuer 存在但凭据不全时拒绝启动。启用 OAuth 后默认禁用旧 API_TOKEN 通道；只有显式 `ALISA_OAUTH_ALLOW_API_TOKEN=true` 才保留紧急管理通道，应受控使用。前端仅在服务端允许时显示令牌表单。

## 发布与验收

源码 PR、已验证镜像、发布 PR 和用户评审依次进行；不以修改源码冒充线上已启用。先在测试环境连接真实 Alisa Cloud 验证：允许的管理员登录成功、普通成员可以登录但管理 API 返回 403，错误租户、错误业务和后台 token 被拒绝、撤权立即生效、Cookie 写操作无 CSRF 被拒绝、注销后原会话不可使用、授权服务中断不影响访客目录。再注入生产配置并发布。

本地自动测试使用独立协议测试服务器验证真实 HTTP 授权码交换、PKCE、内省和 PDP 请求，不连接生产数据库。它不能替代真实授权中心、MySQL、反向代理和浏览器 Cookie 联调。

配套 alisa-cloud 分支 `feature/portal-oauth` 提供 `/auth/` HTTPRoute、受信任前缀处理、路径前缀页面修复及默认关闭的 scope 配置；见该仓库 `PORTAL_AUTH.md`。创建账号与授予门户权限是独立步骤，账号创建不表示 OAuth 已启用。

## 本地连接真实 alisa-user（无需模拟服务）

本地联调用真实 Spring Authorization Server、MyBatis 和授权引擎，数据库是只绑定本机的独立 H2 文件库。不会使用或修改线上 MySQL。`deploy/local/alisa-account.json` 保存用户明确创建的 alisa 账号凭据；启动时仅在全新本地库中建立同名开发账号，不在重启时重置密码或已有数据。凭据、H2 文件、客户端密钥、JWT 签名密钥均在 Git 忽略的 `deploy/local` 下，目录 0700 / 私密文件 0600。

先在包含配套修改的 alisa-cloud 工作树运行 `mvn -pl alisa-user -am test`，然后在门户仓库打开独立终端：

```sh
# 终端 1：真实用户服务，按实际工作树路径填写
python3 scripts/run-alisa-cloud-local.py --cloud-root ../alisa-cloud
# 服务固定监听 http://127.0.0.1:9091

# 首次启动后：通过真实接口登记本地门户角色、保密客户端与内省客户端
python3 scripts/configure-alisa-cloud-local.py

# 终端 2：Go 门户 API，自动读取本地私有配置
python3 scripts/run-api-local.py
# 终端 3：Next.js
cd frontend && npm run dev
```

打开 `http://127.0.0.1:3000`，点击登录后将到 `http://127.0.0.1:9091/login` 输入 alisa 账号密码。身份验证、同意记录、PKCE code/token、内省及权限判断均由真实 alisa-user 处理。已同意的范围可以由授权中心复用，不保证每次都重复显示确认页。当前授权中心使用单独的本地签名密钥和持久化库，属于真实代码的本地实例，不是生产授权服务已经启用。

验证命令 `python3 scripts/check-alisa-cloud-local.py` 会完成真实 HTTP 登录/同意/回调/管理请求/注销，并临时撤销、恢复**本地** portal-admin 的 alisa 绑定，确认只有登录而没有业务权限时不能进入门户。脚本固定请求 loopback 地址，不用于生产。运行时不要同时手动修改这个本地测试角色。

协议参考：[PKCE RFC 7636](https://www.rfc-editor.org/rfc/rfc7636)、[Token Introspection RFC 7662](https://www.rfc-editor.org/rfc/rfc7662)。这次联调验证了相关流程，不等同于完整 OAuth 安全审计或 OIDC 认证。
