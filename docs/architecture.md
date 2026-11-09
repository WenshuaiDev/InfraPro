# 工程目录和依赖边界

## 当前范围

Go workspace 包含四个独立 module：HTTP API、项目统一运维 CLI、项目共享 platform 和通用 foundation；前端保留一个 React 应用与五个 TS 共享包。当前业务功能为账号管理，账号、会话、邀请和审计持久化到 PostgreSQL，Redis 提供共享缓存与限流。工程目录基线中的演示内容已移除。

```text
InfraPro/
├── apps/
│   └── console/                  唯一前端，Vite + TanStack 文件路由
├── services/
│   └── api/
│       ├── cmd/api/              可执行程序入口
│       ├── internal/
│       │   ├── bootstrap/        HTTP 监听、后台任务与优雅关闭
│       │   └── transport/http/   Gin 路由、身份适配、中间件与运行探针
│       ├── go.mod
│       └── go.sum
├── tools/infrapro/            项目统一运维 CLI，独立 Go module
│   ├── cmd/infrapro/          进程入口
│   └── internal/command/        根命令与 database、account 命令组
├── packages/
│   ├── go/
│   │   ├── foundation/          通用 config、logging，独立 Go module
│   │   └── platform/            项目共享能力，独立 Go module
│   │       ├── config/          项目配置与按命令读取的数据库配置
│   │       ├── identity/        账号规则、事务、密码与会话缓存
│   │       ├── migrations/      嵌入式 Goose 项目数据库迁移
│   │       ├── runtime/         数据库、Redis 与业务装配
│   │       └── testsupport/     仅测试使用的隔离环境
│   └── ts/
│       ├── ui/                   shadcn 组件、通用组合、主题与 Recharts 图表
│       ├── layouts/              Admin/Auth/Settings 布局与 Zustand 布局状态
│       ├── api-client/           Fetch 客户端、生成类型、可选 React 适配入口
│       ├── tsconfig/             统一严格 TS 配置
│       └── eslint-config/        统一 ESLint 配置
├── contracts/openapi/            账号接口与运行探针的契约源
├── configs/                      项目共用 YAML 配置示例
├── deploy/{docker,compose,nginx}/ HTTP 镜像、持久化依赖与代理部署
├── scripts/                      workspace 检查、进程管理、类型生成
├── tests/e2e/                    浏览器与代理链路验证
├── docs/
├── go.work
├── package.json
├── pnpm-workspace.yaml
├── pnpm-lock.yaml
└── Makefile
```

Go 在需要时生成 `go.work.sum`；它不同于每个 module 的 `go.sum`，不手工创建占位校验文件。

## Go workspace

模块路径与当前 Git 仓库地址一致：

- `github.com/WenshuaiDev/InfraPro/services/api`
- `github.com/WenshuaiDev/InfraPro/tools/infrapro`
- `github.com/WenshuaiDev/InfraPro/packages/go/platform`
- `github.com/WenshuaiDev/InfraPro/packages/go/foundation`

根目录不创建 `go.mod`，`go.work` 是工作区模块清单。`scripts/go.mjs` 读取该清单，逐模块执行下载、检查、测试与构建，覆盖嵌套 module。

API 和 CLI 的 `go.mod` 保留到 platform、foundation 的仓库内相对 `replace`，platform 保留到 foundation 的相对 `replace`。模块级下载、`go mod tidy` 和 `GOWORK=off` 均能解析尚未独立发布的共享模块；没有机器绝对路径。标准开发和构建仍以根 `go.work` 为入口。API 与 CLI 可以分别构建，彼此没有模块依赖。

服务和 CLI 内部代码通过各自 `internal` 隔离。共享项目规则放在 platform，通用工具放在 foundation；共享模块不得反向导入 API 或 CLI。首期只有一个 HTTP API 服务，CLI 是按需执行的运维程序。

## pnpm 共享包与单应用

`apps/*` 和 `packages/ts/*` 构成工作区。每个应用有独立 `package.json`、Vite 配置、入口、环境变量、TypeScript 项目和构建产物。共享包使用 `workspace:*` 和显式 `exports` 引用。

共享 TS 包以源码形式交给 Vite 编译，开发时支持热更新。UI 和 layout 先在 packages 内抽象，console 只装配业务。`ui` 管理 shadcn 基础组件、通用组合、主题和 Recharts；`layouts` 组合 UI 并通过 Zustand 持有非敏感布局偏好，接收品牌、菜单、用户展示和链接渲染函数。两者不请求业务 API、不依赖具体 Router。

`api-client` 主入口不导入 React，React 使用方通过 `@infrapro/api-client/react` 接入 `IdentityProvider` 和 `useIdentity`，管理当前身份、CSRF、依赖故障和用户活动。TanStack Query 管理邀请、会话和安全记录；React Hook Form + Zod 管理表单。退出/切换账号时卸载原账号子树，取消并清理该账号的查询和页面状态。

应用内部：`app` 装配 Router、Query 和身份；`routes` 定义文件路由；`features` 包含页面与业务交互；`config` 定义品牌和真实导航；`styles/global.css` 装载 Tailwind 并显式扫描共享包；`testing` 是与产品路由隔离的 UI 验证入口。`routeTree.gen.ts` 自动生成，不手工编辑。应用仅通过包的公开 exports 引用共享源码，ESLint 检查依赖方向。用法见[前端开发约定](frontend-development.md)。

## 账号运行链路

```text
浏览器 console :5173 ── 同源账号 API → Vite 代理 → Gin :8080
                                                      ↓
                    Cookie / CSRF / 身份与归属校验 → identity
                                                        ↓
                                           PostgreSQL + Redis
```

部署探针留在 HTTP 接入层：`GET /healthz` 表示进程存活；`GET /readyz` 检查 PostgreSQL、Redis 与迁移状态。成功时仅返回 `{"status":"ok"}`，不再提供服务版本与响应时间的演示数据。它们设置 `Cache-Control: no-store` 并附带 `X-Request-ID`；不作为前端页面内容。

OpenAPI 契约生成 `api-client/src/schema.d.ts`，`pnpm contracts:check` 检查是否同步。客户端统一处理账号请求、CSRF 和业务错误。未登录请求按公开白名单处理；鉴权通过后的未知路由与不支持的方法分别返回 JSON 404、405。

## 扩展方式

- 新增 Go 服务：创建 `services/<name>/go.mod`、`cmd/<name>`、`internal`，执行 `go work use ./services/<name>`；共享依赖按现有 API 的方式声明。
- 新增运维能力：在 `tools/infrapro/internal/command` 增加命令组或子命令，调用对应 platform 业务包；项目统一使用 `infrapro`，不在 API 或各业务模块另建 CLI。
- 新增 Go 共享模块：放在 `packages/go/<name>` 并加入 `go.work`。
- 当前新增业务先作为 console 的 feature 和独立路由；只有确需独立发布/部署时再拆 `apps/<name>`，继续复用 UI 和布局。
- 新增 TS 共享包：放在 `packages/ts/<name>`，声明包名、exports、实际依赖与类型检查命令。
- 新增业务接口：先更新 `contracts/openapi/api.yaml`，更新服务和客户端，再运行 `pnpm contracts:generate` 与相关检查。
- Go 单元测试放在源码旁；跨应用验收放在 `tests/e2e`。

`make dev` 当前启动 API 和 console 两个进程；新增入口时同步修改启动编排和对应 E2E 配置。`make build` 与 Go 检查脚本按工作区清单遍历，pnpm 应用构建按 `apps/*` 遍历。

## 账号业务边界

`packages/go/platform/identity` 包含业务 service、集中 SQL repository、Argon2id 和 Redis cache，不导入 Gin。数据访问与事务只在 repository，HTTP 和统一 CLI 共用 service。`services/api/internal/transport/http/identity` 负责 HTTP 适配、Cookie 与请求响应；传输层采用精确公开白名单，所有其他路由默认鉴权，写请求检查完整 Origin 与 CSRF。

platform/config 组合 foundation 的基础配置，提供同一项目配置来源。platform/runtime 装配 pgxpool、Redis 和账号业务，API 的 bootstrap 负责可取消后台任务与 HTTP 生命周期。platform/migrations 的 Goose SQL 由统一 CLI 执行，API 仅检查迁移版本。会话每次鉴权关联 PostgreSQL 当前状态，缓存不能恢复撤销访问。维护操作的缓存失效记录持久保存在队列表。

浏览器账号页面统一在 console 内实现，旧 screen 入口只保留 Nginx 固定跳转。共享 UI 不访问 API；OpenAPI 是类型契约源。部署使用 Nginx 单主机路径；测试使用专用 PostgreSQL schema 与 Redis 命名空间，不连接正式环境。操作说明见 [账号运行手册](operations/account-management.md)。
