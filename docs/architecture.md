# 工程目录和依赖边界

## 本轮范围

提供两个 Go module、两个 React 应用、四个 TS 共享包和可运行的健康检查链路。业务模块随需求加入。本轮不连接数据库、不产生模拟业务数据。

```text
InfraPro/
├── apps/
│   ├── console/                  管理入口，独立 Vite 项目
│   └── screen/                   展示入口，独立 Vite 项目
├── services/
│   └── api/
│       ├── cmd/api/              可执行程序入口
│       ├── internal/
│       │   ├── bootstrap/        配置装配、监听、优雅关闭
│       │   ├── transport/http/   Gin 路由、中间件
│       │   └── modules/health/   真实进程健康检查
│       ├── configs/              YAML 配置示例
│       ├── migrations/           本服务未来的数据迁移
│       ├── go.mod
│       └── go.sum
├── packages/
│   ├── go/foundation/            config、logging，独立 Go module
│   └── ts/
│       ├── ui/                   共享 React 展示组件与主题
│       ├── api-client/           Fetch 客户端、生成类型、可选 React 适配入口
│       ├── tsconfig/             统一严格 TS 配置
│       └── eslint-config/        统一 ESLint 配置
├── contracts/openapi/            健康检查接口唯一契约源
├── deploy/{docker,compose}/      后续部署文件位置，本轮为说明文档
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
- `github.com/WenshuaiDev/InfraPro/packages/go/foundation`

根目录不创建 `go.mod`，`go.work` 是工作区模块清单。`scripts/go.mjs` 读取该清单，逐模块执行下载、检查、测试与构建，覆盖嵌套 module。

API 的 `go.mod` 保留到 foundation 的仓库内相对 `replace`，用于让 `go mod tidy`、模块级下载和 `GOWORK=off` 的仓库内检查也能解析尚未独立发布的共享模块；没有机器绝对路径。标准开发和构建仍以根 `go.work` 为入口。后续独立发布共享 module 时再改成正式版本依赖。

服务内部代码通过 `internal` 隔离；不同服务通过接口通信或共享库复用。首期只有一个 API 服务，未来是否拆分服务按独立运行与部署需求决定。

## pnpm 多应用

`apps/*` 和 `packages/ts/*` 构成工作区。每个应用有独立 `package.json`、Vite 配置、入口、环境变量、TypeScript 项目和构建产物。共享包使用 `workspace:*` 和显式 `exports` 引用。

共享 TS 包以源码形式交给 Vite 编译，开发时支持热更新。`ui` 不依赖 API 客户端；组件通过 props 接收状态。`api-client` 主入口不导入 React，React 使用方通过 `@infrapro/api-client/react` 选择健康检查 hook；该 hook 负责请求取消、加载状态、错误和重试。

应用之间不跨目录引用源码。每个应用内部：`app` 装配入口，`pages` 组织页面，`features` 包含业务交互，`components` 放本应用复用组件，`assets` 和 `public` 分别放需打包与原样提供的资源，`styles` 放应用样式。当前单页入口未引入额外路由库。

## 最小运行链路

```text
浏览器 console :5173 ─┐
                      ├─ 同源 /api/v1/health → Vite 代理 → Gin :8080
浏览器 screen  :5174 ─┘                                  ↓
                    共享客户端校验响应 ← status/service/version/time
                              ↓
                    共享 UI 显示成功、加载或失败
```

`GET /healthz` 与 `GET /api/v1/health` 返回当前进程状态，设置 `Cache-Control: no-store`，并附带 `X-Request-ID`。健康状态不代表数据库或外部依赖已就绪。错误路由与不支持的方法分别返回 JSON 404、405。

OpenAPI 契约生成 `api-client/src/schema.d.ts`，`pnpm contracts:check` 检查是否同步。客户端还校验响应关键字段，避免错误地址返回的 HTML 或不完整 JSON 被当作健康状态。

## 扩展方式

- 新增 Go 服务：创建 `services/<name>/go.mod`、`cmd/<name>`、`internal`，执行 `go work use ./services/<name>`；共享依赖按现有 API 的方式声明。
- 新增 Go 共享模块：放在 `packages/go/<name>` 并加入 `go.work`。
- 新增应用：参照已有应用放入 `apps/<name>`，设置唯一包名、端口和独立配置；pnpm glob 自动纳入。
- 新增 TS 共享包：放在 `packages/ts/<name>`，声明包名、exports、实际依赖与类型检查命令。
- 新增业务接口：先更新 `contracts/openapi/api.yaml`，更新服务和客户端，再运行 `pnpm contracts:generate` 与相关检查。
- Go 单元测试放在源码旁；跨应用验收放在 `tests/e2e`。

`make dev` 当前启动 API、console、screen 三个入口；新增入口时同步修改启动编排和对应 E2E 配置。`make build` 与 Go 检查脚本按工作区清单遍历，pnpm 应用构建按 `apps/*` 遍历。
