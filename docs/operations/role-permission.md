# 角色权限部署与维护

本功能按全新数据初始化。当前发布只有一种账号类型；授权始终经过权限 → 角色 → 账号。源码目录位于 `packages/go/platform/access/catalog.go`，HTTP 不提供写目录接口。

## 本地启动

2026-11-09 已停止旧本地开发进程，并仅重建 `infrapro-local_postgres_data`、`infrapro-local_redis_data` 两个项目专用卷；迁移、目录同步和一致性检查已完成。`.env` 保留，未清理其他项目或测试环境的数据。新库没有默认账号或默认密码。

在项目根目录的交互终端中执行：

```bash
pnpm infrapro account bootstrap
pnpm dev
```

bootstrap 提示输入用户名、显示名称和密码，密码不回显；账号、密码、管理员角色绑定、初始化标记和审计同事务提交。完成后再次执行 bootstrap 会拒绝。若开发服务已启动，无需重复执行 `pnpm dev`。

入口为 <http://127.0.0.1:5173/>，健康检查为 <http://127.0.0.1:8080/readyz>。初始化账号获得 22 项明确权限；受邀账号只获得基础成员的 6 项本人权限。

新建其他本地环境：

```bash
pnpm setup:local
pnpm infrapro access check
pnpm infrapro account bootstrap
pnpm dev
```

`setup:local` 只创建缺失配置、启动依赖、迁移并同步目录；它不会清空已有库。不要用它作为数据删除命令。

## Docker 全新部署

使用 `deploy/compose/compose.yaml`，在该目录的 `.env` 配置密码、CSRF 密钥、HTTP 地址和维护联系方式。账号规则及 Cookie/CSRF 沿用[账号运行手册](account-management.md)。

```bash
cd deploy/compose
cp .env.example .env
# 编辑 .env，设置此环境自己的配置

docker compose --env-file .env build
# 顺序：数据库 → 迁移 → access-sync → API → web
# access-sync 使用与 API 同一源码构建的 CLI，API 启动不写授权数据
docker compose --env-file .env up -d --wait

docker compose --env-file .env run --rm ops access check
docker compose --env-file .env run --rm ops account bootstrap
```

页面位于所配置 HTTP 地址的 `/console/`。`/readyz` 检查结构版本、全部目录字段、内置角色固定属性和关系、数据库及 Redis；不一致返回 503，API 启动也会拒绝。不得通过关闭校验或添加账号特权绕过故障。

**首次替换旧版账号数据**：先停止此部署的 API 和后台任务，明确确认本部署的 Compose 项目、专用数据库及 Redis。只有当 PostgreSQL、Redis 均为此 Compose 文件管理的专用命名卷且数据确定不保留时，在该部署目录执行：

```bash
docker compose --env-file .env down --volumes
# 然后按上述全新部署步骤重新创建
```

这会删除该 Compose 项目命名卷的数据。外接数据库、共享 Redis 或多个项目共用的数据源不适用此命令；只重建已确定属于 InfraPro 的目标 schema，并按配置前缀扫描删除 Redis 键，不能执行共享实例的 FLUSHALL 或删除其他 schema。

## 发布、检查与生成

权限新增或退役在 Go 目录维护，内置基础成员默认权限在 `builtins.go` 显式指定。退役项保留目录定义，设置 `lifecycle=retired`，不复用原编码。自定义角色和现有账号绑定不会被同步覆盖。

```bash
pnpm permissions:generate
pnpm check
pnpm build
# 停止旧 API 和所有写入任务，部署同版本 API 与 CLI
pnpm infrapro database migrate
pnpm infrapro access sync
pnpm infrapro access check
```

同步幂等：完全一致时不增加版本或审计；变化时目录、内置角色、邀请资格撤销、修订号及审计原子提交。新权限不会自动加入自定义角色。旧实例因校验值不匹配不能继续服务，不采用混合版本滚动写入。

如果当前完整管理能力只来自自定义角色，新增权限可能令同步返回 `last_platform_manager`。先明确给一个正常账号绑定当前内置管理员角色，再重新同步；不扩大全部自定义角色。

## 恢复管理能力

```bash
pnpm infrapro access recover-admin --username <已有用户名> --reason '已核验维护原因'
pnpm infrapro account enable --username <已有用户名> --reason '凭据恢复完毕'
```

`recover-admin` 仅添加真实的内置管理员角色绑定，不创建账号、不改密码、不启用停用账号、不重开初始化、不恢复历史会话或链接。执行者依靠主机维护权限访问数据库，审计明确标注 maintenance；它不是应用账号，也没有对应 HTTP 绕过入口。

确认账号失陷的紧急处置可保留原命令：

```bash
pnpm infrapro account disable --username <用户名> --emergency --reason '已确认失陷'
pnpm infrapro account reset-link --username <用户名> --reason '已核验本人身份'
```

应急停用允许暂时失去全部管理账号；后续必须显式恢复角色、凭据及账号状态。网页和常规 CLI 停用继续保护最后正常账号与最后完整管理账号；当前租户管理员还必须先交接其全部租户。应急影响及恢复规则见[租户运行手册](tenant-foundation.md)。管理员密码也不能通过网页代改。

## 日常操作和故障

- 先创建空角色，再配置权限，再给账号绑定角色。账号角色可以清空；停用角色不贡献权限但保留绑定。删除角色要求全部解绑，包括停用账号的绑定。
- 管理写入必须填写原因，并在最近 5 分钟内完成密码复验。复验后由用户再次提交，不自动重放请求。
- 409 `version_conflict`：重新读取目标、比较差异后提交；不覆盖别人更新。`role_in_use`：先解绑；`last_platform_manager`：先配置另一个正常账号的完整管理权限。
- 403 `grant_scope_exceeded`：目标角色或账号的有效配置超出操作者范围，即使目标角色停用也不能跳过。
- 403 `permission_denied`：前端刷新身份权限；保留登录状态。权限减少会销毁页面查询、草稿和邀请明文，旧响应不会恢复内容。无权限账号仍可登录、获取帮助并退出。
- 每次受保护数据库操作在授权共享锁内重新计算权限；授权变更取排他锁。普通角色变化不撤销会话，下次请求即时使用新权限。前端显示最长约 30 秒更新，也会在焦点、可见性或响应修订变化时刷新。
- 失去创建邀请权限会永久撤销该账号全部未使用邀请。恢复权限不会复活链接；移除重叠角色但仍保留创建权限则不撤销。
- 请求结果因断网不确定时，读取当前版本并在平台审计核对原因、请求 ID 和前后差异，然后决定是否重新提交。
- PostgreSQL、Redis 或目录故障均不降级放行。成功审计失败则业务回滚；越权拒绝写入限频安全日志。日志及审计不记录密码、Cookie、邀请或重置链接明文。

## 人工验收

1. CLI 初始化 A，登录后打开权限目录，看到 22 项只读能力及两个不可修改的内置角色。
2. A 邀请 B；B 登录可使用本人资料、安全功能，直达 `/access/accounts` 和 `/invitations` 显示 403。
3. A 创建“邀请办理”角色，勾选查看本人邀请、创建基础成员邀请，填写原因保存；在账号管理给 B 绑定该角色。
4. B 不重新登录，切回页面或等待刷新即可创建邀请。A 停用角色后，B 原 Cookie 调用邀请接口立即被拒绝，页面中的邀请明文清除。
5. A 恢复角色，B 获得能力，但之前未使用的链接仍无效。
6. 两个页面编辑同一角色，后保存的旧版本返回冲突。尝试删除仍绑定的角色时按钮说明需先解绑。
7. 尝试移除唯一完整管理账号的全部角色或停用该账号，返回保护提示，原管理能力保持。
8. 查看平台审计的操作者、目标、原因和前后差异；使用手机宽度及键盘操作分组复选框，部分选中状态清晰。

更多自动证据和仍待用户验收的范围见[验证记录](../verification/role-permission.md)。
