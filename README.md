# InfraPro

Go + Gin、React + TypeScript + Vite 工程骨架。根目录同时管理 Go workspace 和 pnpm workspace。

工程骨架：**已验收**（用户于 2026-11-09 确认通过）。已提供一个真实 API 健康检查、两个独立应用、共享组件与客户端、开发代理和自动检查。业务模块、数据库及生产部署在后续迭代中实现。

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
make dev
```

无需复制配置文件或启动数据库即可运行。访问：

| 入口 | 默认地址 |
| --- | --- |
| 管理端 console | http://127.0.0.1:5173 |
| 展示端 screen | http://127.0.0.1:5174 |
| API 健康检查 | http://127.0.0.1:8080/api/v1/health |
| 进程存活检查 | http://127.0.0.1:8080/healthz |

两个应用均应显示“服务连接正常”、`infrapro-api`、版本和最近响应时间。按 `Ctrl+C` 停止本次命令启动的全部进程。端口冲突会报错并停止本轮启动的进程，不自动切换端口。

React/共享 TS 源码支持 Vite 热更新。Go 源码修改后重新执行启动命令；当前未引入 Go 热重载工具。

单独运行时使用三个终端：

```bash
# 终端 A：API
make dev-api

# 终端 B：管理端
pnpm dev:console

# 终端 C：展示端
pnpm dev:screen
```

## 命令

| 命令 | 行为 |
| --- | --- |
| `make install` | 按 pnpm 锁文件安装依赖，并逐 Go module 下载依赖 |
| `make dev` | 构建并运行 API，启动两个 Vite 应用 |
| `make build` | 构建全部 Go module、服务可执行文件与两个应用 |
| `make check` | gofmt、Go vet、ESLint、TypeScript、OpenAPI 类型同步检查 |
| `make test-go` | 对 go.work 中每个 module 执行带 race 检测的 Go 测试 |
| `make test-e2e` | 自动启动独立测试环境，运行 Chromium 端到端测试 |
| `make test` | Go 测试 + 浏览器端到端测试 |
| `make preview` | 启动 API，并通过 Vite preview 预览已构建的两个应用 |
| `make fmt` | 格式化工作区内的 Go 源文件 |
| `pnpm contracts:generate` | 根据 OpenAPI 重新生成客户端类型 |
| `pnpm test:e2e:preview` | 对已构建应用运行同一套浏览器测试 |

首次运行浏览器测试前安装浏览器：

```bash
pnpm exec playwright install chromium
make check
make build
make test
pnpm test:e2e:preview
```

E2E 使用独立端口：API `18080`、console `15173`、screen `15174`；不复用已有服务，结束时关闭自己启动的进程。失败时查看 `playwright-report/` 与 `test-results/` 中的报告、截图和 trace。测试会注入 HTTP 503 和无效 JSON，验证页面不会把失败响应显示成健康状态。

单独构建应用：

```bash
pnpm --filter @infrapro/console build
pnpm --filter @infrapro/screen build
```

产物：`build/bin/api`、`apps/console/dist/`、`apps/screen/dist/`。`make preview` 使用默认的 8080/5173/5174 端口，与开发服务互斥。Vite preview 用于本地验收构建产物；正式托管需要配置静态服务与 `/api` 反向代理。

## 配置

API 优先级：默认值 < `INFRAPRO_CONFIG` 指定的 YAML < 环境变量。

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `INFRAPRO_CONFIG` | 空 | 可选 YAML 文件路径，相对启动工作目录 |
| `INFRAPRO_HTTP_ADDR` | `127.0.0.1:8080` | API 监听地址 |
| `INFRAPRO_LOG_LEVEL` | `info` | 结构化日志等级 |
| `INFRAPRO_SHUTDOWN_TIMEOUT` | `5s` | 优雅关闭超时 |

配置示例为 `services/api/configs/config.example.yaml`。需要本地 YAML 时复制为 `config.local.yaml`，并显式设置 `INFRAPRO_CONFIG`。未知配置字段会直接报错。

每个应用的 `.env.example` 列出默认值；可复制成该应用目录下的 `.env.local` 后修改，并重启 Vite。API 不自动读取应用的 `.env.local`。

| 应用配置 | 作用 |
| --- | --- |
| `CONSOLE_PORT` / `SCREEN_PORT` | 各应用的监听端口 |
| `API_PROXY_TARGET` | Vite 转发 `/api` 请求的服务地址，仅在开发/本地预览进程使用 |
| `VITE_API_BASE_URL` | 浏览器中的 API 前缀，默认 `/api/v1` |
| `VITE_OTHER_APP_URL` | 右上角另一应用的地址 |

例如切换 API 端口，同时让两个应用代理到新地址：

```bash
INFRAPRO_HTTP_ADDR=127.0.0.1:9090 API_PROXY_TARGET=http://127.0.0.1:9090 make dev
```

更改应用端口时同步修改另一应用的 `VITE_OTHER_APP_URL`。`VITE_*` 会进入浏览器构建产物，仅用于公开配置。

## 工程结构

- [目录和依赖边界](docs/architecture.md)

Go 工作区机制参考 [Go 官方文档](https://go.dev/doc/tutorial/workspaces)，应用依赖使用 [pnpm workspace 协议](https://pnpm.io/workspaces)，浏览器环境编排使用 [Playwright webServer](https://playwright.dev/docs/test-webserver)。
