# InfraPro 技术选型

基线：2026-11-12。本文记录当前工程采用的技术、用途、选择理由及约束。业务范围见[软件需求规格说明书](requirements.md)，模块协作及运行方式见[架构设计](architecture.md)。这次整理不升级依赖，也不把待开发的监测数据技术方案纳入当前运行栈。

## 1 选型原则

- 复用单 API、单 console、统一 CLI 与现有工作区，按业务边界划分包；只有独立部署需求明确后才拆服务或应用。
- PostgreSQL 保存业务与授权真相；数据库约束和事务承担完整性，缓存及前端状态不能替代最终校验。
- 权限、接口和字段语义保持明确的定义源与生成流程，避免服务、维护程序和前端分别解释同一规则。
- 通用 UI 和布局先在共享包实现，应用负责业务组合；沿用已有可访问性、主题和交互体系。
- 依赖版本按仓库锁定，不因在线文档或上游版本变化自动升级。这里只整理本仓库的选择，不作新的第三方产品推荐。

## 2 语言与工作区工具链

| 技术 | 当前基线 | 使用位置与方式 | 选择理由／约束 |
| --- | --- | --- | --- |
| Go | 1.27.0 | `services/api`、`tools/infrapro`、`packages/go/platform`、`packages/go/foundation` | API 与 CLI 复用相同业务事务；四个独立 module 由 `go.work` 统一管理。 |
| Node.js | 26.10.0 | 前端开发、构建及 `scripts/*.mjs` | 单一工具版本支持 workspace 编排；不作为当前业务 API 运行时。 |
| pnpm | 12.18.0 | 根 `package.json`、`pnpm-workspace.yaml`、锁文件 | `workspace:*` 管理 TS 共享包，锁定依赖并按包执行检查。 |
| TypeScript | 5.9.3 | console、UI、布局、API client 及工具脚本类型检查 | 严格类型和生成契约；保持现有 ESLint／生成器兼容范围。 |
| Make | 系统工具 | 根 `Makefile` | 提供 install、dev、build、check、test 等统一入口，调用实际工作区脚本。 |

版本来源为 [.go-version](../.go-version)、[.node-version](../.node-version)、[go.work](../go.work)、[package.json](../package.json) 及各模块清单；精确传递依赖由 `go.sum` 和 `pnpm-lock.yaml` 固定。

## 3 后端、存储与安全

| 技术 | 当前基线 | 使用位置与方式 | 选择理由／约束 |
| --- | --- | --- | --- |
| Gin | 1.12.0 | `services/api/internal/transport/http` | 路由、中间件、严格 DTO 解码及错误映射；不把 Gin 带入共享业务包或 CLI。 |
| PostgreSQL | 17，Compose 使用 `postgres:17-alpine` | 全部业务、版本、授权、历史、审计及幂等收据 | 事务、行锁、组合外键、唯一索引和延迟约束保护跨对象一致性。 |
| pgx / pgxpool | v5.12.0 | platform 的 repository／queries 与 runtime 连接池 | 显式参数化 SQL 和事务，便于审查范围过滤、锁顺序及批量投影；不引入 ORM。 |
| Goose + Go embed | v3.29.0 | `platform/migrations`；`infrapro database migrate` | 编号 SQL 随 CLI 发布，使用会话锁协调迁移；API 只检查版本。 |
| Redis / go-redis | Redis 7.4；客户端 v9.23.0 | identity 会话快照、活动合并及各入口限流 | 多进程共享 TTL 和原子计数；命中缓存仍复验数据库。AOF、容量上限及 `noeviction` 由 Compose 配置。 |
| PostgreSQL `tstzrange` / GiST / `btree_gist` | PostgreSQL 17 扩展 | v11 的安装、输出和来源期间约束 | 半开时间区间及 UUID 等值组合排斥，阻止同对象期间重叠；扩展安装属于迁移前提。 |
| Argon2id / `x/crypto` | 依赖按 Go 清单；当前参数 64 MiB、3 次、2 lanes | `platform/identity` 的密码摘要 | 慢摘要、随机盐、可升级参数；一次性高熵令牌单独使用 SHA-256 摘要。 |
| gorilla/csrf + Origin 校验 | v1.7.3 | HTTP 边界与 api-client CSRF 流程 | Cookie 会话下保护写请求；精确匹配可信 Origin，密码复验不触发自动重放。 |
| Go `flag` / `x/term` | 随 CLI 模块锁定 | `tools/infrapro/internal/command` | 统一命令分组与密码无回显输入；无需另建命令框架或功能占位入口。 |
| JSON Schema 2020-12 | Go `jsonschema/v6` v6.0.3 | 工程元模型编译、属性和样例校验 | 只编译内部有限模型；显式格式校验，禁止任意脚本、外部引用及动态远程加载。 |

实际版本见 [API Go 清单](../services/api/go.mod)、[platform Go 清单](../packages/go/platform/go.mod)、[CLI Go 清单](../tools/infrapro/go.mod) 和 [Compose](../deploy/compose/compose.yaml)。容器镜像标签表示当前主版本系列，不声称固定到某个补丁版本或镜像摘要。

## 4 前端与共享交互

| 技术 | 当前基线 | 使用位置与方式 | 选择理由／约束 |
| --- | --- | --- | --- |
| React / React DOM | 19.3.1 | 唯一应用 `apps/console` | 页面、表单及状态组合；其他展示场景先使用独立路由和布局。 |
| Vite | 8.3.6 | console 开发、生产构建和本地 preview | TS 共享包源码编译及热更新；部署 base 与 Router basepath 一致为 `/console/`。 |
| TanStack Router | 1.168.22 | 文件路由、工作空间边界、详情深链接 | 应用负责导航意图与合法回跳；生成路由文件不手工修改。 |
| TanStack Query | 5.99.0 | 服务端查询、游标分页、身份／范围修订隔离 | 传递 AbortSignal，聚焦和授权变化复核；避免把接口数据重复存进客户端 store。 |
| TanStack Table | 8.21.3 | 共享 DataTable 与业务列定义 | 紧凑表格、列显隐、稳定行身份；服务端分页，不虚构总数。 |
| React Hook Form / Zod | 7.72.1 / 4.3.6 | feature 表单和共享字段控件 | 表单草稿、字段错误与统一输入校验；Zod 使用 `jitless` 入口兼容 CSP，后端仍最终校验。 |
| shadcn/ui / Tailwind CSS | 源码组件；Tailwind 4.2.2 | `packages/ts/ui`、共享样式和主题 | 沿用 shadcn-admin 的视觉与组合方式，先抽象再使用；只保留一套基础组件与主题。 |
| Radix UI / Base UI | Radix 各包锁定；`@base-ui/react` 1.9.0 | Dialog、Sheet、菜单、标签及 ResponsivePanel；工作空间类型 Select | 保留成熟焦点／键盘语义。Drawer 使用 Base UI，其余组件按现有实现；不因单个组件调整迁移整套 UI。 |
| Zustand | 5.0.12 | `packages/ts/layouts` 桌面侧栏偏好 | 已实际接入非敏感布局状态，明确持久化白名单；不保存密码、会话或业务 API 数据。 |
| Recharts | 3.8.1 | `@infrapro/ui/charts` 的基础图表与 SeriesChart | 统一主题、响应尺寸、图例和可访问数据表；不承担指标含义、单位换算或监测查询。 |
| Lucide / Sonner | 1.8.0 / 2.0.7 | 图标及结果提示 | 与组件主题一致；字段错误仍保留在表单中。 |
| Inter Variable | 本地 `@fontsource-variable/inter` 5.3.0 | UI 字体样式 | 字体同源托管，不依赖生产外部 CDN。 |

版本来源为 [console 清单](../apps/console/package.json)、[UI 清单](../packages/ts/ui/package.json)、[布局清单](../packages/ts/layouts/package.json) 和 [pnpm 锁文件](../pnpm-lock.yaml)。shadcn-admin 参考提交、改造范围与许可证见[第三方来源](../third-party/README.md)，不把上游示例数据和页面作为产品功能。

## 5 契约、验证与交付工具

| 技术／工具 | 使用位置 | 用途与边界 |
| --- | --- | --- |
| OpenAPI 3.1 / openapi-typescript 7.13.0 | `contracts/openapi/api.yaml` → `api-client/src/schema.d.ts` | 唯一 HTTP 类型契约；`contracts:generate/check` 生成及核对，不手工维护另一套类型。 |
| Go 权限目录生成 | access → TS 常量／OpenAPI 权限枚举 | `permissions:generate/check`；运行时以明确 Scope 和动作复验，不信任客户端权限快照。 |
| 共享语义夹具 | `tests/fixtures/engineering-values.json`、`contracts/fixtures/device-semantics.json` | Go／SQL／TypeScript 对照数值、Unicode、换行及七项测量语义；禁止默认单位转换。 |
| Go test / race / vet / gofmt | 源码旁测试、`scripts/go.mjs` | 真实数据库约束、事务、并发和独立模块验证。 |
| ESLint / TypeScript | 根配置与共享配置包 | 依赖方向、公开导出及类型检查。 |
| Playwright | 1.65.0；`tests/e2e`、`tests/http` | Chromium 操作和 HTTP／Nginx／CLI 回归；不代表已覆盖 Safari、真机或所有辅助技术。 |
| Docker Compose / Nginx | `deploy` | 独立依赖、迁移／目录同步顺序、静态应用与同源 API 代理；不在本次引入集群调度。 |

## 6 已明确的技术边界

1. 当前认证属于 InfraPro，本项目不接入跨系统身份平台、OIDC、SSO 或新的 MFA 流程。
2. 当前交付为 HTTP 同源部署，Cookie 的 `Secure=false`；HTTPS、证书和传输加密升级是另行需求，不能只改 Cookie 选项而声称已支持。
3. 工程位置使用受校验的结构化字段；当前未采用 PostGIS、地图引擎或坐标转换库。桩号及局部坐标只是业务资料，不能推导虚构 GIS 结果。
4. 权限和来源变更使用 PostgreSQL 短事务及共享协调锁，不引入分布式锁、跨库授权同步或隐式最终一致放行。
5. 模板和产品采用不可变版本、独立启停和固定依赖；没有审批引擎、任意公式解释器或在线协议执行器。
6. 监测数据的存储、导入、趋势、统计及集群方案继续以[原总体方案](requirements/monitoring-data-foundation-plan.md)和[原详细方案](requirements/monitoring-data-development-plan.md)为准。两份文档保持原样，本文不替它们重新选型。

## 7 选型变更规则

技术替换或版本升级应先说明业务原因、影响模块、数据／接口兼容与验收，再同步清单、锁文件和对应设计。保留当前可复用接口；未出现实际需求时不预建空应用、空命令、空数据表或未启用的权限。实际构建、测试、迁移和用户验收分别记录，依赖已安装不等同于功能已交付。
