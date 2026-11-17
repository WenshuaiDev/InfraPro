# InfraPro 技术选型

基线：2026-11-15。本文记录当前工程采用的技术、用途、选择理由及约束。业务范围见[软件需求规格说明书](requirements.md)，模块协作及运行方式见[架构设计](architecture.md)。监测数据组件见第8节，空间可视化实现及固定依赖见[第9节](#spatial-technology)。本机自验不代表生产部署或人工验收。

Go 独立解析服务已经实现，代码、存储和执行边界见[解析服务开发规格](development/parsing-service-and-method-binding-spec.md#engineering)。沿用仓库固定依赖，执行子进程使用 Go 标准库及既有 x/sys 的 Linux 资源／seccomp 能力；平台继续使用 pgx、Goose 和 S3 客户端，未引入协议 DSL 或动态插件。自验与部署状态见[验收报告](verification/parsing-service.md)。设备接入增加 Go ingest-relay、独立 PostgreSQL 持久缓冲、内网 ingest-worker 和 foundation 纯协议／契约包：TLS、HTTP、TCP 分帧使用 Go 标准库，业务采用 pgx 事务与 Goose 增量迁移，原文使用现有私有 S3 客户端，页面复用 React／TypeScript、TanStack Query、表格和遮罩 Drawer。独立云库用于断联期间保管原文与投递状态，不持有业务库或 S3 凭证。运行配置和自验事实见[接入手册](operations/device-ingestion-and-raw-messages.md)与[验证报告](verification/device-ingestion-and-raw-messages.md)。

## 1 选型原则

- 复用单 API、单 console、统一 CLI 与现有工作区，按业务边界划分包；只有独立部署需求明确后才拆服务或应用。
- PostgreSQL 保存业务与授权真相；数据库约束和事务承担完整性，缓存及前端状态不能替代最终校验。
- 权限、接口和字段语义保持明确的定义源与生成流程，避免服务、维护程序和前端分别解释同一规则。
- 通用 UI 和布局先在共享包实现，应用负责业务组合；沿用已有可访问性、主题和交互体系。
- 依赖版本按仓库锁定，不因在线文档或上游版本变化自动升级。这里只整理本仓库的选择，不作新的第三方产品推荐。

## 2 语言与工作区工具链

| 技术 | 当前基线 | 使用位置与方式 | 选择理由／约束 |
| --- | --- | --- | --- |
| Go | 1.27.0 | `services/api`、`services/diagnostics`、`services/parser`、`services/ingest-relay`、`tools/infrapro`、`packages/go/platform`、`packages/go/foundation` | API 与 CLI 复用相同业务事务；七个独立 module 由 `go.work` 统一管理。 |
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
3. 工程位置使用受校验的结构化字段；空间工作台使用 OpenLayers 处理二维地图与明确的 EPSG:4326／3857 转换，未引入 PostGIS。桩号及未知局部坐标不能推导虚构 GIS 结果。
4. 权限和来源变更使用 PostgreSQL 短事务及共享协调锁，不引入分布式锁、跨库授权同步或隐式最终一致放行。
5. 模板和产品采用不可变版本、独立启停和固定依赖；没有审批引擎、任意公式解释器或在线协议执行器。
6. 监测数据的存储、导入、趋势、统计及集群方案以[正式需求规格](requirements.md)和[正式需求规格](requirements.md)为准。实施与用户的本机优先调整记录在原路径，目标规模与部署验证仍未完成。

## 7 选型变更规则

技术替换或版本升级应先说明业务原因、影响模块、数据／接口兼容与验收，再同步清单、锁文件和对应设计。保留当前可复用接口；未出现实际需求时不预建空应用、空命令、空数据表或未启用的权限。实际构建、测试、迁移和用户验收分别记录，依赖已安装不等同于功能已交付。

## 8 监测数据本轮实现

| 技术 | 锁定版本／来源 | 实际用途与边界 |
| --- | --- | --- |
| ClickHouse + Keeper | 26.8.36.3；本机 Compose 锁定镜像 digest | 一个或多个真实节点保存类型化不可变采用载荷，按月分区；PG 清单决定业务可见性，核验全部已配置节点，不靠最终去重。单节点及同机 Keeper 不提供冗余或容灾。 |
| SeaweedFS S3 | 4.53；本机 Compose 锁定镜像 digest | 原文件、不可变摘要、导出和 CH 在线历史磁盘；本地缓存256 MiB／副本。备份使用独立服务与身份，仍在同一主机。 |
| clickhouse-go / minio-go | platform go.mod | CH 原生精确读写、取消及逐副本核验；S3 流式、multipart、条件写。 |
| Excelize / 有界 XML 解析 | v2.11.1 | 工作簿元数据和合成样本，实际行与共享字符串使用有界流／磁盘处理，不执行公式。 |
| Apache Arrow Go | v18.8.0 | 后台 Parquet 2.6 分卷，保留 UTC 纳秒、Int64、Decimal(38,12) 和枚举。 |
| math/big / shopspring decimal | Go 标准库／v1.6.0 | 精确整数矩、38位原值、总体标准差和half-even舍入，大数不经过JS Number。 |
| ECharts Canvas | 6.1.0；console package.json | 按需加载的监测趋势模块、固定视窗、真实极值点、精确tooltip和键盘等价明细。普通图表继续使用共享Recharts。 |

监测原始明细不写入 PostgreSQL；PG 只保存控制元数据、授权、清单、版本和任务。存储许可、兼容性与故障行为见[本机工程环境与人工验收](operations/local-environment.md)，启动和恢复见[维护手册](operations/monitoring-data.md)。镜像及依赖锁定不代表生产容量保证。

<a id="spatial-technology"></a>

## 9 工程空间可视化技术

范围见[SV-01–SV-10](requirements.md#spatial-scope)，数据流与接口见[空间架构](architecture.md#spatial-design)，实际操作见[空间手册](operations/spatial-visualization.md)。

| 技术 | 使用位置与方式 | 选择理由及明确限制 |
| --- | --- | --- |
| React／TypeScript、TanStack、既有 shadcn/ui | console 空间资料、授权及浏览工作台；三视图共享业务对象选择与受控查询。 | 沿用项目现有栈，保留 scope、修订、取消、主题和可访问性；不新增应用或状态库。 |
| OpenLayers | 二维地图／GeoJSON 图层，以及使用独立图面坐标的布设图浏览。 | 同一引擎承担图层、缩放、选中和定位；官方有自定义图片坐标示例，不把图面位置当成经纬度。[官方示例](https://openlayers.org/en/latest/examples/static-image.html) |
| OpenLayers `ol/proj` | EPSG:4326／3857 显示转换及明确坐标对象。 | 只接入本轮声明的转换；其他工程 CRS、垂直基准和供应商偏移不作隐式等价。[投影 API](https://openlayers.org/en/latest/apidoc/module-ol_proj.html) |
| Three.js、GLTFLoader、Raycaster | 设施级 GLB 显示、相机、命中、对象高亮、模型标记和基础剖切。 | 前端只渲染与交互，持久业务身份由版本映射解释；解码器／纹理同源托管。[GLTFLoader](https://threejs.org/docs/pages/GLTFLoader.html)、[Raycaster](https://threejs.org/docs/pages/Raycaster.html) |
| Python＋IfcOpenShell | 独立转换进程使用 IfcOpenShell 几何及属性 API 解析 IFC，生成 GLB、源构件树、属性索引及命中映射；同时制作合成 IFC 资料。 | 几何与语义分别保留、同版封存；未依赖另装的 IfcConvert 可执行文件。[属性 API](https://docs.ifcopenshell.org/autoapi/ifcopenshell/util/element/index.html#ifcopenshell.util.element.get_psets) |
| pypdfium2／PDFium | 在隔离处理进程将 PDF 页渲染成归一化图面及浏览图片。 | 浏览无需获得原 PDF，便于独立原件权限；PDFium 不允许并发多线程调用，采用进程并发。[官方 API](https://pypdfium2.readthedocs.io/en/stable/python_api.html) |
| Go、pgx、Goose、PostgreSQL | 现有 API module 增加 spatial 领域包与独立 worker；元数据、关系、版本、任务、授权和审计采用 PG 事务。 | 保留当前身份和授权规则；本轮不增加 PostGIS，普通包围框候选与地理显示不冒充专业空间分析。 |
| 现有 S3 兼容服务／minio-go | 保存来源及浏览产物；以独立空间命名和清单管理，不复用监测原文件的访问授权。 | 复用实际 SeaweedFS 基础设施，提取有限 BlobStore 适配；受控同源流式读取，不提供长期裸对象地址。 |
| 现有 monitoring／派生 Provider／ECharts | 按获准对象查询原始或派生最新值、趋势和状态；成果由分析模块公开读取。 | ClickHouse继续由监测模块拥有；空间模块不另建序列、计算器或图表统计口径。 |
| Go test、Playwright、既有契约检查 | 格式／坐标／引用与权限集成、真实浏览、撤权竞争、换版与恢复。 | Codex 自制真实文件经过正式处理管线；模拟器只能用于故障注入，不能代替端到端结果。 |

实际固定版本：OpenLayers `10.11.0`、Three.js `0.187.0`，由 console package 与 `pnpm-lock.yaml` 锁定；Python `3.12` 环境使用 IfcOpenShell `0.9.0.post1`、pypdfium2 `5.15.2`、Pillow `13.0.0`、NumPy `2.5.4` 和仅供样本生成的 ReportLab `5.1.0`，完整传递依赖及 SHA-256 保存于 `tools/spatial-converter/requirements.lock`。来源及许可见[第三方清单](../third-party/README.md#空间可视化)。

`deploy/docker/spatial.Dockerfile` 构建同一 API／独立 worker 镜像，采用 Python 3.12 的 Debian trixie 基础环境、非 root 用户、`libgomp1` 和完整哈希安装。该 IfcOpenShell Linux wheel 需要较新的 glibc，bookworm 不满足；构建中执行真实导入检查，转换在独立受限子进程运行。部署时须记录实际镜像摘要，转换依赖以固定哈希校验。

MapLibre 和 web-ifc 是可行替代路线，本轮不同时引入。CesiumJS／3D Tiles 留给完整三维地理场景；它们不是设施局部模型浏览的必备依赖。相关能力及官方来源保留在[正式需求规格](requirements.md)，研究中的候选说明不覆盖本节的规划选择。

**交付不依赖用户提供资料。** 已使用脚本生成 PNG、JPEG、PDF、GLB、IFC、GeoJSON、本地示例瓦片和多版本／异常样本。图纸／模型生成用于开发与演示准备，产品本身不提供建模功能。样本标识为合成示例；不配置第三方地图密钥也可完成本地演示。

<a id="in-app-notifications"></a>

## 10 站内通知技术选择

沿用仓库当前已锁定依赖，不引入消息中间件或新的 UI 框架。业务规则见[需求](requirements.md#in-app-notifications)，模块和事务见[架构](architecture.md#in-app-notifications)。

| 技术 | 本轮使用位置与理由 |
| --- | --- |
| Go、pgx、Gin | 中性事实写入接入原 pgx.Tx，独立 Go worker 投递；Gin 人员 API 复用身份、CSRF、错误和路由策略。 |
| PostgreSQL、Goose | 来源事实／接收快照、租约 fence、个人接收序号与修订、状态、偏好、条件、桥接收据；增量迁移 36，支持事务回滚、重复领取、备份恢复。 |
| React、TypeScript、TanStack Query/Router | 账号级铃铛／中心、共享查询和取消、15s/5s 轮询、白名单原业务导航；复用既有登录失效及授权缓存边界。 |
| 现有 UI/layouts、Tailwind | headerActions 注入，语义未读提示、键盘和手机布局；共享布局不查询业务数据。 |
| OpenAPI／既有生成工具 | 通知与个人邀请操作、权限策略、TS 客户端同步；生成检查作为发布自验。 |
| Go race、真实 PG、Node、Playwright | 事务／租约／竞争／迟提交／备份恢复、客户端取消和实际业务浏览器；合成文件走正式管线。 |

短轮询是本轮明确的交互方案，可靠性来自持久数据库。worker 不需要 Redis、ClickHouse、S3、空间转换器、WebSocket 或 SSE；原业务处理仍依赖各自存储。大屏订阅流和人员通知身份不同，不复用 SDK ack 或终端会话实现已读。生产容量、自动保留期限和实时推送扩展另行验证，不从功能样本推出吞吐保证。

## 文件与附件技术

沿用 Go/Gin/pgx/PostgreSQL 与私有 S3，新增独立 Go worker/checker；普通文件无需 ClickHouse。Go 标准库完成图片重新编码与 Office ZIP/XML 有界核验，Linux seccomp/rlimit 配合容器或 systemd 隔离。React/TypeScript 使用 Web Worker 增量 SHA-256、AbortSignal、TanStack Query 和纯 UI 模式组件；不引入全文件 WebCrypto 缓冲、公共对象 URL 或预签名下载旁路。运行配置与平台限制见 operations/files-and-attachments.md。
## 审计与操作追踪技术

沿用 Go、PostgreSQL、React／TypeScript 和现有 S3 协议，不增加消息队列或独立审计搜索集群。Go 的 `auditrecord` 在来源事务中写事实；`audit` 用 pgx 事务、MVCC、租约和 fence 完成查询、固定快照与任务执行。PostgreSQL 保存事实、明确关联、授权向量、收据和物化行；S3 只保存注册的私有 CSV，不能作为事实真相或授权判断来源。

React 工作区复用现有表格、响应式弹层和 DomainFiles，TanStack Query／Router 保存有界 URL 条件并隔离账号空间缓存。OpenAPI 从 Go DTO／权限目录生成，TypeScript 客户端只消费生成模型。CSV 使用标准 encoding/csv、UTF-8 BOM 和公式转义，文件大小和快照预算在生成过程中执行。

独立 Go worker 使用 PostgreSQL 领取及持久重试，不引入 Redis 队列。Redis 仍只承担既有身份功能。业务元数据 Adapter 不启动 ClickHouse、S3 或文件检查器；导出／下载按需要检查其依赖。部署预算见 `deploy/audit/worker.example.yaml`，只允许下调上限。

验证使用真实隔离 PostgreSQL schema、私有 S3 桶、实际来源 worker、Playwright 浏览器和 Go race detector；恢复用 pg_dump／pg_restore 到独立数据库并复制／校验实际对象。纯状态测试的替身不替代完整来源验证。具体证据和未覆盖的生产运行条件见[本机工程环境与人工验收](operations/local-environment.md)。

## 独立诊断技术

Go 1.27 net/http 提供独立 server 与 observer，bbolt 1.5 保存本机有限历史及顺序屏障，不新增业务 migration。Argon2id 保存人员密码散列，crypto/tls 校验证书，securefile 使用 no-follow、编辑锁和原子替换保护配置。gopsutil 与 Linux cgroup 分别采集进程、宿主和容器资源；godbus 只读取 systemd 固定属性，Docker 限制为固定 GET。pgx、go-redis、clickhouse-go 与 minio-go 只执行登记的固定探针。

React 19、TypeScript、TanStack Router/Query 和现有 UI/layouts 构建独立页面；客户端只消费 diagnostics OpenAPI。Vite 参与构建及显式开发，release 将资源嵌入 Go；Docker 最终镜像没有 Node。Compose/systemd 生命周期独立于业务。版本由 go.mod 和工作区 lockfile 固定；真实 owning process、TLS 网络、实际容器及私有 D-Bus 契约证据不代表已验证真实多服务器或 systemd unit。

## 大屏命名读取技术

Go／pgx 实现领域权限、严格 JSON DTO 和读取协调；PostgreSQL／Goose 保存不可变成员、固定结果与持久任务，避免引入第二套权限库或 Redis 任务真相。ClickHouse 的一个或多个已配置节点和私有对象存储承担正式原始／派生观测及摘要读取；统计不重新执行派生规则。事务内成功证明复用只减少同一次读取的重复 SQL，交付仍新开事务复验。

公开 SDK 1.1 是无框架依赖 ESM 与 TypeScript 声明，Go DTO 生成管理／运行协议，Bridge 主版本继续为 1。开发工具保留 Manifest 1，并支持 Manifest 2 模板、九种显式模拟场景、独立 tgz 安装和真实预览。独立验收工程使用 TypeScript、esbuild 和锁定的 Three.js 0.187.0：esbuild 将依赖全部打入本地 ZIP，Three.js 仅绘制已授权固定 GLB；SVG 绘制地图、图纸区域和趋势，不访问 CDN。React 管理端继续复用既有 Drawer、表单和服务端搜索分页。

Playwright 验证真实三 origin、opaque iframe、终端和开发壳；Go race 与真实 PG／ClickHouse／S3 验证领域计数、固定下钻、后台 fencing、迁移和独立恢复。负载报告明确环境、数据、冷／热定义和延迟，不能据此推断生产容量。
