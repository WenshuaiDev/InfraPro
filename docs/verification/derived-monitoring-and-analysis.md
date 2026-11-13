# 派生监测与分析实施、自验证记录

状态：**Awaiting acceptance（全部功能已实现，必要功能自验通过，待用户人工验收）**。开始日期：2026-11-12；功能交付日期：2026-11-13。人工验收：未开始。

当前结论（2026-11-13）：DM-01–DM-09全部功能已实现并通过必要功能自验；33项真实存储业务检查经组合及定向复验通过，19项桌面／手机流程、11项HTTP实际部署、v33独立恢复及v12→v33升级通过，94张旧表及原绑定／授权保持。最新静态检查通过，230个构建文件hash一致。新独立验收会话的17个派生样本及桌面／手机页面已核对，访问和操作步骤见人工验收手册。性能、压力及长时观察按用户决定暂缓，明确未验证；人工验收尚未开始。以下旧段落保留实施历史，以本段、范围变更和完成账本为当前状态。

**用户范围变更，2026-11-13**：用户在侧聊明确要求“当前把功能需求完成了就行了，性能测试、压力测试这种先不要做了，你直接给主线程说”，并授权传达给本任务。继续完成全部功能需求及必要功能自检；DSV-22、百万负载／计时复测、2小时混合＋24小时实际观察暂缓。性能脚本与已产生证据保留，性能和规模能力仍未验证。人工验收不由自验代替。

实施依据为[详细方案](../requirements/derived-monitoring-and-analysis-development-plan.md)与[已确认范围](../requirements/derived-monitoring-and-analysis-plan.md)。全部 DM-01–09、R-01–11、MA-01–20 保持在本轮范围内；阶段完成不代表整轮完成。

## 基线与实施顺序

- 当前 checkout：`/Users/hijacker/.codex/worktrees/ad0c/InfraPro`，HEAD `22e236ab4e8f86901572654511720e9b9d655872`（detached）。不创建分支或 worktree。
- 两份需求文档从用户指定的原目录同步；原目录其他未提交工作不在本轮覆盖范围。保留当前工作树中用户对AGENTS.md的修改，不将其作为本功能修改。
- 基础迁移基线为 `00012_monitoring_metadata.sql`，本轮增量为13–33。只在隔离测试／恢复环境执行，既有应用数据库不自动迁移。
- 实施顺序：共享数值／严格 DSL 和计算器 → 定义、资源与依赖模型 → 原始 Reader、任务、变化日志、发布 → 查询、分析、权限、导出 → 全界面 → 故障、迁移、恢复与负载 → 交付。
- 技术：Go 1.27、pgx/Goose/PostgreSQL 17 维护配置与原子可见性，ClickHouse Int256/scale 保存正式结果，S3 保存不可变依据；React/TypeScript/TanStack 复用监测工作台。无新增脚本执行器或消息中间件。

## G-01：基础接口映射

| 接缝 | 现有实际实现 | 本轮改造 |
| --- | --- | --- |
| 原始快照与扫描 | `monitoring/queries.go` 的 `queryPlan`，`storage.go` 的 `rawRead/readOne`，原始采用清单 | 公开固定快照 Reader；用户授权和受限系统委托分别验证，历史依据读取不混入当前值。 |
| 数值／时间 | `monitoring/internal/observation` 的整数／Decimal128、精确矩、时间解析 | 有限共享数值包；保留原始输入与输出协议及原有夹具。 |
| 变化与提交位置 | `publication.go`、`revocation.go`、`readopt.go`；`monitoring_tenant_states.commit_sequence` | 同事务持久事件及范围索引；独立 consumer 游标，不消费摘要 dirty 作为事件真相。 |
| 任务 | `jobs.go` 固定 kind 领取与分派、30 秒租约、5 秒心跳 | 明确注册 derive/analysis/replay/compare，统一可配置租约，保留 fence、取消和收据。 |
| 资源 | v9 通道 point/public metric，v10 资源授权，v11 来源绑定 | direct 分支兼容；derived 三种 owner、条件索引及专用维护委托，旧授权默认不扩权。 |
| 查询／导出 | selections/queries 与原始导出 schema | 窄 Provider；判别 DTO、派生数值 schema 和递归数据权限。 |
| 组合入口 | `runtime/business.go` 聚合设备／监测引用 | 新增派生引用和 Provider，保留已有保护。 |

## G-02–08 与环境

本机工具实测：Go 1.27.0、Node 26.10.0、pnpm 12.18.0、Docker Server 29.9.3；arm64，主机 14 CPU、36 GiB 内存，初查约 660 GiB 可用磁盘。精确组件镜像与依赖沿用 `deploy/monitoring-contract/versions.json`；这些信息不代表生产性能。

| 门槛 | 状态／仍需证据 |
| --- | --- |
| G-01 | 接缝已实现；固定输入、共享任务、正式发布、统一查询及兼容行为通过最终组合及定向真实回归。 |
| G-02 | 全算子、单位、数值、状态和时间语义夹具通过race，真实功能边界通过；百万负载规模判定按用户决定暂缓。 |
| G-03 | 真实Int256/scale/纳秒、JSON/int8range、双副本发布、混合及历史CSV/Parquet往返通过最终功能组合及定向复验。 |
| G-04 | 镜像digest、Go版本、规则／编译器／引擎／存储协议均固定，恢复清单含实际可执行文件hash及算法夹具。 |
| G-05 | 主机与Docker资源、队列和部署预算已记录；ENOSPC、取消、检查点回收通过功能检查；规模资源及长时有界性按用户决定暂缓，未验证。 |
| G-06 | 已执行 v12 旧二进制备份、独立恢复后升级至 v33；94 张旧表原有列／行 hash 保持一致，设备绑定、项目授权、退役／删除资源与原始历史保留。最新证据见v33旧库升级最终记录。 |
| G-07 | 原DP-0硬件与阈值记录保留；性能、压力、最大活动窗口规模及2小时混合＋24小时观察按2026-11-13用户决定暂缓。完整组合未通过，DSV-22及DP-1–3均不得宣称通过；功能／故障／恢复检查继续完成。 |
| G-08 | API／worker／CLI就绪门禁、旧实例拒绝新迁移、direct资源及授权兼容已实现；v12旧实例拒绝v33数据库的实际检查已通过。 |

## 需求完成账本

| 范围 | 证据要求 | 当前状态 |
| --- | --- | --- |
| DM-01 定义与模板 | DSV-01/07；MA-01/03/12 | 已实现，严格规则、模板／版本、定义API及真实浏览器功能检查通过；待人工验收 |
| DM-02 正式通道与资源 | DSV-07/08/18；MA-01/02/12/13/15 | 已实现，三归属、配置与资源生命周期通过；旧v12→v33升级及94张原表、绑定／授权保持通过；待人工验收 |
| DM-03 可组合计算 | DSV-01–06；MA-03/04/05/06 | 已实现，全算子、精确落盘、时间／质量／单位、16独立输入及外部排序恢复功能检查通过；规模验证暂缓；待人工验收 |
| DM-04 自动计算与重算 | DSV-06/10/11/12/14/15；MA-07–11/18/19 | 已实现，八层依赖、自动续算、配置更正、撤销、并发发布与显式上游回算通过；长时验证暂缓；待人工验收 |
| DM-05 结果与追溯 | DSV-03/09/13/20/21；MA-10/12/17/20 | 已实现，当前／历史、来源、回放、完整性隔离与新代次修复通过；v33独立恢复及4个历史代次回放通过；待人工验收 |
| DM-06 统一持续监测 | DSV-13/19；MA-01/02/05/17 | 已实现，混合明细／最新值／趋势／统计／导出及管理页面通过；含原始兼容的19项浏览器流程通过；待人工验收 |
| DM-07 历史分析 | DSV-16/17/19；MA-06/16/17/18 | 已实现，多输出原子成果、比较、局部授权、结果／输入导出与非因果历史基准功能检查通过；待人工验收 |
| DM-08 授权与导出 | DSV-17/18/20；MA-14–17 | 已实现，递归当前／历史／导出、撤权／归档、下载首个128 KiB后撤销停止通过；待人工验收 |
| DM-09 工程交付 | DSV-14/18/20/21；MA-18–20；DSV-22暂缓 | 功能交付完成；CLI、迁移、恢复、预算保护、保留策略、日志、手册及可访问合成环境通过；DSV-22按用户决定暂缓；待人工验收 |

## 验证结果

### 最终自验逐项追踪

以下列出功能检查入口。DSV-01–21在本轮功能范围内已有通过证据，见末尾最终组合、定向复验及独立迁移／恢复／浏览器记录；DSV-15涉及持续负载的部分和DSV-22按用户决定暂缓，不属于通过结论。

| 检查 | 实际检查入口／判定 |
| --- | --- |
| DSV-01 | compute 的 StrictRuleAndReferenceRejection、UnitAndCausalContracts、FormulaWhitelistAndBudget；严格JSON及参数hash。 |
| DSV-02 | AcceptanceArithmeticAndOrder、NegativeBaselineLagAndMissingWeightedInputs、RepeatedQuantileRanksAndSingleSampleVariance、ExtremaTiesChooseEarliestTime；17个真实合成样本。 |
| DSV-03 | PrecisionAndStatus、monitoringvalue、真实Int256/scale存储协议、AnalysisExternalWindowExactAndCheckpointRecovery；无float64输入。 |
| DSV-04 | CalendarDSTLeapAndDomain、MaximalNanosecondIsPublishableWithoutOverflow、WholeRangeLabelIsOnlyValidAtItsSealedBoundary；项目whole_range真实成果。 |
| DSV-05 | MatchingUsesActualTimesAndPrimaryClock、ExclusionsGapsAgeAndConfigurationBoundaries、AnalysisNearestAndFutureFixedBasisRetainEvidence；固定基准因果检查。 |
| DSV-06 | PartitionAndCancellation、DerivedActualPredecessorAcrossGapAndSubrange、DerivedSampleWindowUsesNthActualPredecessor、完整后台试算与外部排序检查点恢复。 |
| DSV-07 | DefinitionLifecycleConcurrencyAndPermanentVersions、DerivedConfigurationCandidatesAssignmentsAndIntent、SemanticIdentityWhitelist、真实历史配置修正。 |
| DSV-08 | DerivedChannelOwnershipAtomicityAndInputContracts、原始导入／来源绑定兼容及三归属真实浏览器入口。 |
| DSV-09 | DerivedFormalPublicationUsesSealedAttempts、DerivedResultPhysicalProtocol；失败提交不可见、空代次替换、稳定逻辑身份。 |
| DSV-10 | DerivedFutureInputImportedBeforeEnable、DerivedClockDueRestartAndEmptyCount、DerivedPrimaryClockAdvancesPublishedPrefixBeforeUncomputedTail；控制委托与退役恢复。 |
| DSV-11 | 固定基准追加／撤销／重新采用、真实配置修正补录、ImpactRetainsDistantSuccessorsAndComposesWindows。 |
| DSV-12 | DerivedConcurrentGraphEditsCannotCommitOppositeEdges（两个实际账号）、八层正式链及递归撤销、显式依赖回算和16独立输入。 |
| DSV-13 | 正式发布的统一明细／latest／统计／趋势、CalculationStatisticsKeepCanonicalPrecisionAndSeparateLatest、精确摘要往返、固定分页及浏览器任务返回。 |
| DSV-14 | 正式PG发布与消费者提交失败、共享租约接管／旧fence、结果存储缺失／篡改、外部排序中断恢复、最大预算／取消／临时盘耗尽。 |
| DSV-15 | DerivedConcurrentPublicationRechecksAndMerges：真实封存后追加、撤销、更正及另一不相交范围发布；NewerDirtyIsNotClearedByOlderCompletion。持续负载部分按用户决定暂缓。 |
| DSV-16 | AnalysisAtomicMultiOutputVersionsAndChangedInputs、AnalysisWholeRangeProjectOwnershipHistoryAndArchive；固定配置与比较。 |
| DSV-17 | 项目委托、递归候选／前驱／基准权限、局部成果、下载128 KiB首块后撤权、真实浏览器缓存清除；无租户入口回退。 |
| DSV-18 | v12旧二进制备份恢复后真实升级；94张旧表原列hash、direct／来源／自定义角色／项目默认能力不变，旧就绪检查拒绝新库。 |
| DSV-19 | 派生浏览器桌面／手机：长名、暗色、键盘、错误焦点、试算／启用、比较／导出返回、首次权限版本及R7百分位输入。 |
| DSV-20 | 混合／仅派生／分析CSV与Parquet、分卷清单、百万逐行结果及依据核验、远端分区首次读取、永久历史与临时保留边界、只读对账。 |
| DSV-21 | 源服务暂停后的独立PG/CH/S3实际恢复；4个历史代次逐代次回放、固定基准、40行多输出分析、权限／暂停／退役状态保持。 |
| DSV-22 | 用户决定暂缓，未验证；完整DP-0组合未通过，最大窗口及百万加一未执行，26小时观察从未开始。已有部分日志保留，不作为整项通过结论。 |

R-01–R-11分别由以下检查约束：R-01→DSV-07/08/18；R-02→DSV-05/09/10/11；R-03→DSV-05/06/12/17；R-04→DSV-02/03/20；R-05→DSV-07；R-06→DSV-12/15；R-07→DSV-09/11/17；R-08→DSV-09/14/15；R-09→DSV-09/14/16；R-10→DSV-05/06/17；R-11→DSV-18/20/21。人工MA-01–MA-20单独记录，任何自验通过均不自动转为Accepted。

### 2026-11-12：精确计算与存储契约迭代

新增 `monitoringvalue` 共享包（原始输入解析不改）；原始统计复用精确 half-even／开方舍入，时间与数值格式化复用同一实现。新增 `derivedmonitoring/internal/compute`：严格 JSON 与字段路径、版本化编译、参数、单位与因果校验、公式白名单、8 个模板、全部首批算子、五类窗口、质量和配置边界、固定基准、可细化的非有理中间值及有界滑动统计／精确秩选择。纯规划包提供八层无环检查、倒排影响次序和纳秒区间运算。

实际执行：

| 命令／证据 | 结果与覆盖边界 |
| --- | --- |
| `node scripts/test-derived.mjs --contract --keep` | 通过。纯规则、共享数值及原始数值回归使用 race detector；真实 PG/CH 数值及 Parquet 契约通过。证据目录 `.artifacts/derived-monitoring/2026-11-12T08-13-30.167Z/`。 |
| `go test -race ./packages/go/platform/derivedmonitoring/... ./packages/go/platform/monitoringvalue ./packages/go/platform/monitoring/internal/observation` | 通过（含随后新增的规划、空输入、负基准、lag、区间匹配、极值并列及最大纳秒回归）。覆盖手算样本、10^-24 量级方差、76 位系数与舍入溢出、下溢、非有理中间值、除零、负基准协议、重复键／非法语法、单位、最近匹配等距、质量中断、配置边界、DST 23/25 小时日、闰日、whole_range、空窗、样本窗与固定 halo 分块等价。 |
| 独立滑窗 oracle | 150 个确定性随机样本逐窗重扫并用有理数独立排序，逐点比较 sum/mean/variance/min/max/range/median/P95；验证滑动删除、重复秩与开闭边界。 |
| `go vet ./packages/go/platform/derivedmonitoring/... ./packages/go/platform/monitoringvalue` | 通过。 |
| `go test ./packages/go/platform/monitoring/...` | 通过；现有原始监测纯测试保持兼容。 |
| `pnpm check` | 通过：gofmt、所有 Go module vet、ESLint、TypeScript、OpenAPI 与权限目录同步检查。路由生成器仍有非失败 circular dependency warning；本次未修改路由。 |
| `node scripts/test-derived.mjs`（最终纯规则复验） | 证据目录 `.artifacts/derived-monitoring/2026-11-12T08-22-08.287Z/`；含完整计算与纯规划测试，不含 API／数据库发布事务。 |

### 计算器百万输入基线（非完整 DP-0）

命令：`GOMAXPROCS=2 GOMEMLIMIT=1GiB go test -tags derivedload -run '^TestMillionInputCalculator$' -v -count=1 -timeout=20m ./packages/go/platform/derivedmonitoring/internal/compute`。可重复入口：`pnpm test:derived:load`。

每种规则各 100 万逻辑输入／输出，10 个 10 万行分块，包含完整 halo，1000 样本活动窗口。每个输出与闭式期望比对；执行前在测试中固定门槛为 rate ≥20,000 行/秒、mean/variance ≥2,000 行/秒、P95 ≥1,000 行/秒，单块≤60秒，观测堆内存≤1GiB。GOMEMLIMIT 是 Go 运行时目标，不是操作系统硬限制。本项没有读取 CH/S3、没有 API 并发，不代替历史任务端到端吞吐门槛。

| 算子 | 总耗时 | 输出行/秒 | 最慢分块 | 观测峰值堆字节 |
| --- | --- | --- | --- | --- |
| rate | 1.602秒 | 624,104 | 176毫秒 | 176,232,328 |
| mean | 2.362秒 | 423,377 | 251毫秒 | 154,950,016 |
| variance | 2.626秒 | 380,845 | 274毫秒 | 171,941,200 |
| P95 R7 | 4.401秒 | 227,209 | 452毫秒 | 161,500,408 |

四类均通过本项预设门槛。不能据此宣称自动更新参考包络、目标部署、独立恢复或整轮功能已通过。

证据不证明 DSV-01–06 全部关闭：仍需真实 RawReader／分块规划接入、完整预算、外部排序与恢复检查。纯图算法不能代替数据库图锁下的并发无环验证；纯区间运算不能代替发布 fence／dirty 事务验证。

### 2026-11-12：定义版本与派生资源建档迭代

已增加 PostgreSQL 13–14 增量迁移（仅在隔离测试库执行），28 个明确的租户／项目权限，定义模板展开、草稿保存、校验收据、不可变发布版本、停用、独立复制、删除后编号保留、分页 API 和生成的客户端类型。项目定义查询只使用可用发布名称，不用草稿名称筛选。

工程通道保留原 ID；已有行采用 direct/point 默认值，派生通道可归属设施、构件或测点。建档原子写入资源、配置、依赖、结果头、收据和审计；延迟约束拒绝不完整资源。固定配置保存完整源语义与输出契约。真实数据库检查八层依赖上限、单位／方向／基准、同测点编号共用、配置不可修改及设备来源与计算来源分离。资源退役／恢复原子推进计算控制修订；恢复后为 paused。旧自定义角色和旧项目委托不自动扩权。

实际验证：

| 命令／证据 | 结果与覆盖边界 |
| --- | --- |
| `pnpm test:derived:database` | 通过；证据 `.artifacts/derived-monitoring/2026-11-12T09-25-48.464Z/`。包括规则 race 测试、真实隔离 PG/Redis 定义与通道事务、HTTP 格式与 CSRF、原有直接资源生命周期、项目显式授权、共享预览、归档、耦合写回滚及双身份不回退。没有调用正式计算 Worker。 |
| `TestDerivedProjectDelegationHasNoTenantFallbackOrDataGrant`（后续新增，race） | 通过。旧 create_channel 委托不能创建派生通道；明确 create_derived_channel 后可建档，自动资源授权包含计算维护且不产生监测数据授权；撤销输入后配置／依赖内容受限，旧建档收据不能绕过撤权；归档后可读，禁止新建。 |
| `pnpm check`（本迭代前段） | 通过；随后资源生命周期 Adapter 与追加项目测试须在后续检查覆盖。不能将这一较早结果表述为最终固定源码整体验收。 |

### 2026-11-12：固定输入、共享任务与真实试算迭代

新增 migration 15 的不可变原始输入快照、永久变化事件及独立消费者游标。原始发布、撤销、重新采用在原事务内记录事件，沿用租户提交位置锁；摘要维护不能消费掉派生消费者的依据。RawReader 提供独立的当前用户、系统委托及历史回放入口；无服务端委托验证器时系统入口拒绝访问。原始值保留 observation/adoption 区分和十进制字符串。

快照分开保存候选输出时刻范围与完整输入授权范围。区间型观测在事务外根据固定清单确认完整 interval，再于提交时复核授权；不能因 interval_start 早于候选范围而静默丢样。读取按行数、字节预算流式执行，并周期复核当前资格；撤销使当前读取失效，获准历史回放仍使用原清单。范围外追加不使相关变化检查失效。

共享 monitoring_jobs 已支持执行器注册、derive/analysis/derived_replay/analysis_compare kind、租约检查事务、共享检查点、进度及取消／重试接缝。接管递增 fence；旧执行器不能更新新检查点。取消检查周期为1秒，租约仍为30秒、心跳仍为5秒。租约配置统一化、队列公平性和全类并发预算仍待完成。

新增 migration 16 的不可变运行／固定输入关联／试算收据；状态仍以 monitoring_jobs 为准。已实现派生通道原始数值输入的真实试算、节点与实际匹配时间差说明、30分钟收据、同步预算转后台、运行分页、取消／技术重试、当前输入变化／撤销下的读取行为。试算不写正式结果头。OpenAPI、中央操作、TypeScript 客户端和 runtime 已接入。完整派生上游读取、非 constant 基准绑定、正式启用与发布仍待实现。

| 命令／证据 | 结果与覆盖边界 |
| --- | --- |
| `node scripts/test-derived.mjs --database --contract --keep` | 通过；证据目录 `.artifacts/derived-monitoring/2026-11-12T09-58-17.936Z/`。包含固定快照／变化范围／项目撤权／共享任务接管与取消测试，真实 PG/CH/对象存储的完整区间读取、撤销回放与重新采用，以及原有 CSV 发布、提交失败恢复和重复采用回归。本次证据产生在试算执行器完成前，不能覆盖其后新增代码。 |
| `go test -race -tags monitoringintegration ./packages/go/platform/resources -run '^TestDerivedTrialUsesRealSnapshotAndSharedTask$' -count=1 -timeout=5m` | 通过（14.708秒，含隔离环境和资源建档）。真实导入2、5、9，逐点核对无前驱／3／4、实际时间差、节点状态计数、请求重放、正式结果头为空、范围外追加不失效、相关追加标 stale、撤销隐藏旧值、长范围排队及取消收据。此为单通道小样本功能检查，不是完整 DP-0。 |
| `pnpm check` | 通过；覆盖新增 RawReader、任务宿主、运行及试算 API 的 gofmt/vet、ESLint、TypeScript、OpenAPI 和权限目录一致性。路由生成器仍有原有非失败 circular dependency warning；无新功能页面。 |

### 接续实施设计：启停控制、执行委托和正式结果

- **What**：启用、暂停、恢复使用可复核的操作预览；启用需要有效试算收据，零输入只能经显式等待选项通过。正式计算使用租户执行委托，保留当前和历史代次，并通过既有监测查询入口展示。
- **How**：预览固定规范化起点、配置／控制修订、试算依据和完整输入范围。提交时重新验证并保存不可变委托与配置分配；项目范围不得超出当前输入数据授权。自动任务不借用原创建者会话，检查租户、控制／委托修订、配置分配及输入范围。暂停递增 fence 依据，保留 dirty 和既有结果。原始和派生变化共用提交位置与永久日志。
- **Technology**：Go 继续使用共享任务宿主与精确计算器；PostgreSQL 保存委托、任务目标、检查点、不可变代次及当前区间映射。派生存储模块通过公开的物理连接／对象传输接缝使用现有 ClickHouse 副本及 S3，不引用原始监测内部 SQL。ClickHouse 采用 Int256+scale、UTC 纳秒与独立 attempt；S3 保存封存结果和来源清单。外部写入、校验均在 PG 事务外，最终短事务复验后选中正式集合。
- **Acceptance**：可手算输入产生同值正式序列；暂停后旧 worker 不能发布；成员离开不撤销合法租户委托；有限历史授权不能开启无限未来计算；撤销先使相关旧值不可作为 current，再保留历史依据。模拟不确定物理写和 PG 提交失败，均不得出现半块正式结果或重复当前值。

### 2026-11-12：执行委托、正式发布与持续更新迭代

新增 migrations 17–18：启停预览、不可变执行委托、失效范围／窗口位置、正式任务目标、写入尝试、不可变代次和发布收据。启用需当前配置试算收据，空输入需明确等待确认；暂停与工程退役会取消并 fence 旧自动任务。系统运行从持久委托取得输入权限，原创建者离开项目不会撤销既有合法委托；有限项目输入授权不能开启无限计算。

派生存储使用 ClickHouse Int256/scale/rounded、UTC 纳秒和独立 attempt。完整副本校验与 S3 清单封存后，PG 短事务选中代次、区间根和历史根，并写入变更事件、审计、任务成功和收据。空代次完整替换范围，旧值保留在历史集合。持久区间树按范围替换，PG 不保存不断增长的结果行清单。

永久变化消费者单次最多100事件，按当前图拓扑传播；失效范围与独立游标原子提交。无法在预算内确定 rank／前驱精确影响时保守扩大范围，并保留原因；通道碎片超过128时合并为保守范围。固定网格／自然周期有持久 next_due，按 allowed_lateness 判断到期；任务入队与 next_due 推进同事务。共享 monitoring worker 已接入独立调度循环，执行仍使用同一 monitoring_jobs 真相。

现有监测明细可混合直接观测与派生结果；查询冻结区间根，读取前后核对叶子变化和权限，撤销无需等待消费者即可隐藏旧 current。DTO、OpenAPI、TS 客户端和明细表显示 source_kind、正式身份、数值精度、无结果状态与覆盖状态。此时尚未完成派生通道导航、最新值／统计／趋势／导出及完整历史入口。

| 命令／证据 | 结果与覆盖边界 |
| --- | --- |
| `go test -race -tags monitoringintegration ./packages/go/platform/resources -run '^TestDerivedFormalPublicationUsesSealedAttempts$' -count=1 -timeout=5m` | 通过，最新15.259秒。实际2/5/9输入、null/3/4正式结果、两副本完整、注入PG发布失败后封存块复用、自动任务查看／取消后重调度、直接／派生混合查询、撤销立即隐藏、消费者游标提交失败回滚／重放、空代次替换及旧历史保留。 |
| `go test -race -tags monitoringintegration ./packages/go/platform/resources -run '^TestDerivedClockDueRestartAndEmptyCount$' -count=1 -timeout=5m` | 通过，12.718秒。注入服务器时钟，等待10秒前不运行、到期空count=0、新服务实例补齐停机窗口、时钟回拨无重复。无真实等待、无真实业务数据库写入。 |
| `go test -race ./packages/go/platform/derivedmonitoring/internal/compute ./packages/go/platform/derivedmonitoring/internal/planning` | 通过。影响传播包含远距离后继及内部窗口跨配置起点的保守覆盖。 |
| `pnpm check` | 通过：Go格式／vet、ESLint、TypeScript、OpenAPI、权限目录同步。存在既有路由生成器 circular dependency 非失败警告。此检查不等同于浏览器验收。 |

### 接续修正：真实前驱与分块等价

检查发现原 finite max_gap/max_span halo 可能把实际很远的前驱裁掉，使 gap_exceeded 被误报为 missing_input／insufficient_samples。后续采用固定源清单下的实际前驱搜索，保存完整输入授权／不存在证明，并为每个分块恢复所需前缀；不得以块大小截断 N。先加入可重复的大间隔、无效样本、块边界样例，再贯通试算／正式运行／当前查询的新鲜度证明。未修正前，既有密集三点样例不证明 DSV-06 完整通过。

### 2026-11-12：实际前驱、固定基准与启用前未来观测

前驱与样本数量窗改为在固定原始清单中寻找实际前驱，再按完整候选范围验证当前读取授权。max_gap/max_span 只判定结果状态，不截断前驱搜索。正式分块与当前结果子范围查询使用相同解析器；启用预览的执行委托包含获准的前驱搜索范围。直接输入的前驱已通过真实大间隔及样本窗测试；对计算节点的 rank 前缀仍保守读取历史并受预算限制，不能据此宣布完整 DSV-06 或 DP-0 已通过。

基准绑定使用 `baseline_bindings` 显式选择输入槽。observation 绑定纳秒时刻、observation ID 与 adoption ID；reference_window 使用规则固定范围、统计方法及质量。首次成功试算将快照和精确有理数依据写入 migration 19 的不可变表。试算、正式计算、发布复验、运行读取与当前结果查询共同检查同一基准证据；新增数据不会更换基准，原采用撤销后旧当前值立即隐藏，新代次发布 baseline_invalid，通道健康为 blocked。重新采用不能复活原基准身份。基准证据存储错误与明确撤销分别处理。

primary_input 启用时读取已正式采用的来源覆盖元数据，将已经导入但尚未到期的范围持久化为未来失效目标。即使原事件已消费，重启后也能按观测时刻及等待时长调度；不依赖再次导入。当前采用范围按批次边界保守合并，精确稀疏跳跃调度仍属于规模优化待验证项。

| 检查 | 结果与边界 |
| --- | --- |
| 真实 PG/CH/S3 `TestDerivedActualPredecessorAcrossGapAndSubrange` | 通过，15.731秒；两小时间隔返回 gap_exceeded、实际时间差完整，撤销远处前驱仅使依赖它的输出失效，重算稳定 result ID。 |
| 真实 PG/CH/S3 样本窗、正式发布、停机补窗与真实试算组合 | 通过，49.135秒；包含第 N 个实际前驱、窗口内排除质量、查询子范围等价。该次证据早于 migration 19，不能覆盖基准新增代码。 |
| `TestDerivedFixedBaselineAppendRevokeAndReadopt` | 通过，19.577秒（测试主体17.59秒）；固定均值3、输出6，补录99不改基准，撤销后来追加不误伤，撤销原依据后隐藏旧值并发布 baseline_invalid，重新采用保持失效。 |
| `TestDerivedFutureInputImportedBeforeEnable` | 通过，13.573秒（主体11.65秒）；启用前未来观测、已消费事件、等待边界、服务重建后自主到期输出。 |
| `pnpm check` | 通过，日志 `/tmp/infrapro-derived-baselines-check.log`；Go、ESLint、TypeScript、OpenAPI及权限生成同步。随后新增的测试与小范围修改仍需新一轮组合检查。 |

### 下一步与未完成项

1. S-02 仍需计算节点 rank 的完整前缀检查点、外部排序、工作集／临时盘硬预算及完整分块／重启等价；不能将现有纯百万计算器测试等同于端到端百万验证。
2. S-03 配置普通变更／未来生效／历史修正、派生结果基准绑定、完整递归依赖、定义与配置 UI、使用摘要及旧库升级比较未完成。
3. S-04–07 上游正式派生读取、递归数据授权、统一最新值／统计／趋势／导出、历史代次／回放、人工回算、历史分析及多输出成果／比较、完整页面仍未完成。队列公平性、统一租约配置和并发／工作集预算仍需完成。
4. S-08–09 完整故障矩阵、真实旧库迁移、独立恢复、混合负载、两小时稳定性与24小时持续观察、操作手册、启动／人工验收材料尚未完成。

没有迁移应用数据库、创建真实业务数据、部署、提交或推送。全目标继续处于 In progress，尚未进入 Awaiting acceptance。

### 2026-11-12：统一查询、对象通道导航与混合导出

最新值／统计／趋势／自然周期分组已读取正式派生代次及实际来源证明。统计使用规范数值的精确有理矩，原始样本与派生逻辑输出、有效结果、无结果分别计数；最新无结果与上一有效值分别展示。缓存命中仍复核输入。现有监测工作台增加全部对象目录与直接／派生筛选，设施和构件通道不伪造测点；单位按固定表达式显示。原始导入选择器仅允许直接通道。

混合 CSV／Parquet 保留原始 Int64／Decimal128 列；派生数值使用规范十进制字符串，附 scale、状态、配置／代次／运行和来源身份。导出清单固定派生根及覆盖范围；封存后 PG 短事务、文件列表和实际下载均检查输出、递归原始输入及基准授权／撤销证据。无结果保持 null，统计不把其计作有效 N。

实际证据：三归属目录与资源兼容检查 `TestDerivedChannelOwnershipAtomicityAndInputContracts` 通过（10.122秒）；扩展正式发布测试通过（26.643秒，日志 `/tmp/infrapro-derived-export-leaf.log`），包含混合 CSV／Parquet 六行、仅派生三行、统计 N=2／均值3.5／标准差0.5、撤销后已获取的下载对象仍在发送前停止。直接观测旧 Parquet 和 CSV 安全／统计导出回归分别通过（10.45／10.96秒）。新增测试曾将派生计数误写为原始 sample_count；已按分离计数契约改为 N 并重新通过。浏览器交互、项目递归 export 权限、百万输入及长时负载仍未据此完成验收。

### 接续实施设计：配置生效分配、历史修正与回算

- **What**：保存不可变候选配置，独立试算；普通变更在当前／未来边界生效，历史修正明确区间、原因和下游影响。查询可查看任一配置分配修订。手动回算使用同一正式发布流程。
- **How**：保留原始输出身份；普通变更仅放行已约定白名单，同业务位置输入替换核对固定槽位语义。明确算法修正需声明保持业务意图并保持归属、量纲、窗口、对象组合与语义键。预览固定配置、图和控制修订、输入时段与试算证明；应用时复核并原子关闭旧分配、复制保留区间、增加新分配，记录持久变化与重算目标。系统委托覆盖各分配实际所需输入，旧 worker fence 失效。
- **Technology**：Go 规则编译和区间算法；PostgreSQL int8range 排斥约束与不可变配置／分配修订；现有监测任务、ClickHouse 和 S3 正式发布接缝。React 表单展示候选配置、试算、预览和任务状态，OpenAPI 作为同一契约。
- **Acceptance**：边界时刻使用新配置，边界前仍可读取原配置结果；历史修正发布新代次，逻辑结果身份稳定且旧代次可追溯。1小时改24小时或改所属对象需新通道。并发预览或权限变化拒绝过期提交；暂停／恢复与多配置历史仍按完整委托执行。

### 2026-11-12：配置分配、手动回算与历史代次

新增 migrations 20–21，配置保存为不可变候选；普通变更和明确历史修正分别验证语义身份、试算依据、当前输入授权及生效区间。修正中间区间时保留两侧分配，查询按当前分配切分旧根，受影响部分立即待更新；旧配置分配可按修订重建。多配置自动任务使用当前分配和完整执行委托。固定基准在普通配置变更中沿用原证据，重置必须明确历史修正。

手动回算按现有配置分配拆成共享任务，逐次检查发起者当前资格，不借用租户自动委托；暂停状态可执行且不会隐式恢复自动运行。取消后显式技术重试复核原范围和配置，再取得新 fence。当前实现仍要求回算范围已有生效分配，启用起点之前的无分配历史与退役前补算尚待补全。

历史代次列表、明细和来源接口读取永久封存依据，并复核输出及输入历史资格。撤销后保留原数值，标记 historical／revoked；子区间查询隐藏全代次计数。历史接口已写入中央权限、HTTP、OpenAPI 和 TS 客户端。

| 自验 | 结果与边界 |
| --- | --- |
| `TestDerivedConfigurationCandidatesAssignmentsAndIntent` | 通过，14.340秒；候选不自动生效、幂等、普通追溯禁止、历史三区间修正、旧图重建、未来边界与意图变化拒绝。 |
| `TestDerivedConfigurationCorrectionKeepsUnaffectedCurrentAndHistory` | 通过，28.098秒；真实输入下中间精度修正、两侧 current 保留、迟到数据按多配置重算、旧代次保留；暂停后跨三分配回算、取消／重试及无自动恢复。日志 `/tmp/infrapro-derived-configuration-real.log`。 |
| `TestDerivedFormalPublicationUsesSealedAttempts` | 扩展后通过，29.158秒；保留原发布故障、混合查询和导出测试，新增撤销后历史分页、部分范围统计隐藏、实际前驱来源和新旧代次列表。日志 `/tmp/infrapro-derived-history.log`。 |
| `pnpm check` | 通过，日志 `/tmp/infrapro-derived-history-check.log`；静态与契约检查，不代表浏览器验收。 |

### 接续实施设计：原依据回放与来源解释

- **What**：从指定历史代次、可选输出范围发起 historical_replay；验证原快照下的规范数值、精度、状态与统计依据，返回匹配报告及有界步骤样本。
- **How**：固定原代次、配置、编译计划、执行时钟、原输入清单及当时基准有效状态。共享任务逐块读取历史授权内证据，按原规则计算并与封存行比较；检查点支持技术重试。完成事务只写回放报告和任务状态，不写当前结果头、正式代次或失效范围。来源／回放访问在请求、分块与最终读取复核当前权限。
- **Technology**：Go 同一精确算子与规划器，PostgreSQL 不可变目标／验证报告，现有 monitoring_jobs 租约与取消；原始历史 Reader、S3 清单和 ClickHouse 正式块作为固定依据。
- **Acceptance**：撤销后有历史权限仍可验证旧结果；新增输入不进入回放；配置切换不改变原规则。逐项差异明确，重复请求不增任务，暂停或回放不会改变 current 根，缺失历史输入资格拒绝。

### 2026-11-12：原依据回放及历史授权自验

migration 22 保存不可变回放目标与验证报告。historical_replay 使用原配置／编译计划、执行时钟、原始采用清单与发布时基准状态；复算后逐项比较完整正式行 hash（数值、精度、状态、质量、区间、计数等），有界示例保留实际匹配身份和时间差。后台分块和检查点沿用共享任务；结果报告不更新 current 根，也不创建新正式代次。历史输入部分读取新增专用 ReplayPartUser，完整检查候选和区间型来源的历史授权。

- 扩展 `TestDerivedFormalPublicationUsesSealedAttempts` 通过（35.722秒，`/tmp/infrapro-derived-history-project.log`）。覆盖撤销后的精确回放、请求重放、current 与正式代次数量不变，以及项目仅获中间输出／前驱范围的部分回放。输出 history 加输入 read 不足以访问历史；补全输入 history 后可读取，撤销后明细、来源、完成的运行报告和幂等请求均拒绝。
- `TestDerivedFixedBaselineAppendRevokeAndReadopt` 扩展后通过（24.614秒，`/tmp/infrapro-derived-replay-baseline.log`）。验证发布时有效、后来撤销的基准复算仍为6；发布时已失效的基准复算保持 baseline_invalid，不因读取历史而复活。
- `pnpm check` 通过（`/tmp/infrapro-derived-replay-check.log`）。首次运行发现 OpenAPI JSON 规范化与权限生成器不同步，已重新生成并完整复验通过。

这些结果仍是小规模真实功能自验；多级派生、完整分析与新页面、运维恢复、规模／长时观察继续实施中。

### 接续实施设计：固定上游正式输入与递归资格

- **What**：正式派生可作为试算、持续计算、人工回算、历史分析及固定基准的输入；保留 null／状态、逻辑身份、代次和配置边界。支持最多8层合法依赖，缺少正式覆盖时明确待上游／需回算。
- **How**：先按规则计算直接候选范围，再解析所选上游范围的不可变正式清单；前驱／样本数量窗在固定上游时轴中寻找真实前缀。每层以实际固定配置展开输入范围，叶子由原始 Reader 提供同提交依据。运行固定上游代次引用和原始叶子快照，分块复核来源、当前资格和完整性；上游普通追加不改写运行清单，相关撤销／配置修正会即时使旧 current 失效。系统依旧只使用当前租户委托，用户任务依旧使用发起人的当前空间权限。永久日志按拓扑唤醒下游，未覆盖段不能冒充空窗口。
- **Technology**：Go 计算输入适配层位于 derivedmonitoring，复用公开 RawReader 和 resultstore；PostgreSQL 存不可变输入关联及短事务证明，S3 保存有界封存来源清单。业务算子继续只消费精确 Sample，不直接识别数据库或网络。查询／导出使用同一递归证明。
- **Acceptance**：两层可手算差分、8层组合、跨配置边界与部分区间等价；上游追加／补录／撤销驱动下游且旧 worker 无法覆盖新目标。输出和直接输入有权但任一递归叶子缺权仍拒绝，补齐实际前驱区间后允许；历史回放保持原代次与原基准解释。未覆盖上游必须列出缺口，明确提交获准依赖回算后才继续。

### 2026-11-12：递归正式输入与固定派生基准

migration 23 保存与原始叶子快照关联的不可变上游代次证明。统一输入读取层在试算、自动计算、回算和原依据回放间共享；值来自上游已发布的规范十进制，保留 null、状态、结果／代次身份及配置边界。当前查询、历史查询、来源和导出提交／下载沿递归证明复核，每层实际依赖范围使用原固定配置。自动委托显式包含中间通道及原始叶子。调度检查上游尚未完成的相关区间；上游等待使用现有 retry_wait，不消耗故障重试次数，永久变化日志可提前唤醒。

派生结果可用 result_id + generation_id 作为固定单点基准；参考窗同样固定正式上游清单。基准读取使用原采用集合，普通追加不改参考值，原始来源撤销后不因重新采用而复活。试算响应区分原始输入示例和派生输入示例，来源契约同步到 OpenAPI／TS。

| 自验 | 结果与边界 |
| --- | --- |
| `scripts/test-derived.mjs --database --contract --keep`（接入递归输入前） | 完整脚本通过；证据 `.artifacts/derived-monitoring/2026-11-12T13-09-46.216Z/`，日志 `/tmp/infrapro-derived-regression.log`。不能替代随后改动的复验。 |
| 实际前驱与多输入原始快照 | 前驱三组真实测试通过（60.471秒，`/tmp/infrapro-derived-exact-prefix.log`）；多通道快照子集及区间历史授权通过（12.17秒，`/tmp/infrapro-derived-multi-source-part.log`）。修正子集完整性版本及重载区间语义，避免其他输入的完整性版本误伤选择子集。 |
| `TestDerivedEightLayerFixedSourcesAndRecursiveRevocation` | 八层真实 PG/CH/S3 链路及递归项目历史授权通过（42.25秒，`/tmp/infrapro-derived-chain-and-host.log`）；包含空值传递、九层拒绝、叶子撤销即时失效、导出下载复核及原依据回放。新增拓扑重算断言随后复验中。 |
| `TestDerivedPublishedBaselineKeepsOriginalReferenceAndRevocation` | 通过（14.40秒，`/tmp/infrapro-derived-upstream-baselines.log`）；派生单点／参考窗固定、追加不进入原参考窗、撤销后 baseline_invalid。 |
| `TestCalculationSharedTaskLeaseFenceAndCancellation` | 扩展后通过；原 fence、取消和权限失效检查保留，六次上游等待不耗尽故障重试次数且可取消。日志 `/tmp/infrapro-derived-chain-and-host.log`。 |
| `pnpm check` | 通过，`/tmp/infrapro-derived-inputs-check.log`；仅静态、类型及契约检查。 |

所有记录均为自验证，当前仍为 In progress，未请求或声称人工验收。上游缺口的显式依赖回算、完整历史分析、新管理页面、运维恢复和规模／长时观察仍待完成。

### 接续实施设计：明确历史范围、依赖回算与退役边界

- **What**：补齐自动起点前的历史回算、退役前历史修正、明确依赖回算及启用时选择历史范围。无分配历史必须明确选择配置，预览列出新增分配与所有子任务，不静默套用当前配置。
- **How**：回算预览固定父通道及依赖通道的修订、配置、范围、授权和来源缺口；一次提交原子建立显式历史分配、用户子任务和依赖关系。下游等子任务完成后冻结输入，失败／取消保持可诊断，不冒充空数据。新增历史分配不扩大原自动输出委托；暂停／退役状态不因回算恢复。祖先退役同事务停止其所属派生通道，保存停止边界；恢复后保持暂停。
- **Technology**：Go 使用既有配置分配、依赖规划和任务宿主，PostgreSQL 保存不可变目标关系与修订，HTTP／OpenAPI／TS 显式表达所选配置和依赖范围。物理输入校验继续在事务外执行，短事务只复核范围、版本及证明。
- **Acceptance**：自动起点前只有显式选定配置的范围产生结果；未选择的历史不扩展。全部子任务在当前用户授权下执行，子任务失败时父任务等待／需处理。退役后不能生成停止边界后的新结果，获准历史修正仍可执行；祖先恢复不会自动续算。

### 接续实施设计：历史分析与成果

- **What**：补齐租户／项目独立的分析草稿、不可变执行版本、试算与多输出运行、完整成果版本、受影响说明、比较、归档和导出。分析不建立工程通道；同一输入授权不隐式共享其他空间成果。
- **How**：配置版本固定规范 DSL、绑定、参数及编译器／引擎版本；共享任务记录运行身份，固定原始快照和递归派生来源。所有输出块与来源清单验证后，短 PostgreSQL 事务同时写入成果版本与任务成功状态。追加仅标记旧成果受影响，撤销则阻止未完成运行发布；重算创建新版本。比较按固定输出、时刻及高精度数值执行。读取、比较、导出及下载逐边界复核当前空间与递归输入资格。
- **Technology**：Go 复用现有受限计算引擎、monitoring 任务宿主及 RawReader；PostgreSQL 保存空间归属、版本、收据和发布事实；ClickHouse 保存有类型的计算载荷；S3 保存不可变清单、来源和导出文件；OpenAPI 生成 TypeScript 契约，React/TanStack Router 页面复用现有共享组件。
- **Acceptance**：同一配置保存两版后旧运行仍使用原版；多输出中任一失败均无正式成果；追加后旧值保持、原因可见，新运行生成新成果版；撤销／撤权／项目归档分别正确限制执行与读取；比较列明配置、范围、输入和数值／状态差异；归档保留获准历史读取和导出。

八层正式依赖及撤销后的逐层重新发布已通过真实 PostgreSQL／双 ClickHouse／S3 验证（48.39 秒，`/tmp/infrapro-derived-backfill-dependencies.log`）。明确自动起点前历史配置与退役后修正复测通过（8.92 秒，同日志）；显式依赖回算等待及选定上游完成后唤醒通过（14.04 秒，`/tmp/infrapro-derived-backfill-dependencies-fix.log`）。


### 2026-11-12：历史分析、多输出成果、比较和精确导出

migration 25–28 增加空间独立的分析草稿／固定执行版本、多输出完整成果、比较目标和成果导出。运行首次冻结原始快照、递归正式输入和固定基准；全部指定输出同时发布。比较使用固定版本及精确整数／scale，差异支持完整分页。导出复用监测后台任务、CSV／Parquet 分卷与 S3 封存，下载逐边界复核成果和递归输入资格；导出后来源撤销使原下载失效。原成果在输入追加后保留值并列出受影响范围，归档保留获准历史读取和导出。

| 自验 | 结果与边界 |
| --- | --- |
| 多输出与成果导出 | 真实 PG／双 CH／S3 测试通过：多输出原子版本 16.34 秒、项目边界 11.83 秒；日志 `/tmp/infrapro-analysis-export-real.log`。 |
| 非因果分析 | `TestAnalysisNearestAndFutureFixedBasisRetainEvidence` 通过，11.57 秒；验证网格最近匹配、等距取早、偏差解释、未来基准、追加不改原基准、撤销后的成果与历史导出；日志 `/tmp/infrapro-analysis-causality-final.log`。 |
| 交互试算转后台 | 广泛复验发现 3 秒截止被误计为技术重试；新增真实任务回归先失败，再修复为 queued、无错误、尝试次数不增加、可立即领取。`TestDerivedInteractiveBudgetHandsOffWithoutFailure` 与八层链及非因果分析在 `-race` 下通过，152.735 秒；日志 `/tmp/infrapro-trial-handoff-green.log`。此前 `/tmp/infrapro-analysis-regression.log` 的失败不能计为通过。 |
| 三种 owner 目录 | 设施／构件／测点直属筛选和项目委托边界通过，7.376 秒；日志 `/tmp/infrapro-derived-owner-directory.log`。 |
| 界面与契约静态检查 | 新定义／通道／分析／成果／运行页面接入租户及项目真实路由；`pnpm check` 通过，日志 `/tmp/infrapro-derived-ui-check3.log`。随后浏览器修复与资源通用路径仍需复验。 |

浏览器验收使用隔离 PG schema、独立 CH database 和测试租户对象前缀；没有修改现有业务数据库。真实浏览器发现模板请求与保存并发会保存旧规则，已加入生成期间编辑／保存保护，正在复验全部流程。此处仍为 In progress，未进行人工验收。

### 2026-11-13：结果概况、共享队列与界面复验

- 新结果块封存精确分质量统计和末端索引，封存校验重建其总数、精确矩、极值和最新值。旧块无索引时仍回退扫描。完整覆盖块直接合并摘要，部分覆盖仍读取实际行。真实正式发布／撤销／历史回放回归已通过；摘要 JSON 往返后与逐行精确计算一致。
- 通道详情和列表显示固定指标名／当前配置、实际输入最后时间、已计算截至、最新状态／最后有效值、待处理范围。启用可明确选后续历史回算范围；启用后需独立预览并提交回算。
- 分析导出任务可从运行页面返回；详情、取消、重试和下载均沿既有当前资格复核，URL 保留 export_id。
- 迁移 29 保存租户队列领取位置；默认前台／历史各一槽，避免历史任务占满自动任务。任务租约／心跳／重试归宿主配置，退避为1/2/4/8/16秒加抖动。分析试算进入前台，完整分析进入历史队列。单任务入口保留导入发布优先级与30秒老化提升。
- derived.* 支持规则／图预算、试算行数／期限、样本窗、内存、写入块、临时盘、批量、试算和导出 TTL；参数及数值默认仍来自固定业务配置。临时盘及大窗外部排序的最终执行预算仍待完成。

证据归档于 `.artifacts/derived-monitoring/2026-11-13-runtime-storage/`：

| 检查 | 结果与边界 |
| --- | --- |
| Playwright 派生与分析四流程，desktop/mobile，串行 | 4 passed，2.7分钟。包含长名称、暗色、页面无横向溢出、未保存离开确认、错误焦点、真实试算／自动发布／退役恢复，以及比较和导出任务返回。后续就绪门禁尚需复验。 |
| 共享任务原 fence／租约／取消测试 | 通过，包含六次上游等待不消耗故障次数。 |
| 租户公平性与保留队列测试，race | 通过，10.11秒；历史任务阻塞时自动任务和分析试算仍执行，取消在两秒内返回。 |
| 部署预算 YAML／环境变量／CLI loader | 通过；拒绝非法并发、相互矛盾的内存预算、超限试算、租约和未知语义覆盖项。 |
| CLI 登录会话、项目路径、CSRF、预览和原幂等键 | 契约测试通过；项目拒绝直接返回，不回退租户入口；禁止向重定向目的地转发凭据、拒绝公开可读会话文件和尾随 JSON。尚需完整实际 CLI 场景复验。 |
| pnpm check | 通过。此后新增的恢复与基准传播修正另需复验。 |

### 接续实施：对账与跨存储恢复

备份沿既有 repeatable-read／pg_dump／ClickHouse native backup／独立对象归档执行。新增有限的 CalculationRecovery 接口；含派生数据时拒绝遗漏恢复 Provider。新备份清单记录 DSL、编译器、引擎和物理布局版本；恢复在写入前匹配支持版本，显式恢复 derived_results_local，逐副本同步，再核对不可变代次、artifact、封存 attempt、精确摘要、源快照／固定基准、递归上游、范围树、历史链、consumer／dirty位置。通过后才进入只读状态；restore open 再次核验后放开写入。

独立恢复样本已扩展为两层派生，第一层使用原始参考窗固定基准；原始批次撤销后保留旧代次及 baseline_invalid 新代次，第一层暂停、第二层退役。恢复后将逐代次比对明细和回放 hash，并复查项目不因恢复获得递归输入权限。该演练进行中，尚未通过。

前两次演练分别暴露测试 ClickHouse 内存上限和已知无效基准下游传播缺陷；失败不作为恢复证据。已重启隔离源测试节点释放积累内存，且修正“已发布的 baseline_invalid”与“运行期间新发生的撤销”的判别，继续复验。

独立恢复后续复验：`node scripts/test-monitoring-restore.mjs` 已通过，完整测试50.72秒。证据 `.artifacts/monitoring-restore/2026-11-12T16-46-33.247Z/`。归档181文件、1,430,843字节，备份2.18秒，恢复及检查22.10秒；源CH双节点、源S3和备份暂存S3暂停期间完成。原始当前22／历史122条、项目可见10条保持一致；4个正式派生代次、撤销固定基准及两层依赖逐代次回放匹配，暂停／退役状态保留，原会话全部失效；放开写入后成功导入新原始样本并查得23条。测试在独立PG、CH、Keeper、S3、Redis容器执行，仍处于同一本机故障域，不证明异地灾备或PITR，也不代替后续规模及长时观察。

### 2026-11-13 独立恢复扩展与真实旧版升级

- `node scripts/test-monitoring-restore.mjs`：通过；证据 `.artifacts/monitoring-restore/2026-11-12T16-55-49.768Z/`。源 CH、源 S3 和备份暂存服务暂停后，独立恢复 4 个派生代次、固定撤销基准、两层依赖和 40 行双输出分析产物；历史回放匹配。输出通道已有项目资源和数据权限，但输入范围／history 不足，递归读取仍拒绝。恢复后暂停／退役状态保留，新发布正常。本机恢复及核验 23.63 秒，不代表生产 PITR 或异地恢复 SLA。
- `node scripts/test-monitoring-restore.mjs --legacy`：通过；证据 `.artifacts/monitoring-restore/2026-11-12T17-01-38.976Z/`。以当前提交 `22e236ab4e8f86901572654511720e9b9d655872` 的 v12 源码归档编译旧测试，在独立容器真实恢复其备份，再用本工作树执行迁移。未创建 Git 分支或 worktree。94 张旧表按原列投影比较行数与 hash，无数据变化；直接通道仍为 direct/point，新增项目能力默认 false，没有样例派生业务数据，自定义角色仅原 profile 权限。当前 23 行／历史 123 行／项目可见 10 行，设备来源绑定、退役和删除状态保持。旧版迁移就绪检查明确拒绝新库。迁移及服务核验 4.51 秒，仅代表此合成样本。
- 真实 PG 运维状态查询通过：全局及指定租户均返回有界聚合。窗口到期计数只计 enabled 通道。启用预览和提交均检查依赖就绪；依赖故障时暂停仍成功并隔离旧任务。相关 race 测试分别 9.91 秒、12.81 秒。
- 后台完整试算增加按输出范围自适应分块、每块完整 halo、固定快照及持久恢复进度。示例保持有界；节点和输出计数覆盖完整范围。检查点超过宿主 256 KiB 时写入带 hash 的对象，PG 仅存引用。临时检查点清理、外部排序和百万输入完整链路仍在实现／验证，不据此宣称 DSV-22 完成。

## 2026-11-13 外部精确统计、历史回放与恢复补证

- `pnpm check` 第 8 轮通过：Go 检查、ESLint、TypeScript、OpenAPI 和权限目录同步。日志 `/tmp/infrapro-derived-check8.log`。
- 真实 PG/双 CH/S3 的 40,000 条大数值细小差值输入：均值、中位数、R7 P95、总体方差与独立有理数期望一致。第 16,384 条后的中途故障恢复复用两个已封存排序段；再次在结果写入前故障，复用完整窗口检查点；最终 6 个排序段及 4 个输出无重复或截断。race 日志 `/tmp/infrapro-analysis-sort-resume1.log`，93.28 秒。此项不是 DP-0 百万完整链路，也不是稳定性验收。
- 外部固定窗口历史回放：原始批次撤销并发布新输入后，仍按原固定快照得到中位数 23；回放不增加正式代次。race 日志 `/tmp/infrapro-replay-spill-physical2.log`，16.56 秒。
- 私有临时文件、磁盘配额、ENOSPC/EDQUOT 分类、排序段完整 hash/数量/顺序核验、多趟合并、封存前缀恢复及归并取消清理通过 race。日志 `/tmp/infrapro-sort-resume-unit.log`；取消目标 2 秒内。
- 固定中位数基准首次快照跨前台/后台切换保持不变；追加基准范围数据不进入既定基准。基准撤销、重新采用、正式发布、旧依据重放回归通过。日志 `/tmp/infrapro-baseline-spill-regression1.log`。
- 第 7 轮独立备份恢复通过：源 CH/S3/备份服务暂停，目标使用独立容器；备份包含 executable SHA256/Go 版本及精确数值恢复夹具；4 个历史代次回放一致，40 条多输出分析结果保持。备份 2.871 秒，恢复与验证 23.680 秒。证据 `.artifacts/monitoring-restore/2026-11-12T17-30-51.474Z/`，日志 `/tmp/infrapro-derived-independent-restore7.log`。
- 历史区间树验证覆盖被新根替代的旧根；删除仅旧根引用的对象后，最新根仍可读但完整历史核验失败。日志 `/tmp/infrapro-replay-spill-physical1.log` 中 `TestDerivedResultPhysicalProtocol` 单项通过；同一命令的第一版回放测试因夹具缺少显式窗口 max_gap/max_age 失败，修正后以上第二轮通过，不能把第一轮整体记为通过。
- 浏览器第 12 轮 4 项通过，含桌面/窄屏定义到正式结果、退役恢复、多输出成果比较及导出离开返回；日志 `/tmp/infrapro-derived-ui-browser12.log`。第 11 轮错误环境变量导致跳过，不计通过。

仍在实施完整目标。输入依据完整导出、受控生命周期清理、已知缺失结果隔离、百万完整链路、混合/持续观察及最终操作手册与验收材料尚待闭合。用户手动验收未执行。

### 2026-11-13 输入证据、保留策略与局部成果权限

已完成输入依据 CSV/Parquet 导出（固定规则、配置、来源身份、精确值、质量及递归分组）、结果完整性隔离、完成任务临时产物回收和过期导出失效。完整成果与历史来源永久保留；仍可重试的任务检查点不回收。临时目录通过独占文件锁保护运行中的排序。

- `pnpm check` 第 11 轮通过，日志 `/tmp/infrapro-derived-check11.log`。
- 实际 PG/双 CH/S3：多输出结果及输入导出、原始撤销后历史依据、项目局部输入授权和已取得下载对象的撤权阻断通过；40,000 条精确排序恢复、完成任务临时清理、完整区间查询通过。日志 `/tmp/infrapro-input-evidence-physical1.log`，294.823 秒。
- 固定基准与 whole_range 项目成果复验通过，日志 `/tmp/infrapro-input-evidence-physical2.log`。同轮八层试算在 3 秒边界将授权适配器隐藏的数据库超时误记为权限变化，因此该整轮失败；共享宿主现统一处理交互任务期限，正在复验授权检查及实际执行两个超时阶段。
- 修复 whole_range 在原输入区间末端标记的合法结果被完整性检查误隔离、查询半开边界丢弃的问题。仅允许固定 whole_range 原始区间末端，普通结果仍使用半开区间。纯边界回归和上述实际大窗口测试通过。

接续局部成果权限设计：Go 依据不可变 run_rule、选中输出和查询范围投影原始／上游依赖及基准；存储读取仍使用原 PG/S3 固定快照。共享请求、导出和下载在短事务复核同一投影，未查询输出不扩大所需数据授权；前驱、容差和基准不能缩减。局部结果不返回全成果的隐藏计数或无关变更记录。React/TypeScript 展示局部范围状态。人工验收应验证：仅获准片段可以查询／导出，扩大范围失败；差分首点仍需要范围外前驱；whole_range 仍要求完整 source_range；撤销权限后旧分页与下载均失败。

### 2026-11-13 局部查询、浏览器与百万链路首跑

- `TestAnalysisSelectionRequiresOnlyCompleteSelectedDependencies` 在真实PG/双CH/S3通过，23.71秒（`/tmp/infrapro-analysis-selection-physical2.log`）。仅一分钟输入授权可读值5并比较、CSV/Parquet导出结果和原依据；扩大范围或请求需范围外前驱的差分失败，撤权后已获取下载也在写出前失败。随后选定范围的递归基准事件追踪复用完整分析依赖展开，仍待随最终物理回归复验。
- `TestDerivedEightLayerFixedSourcesAndRecursiveRevocation` 与交互超时两路径回归通过，合计234.528秒（`/tmp/infrapro-input-evidence-physical3.log`）。交互任务宿主能区分自己的3秒预算到期与真实撤权，即使下层将数据库超时包装为通用授权错误，也转后台完整执行并重新授权。
- Playwright四流程桌面/375px窄屏最终通过，3.3分钟，`/tmp/infrapro-derived-ui-browser15.log`。包含发布历史输入证据CSV、成果结果/输入CSV、导航返回、暗色、长名称、键盘/焦点及退役恢复。先前两轮失败发现页签定位、重复表名和窄屏发布时间按钮被相邻单元格遮挡，均已修正并复验。
- 对账新增配置绑定与永久依赖索引一致性、活动/未来图循环与深度、通道头及图状态存在性。真实损坏注入检查 `TestDerivedRecoveryRejectsMissingDependencyAndActiveCycle` 通过8.97秒，`/tmp/infrapro-derived-recovery-selection-physical4.log`。此命令的第二个正则没有匹配选择权限测试，不将其计为局部权限复验。
- 百万实际链路冻结目标、失败记录与环境修正见 `derived-monitoring-dp0-load.md`。仍在执行，2小时混合和24小时持续观察尚未开始。

诊断新增物理输入行/编码字节、实际计算输出数及状态分类、阶段耗时、进程堆采样峰值和已观测临时文件峰值。物理输入累计包括halo、重新分块和重读，不能当作唯一逻辑输入数；堆是当前worker进程，不宣称单任务独占内存。标签仅固定阶段/状态集合，任务和版本标识仅在结构化日志字段中。对账另分当前根引用的完整载荷块、仅历史引用的派生块、不可变分析载荷；部分块仍被当前引用时整块计入当前物理保留成本，不把其未选中行重复计为历史字节。上述诊断扩展编译/单元通过，最终真实恢复和整体复验待执行。

### 2026-11-13 真实输入边界与持续推进复验

- 最新定向物理回归通过，288.470秒，日志 `/tmp/infrapro-derived-latest-physical1.log`：局部成果完整依赖23.40秒、40,000条精确外部窗口和检查点恢复204.27秒、下载首块后撤销15.63秒、依赖索引／活动图损坏拒绝8.96秒、16个独立输入正式结果和原依据回放18.65秒、后台完整试算断点恢复15.11秒。该命令的外部窗口回放正则未匹配，不计为本轮重复覆盖。
- 下载中途撤销以600行CSV实际产生多于128 KiB内容；写出第一块后撤销原始采用，第二块写出前重新校验失败，实际仅写出一个128 KiB块。
- `TestDerivedSixteenDistinctInputsFormalAndReplay` 使用16个不同原始通道、每个3个时刻，共48条实际输入；正式输出120/136/152，固定原依据回放一致。
- 两层primary_input自动推进专项通过，测试19.63秒、包21.982秒，日志 `/tmp/infrapro-derived-horizon-physical2.log`。子通道先发布上游已有前缀，未计算尾部继续保留dirty；稀疏新数据到达时补齐同配置的空白范围，两层继续发布8及28。迁移33新增发布上界索引，实际完整快照读取仍复核覆盖、当前配置和递归依据。
- 局部成果浏览器桌面／手机两项通过，2.0分钟，日志 `/tmp/infrapro-derived-ui-partial2.log`。真实项目仅一分钟原值授权可查、比较和下载；请求差分所需范围外前驱、扩大时间范围均拒绝；撤权清除旧表格及下载入口。第一轮桌面失败发现权限资格刷新会重置正在填写的片段表单，保持选择组件挂载后复验通过。
- `pnpm check` 第17轮通过，日志 `/tmp/infrapro-derived-check17.log`。本轮仍不是最终固定源码全范围自验。

### 2026-11-13 CLI实际操作、导出事务及验收材料

- 实际编译CLI通过伪终端完成隐藏密码输入和HTTP登录，0600会话文件只用于本次运行，结束删除；随后真实预览／暂停／恢复、回算等待、原依据回放比较和只读check/status/reconcile/recovery通过。流程27.6秒，连同浏览器启动41.2秒；证据 `.artifacts/derived-cli/2026-11-12T19-55-58Z/`，二进制SHA-256保存在该目录。最初两次失败是测试使用了错误的响应字段名，按实际OpenAPI契约修正后通过。
- 第四轮持续短测完成126.128秒混合阶段和120.337秒观察阶段，最终全历史导出时任务状态查询触发账户行锁5秒超时，因此整轮失败。数据库日志确认导出逐代次复核持有同一事务过久；没有把失败改记为通过或降低自动可见／查询延迟门槛。
- 导出提交的资格预检限500毫秒，超时仅交由后台完整核对，超时结果不能当作有效证明。共享导出不再每个输出缓冲重复发现前驱并读取S3；使用服务端产生、绑定准确代次／配置／快照／范围的读取证明。正式发布和每个128 KiB下载块仍复核当前权限、来源变化、配置、完整性和基准。
- 随后将同一事务中的代次身份／配置分配、输出数据授权和原始快照变化检查改为批量查询；每份快照保留自己的source_position、候选范围与完整区间授权，不缓存跨事务权限结果。快照内容仍逐份校验hash。多代次共享同一固定基准时按配置去重复核。
- 三项真实存储回归通过69.128秒：预检到期转后台后撤销阻断14.27秒、下载首个128 KiB块后撤销15.57秒、正式发布／混合与仅派生CSV及Parquet／原子故障恢复37.32秒。日志 `/tmp/infrapro-derived-batch-physical1.log`。
- 八层正式链、递归撤销及原依据回放187.56秒，区间型固定快照／部分项目授权／history／重新采用13.13秒，合计202.778秒通过，日志 `/tmp/infrapro-derived-batch-physical2.log`。新增批量快照核验覆盖：完整interval授权不能截断、同快照无关输入不扩大授权、范围外追加不失效、read不能代替export、原采用撤销即阻断。
- `pnpm check` 第19、20轮通过；第20轮包括批量复核、百分位输入提示修正、合成验收脚本及新浏览器用例的静态检查，日志 `/tmp/infrapro-derived-check20.log`。不代替最终真实浏览器和负载组合执行。
- 新增 `pnpm derived:demo` 及[MA-01–MA-20人工验收手册](../acceptance/derived-monitoring-and-analysis.md)。合成会话在真实后端核对A/B固定期望，随机账号与独立存储仅供本次会话；停止后清理本次数据库和对象，保留本地材料。首轮16个计算样本通过，最后的C_MEAN因上游未显式覆盖前30分钟候选空白而拒绝；正在修正验收夹具的上游计算起点，尚未把整套样本标记准备完成。

### 2026-11-13 查询初始／最终批量资格复验

- 持续短测第五至第七轮均保留为失败记录，详见DP-0负载文档。第七轮完整2620行一致，自动P95=9.163秒，查询P95=2.184秒，仍需修复查询目标。
- 读取开始时按准确选中范围一次授权输出，随后解析每份不可变依赖；读取载荷前及返回前分别批量复核当前输出、递归输入、配置分配、完整性和固定基准。只有相关变化才回退逐片段判断pending；不缓存跨事务资格。无前驱、无区间输入的原始片段解析没有外部I/O，可在同一个授权事务完成。
- 定向真实PG／双CH／S3 race回归通过：历史配置修正保留未改区间与旧历史27.70秒，正式发布／封存尝试／故障及混合导出35.86秒，区间型快照／部分范围／撤销和重新采用13.13秒，日志`/tmp/infrapro-derived-read-batch-physical3.log`。纯规则及监测race通过，日志`/tmp/infrapro-derived-read-batch-unit2.log`。随后增加根代次固定计划hash匹配检查，与递归输入路径一致，随下一轮及最终组合复验。

### 2026-11-13 短测通过与合成验收入口复验

- 第八轮持续短测通过，证据 `.artifacts/derived-soak/2026-11-12T20-45-16.699Z/`：混合121.329秒、观察120.192秒，查询P95为1.709秒、自动可见P95为7.549秒；1048条原始输入、2620条当前结果，108次查询、4轮后台任务及5份同一固定快照导出逐行一致。观察峰值Go堆13,541,592字节、任务临时文件263,894字节。`full_qualification=false`，不代替真实26小时。
- 新增宿主进程树采样，只记录本次Go测试进程及其子进程的PID、PPID、CPU百分比和RSS，不记录命令行、环境或用户信息；与Docker采样一起归档。RSS求和可能重复计算共享页，CPU是操作系统报告的进程生命周期比例；不把它们误报为隔离生产API／worker容量。
- 合成验收会话 `.artifacts/derived-demo/2026-11-12T20-50-48.727Z/` 的17个派生通道全部按独立期望核对，`acceptance.json` 为 `complete: true`。桌面1365和手机375宽度分别核对P95、C_MEAN三行，截图及 `browser-verification.json` 保留；页面无横向溢出、无脚本错误。会话已正常停止并清理其专属后端数据，交付时另起新会话。
- 浏览器发现首次取得权限版本会因查询键变化重置刚打开的历史页签。查询键现使用客户端权限epoch；首次确认版本保持已获准页面，实际版本改变、拒绝或离开作用域仍递增epoch并清空旧缓存。已补入首次进入立即操作历史页签的回归，完整撤权流程随最终浏览器组合复验。

### 2026-11-13 最终浏览器及并发发布补证

- `pnpm check` 第22轮通过（`/tmp/infrapro-derived-check22.log`），含首次权限版本修复和宿主资源采样；随后新增并发测试尚需最终静态检查。
- 浏览器第16轮7项通过、2项失败、1项跳过（`/tmp/infrapro-derived-ui-browser16.log`）：桌面／手机的建档至正式结果、完整多输出分析、局部成果授权及撤权均通过；实际CLI桌面通过，手机不重复执行终端。两项失败均为新分位数用例漏填固定版本发布原因，真实页面正确阻止无原因操作。补齐测试步骤后，仅复验这两项，49.0秒全部通过（`/tmp/infrapro-derived-ui-percentile2.log`）。两轮间前端及后端生产代码未变；合计九项真实流程通过，不能把第一轮本身改记为全通过。
- 新增 `TestDerivedConcurrentPublicationRechecksAndMerges`，在真实S3历史清单PUT处暂停一个worker；此时结果已封存、当前头已读取且不持有PG事务。并行执行无关追加、原始批次撤销、配置历史更正、另一不相交范围发布，然后恢复旧worker。四项race通过，分别16.22／14.63／16.99／17.89秒，总65.73秒（包67.695秒），日志`/tmp/infrapro-derived-concurrent-publication1.log`。新增dirty保留并收敛到5行；撤销立即不再返回旧current；更正取消旧任务并发布24位精度；不相交发布重读结果头并保留两个正式代次。
- 新增两个不同账号同时提交相反依赖的检查。保持固定输出的业务身份校验：不兼容替换可以先以`new_channel_required`拒绝，不能为触达图检查而放宽R-05；达到图检查的环以`dependency_cycle`拒绝。最终断言最多一个方向提交且拒绝请求无残留配置；此项随完整组合执行。纯图环／八层上限及真实恢复异常图另有独立检查，不把较早的身份拒绝伪称为图锁证据。

### 2026-11-13 完整组合中的完整性错误契约修复

第一轮最终组合入口 `node scripts/test-derived.mjs --contract --database --keep` 的证据目录为 `.artifacts/derived-monitoring/2026-11-12T21-09-50.195Z/`。规则／事务／HTTP与物理数值组通过，真实完整性用例发现：维护任务确认缺块并隔离之后，批量资格检查返回派生领域Error，统一统计入口的存储错误适配将其转成通用`monitoring_storage_unavailable`，没有保留明确的`result_integrity_unavailable`。没有把该轮记为通过。

修复将单结果和批量结果不可用判定统一为共享监测公开Error类型；不更改隔离条件、授权、读取资格或数值路径。原真实用例继续覆盖明细、latest／overview／statistics／trends、固定历史及旧导出下载，并进一步确认重新计算生成新代次后恢复可用、旧损坏历史仍保持隔离。测试断言补充入口名称，避免失败时丢失具体路径。修复发生在第一轮测试二进制编译之后；该轮其他结果保留为旧二进制证据，修复后须独立复验并进入最终固定源码组合。

完整组合第一轮已结束，仅上述一个用例失败；其余实际存储用例通过，resources组总计1045.151秒。修复后独立race复验 `TestDerivedMissingBlockQuarantinesCachesAndRepairsByNewGeneration` 通过（用例24.22秒、包26.675秒），证据`/tmp/infrapro-derived-integrity-final1.log`。最终完整组合仍待使用修复后的固定源码重新执行。

### 2026-11-13 原始监测兼容性中的对象重启就绪修正

完整原始监测入口 `node scripts/test-monitoring.mjs --keep` 第一轮在冷分区恢复检查失败（`.artifacts/monitoring-contract/2026-11-12T21-34-39.730Z/`）：停掉S3时读取明确失败，重启后的桶列表先恢复，但SeaweedFS日志显示master选主与volume登记尚未完成，随后立即执行的CH查询达到既有15秒读取超时。不是完整数据返回成功或缺块被当成空结果。

修正仅作用于故障实验室：断连前写入128 KiB实际对象，重启后在原20秒就绪预算内读回并逐字节核对，再执行原CH完整性检查；保留断连必须失败的断言。桶列表仍供初始元数据接口检查，不能独自作为冷分区载荷已恢复的证据。产品查询超时、数值和授权没有调整；第二轮完整兼容性检查继续执行。

第二轮完整存储契约通过（`.artifacts/monitoring-contract/2026-11-12T21-37-06.607Z/`，包55.151秒），其中真实冷分区断连与恢复35.07秒。随后业务组22项通过、5项失败、1项独立恢复跳过，包298.557秒；失败集中于统计／导出／摘要修复／项目导出夹具仍假设`RunOne`必然领取当前请求。

定向复现明确记录：失败的是旧任务`7cf274b3-8767-4f60-bda7-914c663fbeed`，被等待的新统计任务`a8e9882e-cb22-4779-b904-da2ac809cf8a`仍为queued（`/tmp/infrapro-monitoring-summary-diagnosis1.log`）。新公平调度可先运行维护或旧快照任务，旧快照在撤销后正确拒绝。夹具改为按返回JobID调用同一领取／租约／fence协议的`RunTask`，保留全部数值、撤权、原子性和故障断言；公平领取与前后台保留槽由独立真实PG任务宿主测试继续覆盖。第三轮复用第二轮已经通过的物理契约，重新执行完整原始监测业务、客户端及HTTP／CLI组，未修改产品队列规则。

`pnpm check` 第23轮与`pnpm build` 最终第二轮均通过（`/tmp/infrapro-derived-check23.log`、`/tmp/infrapro-derived-build-final2.log`）。构建包含API、独立worker、CLI和前端；后续修改仅为上述测试等待方式及记录。已有路由生成器循环依赖提示及图表包体积提示为非失败warning。

第三轮已通过原有五个任务顺序相关场景中的统计、固定快照导出、摘要修复及项目撤权导出；归档导出也完成并可下载，但其最后一步直接更新`expires_at`被v28／v32的导出身份不可变触发器拒绝。过期夹具现先断言该直接修改必须以23514拒绝，再仅在本次独立schema内的一个事务中准备已过期数据并恢复同一个触发器，最后保留`export_expired`断言；没有放松产品到期身份约束。此修改发生于第三轮编译后，定向复验结果单独记录。

第三轮结束为26项通过、上述过期夹具1项失败、独立恢复1项跳过（包306.738秒，`/tmp/infrapro-monitoring-final-compat3.log`）。修正后定向真实race通过：`TestMonitoringExportCommitFailureAndArchivedProject` 13.09秒、包15.431秒；随后所有HTTP子包与CLI检查通过，HTTP主包16.498秒（`/tmp/infrapro-monitoring-final-compat4-target.log`）。第三轮与该定向复验合计27项原始业务检查通过，产品代码相同；不将第三轮日志本身改记为通过。独立恢复另行使用v33真实目标执行。客户端4项与原始数值规则均通过；存储故障契约沿用第二轮通过证据。


### 2026-11-13 全部监测浏览器组合与局部比较范围修正

第一轮原始／派生组合结束为12项通过、7项失败、1项跳过，10.5分钟（`/tmp/infrapro-monitoring-derived-ui-final1.log`）。九项派生真实流程全部通过，手机不重复执行终端CLI；原始导入冲突桌面／手机及手机深链接通过。其余原始流程仍定位旧标题“选择测点与通道”或未切换“按测点浏览”，停止在操作入口，不能视为对应业务已复验。测试现按新统一通道目录进入保留的测点视图，明细表名更新为“监测明细”，同时断言导入映射仅能选择直接观测；保留原精度、分页、错误重试、授权撤销和键盘断言。

截图复核另外发现：局部成果的值和导出只有获准一分钟，但比较任务回执的输出范围仍取整份不可变版本。新任务改为记录两侧实际获准片段的并集；读取时也按保存的片段重新裁剪，覆盖旧的排队中／已完成记录、幂等重试和运行列表，而不重写历史证据。局部比较的范围变化标记仅描述获准输出范围。物理测试扩展为创建后断言、模拟旧记录、查询／重试／列表与完整性不变断言；浏览器增加明确起止时间断言。复验进行中，尚未将这一轮记为最终通过。

`pnpm check` 第24轮通过（`/tmp/infrapro-derived-check24.log`），包含范围修复及上述回归断言。

范围修复后两项真实PG／双CH／S3 race回归通过（`/tmp/infrapro-derived-selection-final2.log`）：完整多输出／变化依据／成果比较49.50秒，局部完整依赖与旧回执范围裁剪24.03秒，包75.941秒。创建、旧排队回执、幂等重试、完成后读取、列表及两侧版本均符合一分钟范围，原不可变记录保持原值；四种局部导出仍各一行。

`pnpm build` 最终第三轮通过（`/tmp/infrapro-derived-build-final3.log`）；230个产物的SHA-256记录于 `.artifacts/derived-delivery/2026-11-13/build-manifest.json`。生产源码包含局部比较范围修复。

第二轮原始／派生浏览器组合结束为17项通过、2项失败、1项跳过，10.7分钟（`/tmp/infrapro-monitoring-derived-ui-final2.log`）。九项派生流程通过，包含新增的一分钟比较回执断言；桌面和手机截图均已复核。原始分页／错误保留、项目授权／归档／撤权、重复／冲突和深链接／枚举／键盘共八项通过。剩余两项在导入映射新断言处失败：测试误将页内“选择监测通道”region定位为dialog。改为实际有名称的region后，仅复验这两个完整导入流程；产品源码未因这两项失败修改。完整截图、失败上下文与trace保留在 `.artifacts/derived-delivery/2026-11-13/browser-final2/`，不会把此轮本身改记为全通过。

导入定向第三条命令的过滤表达式未匹配测试，未执行任何用例；第四轮定位到正确region后，精确label文本查询仍未匹配嵌套select。失败快照明确存在名称为“来源类型”的disabled combobox且direct已选中，改用该实际可访问角色断言后继续第五轮。两轮日志与失败截图均保留，不计为产品检查通过。

导入第五轮两项通过，桌面42.3秒、手机41.4秒，总1.6分钟（`/tmp/infrapro-monitoring-ui-import-final5.log`），完整覆盖上传、映射、精确原始明细／统计、导出丢失响应后的同请求重试、下载、撤销及重新采用。与第二轮17项通过合计19项实际流程通过，另1项手机CLI按设计不重复。两轮生产源码相同；这是组合加定向复验的证据，不修改此前失败结论。


### 2026-11-13 v33独立恢复与旧库升级最终复验

v33独立恢复通过，用例58.44秒、包60.758秒（`/tmp/infrapro-derived-restore-final8.log`）；完整证据 `.artifacts/monitoring-restore/2026-11-12T22-26-19.175Z/`。使用与源不同的PG／双CH／S3，源存储暂停后验证恢复目标，包含当前与历史代次、固定基准、多输出成果、权限状态和原引擎回放。目标专属容器与卷在完成后清理，用户业务环境未改动。旧v12升级另行执行。

旧库最终复验全部通过（`/tmp/infrapro-derived-legacy-final3.log`，证据 `.artifacts/monitoring-restore/2026-11-12T22-27-49.651Z/`）：旧v12实际备份恢复17.79秒，升级至v33及兼容核对4.71秒，旧二进制就绪拒绝0.01秒。94张旧表原有列／行hash一致，23条当前、123条历史、10条项目可见样本保持；设备绑定及自定义角色不变，新项目计算能力默认false。仅使用固定提交的归档源码作为测试输入，没有创建Git分支或worktree。

备份清单复核发现，Go测试及不携带Git元数据的容器构建可能没有`vcs.modified`设置；此前bool零值会错误报告`source_modified:false`。现仅在构建信息明确提供true／false时填写该可选字段，缺失表示未知，仍保留实际可执行文件SHA-256和大小。兼容读取旧清单中的bool，不改变迁移、存储与恢复数据；真实恢复复验及重新检查正在执行。

构建身份修正后v33独立恢复再次通过，用例58.74秒、包60.691秒，证据 `.artifacts/monitoring-restore/2026-11-12T22-29-00.814Z/`（`/tmp/infrapro-derived-restore-final9.log`）。实际清单无VCS设置时不再填写`source_modified`，保留77,031,170字节测试可执行文件的SHA-256；4个派生历史代次回放、40行多输出成果保留、源服务暂停验证均通过。`pnpm check` 第25轮及`pnpm build` 最终第四轮通过；构建清单已更新。


### 2026-11-13 实际HTTP容器部署最终复验

`pnpm test:http` 通过全部11项检查（`/tmp/infrapro-derived-http-final1.log`）。使用当前源码实际构建Linux API／worker、CLI和前端镜像，独立PG／Redis迁移及权限同步后，经普通HTTP反向代理验证CLI初始化、深路由、凭据／缓存／依赖故障恢复、严格CSP、工程配置／资源、设备和项目等受影响部署路径。此组验证部署兼容；完整派生存储功能由独立浏览器及物理组覆盖。三个实际镜像ID保存在 `.artifacts/derived-delivery/2026-11-13/http-deployment-images.json`。测试专属容器、网络和卷已正常清理。

完整DP-0入口 `node scripts/test-derived-load.mjs` 已开始，按冻结环境依次检查交互目标及四个百万算子，P95尾部另核对百万活动窗口和百万加一明确拒绝。未启动其他物理／高负载检查与之竞争；结果尚未产生，不能宣称本组通过。

### 2026-11-13 用户暂缓性能验证，保留功能交付

用户通过侧聊授权传达本轮范围调整后，立即检查运行进程。负载驱动及Go测试进程均已退出；原组合在23:13:55Z以1退出，没有额外终止业务服务。交互项及四项百万结果／输入依据核对的原始记录保留，但最大活动窗口夹具在发布规则时返回`invalid_rule`，组合为失败且`fullQualification=false`。不复跑此项，不启动修正计时后的速率复测；26小时观察从未开始。详细历史见[负载记录](derived-monitoring-dp0-load.md)。

`pnpm check`第26轮通过，涵盖Go格式／vet、ESLint、TypeScript、OpenAPI和权限目录。最终功能组合入口`node scripts/test-derived.mjs --contract --database --keep`已在新范围下启动，证据 `.artifacts/derived-monitoring/2026-11-12T23-17-04.881Z/`；不启用`derivedload`或`derivedsoak`标签。仍须完成此功能组合、准备新的独立合成验收入口，并记录最终源码与交付清单。

最终功能组合期间，主机电源日志记录07:22:04合盖休眠及其后多次维护唤醒，至07:41:57完整唤醒。`TestAnalysisNearestAndFutureFixedBasisRetainEvidence`在依据导出处遇到S3请求时间偏差，夹具清理也报告`The difference between the request time and the server's time is too large`。这是实际失败记录，不能改记通过。当前主机与S3容器UTC秒一致；为当前检查进程设置保持唤醒，不改变签名校验、业务代码或测试断言。其余功能组继续执行，完成后仅对受影响用例进行真实存储复验；证据保存在交付目录`functional-regression-sleep-evidence.txt`。

同一组合的派生缺块检查还暴露测试准备依赖：旧导出助手曾顺便领取原始摘要任务，改用明确Job ID后不会再执行无关任务；夹具未先完成原始摘要，对账因摘要未就绪同时隔离原始通道，读取过期试算遂返回`monitoring_integrity_unavailable`。现仅修正夹具：在制造派生缺块前明确生成原始概况，并要求原始只读对账为passed；核验后要求唯一发现是`calculation_result_integrity_unavailable`。所有缺块、缓存、历史、下载、过期凭据和新代次修复断言保留，未修改业务代码或放宽完整性要求。此修正与受休眠影响的分析用例一同定向复验。

### 2026-11-13 最终功能自验闭合

最终组合第二轮的规则／数值／规划、PG事务、HTTP、资源兼容和权限组全部通过；真实数值存储2项及结果物理协议1项通过。真实业务组为31项通过、2项失败、旧库升级1项跳过，包1066.891秒；原始结果保留为failed，不改写日志。旧库升级已由独立v12恢复→v33流程通过。

两项定向真实PG／双CH／S3、race复验均通过（`/tmp/infrapro-derived-functional-target-final1.log`）：历史非因果来源及固定基准25.87秒；派生缺块隔离、临时保留、缓存／旧下载阻断及新代次修复25.39秒，包54.072秒。产品源码与组合第二轮一致，只修正了缺块测试的原始摘要准备并加强发现类型断言。合计33项真实存储业务用例全部获得通过证据，两个原失败原因已处理。`pnpm check`第27轮也通过；当前产品源码与最终第四轮构建一致。

功能自验按以上组合及定向证据判定通过；不将性能、压力或长时稳定性列入通过项。当前正在准备并检查新的独立合成验收会话，人工验收仍未开始。机器可读证据索引为 `.artifacts/derived-delivery/2026-11-13/functional-evidence.json`。

### 最终访问与交付材料

新会话 `.artifacts/derived-demo/2026-11-13T00-00-44.734Z/` 已就绪：2个原始通道、17个派生通道全部核对，`acceptance.json`为`complete:true`。真实浏览器桌面1365和手机375宽度均加载17条通道当前状态，分别逐行核对P95与C_MEAN，无横向溢出和脚本错误；6张截图及核对JSON保存在 `.artifacts/derived-delivery/2026-11-13/demo-final-screenshots/`。

访问 `http://127.0.0.1:15373`；账号资料为该会话目录的`login.json`（0600），不复制密码到交付日志。保持合成会话运行供用户验收；它只使用专属schema、CH database和S3前缀。停止后重新运行`pnpm derived:demo`可创建新会话，详见[人工验收手册](../acceptance/derived-monitoring-and-analysis.md)。

全部功能、必要自验、启动／访问及MA-01–20操作材料已经闭合，状态为Awaiting acceptance。性能和压力组合保留原失败／部分证据，26小时观察未开始，均按用户决定暂缓且未验证。机器可读交付入口为 `.artifacts/derived-delivery/2026-11-13/functional-evidence.json` 与`final-manifest.json`。未替用户确认人工验收。

最终源码校验发现，验收会话启动会重新生成`routeTree.gen.ts`；其当前SHA-256为`9e3db7527f466c4e284d575645c1e436f67bb8bb0af0bcae2e5826122c5cffd5`，与最终构建后22:34的源码快照一致，与23:17功能组合启动时的CLI生成文件hash不同。最终全部业务／构建源码逐文件与22:34快照一致，当前生成路由也已用于新的桌面／手机浏览器核对；该生成文件差异单列在最终清单，不将其隐瞒为零差异。测试源码另有明确的夹具准备修正，文档反映用户调整后的范围。
