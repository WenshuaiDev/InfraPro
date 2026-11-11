# InfraPro

Go + Gin、React + TypeScript + Vite 项目；前端使用 shadcn/ui、TanStack Router/Query/Table、Zustand 与 Recharts。根目录同时管理 Go workspace 和 pnpm workspace，当前业务功能包括账号、平台角色权限、租户基础管理、成员与角色授权、租户启停恢复、项目管理，以及数据驱动的设施类型定义和监测模板。

工程目录基线已于 2026-11-09 验收，其运行状态演示页面与配套示例现已移除。当前功能包括账号、租户、项目、工程定义及租户工程资源，使用 PostgreSQL、Redis 与 HTTP 部署；前端单应用重构见下方工程文档。最新提交范围和统一复验见 [2026-11-11 PR 交付记录](docs/verification/pr-delivery-20261111.md)，各功能的自验与人工验收仍分别记录。

## 全功能人工验收环境

已按当前功能准备独立、持久化的本机验收账号和业务数据，入口为 <http://localhost:25173/login>。账号密码及精确数据链接保存在本机 `.artifacts/manual-acceptance/账号与验收入口.local.md`；建议先使用 `accept_admin`、`accept_platform`、`accept_project`。无需自行创建账号或录入初始资料。

完整数据清单、27 步人工验收及重启方式见[全功能人工验收手册](docs/operations/manual-acceptance.md)，自验结果见[交付记录](docs/verification/manual-acceptance-data.md)。普通开发库及原有账号保留；状态为 Awaiting acceptance，自验通过，等待用户人工验收。

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

当前只保留 console 一个前端应用。登录后显示当前账号信息，并提供个人资料、账号安全及账号开户邀请入口；首页无业务侧栏，通过左上工作空间切换器进入租户、项目或平台；账号菜单仅位于右上角。按 `Ctrl+C` 停止本次命令启动的全部应用进程；数据库和 Redis 持续运行。端口冲突会报错并停止本轮启动的进程，不自动切换端口。

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
- [租户基础管理首轮方案（已实施，待人工验收）](docs/requirements/tenant-foundation-plan.md)
- [租户成员管理首轮方案（已实施，待人工验收）](docs/requirements/tenant-membership-plan.md)
- [账号与授权业务词汇](GLOSSARY.md)
- [设施实例、构件、测点与业务监测通道开发方案](docs/requirements/engineering-resource-management-plan.md)
- [租户角色与成员授权方案（已实施，待人工验收）](docs/requirements/tenant-role-authorization-plan.md)
- [租户启停与恢复方案（已实施，待人工验收）](docs/requirements/tenant-lifecycle-plan.md)

Go 工作区机制参考 [Go 官方文档](https://go.dev/doc/tutorial/workspaces)，应用依赖使用 [pnpm workspace 协议](https://pnpm.io/workspaces)，浏览器环境编排使用 [Playwright webServer](https://playwright.dev/docs/test-webserver)。
## 角色与权限

角色权限已接入统一 API、CLI 与 console。部署需要迁移 → `access sync` → `access check` → 首次 `account bootstrap`；`pnpm setup:local` 已包含前三步。详见[运行手册](docs/operations/role-permission.md)、[需求方案](docs/requirements/role-permission-plan.md)和[验证记录](docs/verification/role-permission.md)。


## 租户基础管理

平台可创建租户、指定或交接管理员；租户管理员从左上工作空间切换器进入独立工作空间，维护档案并查看本租户操作记录。平台和租户角色分别授权，当前管理员必须先交接全部租户才能常规停用。

已有环境升级：备份并停止旧 API → `pnpm infrapro database migrate` → `pnpm infrapro access sync` → `pnpm infrapro access check` → `make dev`。本机已完成增量升级，保留原有账号，不重复 bootstrap。访问 `/platform/tenants` 或 `/tenants`。

详见[启动、恢复与人工验收手册](docs/operations/tenant-foundation.md)、[50 项自验证据](docs/verification/tenant-foundation.md)和[已确认方案](docs/requirements/tenant-foundation-plan.md)。功能状态为 Awaiting acceptance，人工验收待用户确认。

## 租户成员管理

租户管理员可定向邀请已有账号，由本人登录并明确确认加入；新成员默认只读租户资料，后续可由租户管理员配置角色。支持成员历史、暂停/恢复、移除后以新关系再次加入、邀请撤销/替换、账号与管理员交接联动，以及范围隔离和审计。

本机已完成增量升级并保留现有账号，从仓库根目录 `make dev` 启动即可。成员功能位于左上切换器选择租户后的“成员管理 / 成员邀请”。账号开户邀请仍与成员邀请分别使用。

详见[启动及人工验收步骤](docs/operations/tenant-membership.md)和[52 项验收证据映射](docs/verification/tenant-membership.md)。功能状态为 Awaiting acceptance；未部署生产，人工验收待用户确认。

## 租户角色与成员授权

租户管理员可直接分配预置角色、复制或创建本租户角色，并配置成员多角色或清空授权。该轮统一目录区分 11 项可配置业务权限与 8 项管理员保留能力（本轮项目治理扩展后为 13/9）；变更前预览实际权限及邀请影响，提交后按最新权限生效。权限来源、角色历史和租户审计可查。

该轮本机已从 v4 增量升级到 v5，备份实际恢复核对通过，原账号、密码记录、会话和租户关系保留。运行 `make dev` 后访问 `http://127.0.0.1:5173/`，从工作空间切换器进入对应租户的“租户角色 / 成员管理”；权限目录已整合到角色页面。不重复 bootstrap。

详见[启动、升级和 13 步人工验收](docs/operations/tenant-role-authorization.md)及[验证结果与 57 项证据映射](docs/verification/tenant-role-authorization.md)。状态为 Awaiting acceptance；未部署生产，人工验收待用户确认。

## 租户启停与恢复

平台可预览影响、填写原因后停用或恢复租户。停用关闭租户业务读写，保留平台治理、本人最小状态和原身份历史；其他租户与个人账号功能继续可用。邀请暂时受限但不延期，真实失权及终态不因恢复而复活。

该轮本机已增量升级到 v6（当前已升级到 v8），备份在独立库恢复核对通过，原账号密码、会话及业务关系保留。使用原账号访问 `http://127.0.0.1:5173/platform/tenants`；重启运行 `make dev`，不重复 bootstrap。

详见[运行与 16 步人工验收](docs/operations/tenant-lifecycle.md)及[55 项验证映射与当前状态](docs/verification/tenant-lifecycle.md)。状态为 Awaiting acceptance；人工验收待用户确认，未部署生产。

## 项目基础管理、成员授权与归档恢复

租户有权人员可创建项目并指定负责人及管理员；项目管理员可组织成员、配置多角色、查看权限来源与操作记录，进行管理交接、归档和恢复。创建者、负责人和上层管理员不因此取得项目业务访问。归档保留读取及合法撤权，禁止常规扩权；上层成员结束会同步结束旧项目关系，重新加入不会复活历史授权。

本机已从 v6 增量升级到 v7，完成实际备份恢复核对，原账号、密码和数据保留。使用原账号；仓库根目录运行 `make dev`，访问 <http://127.0.0.1:5173/projects>，该旧链接会打开首页项目选择器；租户治理从左上选择租户后进入“项目治理”。不重复 bootstrap。

[完整方案](docs/requirements/project-management-plan.md) · [启动、升级与 17 步人工验收](docs/operations/project-management.md) · [自验及 80 项证据映射](docs/verification/project-management.md)。状态为 **Awaiting acceptance**，完整自验通过，人工验收待用户确认；未部署生产。

## 设施类型定义与监测模板

平台通过统一元模型编制设施类型、工程属性、构件规则、指标、单位和公共监测模板；租户可独立复制、修改和发布私有模板。支持固定版本依赖、动态样例表单和构件树、历史差异、启停影响、权限隔离与审计。新增设施类型只增加配置数据。

该迭代引入数据库 v8（本机当前已升级 v9）；平台入口 `/platform/engineering/type-definitions`，租户入口从工作空间的工程配置菜单进入。详见[操作、升级与人工验收手册](docs/operations/infrastructure-type-definition.md)、[自验记录](docs/verification/infrastructure-type-definition.md)及[已确认方案](docs/requirements/infrastructure-type-definition-plan.md)。

状态为 **Awaiting acceptance**；该迭代完整浏览器回归 97 项、HTTP 部署检查 7 项全部通过；后续统一复验见本文顶部交付记录，人工验收待用户确认。未部署生产。

## 设施、构件、测点与业务通道

租户通过“工程资源 → 设施管理”直接管理设施、构件树、设施整体或构件测点及业务监测通道。已有定义版本固定引用；最少构件数量仅作参考，最多数量仍校验；位置可留空，坐标基准与真实零值保留。包含退役/恢复、软删除保护、独立变更历史、并发修订与幂等收据。本轮不增加监测方案、完成度、地图、图纸/模型绑定或采集运行功能。

本机数据库已从 v8 升级至 v9，权限目录为 90 项；原账号继续使用，开发入口 [本地 console](http://127.0.0.1:5173/)。工程资源自动检查及人工验收状态见[自验记录](docs/verification/engineering-resources.md)，准备好的隔离验收环境与 13 步操作见[操作手册](docs/operations/engineering-resources.md)。

## 工作空间导航

首页、个人资料与账号安全没有业务侧栏；租户、项目、平台保留各自业务菜单。全站只保留右上账号菜单。左上工作空间切换器统一提供名称 / 编号搜索、类型 / 项目状态筛选、独立分页与重试、本人角色及受限状态查看。首页正文保持不变。

`/tenants`、`/projects` 保留为兼容入口，登录后跳到首页并打开相应选择器；关闭或浏览器后退不会重复弹出。普通登录进入首页，合法业务深链接仍回到原目标。此次改动只涉及前端，无新依赖、后端接口或数据库迁移。

[导航操作与人工验收](docs/operations/workspace-navigation.md) · [实施方案](docs/requirements/workspace-navigation-refactor-plan.md) · [自验证据](docs/verification/workspace-navigation-refactor.md)。功能状态为 **Awaiting acceptance**，人工验收由用户单独确认。
