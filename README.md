# InfraPro

Go + Gin、React + TypeScript + Vite 项目；前端使用 shadcn/ui、TanStack Router/Query/Table、Zustand 与 Recharts。根目录同时管理 Go workspace 和 pnpm workspace，当前业务功能为账号管理。

工程目录基线已于 2026-11-09 验收，其运行状态演示页面与配套示例现已移除。当前保留账号管理、PostgreSQL、Redis 与 HTTP 部署；前端单应用重构见下方工程文档。功能自验及用户验收分别记录在 [交付记录](docs/verification/account-management.md)。

## 工具链

| 工具 | 本工程固定/验证版本 | 配置位置 |
| --- | --- | --- |
| Go | 1.27.0 | `.go-version`、`go.work`、各 `go.mod` |
| Node.js | 26.10.0 | `.node-version` |
| pnpm | 12.18.0 | 根 `package.json` 的 `packageManager` |
| Gin | 1.12.0 | `services/api/go.mod` |
| React | 19.3.1 | 应用 `package.json` |
| Vite | 8.3.6 | 应用 `package.json` |
| TypeScript | 5.9.3 | 各 TS 包 `package.json` |

准备好 Go、Node、pnpm 和 Make。首次依赖安装需要访问软件包源。TypeScript 版本与 ESLint、OpenAPI 生成器的 peer dependency 范围保持兼容。

## 快速启动

在仓库根目录执行：

```bash
make install
pnpm setup:local
pnpm infrapro account bootstrap
make dev
```

`setup:local` 生成本机随机配置并启动持久化数据库与 Redis；首次通过交互命令创建账号，没有默认密码。访问：

| 入口 | 默认地址 |
| --- | --- |
| 管理端 console | http://127.0.0.1:5173 |
| 进程存活检查 | http://127.0.0.1:8080/healthz |
| 依赖就绪检查 | http://127.0.0.1:8080/readyz |

当前只保留 console 一个前端应用。登录后显示当前账号信息，并提供个人资料、账号安全及邀请管理入口。按 `Ctrl+C` 停止本次命令启动的全部应用进程；数据库和 Redis 持续运行。端口冲突会报错并停止本轮启动的进程，不自动切换端口。

React/共享 TS 源码支持 Vite 热更新。Go 源码修改后重新执行启动命令；当前未引入 Go 热重载工具。

单独运行时使用两个终端：

```bash
# 终端 A：API
make dev-api

# 终端 B：管理端
pnpm dev:console
```

## 命令

| 命令 | 行为 |
| --- | --- |
| `make install` | 按 pnpm 锁文件安装依赖，并逐 Go module 下载依赖 |
| `make dev` | 构建并运行 API，启动 console Vite 应用 |
| `make build` | 构建全部 Go module、服务可执行文件与 console 应用 |
| `make check` | gofmt、Go vet、ESLint、TypeScript、OpenAPI 类型同步检查 |
| `make test-go` | 自动准备专用 PostgreSQL/Redis，执行带 race 检测的 Go 测试 |
| `pnpm infrapro --help` | 项目统一运维入口，查看数据库、账号等命令组 |
| `pnpm test:http` | 完整 Nginx/Compose 的普通 HTTP 与维护 CLI 验收 |
| `make test-e2e` | 自动启动独立测试环境，运行 Chromium 端到端测试 |
| `make test` | Go 测试 + 浏览器端到端测试 |
| `make preview` | 启动 API，并通过 Vite preview 预览已构建的 console 应用 |
| `make fmt` | 格式化工作区内的 Go 源文件 |
| `pnpm contracts:generate` | 根据 OpenAPI 重新生成客户端类型 |
| `pnpm test:ui` | 共享组件、布局、主题、Zustand 和图表的三视口浏览器检查 |
| `pnpm test:e2e:preview` | 对已构建应用运行同一套浏览器测试 |

首次运行浏览器测试前安装浏览器：

```bash
pnpm exec playwright install chromium
make check
make build
make test
pnpm test:e2e:preview
```

E2E 使用独立端口：API `18080`、console `15173`；不复用已有服务，结束时关闭自己启动的进程。失败时查看 `playwright-report/` 与 `test-results/` 中的报告、截图和 trace。测试覆盖账号流程、多个标签页的共享登录、依赖故障提示及恢复。

单独构建应用：

```bash
pnpm --filter @infrapro/console build
```

产物：`build/bin/api`、`build/bin/infrapro`、`apps/console/dist/`。`make preview` 使用默认的 8080/5173 端口，与开发服务互斥。正式 HTTP 托管使用提供的 Nginx/Compose，详见 [账号运行手册](docs/operations/account-management.md)。

## 配置

API 优先级：默认值 < `INFRAPRO_CONFIG` 指定的 YAML < 环境变量。

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `INFRAPRO_CONFIG` | 空 | 可选 YAML 文件路径，相对启动工作目录 |
| `INFRAPRO_HTTP_ADDR` | `127.0.0.1:8080` | API 监听地址 |
| `INFRAPRO_LOG_LEVEL` | `info` | 结构化日志等级 |
| `INFRAPRO_SHUTDOWN_TIMEOUT` | `5s` | 优雅关闭超时 |

账号服务还必须配置 PostgreSQL、Redis、CSRF 持久密钥与维护联系方式。完整列表、恢复命令和备份步骤见 [账号运行手册](docs/operations/account-management.md)。配置示例为 `configs/infrapro.example.yaml`；需要 YAML 时复制为 `config.local.yaml` 并设置 `INFRAPRO_CONFIG`，未知字段直接报错。

`apps/console/.env.example` 列出默认值；可复制成该应用目录下的 `.env.local` 后修改，并重启 Vite。API 不自动读取应用的 `.env.local`。

| 应用配置 | 作用 |
| --- | --- |
| `CONSOLE_PORT` | console 的监听端口，默认 5173 |
| `API_PROXY_TARGET` | Vite 转发 `/api` 请求的服务地址，仅在开发/本地预览进程使用 |
| `VITE_API_BASE_URL` | 浏览器中的 API 前缀，默认 `/api/v1` |
| `VITE_APP_BASE` | 应用部署基础路径，开发默认 `/`，部署为 `/console/` |

例如切换 API 端口，同时让 console 代理到新地址：

```bash
INFRAPRO_HTTP_ADDR=127.0.0.1:9090 API_PROXY_TARGET=http://127.0.0.1:9090 make dev
```

更改前端端口时同步更新 `INFRAPRO_PUBLIC_CONSOLE_URL` 和 `INFRAPRO_TRUSTED_ORIGINS`。`VITE_*` 会进入浏览器构建产物，仅用于公开配置。

## 工程结构

- [目录和依赖边界](docs/architecture.md)
- [前端共享包与开发约定](docs/frontend-development.md)
- [前端重构与验收记录](docs/verification/frontend-refactor.md)
- [统一运维 CLI](tools/infrapro/README.md)
- [API 与 CLI 模块拆分方案](docs/requirements/backend-organization-plan.md)
- [角色与权限完整方案（已实施，待人工验收）](docs/requirements/role-permission-plan.md)
- [账号与授权业务词汇](GLOSSARY.md)

Go 工作区机制参考 [Go 官方文档](https://go.dev/doc/tutorial/workspaces)，应用依赖使用 [pnpm workspace 协议](https://pnpm.io/workspaces)，浏览器环境编排使用 [Playwright webServer](https://playwright.dev/docs/test-webserver)。
## 角色与权限

角色权限已接入统一 API、CLI 与 console。部署需要迁移 → `access sync` → `access check` → 首次 `account bootstrap`；`pnpm setup:local` 已包含前三步。详见[运行手册](docs/operations/role-permission.md)、[需求方案](docs/requirements/role-permission-plan.md)和[验证记录](docs/verification/role-permission.md)。
