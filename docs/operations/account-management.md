# 账号管理启动与维护

对应 [已确认方案](../requirements/account-management-plan.md)。实现状态与自验结果见 [交付记录](../verification/account-management.md)；用户手工验收单独记录。

## 本地启动

工具链沿用仓库固定版本；另外需要可运行的 Docker Compose。以下命令从仓库根目录执行：

```bash
make install
pnpm setup:local
pnpm infrapro account bootstrap
pnpm dev
```

`setup:local` 首次生成权限为 `0600` 的根目录 `.env`，包含独立随机数据库密码、Redis 密码和 32 字节 CSRF 密钥；已有文件不覆盖。它启动 `infrapro-local` 的 PostgreSQL/Redis 持久化卷并执行迁移，不创建默认账号。`bootstrap` 在交互终端询问用户名、显示名称、两次密码，密码不回显、不接受参数或管道。密码支持 8–128 个 Unicode 码点，保留首尾空格；拒绝常见弱口令。

- 控制台：[http://127.0.0.1:5173/](http://127.0.0.1:5173/)
- 进程存活：[http://127.0.0.1:8080/healthz](http://127.0.0.1:8080/healthz)
- 依赖就绪：[http://127.0.0.1:8080/readyz](http://127.0.0.1:8080/readyz)

先使用初始化账号登录。前端统一使用 console；不要混用 localhost 与 127.0.0.1。同浏览器标签页共用会话，退出当前会话会使其他标签页在下一次请求时要求登录。`Ctrl+C` 停止应用进程；数据库和 Redis 留在运行状态。需要停止本地依赖时使用：

```bash
docker compose --env-file .env -f deploy/compose/local.yaml stop
```

`pnpm dev`、`pnpm preview`、`pnpm infrapro` 自动读取根目录 `.env`，已设置的进程环境变量优先；Go 二进制本身不读取 `.env`。正式运行时由进程管理器或 Compose 注入配置。构建产物为 `build/bin/api`、`build/bin/infrapro` 及`apps/console/dist/`。

## HTTP 部署

```bash
cp deploy/compose/.env.example deploy/compose/.env
```

编辑该文件：`PUBLIC_ORIGIN` 填实际完整 HTTP Origin（无末尾斜杠，例如 `http://192.168.1.100:8088`）；`HTTP_PORT` 与端口一致；`SUPPORT_CONTACT` 填实际维护联系方式。分别生成独立的 `POSTGRES_PASSWORD`、`REDIS_PASSWORD`、`CSRF_AUTH_KEY`，建议前两项用 `openssl rand -hex 24`，CSRF 用 `openssl rand -hex 32`。保存后限制文件权限为 `0600`。不要把这些值放到 VITE 配置、版本库或前端。

```bash
docker compose --env-file deploy/compose/.env -f deploy/compose/compose.yaml up -d --build --wait
docker compose --env-file deploy/compose/.env -f deploy/compose/compose.yaml run --rm ops account bootstrap
```

访问 `PUBLIC_ORIGIN/console/`。旧 `/screen`、`/screen/` 转到 console 首页，`/screen/login` 转到 console 登录页，其他旧 screen 子路径固定转到 console 首页。迁移由使用 CLI 镜像的一次性 `migrate` 服务执行，API 不自行迁移或回退。统一运维 CLI 在独立 `ops` 容器中按需运行；API 镜像只包含 HTTP 服务。唯一前端以 `/console/` 为 Vite base；Nginx 支持深层路由刷新，API 错误保持 JSON。数据库和 Redis 只在内部网络开放，使用独立持久化卷；Redis 开启 AOF、容量限制和 `noeviction`。API 仅信任专用子网内的代理，Nginx 覆写来源 IP，避免客户端伪造限流来源。

默认专用子网为 `172.30.42.0/24`。若与服务器已有网络冲突，同时修改 Compose 的子网与 `INFRAPRO_TRUSTED_PROXIES`；不要把信任代理设置成任意来源。

本轮明确采用 HTTP：会话 Cookie 为 `infrapro_session`，`HttpOnly`、`SameSite=Lax`、`Path=/`、无 Domain、无 Secure；CSRF Cookie 同样适配 HTTP。没有 HSTS 或 HTTPS 强制跳转。HTTP 不提供传输加密，这是已确认方案的交付边界。邀请与重置凭证放在 URL fragment，页面读取后立即移除，仅保存在内存；刷新后需重新打开原链接。页面不加载第三方分析脚本，API 与 Nginx 不记录 Cookie、请求体或原始 URL。

Nginx 的访问日志保持关闭，并在当前 `server` 配置中将 `error_log` 写入 `/dev/null`，阻止上游连接失败、超时等请求错误附带原始 URL。该设置使用 [Nginx 的 `error_log` 配置](https://nginx.org/en/docs/ngx_core_module.html#error_log)。进程启动和配置错误仍由主配置记录；单个代理请求的错误详情不再保留，排障使用 API 的结构化日志、请求 ID 和 `/healthz`、`/readyz` 探针。不要临时将这个站点的请求错误日志恢复到标准错误输出，否则查询参数可能进入容器日志。

## 配置

优先级为默认值 < `INFRAPRO_CONFIG` 指定的 YAML < 环境变量。未知 YAML 字段、错误时长、错误 Origin、未列入可信 Origin 的 console 地址、缺少密钥或依赖会明确报错。配置文件示例见 `configs/infrapro.example.yaml`。

| 环境变量 | 默认值 / 要求 |
| --- | --- |
| `INFRAPRO_DATABASE_URL` | 必填，PostgreSQL URL |
| `INFRAPRO_REDIS_URL` | 必填，带凭据的 Redis URL |
| `INFRAPRO_REDIS_KEY_PREFIX` | `infrapro:dev:identity:v1:`；每个环境独立，禁止通配符 |
| `INFRAPRO_CSRF_AUTH_KEY` | 必填，64 位十六进制随机值，多进程一致且持久保存 |
| `INFRAPRO_PUBLIC_CONSOLE_URL` | 本地 `http://127.0.0.1:5173/`；生产包含 `/console/`；用于生成邀请与重置链接 |
| `INFRAPRO_TRUSTED_ORIGINS` | 本地默认 `http://127.0.0.1:5173`；多个可信 Origin 用逗号分隔；精确匹配协议、主机、端口，不含路径 |
| `INFRAPRO_TRUSTED_PROXIES` | 默认不信任任何代理；生产填专用代理地址或 CIDR |
| `INFRAPRO_SUPPORT_CONTACT` | 必填，显示在忘记密码页 |
| `INFRAPRO_COOKIE_SECURE` | 当前仅接受 `false` |
| `INFRAPRO_SESSION_IDLE_TIMEOUT` | `2h`，只由实际用户活动续期 |
| `INFRAPRO_SESSION_ABSOLUTE_TIMEOUT` | `12h`，任何活动都不延长 |
| `INFRAPRO_SESSION_CACHE_TTL` | `5m`，同时受剩余空闲和绝对期限约束 |
| `INFRAPRO_INVITATION_TTL` / `INFRAPRO_PASSWORD_RESET_TTL` | `24h` / `30m` |
| `INFRAPRO_INVITATION_RATE` / `INFRAPRO_INVITATION_LIMIT` | 每分钟创建/重新生成最多 `5` 次；有效邀请最多 `20` 条 |
| `INFRAPRO_IP_REQUEST_LIMIT` | 同路由范围每 IP 每分钟 `120` 次 |
| `INFRAPRO_CREDENTIAL_REQUEST_LIMIT` | 同用户名、账号或凭证每分钟 `20` 次 |
| `INFRAPRO_PASSWORD_FAILURES` | `5` 次失败开始冷却，30 秒指数递增，最长 15 分钟 |
| `INFRAPRO_AUDIT_RETENTION` | `2160h`（90 天） |

业务限流使用 Redis 原子脚本；连续密码失败与冷却另存 PostgreSQL，重启不会消除。成功验证密码清除连续失败。Argon2id 当前为 64 MiB、3 次迭代、2 lanes；单进程最多同时执行 4 个密码任务。目标服务器上线前可运行下述基准，根据目标容量评估响应时间；不降低安全基线。

```bash
go test ./packages/go/platform/identity -run '^$' -bench BenchmarkPasswordHash -benchmem
```

## 旧双应用配置迁移

前端已合并为 console，5174 不再提供服务。本次不修改数据库，也不重建账号。已有 `.env` 或 YAML 不会被初始化脚本覆盖：移除 `INFRAPRO_PUBLIC_SCREEN_URL` / `public_screen_url`；从 `INFRAPRO_TRUSTED_ORIGINS` 移除不再使用的 5174 Origin；清理 `SCREEN_PORT` 与 `VITE_OTHER_APP_URL`，保留现有数据库、Redis、CSRF 密钥和 console 地址。旧公开 screen 配置键暂时接受读取并提示弃用，值被忽略；其他未知 YAML 字段仍报错。旧本地 5174 书签请改为 5173；HTTP 部署由 Nginx 兼容上述旧路径。API 的 account-help 响应已移除 `screen_url`，应同时发布前后端。

## 项目统一运维命令

`infrapro` 位于独立的 `tools/infrapro` Go module，面向整个项目运维；本节的 `account` 是账号命令组，`database` 负责项目数据库迁移。`pnpm infrapro --help` 查看总入口，命令组帮助不要求数据库可用。配置与迁移由 API 和 CLI 共用的 `packages/go/platform` 提供。数据库迁移命令只需要数据库连接配置。

本地以 `pnpm infrapro` 为前缀；构建后可直接使用 `build/bin/infrapro`；Compose 使用上述 `run --rm ops` 前缀。维护人员依靠服务器权限操作，不存在网页管理员角色。

```bash
pnpm infrapro database migrate
pnpm infrapro access sync
pnpm infrapro access check
pnpm infrapro account bootstrap
pnpm infrapro account find --username alice
pnpm infrapro account reset-link --username alice --reason '已通过组织内人员信息核验本人身份'
pnpm infrapro account disable --username alice --reason '人员离岗'
pnpm infrapro account enable --username alice --reason '恢复工作资格'
pnpm infrapro account audit
pnpm infrapro account audit --cursor 100
```

- 初始化标记是数据库单行持久记录，不以账号数量判断。事务提交前中断可重试；提交后再执行会拒绝，不能重开初始化来找回账号。
- 发放重置链接前核验本人身份，不能仅凭知道用户名。部署方应指定维护人员、核验方式和交付渠道。链接只在交互终端展示，禁止重定向到日志或文件。发新链接撤销旧链接，但不立即改密或退出设备。
- 重置成功会撤销所有会话及未用邀请。可为停用账号发新链接；重置后仍停用，需单独 enable。
- 常规 disable 拒绝停用最后一个正常账号。确认唯一账号失陷时用 `account disable --username alice --reason '说明已确认的失陷情况' --emergency`，随后核验、reset-link、本人设新密码、enable。恢复不会复活旧会话或链接。
- Redis 故障时停用、恢复、备份后撤销仍可写入 PostgreSQL。命令会提示缓存清理待重试，数据库撤销已经生效；API 后台通过持久化清理队列重试。
- 全量审计只能在维护端读取；网页只显示当前账号的记录。`audit` 每页最多 100 条，返回 `next_cursor`。审计不含密码、Cookie 或原始一次性凭证。

## 故障与恢复

`/healthz` 只表示进程存活；`/readyz` 同时检查 PostgreSQL、Redis 和迁移版本。账号 HTTP 接口在数据库、Redis 或限流写入失败时返回 JSON 503，前端显示服务暂不可用，不把网络故障解释成退出登录。

Redis 缓存键不存在属于正常情况：会话从 PostgreSQL 回源，按原到期时间回填。每次命中缓存仍查询数据库中的撤销、账号状态和认证版本。旧缓存、旧 AOF、并发回填都不能复活撤销会话。后台活动每 30 秒合并落库；失败时停止接受活动续期，恢复成功后继续。活动只由用户交互发送，健康检查与后台读取不续期。

后台任务每批最多清理 500 条过期会话、邀请、重置及审计，按配置保留期清理；连续失败计数在最后活动一天后且冷却结束才回收。初始化、账号本体和密码摘要不被清理。缓存统计日志包括命中、未命中、失败及清理重试。应用收到停止信号后取消任务并进行有时限的活动落库。

## 备份与恢复

备份须同时覆盖账号、密码摘要、初始化标记、审计及所有相关表。保护备份与服务器密钥，按部署方要求加密保存并定期演练。示例（从仓库根目录）：

```bash
docker compose --env-file deploy/compose/.env -f deploy/compose/compose.yaml exec -T postgres pg_dump -U infrapro -d infrapro -Fc > infrapro-backup.dump
```

恢复旧备份时先停止 API 和网页流量，在隔离或确认的目标数据库恢复完整备份，再执行迁移检查。恢复会回到备份时的业务数据状态，需要部署方确认选定的恢复点。恢复完成后、重新开放流量前执行：

```bash
docker compose --env-file deploy/compose/.env -f deploy/compose/compose.yaml run --rm --no-deps ops database migrate
docker compose --env-file deploy/compose/.env -f deploy/compose/compose.yaml run --rm --no-deps ops access sync
docker compose --env-file deploy/compose/.env -f deploy/compose/compose.yaml run --rm --no-deps ops access check
docker compose --env-file deploy/compose/.env -f deploy/compose/compose.yaml run --rm --no-deps ops account revoke-access --reason '从已验证备份恢复，统一撤销历史访问能力'
```

该命令在事务中增加全部账号认证版本，撤销所有会话和未用邀请/重置链接，记录审计，再清除当前环境前缀下的会话和活动缓存。缓存清理未完成时先恢复 Redis 并重试。不要使用 `FLUSHALL`、删除正式数据卷或清空初始化表。用户重新登录；丢失密码仍走维护重置流程。

## 自动验证与手动验收

```bash
pnpm check
pnpm build
pnpm test:identity
pnpm test:e2e
pnpm test:e2e:preview
pnpm test:http
```

`test:identity` 和 E2E 使用 `deploy/compose/test.yaml`：数据库名必须以 `_test` 结尾，每次创建独立随机 schema 和 Redis 前缀，只清理自己创建的对象；不执行全库或全缓存清空。`test:http` 使用独立 `infrapro-http-tests` 栈，在本机 LAN IPv4 的 18088 端口验证非安全上下文 HTTP，完成后删除该测试栈。可设置 `INFRAPRO_HTTP_TEST_HOST` 指定本机 LAN IPv4。它不接入正式数据卷。

用户可按方案 AT-01–AT-32 逐项验收。最短演示路线：初始化 A → 登录 → 创建邀请并在另一浏览器创建 B → B 再生成邀请 → 修改资料 → 两浏览器会话撤销 → 改密 → 维护重置 → 停用与恢复 → 多标签页刷新和退出。预期为身份与资源始终按本人隔离、一次性链接只消费一次、撤销立即生效、HTTP 深层路径可刷新。自验通过不等同于用户验收通过。

角色权限上线后，受邀账号默认仅有基础成员权限，邀请与管理操作需显式授权。全新数据部署和恢复流程以[角色权限运行手册](role-permission.md)为准。
