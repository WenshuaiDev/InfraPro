# 工程目录和依赖边界

## 当前范围

Go workspace 包含四个独立 module：HTTP API、项目统一运维 CLI、项目共享 platform 和通用 foundation；前端保留一个 React 应用与五个 TS 共享包。当前业务功能覆盖账号、租户、项目管理、工程定义编制及租户工程资源；业务数据持久化到 PostgreSQL，Redis 提供共享缓存与限流。工程目录基线中的演示内容已移除。

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
│   │       ├── access/          权限目录、明确范围策略与授权状态
│   │       ├── tenancy/         租户资料、成员、邀请及生命周期
│   │       ├── projects/        项目、成员、角色、管理交接和归档事务
│   │       ├── engineering/     元模型、草稿、不可变版本、依赖、校验与样例预览
│   │       ├── resources/       设施、构件、测点、业务通道、历史与幂等收据
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
├── contracts/openapi/            账号、租户、项目、工程定义、实际资源与运行探针契约源
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


## 租户基础治理

`platform/tenancy` 负责档案、编号归属、初始管理员和交接；通过 `access.Actor` 的可信会话复验与 `RequireScopedTx` 在同一事务重新授权，不导入 identity 或 Gin。HTTP tenancy 适配器从 identity 建立 Actor，调用完整业务操作，不拼接多个独立写事务。`access` 按明确租户读取成员和角色；identity 的共享启停事务调用 access 的管理员保护及治理事件写入，依赖方向不成环。

授权状态锁 → 相关账号按 UUID 排序 → 会话复验 → 租户及成员 → 编号及审计。创建、交接、账号启停使用排他授权锁；资料修改使用共享授权锁。租户成员和角色变化只影响对应租户；平台角色不产生租户成员关系。前端以 URL、账号、租户与成员关系划分查询，传递 AbortSignal，页面卸载废弃写请求完成回调；确认失权清理内容，短暂故障保留草稿并阻止提交。


## 项目业务与上下层联动

`platform/projects` 拥有完整项目命令：在同一事务内完成范围检查、资格复验、版本与预览校验、成员和角色更新、编号保留、幂等收据及审计。`platform/access` 提供项目目录、策略、真实授权解释和上层保护/级联辅助；`tenancy`、`identity` 调用这些辅助处理父关系结束及应急停用。依赖保持为 projects/tenancy/identity → access，无反向依赖，HTTP 只解码各操作的严格输入并调用服务。

项目写入先取排他授权状态锁（包括资料 PATCH，避免写入审计/修订时由共享锁升级），读取取共享锁；随后按账号 UUID、会话、租户成员、项目、项目成员/角色顺序校验。上层操作、目录同步采用同一锁顺序。权限真相是 PostgreSQL 当前关系和角色，不依赖浏览器按钮或 Redis TTL。

项目成员绑定具体 tenant_membership；项目角色只绑定具体 project_membership。租户角色清空不结束关系，也不影响独立项目授权；父关系结束会在原事务中结束全部旧项目关系并撤销绑定。项目管理员不可通过普通角色分配增删。停用账号允许应急不可用；不可用租户管理员的明确修复交接可产生带原因的管理空缺，随后由新租户管理员逐项目修复。

前端区分个人 `/projects`、租户治理 `/t/$tenantId/projects` 与业务 `/t/$tenantId/p/$projectId`。查询按账号、租户成员关系、项目成员关系及授权状态划分；客户端代次校验、AbortSignal 和卸载回调保护共同防止旧结果回填。短暂故障保留草稿且停止提交，真实失权清理内容；取消确认框不会执行成功回调。非危险只读请求仅在范围取消导致 AbortError 且仍被观察时重试一次，写入从不自动重放。

项目完整事件在 `account_security_events.project_id` 下隔离；租户治理只保存允许的摘要，平台审计排除项目内容，包括目录发布的逐成员影响。账号历史清理跳过项目事件。迁移 v7 新增项目实体、永久编号归属、成员、绑定和操作收据，并以外键、唯一性和延迟约束保护完整管理结构。运行与验收见[项目管理手册](operations/project-management.md)。

## 工程定义与通用预览

`platform/engineering` 定义四种元模型职责：设施类型、指标、单位、监测模板。具体桥梁、边坡、路基路面是内容数据。依赖保持 engineering → access；HTTP 从 identity 建立可信 Actor，执行严格 JSON 解码，调用单个业务事务。接口输入不得指定作用范围、发布者或版本号。

锁顺序为共享授权状态 → 当前账号 → 会话及当前范围权限复验 → 工程目录状态 → 定义。发布、复制、创建和状态维护使用排他工程目录锁；保存草稿使用共享目录锁与独立定义行锁。发布版本、编译 Schema、单位类别快照、固定依赖与内容摘要不可变，启停状态单表维护；成功业务结果、幂等收据与审计一次提交。单位类别被引用后不可改义，删除草稿保留身份编号。

JSON Schema 2020-12 由内部有限字段模型生成，Go 使用 `santhosh-tekuri/jsonschema/v6` v6.0.3 显式执行 format 校验并关闭外部资源加载。编译缓存仅缓存内容摘要对应的规则，最多 1024 项；权限和完整依赖链可用性每次查询当前数据库。前端 React Hook Form 保存编制表单，Zod 使用无动态代码生成入口，通用解释器绘制属性、样例树和待绑定要求。共享语义样例位于 `tests/fixtures/engineering-values.json`。

公共版本和租户私有模板分别授权。租户公共目录不暴露草稿和不可新采用版本；已授权历史模板只解析其固定公共依赖，不开放全局历史搜索。复制来源摘要仅供追溯，实际依赖决定新采用资格。平台审计明确排除租户私有工程事件。前端查询按账号、租户、成员关系隔离，写入不自动重放；临时样例无持久化。实际资源通过下述 resources 模块接入固定版本，不创建独立监测方案或运行模块空壳。

迁移 v8 和操作入口见[设施类型定义手册](operations/infrastructure-type-definition.md)。

## 租户工程资源

`platform/resources` 拥有实际设施、构件树、测点和业务通道；`engineering` 继续拥有定义。资源服务通过 `engineering.ResolvePublicTx` 在当前事务解析公共发布版本及其固定依赖，不复制定义解析 SQL。定义影响接口注入 `UsageReader`，仅返回“存在资源引用”的布尔值，不暴露租户、对象名称、ID 或数量。

资源锁顺序为授权状态 → 当前账号/会话及明确 Scope 复验 → 工程目录共享锁 → 设施行锁。租户资源普通维护及既有项目对象更新取共享授权锁；项目创建和授权治理从事务开始取得排他授权锁，避免新增授权时升级锁。构件结构变更另验 `structure_revision`；同请求以账号、租户和请求 ID 去重，业务变更、独立历史、审计摘要和收据一次提交。四类资源分别建表，以组合外键、唯一编号和触发器约束同租户同设施引用、允许关系、循环及最大正常数量。删除保留身份和编号；历史不随账号安全事件清理。

HTTP 使用显式资源策略、严格字段与 256 KiB 请求限制；read/manage 为租户权限，采用新类型或新指标另需目录读取权。React 页面复用 `engineering-fields` 控件与字段校验，位置使用共享 Go/Zod 语义样例；查询按账号、租户和成员关系隔离。构件树按直接子项分页，表单写入不自动重放，响应不确定时由用户查询收据。平台审计在筛选和游标前排除真实资源事件。

运行、迁移 v9 及验收见[工程资源手册](operations/engineering-resources.md)。


## 租户统一资源与项目授权

迁移 v10 保留四类主表与 UUID，新增项目资源政策、设施关联、逐对象授权、独立授权历史及 v2 收据。旧项目默认零关联、零授权且禁止新建设施；创建来源是不可变追溯事实，不参与授予。组合外键、当前唯一索引、能力类型约束与延迟触发器保护关联和资源身份。

资源服务入口显式接收 tenant/project Scope；HTTP 从匹配路由选 Scope，项目请求不回退租户范围。项目角色与资源能力相交，祖先摘要不提供档案、历史或维护权。列表、搜索、计数及历史先以 SQL 可见集合过滤再分页；定义与祖先集合批量解析，项目只获得可见对象的投影。租户授权治理还需项目查看与授权查看权限，不要求加入目标项目。

授权指令采用 add/update/end、预览摘要及 access/link 修订；“当前全部”在设施锁内展开并限制 1000 项，未来新增不继承。项目新对象及其标准能力、来源、两类历史和收据原子提交。共享编辑、结构修改及生命周期操作需要原因和影响确认。归档保留读取，禁止写入与扩权；有效或归档项目引用都阻止删除和项目方破坏性操作。

资源历史与授权历史独立持久保存，项目历史隐藏其他项目人员、身份、原因和历史不可见祖先。通用审计仅存最小摘要，平台审计排除资源/授权事件。v1 租户收据保持原指纹；v2 绑定入口、原成员和关联，提交重放仍检查当前执行资格，只读核对不重新执行。

[重构方案](requirements/project-resource-management-refactor-plan.md)、[自验记录](verification/project-resource-management.md)、[运行及验收](operations/project-resource-management.md)。
