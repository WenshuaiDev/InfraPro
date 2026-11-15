# 项目大屏 SDK 与运行机制自验

日期：2026-11-13。状态：Awaiting acceptance（实现与自验完成）。人工验收尚未进行。

本轮依据 [HTTP 版方案](../requirements/visualization-display-proposal.md)，在 `/Users/hijacker/.codex/worktrees/283d/InfraPro` 当前 checkout 实现；没有新建分支或 worktree。原工作区已有的空间和派生功能继续保留。方案源文件 `/Users/hijacker/Documents/ChatGPT/InfraPro/docs/requirements/visualization-display-proposal.md` 已同步 HTTP 调整；应用代码交付位于本工作区。

## 范围与证据

| 范围 | 交付行为 | 自验证据 |
| --- | --- | --- |
| DM-01 | 默认关闭、启用后 0–N 实例；名称／可选成员、启停／归档、稳定入口及九项权限。 | 生命周期集成、归档读取／禁止新建、项目权限回归；浏览器创建两个独立实例。 |
| DM-02 | SDK 1.0.0 编译 ESM、声明、公开导出、MIT 许可，协议与能力主版本各自为 1。 | 仓库外临时目录从 tgz 离线安装、TypeScript 构建；无 workspace 或 React 运行依赖。 |
| DM-03 | 资源／通道、原始与派生 latest／details／trends／statistics／overview、来源、固定成果、空间 Provider。 | 实际 PostgreSQL／ClickHouse／私有对象存储测试；精确数值 `9007199254740993.125` 与纳秒、固定成果递归撤权、空间块读取及原件拒绝；受控跳转定位设施和监测通道。 |
| DM-04 | 首屏快照、源向量、持久事件、应用成功后 ack、重复投递、取消、代次 reset、独立句柄及卸载。 | Go 重放／跳号拒绝／过期 reset／租约测试；Node 异步消费者、失败不 ack、重复订阅、AbortSignal、Blob URL 释放。 |
| DM-05 | init/dev/login/validate/pack/upload，六种夹具状态、真实预览、一次性配对和上传 resume。 | 外部安装脚本；浏览器真实预览及上传工具从已有分片恢复，等待服务器 ready。 |
| DM-06 | 4 MiB 分片、128 MiB ZIP、独立受限 worker、严格 schema／路径／哈希／类型校验、封存和回收。 | Go／Node 非法包矩阵；真实上传；受限 SQL 角色不能读 accounts；取消与任务 fencing、孤立对象维护检查。 |
| DM-07 | schema 表单、兼容字段继承／待补预览、固定与动态绑定、数量／语义／时段／未来对象上限。 | 配置升级单测；动态范围与显式时间证明集成；服务器重新核验当前授权，配置保存成不可变修订。 |
| DM-08 | 草稿预览、准备摘要、CAS／幂等、发布／回退／撤回及引用保护。 | 并发发布仅一次成功，同请求重试返回同发布；浏览器 ready 后切换；服务测试失败目标不冒充实际版本。 |
| DM-09 | 人员会话与终端委托分离；最小范围／字段／有效期；主体持久归属；返回前再次复验。 | 只读主体不能进入写 API；配对防重放、终端凭据轮换、人员退出后工具失效、撤权与恢复代次失效。 |
| DM-10 | 独立运行进程、三域 HTTP、opaque iframe、MessageChannel、CSP、预算及共享读取。 | 非 localhost HTTP 的 Chromium 完整链路；WASM／Blob Worker／WebGL；共享取消 race 测试、并发订阅预算、HTTP Host／Origin／Cookie 边界。 |
| DM-11 | 明确 Provider 注册／权限／版本／预算，告警类型与 unavailable。 | 未接入告警不能作为必需能力发布；可选告警不影响示例；SDK 含查询／汇总／订阅的公开契约。 |
| DM-12 | 包／发布／目标与实际／最后有效读取状态、受限结构化事件与聚合指标、终端、引用清理与恢复命令。 | 缺包标记 missing、恢复原字节再核验 ready；备份清单、恢复后旧会话／终端失效；Nginx 配置检查及运维说明。 |

主要代码在 `packages/go/platform/display`、`packages/go/platform/access/read_subject.go`、`packages/go/platform/runtime/display_providers.go`、`services/api/cmd/display-*`、`services/api/internal/transport/http/display`、`apps/display-runtime`、`apps/console/src/features/displays`、`packages/ts/display-*`。迁移为 00035，建立 monitoring 快照／选择集／准备任务的 display_subject_id 归属，旧人员查询仍按 actor_id 处理。

## 实际自验

测试使用明确命名的隔离测试数据库、逐用例 schema、合成设施和合成数据；没有给用户项目注入大屏或数据授权。浏览器测试完成后关闭自己的服务并清理 schema，截图中的项目不可作为持久验收入口。

- `pnpm display:pack`：两个 npm tarball 在仓库外离线安装、构建、校验、ZIP 打包成功。产物：[SDK 1.0.0](../../.artifacts/display/packages/infrapro-display-sdk-1.0.0.tgz)、[开发工具 1.0.0](../../.artifacts/display/packages/infrapro-display-devtools-1.0.0.tgz)、[最小程序 ZIP](../../.artifacts/display/example.zip)。
- `pnpm display:browser`：生产构建的控制台，真实 Go API／运行／资产服务，三个 `*.127.0.0.1.nip.io` HTTP host；`window.isSecureContext === false`。完整管理、正式启动、opaque sandbox、Cookie 不可读、SDK 跳转到真实资源详情、重新加载、目标 ready 切换、终端、mock、真实本地预览、工具上传续传、重连间隙撤权卸载、离线租约到期清理和 390px 手机视口通过。
- `pnpm test:display`：9 项 SDK／包／配置 Node 单测和 15 组 Go race 测试，覆盖安全边界和真实领域集成。日志 `.artifacts/display/integration-complete.log`。
- `pnpm lint`、`pnpm typecheck`、`pnpm contracts:check`、`pnpm permissions:check`、`node scripts/go.mjs check`：前端类型、全仓库 lint、API／协议／权限同步及 Go fmt/vet。
- 跨模块 Go race 回归覆盖 access/config/derivedmonitoring/devicecatalog/engineering/identity/migrations/monitoring/projects/resources/spatial 和 API／维护工具。首次发现 access／project／tenant 的旧权限目录计数断言过期（含审计快照数量）、迁移 35 缺少拒绝 Down；修复后对失败项复跑。动态构件筛选的空数组处理、重连间隙撤权、结构化错误传递及业务跳转路由也在补充回归中修复。运行中修改代码造成的旧编译快照错误由最终 API／工具回归覆盖。日志为 `go-regression.log`、`regression-fixes.log`、`project-permissions.log`、`api-tool-regression.log`；完整 tenancy race 单独运行，记录在 `tenancy-regression-final.log`；整包运行时编译快照仍带旧计数，四项对应断言已在 `tenancy-counts-final.log` 最终复跑通过；两份记录合计覆盖全部 34 项，无未解决失败、race 报告或最终超时。首次大回归并发执行时触及 20 分钟包超时，改为单独完成全部用例。
- `nginx:1.28-alpine nginx -t`：三域代理配置语法通过。运行服务 Docker 镜像已构建，可信壳及双入口存在并以非 root 用户运行，见 `docker-build-final.log` 和 `docker-smoke.log`。

安装包和示例的 [SHA256 清单](../../.artifacts/display/SHA256SUMS) 已固定；在 `.artifacts/display/` 执行 `shasum -a 256 -c SHA256SUMS` 可核对传输后的文件。[交付核对清单](../../.artifacts/display/delivery.json) 固定范围、检查及产物摘要。以上日志与截图统一保存在 `.artifacts/display/`。首次失败与修正有各自记录；以最终日志和 `browser/result.json` 为本次交付证据。

浏览器截图：[管理页](../../.artifacts/display/browser/management.png)、[HTTP 运行页](../../.artifacts/display/browser/runtime-http.png)、[真实本地预览](../../.artifacts/display/browser/local-real-preview.png)、[手机管理页](../../.artifacts/display/browser/management-mobile.png)。

## 启动与人工验收

先按 [运行手册](../operations/visualization-display.md) 配置本工作区：构建可信壳与控制台；在目标开发环境执行迁移 35 和权限目录同步；启动 API、控制台、display-runtime 与 display-package-worker。HTTP 示例和三域代理位于 `deploy/display/`。使用现有开发账号登录，不新增默认账号或密码。

1. 打开一个有授权的项目 →「项目大屏」：初始关闭、无默认实例；启用后创建两个实例，列表与入口独立。
2. 安装 SDK／工具 tgz，按工具 README 执行 init、build、validate、pack。上传生成的 ZIP：可见分片／校验状态，最终 ready；修改文件哈希后的包应失败并定位字段。
3. 选 ready 包，填写标题并绑定当前项目设施；配置升级时查看继承／重置／待补摘要。保存后正式页面不变化，草稿预览明确显示预览身份。
4. 授予 resources.read 和所选设施的读取范围／期限；缺必需绑定或必需能力时发布准备失败，修正后显示摘要并发布。稳定入口登录后显示所选真实设施。
5. 修改标题保存新草稿，再发布：目标 ready 后实际版本切换；历史回退生成新发布，仍按当前权限验证。
6. 终端登记并在运行站点 `/terminal` 配对：无需人员密码；同码再次使用失败。重置／撤销后原终端停止运行，重新配对才可恢复。
7. 工具真实预览配对，只读本人当前项目；工具 upload 使用独立上传配对且不会自动发布。模拟模式始终显示模拟标记。
8. 撤销读取授权／停用实例：已打开页面卸载数据内容；归档项目可合法读取和撤权，不能新增或发布。管理运行区分别显示目标、实际、心跳与最后有效读取。
9. 若验收监测／空间能力，用已授权通道和固定空间／成果版本创建相应绑定，按 SDK README 查询／订阅；精确值保持字符串，当前不可用告警返回明确错误。暂停网络后观察断连和有限租约，恢复后重新验证与同步。

## 验证边界

本轮完成机制与必要基础接缝，不提供项目成品大屏、图表／三维业务模板或实际告警引擎。没有执行生产迁移／部署、真实环境恢复演练或公共 npm 发布。合成测试中的缺包恢复与环境代次验证不等同整套生产备份恢复认证。

浏览器自验使用本机 Chromium；未验证 Safari、实体墙屏操作系统自动启动或长时间无人值守。资源预算与并发边界已测试，本轮不据此宣称极限容量或生产负载指标。现有控制台构建仍提示部分空间／趋势图 chunk 大于 500 kB，构建成功；这不是大屏 SDK 的新增运行依赖。

HTTP 是用户本轮明确选择，HTTPS／证书不列为未完成项。运行壳与资产仍保持独立 origin 和受控身份。iframe 不承诺对应用自身 CPU／GPU 的绝对进程隔离。

## 主工作区合入（2026-11-13）

按用户要求，本轮 152 个文件（106 个新增、46 个修改）从 `/Users/hijacker/.codex/worktrees/283d/InfraPro` 合入 `/Users/hijacker/Documents/ChatGPT/InfraPro` 的 `main` 工作区。两个 checkout 的基线均为 `ab8b2feee207b00829ab8765848d11af02c438b7`。沿用此前空间功能的文件合入方式，没有新建分支、worktree 或提交，暂存区保持原状。

主工作区原有空间功能、派生功能、ClickHouse 配置、AGENTS.md 和通知／预警方案保留。文档索引单独合并，保留站内通知条目，并将大屏条目更新为实现与自验完成、待人工验收。SDK／工具安装包、最小 ZIP 和原自验材料也已复制到主工作区；后续验收可从主工作区的本页及运行手册进入。

合入备份、文件摘要和检查日志位于 `.artifacts/display-main-merge/2026-11-13T08-14-56Z/`。合入后检查全部通过，人工验收状态继续为 Awaiting acceptance。普通开发服务未重启，没有向业务数据库执行迁移。

| 主工作区检查 | 结果 | 合入目录中的日志 |
| --- | --- | --- |
| `pnpm install --frozen-lockfile` | 锁定依赖安装通过。 | `install.log` |
| `pnpm display:pack` | SDK／工具在仓库外离线安装、构建、校验及打包通过；产物摘要与原交付一致。 | `pack.log` |
| `pnpm check` | Go fmt/vet、全仓 lint、类型、API／协议与权限同步通过。 | `check.log` |
| `pnpm build` | Go、控制台及可信运行壳构建通过；保留已有 chunk 大小提示。 | `build.log` |
| `pnpm test:display` | 9 项 Node 单测、15 组 Go race／真实领域测试通过。 | `tests.log` |
| `pnpm display:browser` | 主工作区构建下 15 项 HTTP 浏览器检查通过，包含资源跳转、发布切换、终端配对、离线租约与重连撤权清理。 | `browser.log` |
| 合入与保护核对 | 文档索引与合入说明单独整合；自动生成路由仅排序变化，AST 等价；其余文件与交付源一致。所有合入前的无关文件、暂存内容及 HEAD 保持不变；`git diff --check` 通过。 | `manifest.json` |

主工作区的安装包与截图位于 `.artifacts/display/`。本次合入是文件变更进入 main 工作区，未生成 Git 提交、未推送；不会将用户人工验收标记为 Accepted。
