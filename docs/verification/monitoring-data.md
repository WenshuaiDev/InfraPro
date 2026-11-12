# 监测数据开发与自验记录

日期：2026-11-12。功能状态：**Awaiting acceptance（本机功能自验通过，待用户验收）**。方案已获用户确认；人工验收尚未确认，不标记 Accepted。

执行基线：[详细开发方案](../requirements/monitoring-data-development-plan.md)。当前 checkout 为用户指定的 worktree，基线 `cd4820fd6cc10dc7bb6fe5717c099ac5837062ac`；没有创建分支或 worktree。

## 已确认的本轮范围

用户明确没有真实样本与部署目标，要求自行生成样本、优先本机功能。因此合成样本是本轮输入，不冒充实测业务数据。实际部署选型、真实样本适配、P-1–P-3、目标混合负载和长期稳定性留待后续专项验证。功能正确性、跨存储故障与本机实际恢复仍需通过。

## 阶段状态

| 阶段 | 状态 | 证据／剩余工作 |
| --- | --- | --- |
| S-01 | 本轮本机范围通过 | `pnpm test:monitoring:contract --keep`，含 race 检测；完整证据 `.artifacts/monitoring-contract/2026-11-12T01-38-28.546Z/`。 |
| S-02 | 阶段自验通过 | `pnpm check`；迁移全包 race 61.737 秒；监测授权与永久引用、归档、旧设备引用回归通过。日志 `.artifacts/monitoring-s02-*.log`。 |
| S-03 | 核心链路阶段自验通过 | 真实 CSV 上传／封存／校验／CH／原子清单／纳秒明细；独立 worker 和恢复测试。`pnpm check`、集成 race 48.935 秒，见下文。 |
| S-04 | 阶段自验通过 | XLSX／宽表、32 MiB 续传、重复核对、撤销／再次采用、部分范围元数据和下载中撤权；真实组件 race 通过。 |
| S-05 | 核心查询阶段自验通过 | 增量摘要、概况／最新／趋势／精确统计、日历组、部分授权与后台查询；真实组件 race 133.675 秒。 |
| S-06 | 阶段自验通过 | 真实组件 race 200.366 秒；8 项桌面／窄屏场景分组通过，含丢响应重试、再次采用、授权撤销和归档。 |
| S-07 | 本机阶段自验通过 | 真实布局／在线分层、对账隔离修复、复制与迁移故障、独立实际恢复及开放写入；证据见下文。 |
| S-08 | 后续专项 | 依用户调整，目标硬件和大规模验证不属于本轮本机验收。 |
| S-09 | Awaiting acceptance | 本机入口、合成数据、MA-01–16 和操作手册已交付；相关检查及失败项修正后的复验通过，人工验收未确认。 |

以下 S-01–S-06 记录保留当时的阶段状态；最终结论以阶段状态表与 S-09 收口为准。

## S-01 初始发现

- 官方 ClickHouse Go 驱动使用 `time.Time` 传递 DateTime64(9)；其整数时间写入重载存在毫秒语义，禁止拿纳秒整数直接传给该重载。
- 在去重窗口内重放不能证明业务唯一性；测试显式将复制去重窗口设为零再重放，重复 ordinal 的 attempt 必须拒绝封存。
- 丢响应故障须在独立连接确认数据已经写入后断开响应；提前切断可能只是取消了 INSERT，不能作为“已成功但响应丢失”的证据。
- 本机 MinIO 官方镜像获取未成功，选用可获取并锁定 digest 的 SeaweedFS 4.53 作为 S3 候选。只对实际运行的候选声明测试结果。

相关官方依据：[DateTime64](https://clickhouse.com/docs/reference/data-types/datetime64)、[ClickHouse Go](https://clickhouse.com/docs/integrations/language-clients/go/index)、[在线外部磁盘](https://clickhouse.com/docs/concepts/features/configuration/server-config/storing-data)、[SeaweedFS S3 API](https://github.com/seaweedfs/seaweedfs/wiki/Amazon-S3-API)。以锁定版本的实际测试结果为最终依据。

## S-01 已执行结果

- 纯规则：Int64／Decimal128(12) 边界、RFC3339 纳秒、存储时间端点、DST 重叠／缺失、Excel 1900／1904／虚构日期／超纳秒拒绝、质量优先级，以及精确矩合并与 half-even 舍入通过。标准差采用精确整数中点比较，无浮点误差。
- 真实组件：双副本精度往返、窗口外重放检测、原生客户端丢确认、新 attempt、外部清单、空／缺失清单、PG 原子发布试验和纳秒 int8range、副本落后、S3 在线 part、对象故障与恢复、数据库取消通过。
- 对象：multipart 完成前不可见、完整 SHA-256、跨分片 Range GET、续传列表、中止、校验和错误拒绝、命名空间隔离、条件写防覆盖、可信 TLS／不可信证书拒绝及暂态重试通过。TLS 是本机验证代理终止的 TLS，不代表生产证书或内网全链路 TLS 已验证。
- 文件：生成 7 个 CSV、2 个 XLSX 及 SHA-256 清单，Excel 文本保存大整数／高精度小数／纳秒；Arrow Parquet 使用 UTC NANOS 和 Decimal(38,12)，端点及符号往返通过。
- 初始容量：固定种子 20261112/7、128 通道、密集／稀疏各 100,000 条，完整原始列。单副本压缩原始载荷分别 13,708,191／14,056,348 字节，即 137.082／140.563 字节每条；分钟摘要分别 1,773／80,000 行，占对应原始压缩字节 1.21%／35.70%。这里只含被测表数据，未含标记、索引、原文件、备份和临时空间，不能直接乘条数作为采购容量。
- 开启 race 时写入＋另一副本逐值校验分别 6.319／6.478 秒；这不是 API 吞吐或目标性能验收。首次非 race 测量为 0.736／0.815 秒，独立记录，不混用为同条件性能。
- 首轮完整测试发现 Excel 默认 `Date1904=false` 在 XML 中省略，库读回 nil；已按 Excel 默认 1900 系统处理并复验。首次失败证据保留在 `.artifacts/monitoring-contract/2026-11-12T01-36-41.452Z/`。

S-01 未验证生产故障域、三 Keeper 仲裁、生产对象存储、真实业务分布或 P-1–P-3；这些按用户调整留待后续，不影响本轮本机功能继续开发。

## S-02 阶段证据

- 新增 16 项监测权限，目录总数 141；内置租户／项目管理员升级，自定义角色不自动增权；只读 POST／导出及快照续期不受项目归档写入拦截。
- PG 增量迁移 v11→v12，65 张已有表在升级夹具中逐表比较全部行未变化；自定义角色权限未变化，新增监测批次／授权／发布表为空。此夹具含账号、角色等旧数据，不冒充真实业务库升级证据。
- 纳秒范围、明确无界端点、区间完整覆盖、通道授权与资源授权相交、跨项目拒绝、同请求收据、撤权及资源重授权不复活数据授权已通过真实 PG／Redis 测试。
- 归档用真实项目服务执行；归档后 read／export 通过，import 和租户数据授权变更拒绝。
- `runtime.Compose` 聚合设备和监测引用；永久监测引用阻止服务与直接 SQL 删除通道。发现并修正资源详情删除动作未展示外部引用的问题；旧设备安装纠错引用测试复验通过。
- 治理 HTTP 与严格 DTO、OpenAPI／TypeScript 生成已接入；拒绝未知／重复键、null 标量和数字型修订；范围必须明确给出 from/to，null 才表示无界。
- 当前仍未提供导入、数据查询或监测页面，不能视为完整功能验收。S-03 将接通原文件、真实载荷、worker 和发布清单。

## S-03 阶段证据

- 32 MiB 分片上传、片哈希与完整文件 SHA-256 封存，原文件再读全量校验；PG 只保存文件、映射、逻辑块、attempt、清单和任务控制元数据。
- CSV 有界解析和多轮外部排序；纯规则自验覆盖 33,000 条跨 run 排序、超大引用记录与非法编码。XLSX 流式 XML、磁盘共享字符串、直接／共享／数组公式范围预扫及文本精度验证通过。
- 双副本逐字段／ordinal／载荷哈希核对；新 attempt 禁止复用不确定身份，已 sealed 的完整 attempt 在故障恢复后重新验证后选用。PG 最后短事务检查 fence、当前资格、修订和路由，一次发布整批。
- `TestMonitoringCSVPublicationAndFixedDuplicateSet`：时间上端点、相差 2 纳秒的翻页、38 位十进制、默认不改原质量、同值完整确认、不同值阻塞、追加后旧快照稳定。
- `TestMonitoringPublicationCommitFailureAndRecovery`：两个已封存 CH 块在 PG 提交失败后均不可见；恢复后整批可见，sealed attempt 不重写。
- `TestMonitoringUnsealedPhysicalWriteUsesNewAttempt`：真实 CH 写入后注入 PG 封存失败；新 worker 分配新 attempt，物理表保留两份，清单读取只有一份。
- `TestMonitoringExpiredWorkerAndWholeBatchInvalidInput`：过期租约接管提高 fence；一个缺值使整个文件不能发布。
- 真实组件回归（含已有 S-02 授权／引用用例）race 48.935 秒通过，日志 `.artifacts/monitoring-s03-integration.log`；阶段 `pnpm check` 通过，日志 `.artifacts/monitoring-s03-check.log`。S-04 后续变更还须重新执行完整检查。
- 集成发现并修复：ClickHouse 相关 EXISTS 对 nullable 无界范围返回空集合，改为规范化不相交授权范围的显式 JOIN；普通 Go `time.Time` 查询参数只生成秒级字面量，keyset 改用精确 Int64 纳秒比较。测试固定覆盖这两个故障，不能从写入精度往返推断查询绑定也保持纳秒。
- 配置、API、权限目录与生成契约、`infrapro monitoring storage init/check`、独立 `services/api/cmd/monitoring-worker` 已接入。未提供完整监测 UI，尚不可交付最终验收。

## S-04 阶段证据

- `TestMonitoringActualAdoptionsRevokeAndReadopt` 用真实组件验证 A100＋B80 重复／20 新增：当前与累计唯一 120；撤销 A 后只有 20；再次采用 A 后当前恢复 120、累计仍为 120；撤销 B 只减少 20，重新采用 B 也只含原来实际新增的 20。原始 observation_id 保留，新 adoption_id 独立。测试 3.20 秒通过。
- 撤销前生成完整固定影响预览，最后短事务重新核对当前资格、通道修订和 preview_hash；相关旧快照明确失效，物理载荷不删除。
- 原文件下载使用服务端鉴权流，每个 128 KiB 块检查权限修订；项目仅在完整文件业务范围可证明、无未解析错误和未映射内容时允许下载。已验证部分范围元数据不返回整文件总量，以及传输第一块后撤权即停止。

## S-05 阶段证据

- 精确 N／S1／S2、首末、极值与并列次数按质量分别合并。38 位原值的小差值总体标准差保持 `0.000000000001`；不经过 Float64。
- 分钟贡献只来自实际采用的已验证块。S3 保存按 SHA-256 定位的不可变压缩节点：256 分钟叶块、三层稀疏 radix-256 索引。变更只更新对应分钟与祖先路径；每通道 PG 保留一个最新头。CAS 防止旧水位覆盖新头，dirty 清理保留更晚水位。
- 快照固定内容与撤销修订；追加后旧统计仍读原版本，撤销返回 `snapshot_stale`。未构建或超交互预算返回 202 后台任务。派生维护不借原操作者旧权限。
- 部分授权与非整分钟查询回查原始清单。纳秒级部分授权只统计 2,000 条；区间开始早于视窗但仍获完整授权时，按观测结束时刻进入视窗。
- 三类数量保持累计唯一、当前在用、当前质量样本的固定定义；显式采用历史统计另返回 `n` 表明其计算记录数。最新覆盖与最新可用值分列。
- 日／月／年按 IANA 本地自然边界、游标分页及授权范围裁剪，返回 included_ranges／actual_range／partial。测试覆盖 23／25 小时 DST 日、闰年与闰月。枚举仅计事件与样本比例。
- 2,400 点窄屏趋势覆盖完整窗口、保留尖峰与末点，绘图点数不超预算；粗桶保留真实首／最小／最大／末点，明确按包络／事件显示，不声称桶内连续、不插值或补零。
- `pnpm test:monitoring --reuse-contract --keep` 复用已通过 S-01 的本机组件；证据 `.artifacts/monitoring-integration/2026-11-12T03-55-02.719Z/`，真实功能 race 133.675 秒，纯规则 race、HTTP／CLI 通过。`pnpm check` 日志 `.artifacts/monitoring-s05-check.log`。
- 区间与枚举后续联调单独通过（`TestMonitoringIntervalQueryAndEnumStatistics`，8.48 秒）；最终全量复验仍在 S-09 执行。

## S-06 后台导出证据（页面尚未交付）

- worker 固定选择、水位与权限修订；逐块复核授权。明细按清单分页读取，整份结果完成前不开放分卷。
- CSV exact 保持精确文本；spreadsheet_safe 对非数值公式样式单元格作可逆前缀转义，合法负数不改。Parquet 2.6 原始观测使用 UTC NANOS、Int64、Decimal(38,12) 和 UTF-8 枚举；计算结果是附 scale／rounding 的精确文本列。
- 每卷默认 100 万行或 256 MiB，可配置。Parquet 行组最多 8,192 行／约 8 MiB，因此可跨目标大小一个有界行组。完成时重新核对行数与全文件 SHA-256，明细与同快照摘要逐通道对账。
- 两条一卷的真实测试核对 CSV／Parquet 分卷、纳秒上端点、正负 38 位小数、追加后旧快照不变、空结果和日统计导出；`TestMonitoringExportFixedSnapshotAndTypedParquet` 与 `TestMonitoringCSVSafeAndStatisticsExport` 包测试 8.650 秒通过。
- 注入最终 PG manifest 提交失败，临时卷不可下载；重试只有一份完整结果。项目真实归档后仍可后台导出和下载；过期拒绝。`TestMonitoringExportCommitFailureAndArchivedProject` 3.80 秒通过。
- 项目部分授权仅导出 2,000 条；下载第一块后撤权，下一块停止，导航清除已撤权通道。扩展范围测试 4.02 秒通过。
- API／OpenAPI／生成客户端类型及配置接入，`pnpm check` 通过，日志 `.artifacts/monitoring-s06-export-check.log`。页面、浏览器缓存及键盘／窄屏验证仍在进行，不据此标记 S-06 完成。

## S-06 页面与快照联调

- 租户／项目监测入口、工程资源深链接、测点／通道选择、精确明细与统计、Canvas 趋势、导入映射、问题核对、撤销／再次采用、后台任务和授权页面已接入真实 API。
- 上传采用独立 Web Worker 流式 SHA-256。Node 原生 SHA-256 对照覆盖空文件、块边界、不对齐切片及 32 MiB 跨片；纳秒端点、负时间与高精度大数坐标基准校验通过。
- 视窗按支持域起点对齐二次幂时间块，最多三个可见块、左右各一个预取块；三并发、取消、32 MiB／80,000 点 LRU 限制。单请求可降低绘图点预算，服务端仍完整表示请求窗口。枚举状态数量参与预算，不按四个数值极值点估算枚举响应。
- 快照的基础筛选固定；新增 `view_range` 仅允许在基础范围内取子窗口，不能扩大授权或基础时间范围。明细游标绑定子窗口。`TestMonitoringSnapshotViewsAndTileBudget` 通过子窗口精确统计、四点完整趋势、跨窗游标拒绝、基础范围扩大拒绝，以及追加后固定窗口与导出仍为原 120 条。
- `TestMonitoringRecentListsUseStableChronologicalPages` 验证 55 个批次及含隐藏任务的 55 个候选按创建时间／ID 稳定翻页；不因过滤而把第 51 个候选重复返回。`TestMonitoringWorkspaceWithoutDataGrantsIsAnEmptySelection` 验证无数据授权时零通道、零数量和空明细。
- 桌面和 375 px 窄屏已分别通过真实上传／映射／发布／Parquet 下载／撤销、明细翻页、故障保留结果；项目单日授权／归档读取／撤权后清缓存并丢弃迟到响应，以及同值完整核对／异值整批拦截也通过。扩展导出丢响应重试、再次采用用例正在复验，S-06 尚不标记最终通过。
- 浏览器联调发现并修复首次授权版本导致表单重置、日期跳转丢失当前视图、无授权选择用 nil UUID 数组写入 PG 的问题。移动端一次无效测试受运行期间生成文件触发热更新干扰，已在源码冻结后复测通过。
- `pnpm check` 通过，日志 `.artifacts/monitoring-s06-check.log`；当前新增四项 Node 纯规则用例通过。完整真实组件 race 和扩展浏览器证据待下方最终结果更新。使用说明见[监测数据操作手册](../operations/monitoring-data.md)。

### S-06 阶段收口

- `pnpm test:monitoring --reuse-contract --keep` 通过；真实功能集成 race 200.366 秒，纯规则 race、HTTP 和 CLI 通过。证据 `.artifacts/monitoring-integration/2026-11-12T05-26-57.136Z/`，本次复用已通过的 S-01 实验室。
- 最新导出丢响应重试、再次采用、趋势／分页和失败保留结果在桌面和窄屏 4 项通过：`.artifacts/monitoring-s06-browser-retry-readopt.log`。同一请求重试复用 receipt ID。
- 项目授权／归档／撤权迟到响应、重复核对／异值整批阻断共 4 项已在 `.artifacts/monitoring-s06-browser-final.log` 通过；该运行另外两项因测试拦截器过早撤销路由而失败，修正测试竞态后以上复验通过。没有把失败运行记为全绿。
- 视觉证据保留在 `.artifacts/monitoring-s06-browser-qualified/` 和 `.artifacts/monitoring-s06-browser-before-retry/`；已人工检查桌面／375 px 图表截图。当前浏览器为 Chromium，尚未验证 Safari 或真实触屏设备。
- 整体仍为 In progress，S-07 和 S-09 未完成，尚不交付最终验收。

## S-07 运维与恢复收口

- 存储启动检查真实33列布局、类型化约束表达式、排序／分区、复制身份、90天在线放置 TTL、S3 卷与有界缓存；不能只凭布局账本或同名约束认定正确。1900／2000数据实际位于缓存S3磁盘，2262纳秒上端点保留热层；无删除TTL。别名指向同一副本、篡改列类型和伪造同名约束均被拒绝。
- 逻辑分片迁移通过持续复制、暂停、尾部补齐、发布屏障、PG短事务版本切换；跨物理分片按完整键合并分页，旧快照保持原路由。回退包含切换后新增数据的反向复制。注入真实物理写入之后的封存提交失败，旧路由仍完整，uncertain保留，重试新attempt。`.artifacts/monitoring-s07-route-fault.log`，race 15.035秒通过。
- 只读对账不修改状态，核对源SHA、规范原始依据、双副本全载荷／ordinal、当前唯一性、历史稳定身份与唯一首次采用、贡献和树上精确数学状态。一个宽表块不会在最终报告里按所含通道重复计数；全轮发布／路由变化明确报 verification_changed。
- 派生对象故意损坏后，固定预览的修复任务隔离通道，新不可变命名空间重建并readback验证；保留损坏旧对象，原身份及数学值不变，后续撤销读取替换贡献。恢复前旧快照失效。
- worker 每日对账的双worker重复排队测试通过。源文件损坏被发现后，新的修复任务因原依据未通过而失败，quarantine保持；不会把“摘要重建”冒充原始恢复。最终维护四项 race 38.518秒，`.artifacts/monitoring-s09-maintenance-final.log`。
- `storage status` 提供队列／状态年龄、dirty、过期租约、隔离、错误数、物理part／复制／扫描／缓存和临时盘指标；API／job／audit／PG事务／CH query串联trace，结构化日志记录时长、对象吞吐、错误和取消时间。真实持久验收库状态为零隔离、1条已报告格式错误，两副本各817物理行。
- 统一CLI和维护手册已提供检查、verify、固定计划重建、routes、jobs、backup／restore。原始载荷／旧物理路由暂不自动GC；完整对账和解析外部排序都受8GiB任务暂存预算限制，超限明确中止，P-3预算调优尚未执行。

### 独立恢复的最终证据

执行 `pnpm test:monitoring:restore`：`.artifacts/monitoring-restore/2026-11-12T06-41-08.611Z/`，格式2归档，race 16.83秒测试通过。日志索引 `.artifacts/monitoring-s07-restore-final.log`，结果 `restore-proof.json`。

| 项目 | 实际结果 |
| --- | --- |
| 源与目标 | 不同PG／CH／Keeper／S3／Redis容器；测试暂停源CH两副本、源S3及备份暂存S3。 |
| 归档 | PG导出快照checkpoint + CH原生备份 + 原文件／依据；106文件、951355字节，备份1.416634秒。 |
| 恢复与核验耗时 | 4.139194秒，仅小型合成P-0；不是目标环境RTO。 |
| 生效与历史 | 22条当前在用，122条采用历史；含A100、B80重复／20新、已撤销A及纳秒／大数边界。 |
| 授权与凭证 | 项目只见10条，完整混合源文件拒绝；10类PG控制／授权表摘要一致，旧会话撤销。 |
| 恢复开放顺序 | 无ready guard默认拒绝读写→restoring核验→read_only；显式open后可新发布，当前增至23。 |
| 故障边界 | 源组件暂停期间目标读取成功；本地同主机、完整checkpoint备份。WAL连续归档、任意时间点PITR、异地故障、生产RPO/RTO未验证。 |

归档文件hash损坏先于目标写入被拒绝；缺失／损坏原始证据不删PG发布记录“补齐数量”。未完成恢复目标保持不可用。临时目标只清理当前脚本创建的唯一项目，源容器恢复运行；持久验收栈与此实验室分离。

## S-09 最终回归与验收准备

- 26项监测真实组件集成通过，race 255.727秒；独立恢复在该命令中按预期跳过、另以上述真实目标单独执行。纯规则、客户端4项、HTTP／CLI同时通过。统一记录 `.artifacts/monitoring-integration/2026-11-12T06-35-16.809Z/result.json`。
- 后续迁移故障及对账、状态／真实布局变更按上方针对性复验通过。没有把失败试验、未运行或跳过项写成通过。
- 桌面1440px与移动375px，监测端到端10项全部通过（6.0分钟）：`.artifacts/monitoring-s09-browser.log`。覆盖真实导入、错误／重复核对、导出丢响应原收据重试、下载、撤销／再次采用、游标、保留失败前完整视图、项目时间授权、归档、撤权缓存及迟到响应、点／通道深链接、多点兼容分组、枚举、键盘选择／图例／范围焦点。截图 `.artifacts/monitoring-s09-browser-qualified/e2e/`，桌面和窄屏已查看。
- 本机正式应用组合与独立worker运行在15273／18280，专用持久卷。通过真实HTTP创建2测点／4通道、7已发布批次及2待处理批次；全租户817在用／814默认样本，项目10条。`monitoring-local/smoke-result.json`与只读对账记录通过，源文件9个、逻辑块8个、原始记录817。凭据0600、配置0600、资料目录0700，不在报告复制密码。
- 已执行整个本机Docker栈重启，再启动最新API／worker；重新登录、上述数量与项目授权核对通过，命名卷持久化得到实际验证。应用入口及MA-01–16见[人工验收清单](../operations/monitoring-data-acceptance.md)。
- `pnpm check`通过：`.artifacts/monitoring-s09-check-final.log`，含Go fmt/vet、ESLint、TS、OpenAPI和权限生成同步。`pnpm build`通过：`.artifacts/monitoring-s09-build.log`；ECharts按需块522.82kB（gzip176.49kB）有Vite非阻断体积提示，非所有页面首屏加载。
- 全量既有Go race首轮在租户／项目旧测试的固定权限数量处失败：新增9个租户／7个项目权限后旧断言未更新。其他包通过；保留失败日志 `.artifacts/monitoring-s09-existing-regression.log`。更新目录／审计快照数量断言后，项目单项14.009秒、租户四项45.386秒通过；没有改宽授权隔离或审计语义。补跑API／CLI全模块race也通过：`.artifacts/monitoring-s09-http-cli-race.log`。
- 独立模块 Linux 构建首次暴露 API／CLI 缺少 Arrow 传递依赖校验记录；执行模块内 `GOWORK=off go mod tidy` 补齐后，API、独立 worker 和 CLI 均通过 `GOWORK=off CGO_ENABLED=0 GOOS=linux go build -mod=readonly -trimpath`。证据 `.artifacts/monitoring-s09-linux-api.log`、`.artifacts/monitoring-s09-linux-cli.log`，二进制在 `build/monitoring-linux/`。
- 全量 console 浏览器首轮 209 项：193 通过、3 项总测试超时、13 按条件跳过，24.9 分钟，日志 `.artifacts/monitoring-s09-console-regression.log`。三项多身份长流程仅调整总预算为 60／90 秒，未放宽断言；桌面两项复验 54.4 秒、移动一项复验 20.2 秒通过，见 `.artifacts/monitoring-s09-browser-desktop-retry-final.log`、`.artifacts/monitoring-s09-browser-mobile-retry-final.log`。首轮失败截图／trace 保存在 `.artifacts/monitoring-s09-console-qualified/`，不将首轮记为全绿。
- 13 个条件跳过中，10 项监测用例已在真实监测组件模式独立全数通过；2 项路由资源故障在生产构建 preview 模式补跑通过（25.1 秒），日志 `.artifacts/monitoring-s09-preview-route.log`；剩余 1 项是只在桌面执行的断点／键盘场景，桌面版本已通过。合计 208 个适用浏览器场景通过首轮或针对性复验完成验证。
- 交付前最终 `pnpm check` 通过（含本轮测试与独立模块依赖修正），日志 `.artifacts/monitoring-s09-delivery-check.log`；`git diff --check` 通过。API `/readyz` 和前端入口实测 HTTP 200。本机功能进入 Awaiting acceptance；不替用户填写人工验收结论。

### 明确未验证的范围

真实业务样本；P-1一亿／P-2十亿／P-3百亿及目标并发；目标硬件／容量／网络；两小时目标混合负载；24小时稳定性；多主机或异地故障域；生产WAL/PITR／RPO/RTO；Safari、真实触屏及完整辅助技术兼容。用户已将真实样本和目标部署规模移出本轮本机验收前置，以上不记为通过。首次交付时未提交；随后按用户指令完成下述本地主分支合并。未推送、未部署生产；用户人工验收尚未确认。

## 2026-11-12 合并回原工作树

- 用户授权将本次修改合并回 `/Users/hijacker/Documents/ChatGPT/InfraPro` 的 `main`，并自行处理冲突。监测实现提交为 `81a9bde`，从共同基线快进合并；没有创建分支或 worktree，也没有推送远端。
- 原工作树已有 54 个已跟踪文件改动和 35 个未跟踪文件，先完整备份并临时 stash，合并后全部恢复，保持原有未提交状态。备份 stash 为 `86069866ca5b546692aa19a6cbaff651b71c6775`。并行更新的派生监测方案保留其最新内容，本次未编辑该方案。
- 导航、路由生成文件、OpenAPI、生成客户端和两份文档的 6 处冲突已处理。OpenAPI 按对象语义进行三方合并，保留监测接口及原工作树新增的资源树接口，共 419 个路径；随后重新生成路由和客户端类型。导航沿用原工作树分组，加入监测入口；租户设施工作空间已接入测点／通道到监测的跳转，项目原详情入口继续保留。
- 在原工作树执行 `pnpm check`、`pnpm build`、`git diff --check` 均通过。桌面／375 px 窄屏共 10 项集成复验全部通过，耗时 5.4 分钟，覆盖设施树、真实上传与导出／撤销、项目授权／归档／撤权、实际资源入口跳转、多点比较及工作空间导航。构建仍有上文记录的图表块体积提示。
- 本次合并证据位于原工作树 `.artifacts/monitoring-main-merge-20261112T153301/`。前述完整开发自验和持久验收环境仍在 `/Users/hijacker/.codex/worktrees/b143/InfraPro`；Git 合并不迁移忽略的本机运行配置、账号文件及样本。人工验收状态仍为 Awaiting acceptance。
