# 大屏 SDK 监测读取实施与验收账本

状态：**Awaiting acceptance**。用户人工验收：**未开始**。

依据：[完整开发规格](../development/display-sdk-monitoring-read-spec.md)。SDK-001—SDK-100 全部属于本次实施；下面的检查记录与用户验收分开。当前 `main` checkout，基线 HEAD `9c744acfa26a3f754eb0710cd9052d5e5e8be9a7`，实现保留在工作区；原有改动保留，没有新建分支或 worktree，没有提交、推送或生产部署。

## 设计与交付

Go/pgx 的 `displayread` 协调严格请求、不可变范围、分页、缓存、查询任务与 fencing；资源、设备、来源、监测和空间服务独立核验领域资格。PostgreSQL migration 46 保存格式版本、固定集合、查询／订阅与工作租约；观测和派生统计使用现有 ClickHouse 与对象存储正式读取，不另建第二份业务数据。TypeScript ESM SDK／devtools 1.1.0 对接隔离 iframe 和 MessageChannel，React 控制台提供授权／配置范围编辑及发布核验，Three.js 仅用于独立验收工程的本地模型查看。

交付入口：

- [SDK 接入与计数语义](../../packages/ts/display-sdk/README.md)、[开发工具](../../packages/ts/display-devtools/README.md)。
- [独立监测读取工程](../../examples/display-monitoring-read/README.md)：六面板、树／地图／来源、分页图纸与模型、时间筛选、只使用公开 SDK 包。
- [启动、权限、迁移、恢复和预算](../operations/visualization-display.md)。
- `.artifacts/display/` 中 SDK／devtools tgz、旧示例与 `monitoring-read-example.zip`；外部安装目录记录于 `monitoring-read-external.json`，最终摘要见 `monitoring-read/delivery.json`。

## 复现与证据索引

所有自动夹具使用独立测试数据库／对象前缀。表中源码路径相对于仓库；日志在 `.artifacts/display/monitoring-read/`（另有说明的除外）。`pnpm test:display` 自动构建 SDK、检查正反 TypeScript 调用契约，运行 SDK 单测和实际领域 Go race 集成。`TestDisplayRead` 用例采用真实领域服务，监测读经过正式 CSV 发布、撤销、派生计算以及两个 ClickHouse 副本。

| 代号 | 实现与覆盖重点 | 可复现检查与证据 |
| --- | --- | --- |
| W | `displayread/{decode,requests,scope,plan,authority,service}.go`，`display/{manifest,read_scopes,read_authority}.go`，Go 生成 TS／OpenAPI | `pnpm test:display` 中 `TestRead*`、`TestDisplayManifestReadViewsCompatibility`、严格 JSON、预算／字段／未来谓词和 `tests/types/display-reads.ts`；`pnpm check` 的生成一致性检查 |
| D | `resources/display_reads.go`、`display/read_management.go`、`read_scopes.go`、`displayread/service.go` | `TestDisplayReadDirectorySnapshotAndIsolation`：授权用途、隐藏搜索、固定／动态成员、分页、字段、改名快照、撤权、资源统计订阅 |
| C | `resources/display_statistics.go`、`devices/display_history.go`、`monitoring/display_statistics.go` 和正式监测查询 | `TestDisplayReadCanonicalCountsAndOpaqueDrilldowns`：固定夹具、类型版本／树／context、地图、三类统计、产品型号、时间、所有下钻、latest／trends／statistics／overview；`TestDisplayCalculatedCountsUseQualitySelectedPopulation` |
| E | `devices/display_{reads,history,scope}.go`、`sources/display_reads.go` | `TestDisplayReadDevicesSourcesAndDistinctStatistics`：完整／输出 context／restricted、四通道测点、独立输出、历史 DTO、去重、字段拒绝与设备订阅重放／reset |
| X | `spatial/display_{reads,files,map,defaults,scope}.go`、`resources/display_map.go` | `TestDisplayReadSpatial101LocationsFixedVersionAndFiles`：101 位置、固定版本／默认摘要、geometry 门限、本地瓦片、文件块、S3 故障和撤权；`TestReadGeographicBounds` |
| X2 | 同上；独立工程的图纸／模型／GeoJSON 面板 | `TestDisplayReadSpatialRepresentationsAndRelationships`：真实 PDF／GLB／GeoJSON 转换，页／片段、区域／挂载、相机／隐藏元素／空列表契约、受限工程对象和独立属性投影 |
| N | `monitoring/display_{reads,statistics,drilldown,lifecycle,qualification}.go`、`display/read_subscriptions.go`、`displayread/error_adapter.go` | `TestDisplayReadMonitoringFixedCountsAndWindows`：窗口并集、共享缓存、快照下钻、新观测、统计重放、伪造／跨主体／仅汇总下钻拒绝、关闭真实 CH 客户端后 dependency_unavailable／503；`fault-final.log` |
| I | 人员会话与派生输入资格的并发锁 | `TestDisplayReadConcurrentPersonnelInputQualification`：两事务同步持有人员共享锁后递归检查输入，无锁升级死锁；账号写锁仍受阻。修复前 `concurrency-before-fix.log` 稳定失败，修复后 `concurrency.log` 通过 |
| J | 实际服务 SQL 的计划与查询次数 | `pnpm test:display:plans`；`query-plans.log`、`query-plans/result.json` 及五个场景 JSON：项目可见集合、设备／输出授权、来源、空间／默认版本、固定范围、查询分页／缓存／过期与后台领取的实际 SQL；仅导出脱敏计划，不导出参数 |
| L | `displayread/{jobs,cache,selections,sections}.go`、`display/read_subscriptions.go` | `TestDisplayReadWorkerFencingCancellationDeadlineAndRestore`、`TestDisplayReadWorkerInstanceConcurrencyAndRevocation`：租约抢占／fencing、取消、120 秒截止、实例并发、交付复验、环境恢复与清理 |
| P | 旧 Provider／运行主体／归档与发布链路 | `pnpm test:display` 中旧 `TestDisplay*`：固定派生与递归撤权、监测／空间、终端、生命周期、并发归档、上传、持久订阅；旧包与运行语义保持 |
| U | `display-sdk/src/{reads,index,read-types}.ts`、运行壳受控 RPC | `tests/unit/display-{sdk,reads,package,config}.test.mjs`：精确整数、独立消费者、最后释放、迟到响应、generation、URL 回收、新旧能力协商 |
| B | 成员解析、轻量统计投影、预览采样 | `pnpm test:display:load --boundary`；`member-boundary-{environment,result}.json`、`member-boundary.log`：50,000 通过、50,001 拒绝 |
| F | 真实统计／查询缓存与完整分页 | `pnpm test:display:load --load`；`load{,-environment,-result}.json`、`load.log`：2,600 资源、600 通道、100,000 观测、10 实例、冷／热延迟、SQL／CH 扫描、堆、队列与取消 |
| H | React 范围编辑、发布摘要、iframe 运行与固定终端 | `pnpm display:browser`；`real-browser.log`，`.artifacts/display/browser/{result,named-result}.json` 及 PNG：UI 上传／授权／配置／预览／发布／切版／回退／撤权／withdraw、权限及移动端 |
| H2 | 独立工程的真实监测、来源与空间查看 | `pnpm display:rich:browser`；`rich-browser/result.json`、`rich-browser.log` 及 PNG：正式 CSV／派生计算、实际 PDF／GLB／GeoJSON 发布，经 iframe／MessageChannel 查看、下钻和撤权 |
| M | devtools 九场景、运行壳模拟标记与独立工程 | `pnpm display:mock:browser`；`browser/mock-result.json` 及九场景／六面板／移动端 PNG |
| A | SDK／devtools 1.1.0、Manifest 1/2、外部工程 | `pnpm display:pack`；`external-pack.log`：仓库外安装 tgz、类型检查、构建、validate、ZIP pack；H 再验证实际上传运行 |
| G | migration 46、默认格式 1、格式 2 显式升级与旧二进制 schema 门禁 | `pnpm test:display:compatibility`；`compatibility-result.json`、`compatibility-final.log`：真实旧 HEAD 二进制拒绝 schema 46，未知格式拒绝，格式 2 存在时降级原子失败 |
| R | 监测联合恢复、显示包对象备份、环境代次失效 | `pnpm test:display:restore`；`restore-runner.log` 指向独立恢复目录，PG＋双 CH＋S3 新目标，源存储暂停后校验；恢复旧查询／终端失效，重新配对后目录与精确计数可读 |
| Q | 整仓类型、Go、ESLint、协议／权限生成 | `pnpm check`；`final-check.log` |
| DOC | 正式需求、架构、技术选型、操作与接入说明 | 本文交付入口和逐项表；所有实现验证完成后统一转 Awaiting acceptance |

### 最终自验结果

2026-11-17 完成 SDK-001—SDK-100 全范围自验，用户人工验收尚未开始。完整大屏回归通过，包含实际领域服务的 Go race；之后为空空间设置补充的 DTO 空数组回归、101 位置回归、最新整仓检查及真实模型浏览器检查也全部通过。最新 ZIP 的管理、人员预览、发布、固定终端、切版／回退／撤回／撤权均完成真实浏览器校验。

统计固定夹具精确断言：资源 `3/6/8/14`、授权／参与监测设备 `6/5`、raw `120/117/114`、所选 derived `20/16/4`；半开窗口 raw `60/57/54`，下钻逐页与对应样本／当前观测／逻辑输出数一致。H2 的较小浏览器夹具另用 6 条原始观测和两个 A−B 派生通道（各 3 条有效结果），通过真实发布链路显示 PDF 三页、GLB、GeoJSON、挂载与来源；限定到 00:11 后只显示 4、5，不再显示 00:30 的 9。它不冒充规格的 120 条基础夹具。

本轮实际缺陷均已关闭：人员授权共享锁被递归输入检查升级造成的并发死锁、空间设置 UUID／text 比较、空隐藏元素列表的 DTO 契约、GIS 局部范围适配与 MultiPoint 呈现、游标继续页大小及测试终端 Cookie 隔离。历史失败日志单独保留，以下为最终有效证据。

| 检查 | 开始（Asia/Shanghai） | 完成 | 结果文件（均通过） |
| --- | --- | --- | --- |
| `pnpm test:display:load --boundary` | 11-17 01:17:26 | 11-17 01:21:57 | `member-boundary-result.json` |
| `pnpm test:display:load --load` | 11-17 01:40:24 | 11-17 01:48:32 | `load-result.json` |
| `pnpm test:display（含 SDK 构建／类型及 Go race）` | 11-17 02:17:17 | 11-17 02:29:54 | `full-display-result.json` |
| `最终空间表示与 101 位置 Go race 回归` | 11-17 02:27:51 | 11-17 02:29:04 | `representations-result.json` |
| `pnpm test:display:compatibility` | 11-17 02:28:40 | 11-17 02:29:06 | `compatibility-final-run-result.json` |
| `pnpm display:pack（含独立安装／构建）` | 11-17 02:37:11 | 11-17 02:37:16 | `external-pack-run-result.json` |
| `pnpm display:rich:browser` | 11-17 02:37:40 | 11-17 02:40:54 | `rich-browser/result.json` |
| `pnpm display:mock:browser` | 11-17 02:37:41 | 11-17 02:37:43 | `mock-browser-run-result.json` |
| `pnpm display:browser` | 11-17 02:41:20 | 11-17 02:45:16 | `real-browser-run-result.json` |
| `pnpm test:display:plans` | 11-17 02:47:15 | 11-17 02:49:16 | `query-plans-run-result.json` |
| `pnpm check（含协议／权限／类型）` | 11-17 02:47:42 | 11-17 02:48:22 | `final-check-run-result.json` |
| `pnpm test:display:restore` | 11-17 02:49:27 | 11-17 02:50:58 | `restore-runner-run-result.json` |

`pnpm check` 内实际执行 contracts:check、permissions:check 和各包类型检查；SDK 构建与调用类型正反检查见 test:display，SDK／工具打包与外部工程构建见 display:pack。所有命令使用本文同一 baseline 的当前工作区；[交付摘要与 SHA256](../../.artifacts/display/monitoring-read/delivery.json)及其源码文件摘要固定最后交付身份。

联合恢复通过：在独立 PG／双 CH／S3／Redis 目标中恢复，暂停测试源 CH／S3 期间校验；包文件摘要、范围／配置固定集合和正式数据一致，旧查询及终端拒绝，重新配对后仅能读取获准的 10 条观测。恢复记录：`.artifacts/monitoring-restore/2026-11-16T18-49-27.205Z`；备份 2.395 秒、恢复及对账 30.348 秒。恢复后另一次正式发布成功，active 22→23；派生成果与分析行保持。该时长只对应本机合成夹具。

查看器实拍：[PDF](../../.artifacts/display/monitoring-read/rich-browser/drawing-viewer.png)、[GLB](../../.artifacts/display/monitoring-read/rich-browser/model-viewer.png)、[GeoJSON](../../.artifacts/display/monitoring-read/rich-browser/gis-viewer.png)。运行身份凭证未进入 ZIP；浏览器夹具的临时登录文件已移除。

负载环境：macOS arm64，Apple M3 Max 14 核、36 GiB 内存；Docker Desktop 29.9.3 的虚拟机为 14 CPU／8,318,910,464 字节内存，两个 CH 容器各限 2 GiB、S3 限 768 MiB，无单容器 CPU 配额。PG 17，CH 26.8.36.3（固定镜像摘要）；Go 1.27、PG 池 40，显示与监测 worker 各 4。镜像与原始指标见 `load-environment.json`／`load.json`。

| 测量 | 实测 | 判定／口径 |
| --- | --- | --- |
| 规模及完整性 | 10 实例、2,600 资源、600 通道、100,000 正式采用观测 | 目录全页唯一计数及精确统计断言通过 |
| 热统计 P50／P95 | 621.040／1,120.066 ms（30 次） | P95 ≤ 2,000 ms，通过 |
| 目录 P50／P95 | 444.578／657.053 ms（30 次） | P95 ≤ 2,000 ms，通过 |
| 冷统计 P50／P95 | 12,240.100／14,696.842 ms（10 次） | 查询／结果缓存为空，正式发布索引已就绪；后台 pending 后在 120 秒内精确完成 |
| 正式发布索引准备 | 398,409.288 ms | 单独计时，未算作冷查询；不能据冷查询指标推断刚导入即可返回热结果 |
| 排队 P50／P95 | 3,749.735／8,692.922 ms（11 项） | 包括冷查询与取消项；持久任务首次领取时间 |
| 取消确认 | 102.424 ms | 当前查询与 worker 停止交付 |
| Go 堆峰值 | 55,254,080 字节 | 测试进程查询阶段采样，非全容器 RSS |
| SQL | 104,748 次 | 查询测量区间的 PG tracer，包含轮询与后台 worker |
| CH 副本 1／2 扫描 | 27,895,005／27,812,842 行；1,558,782,362／1,521,746,636 字节 | 隔离测试库最近一小时，包含索引准备及读查询；不是单次统计扫描量 |
| 成员上限 | 50,000 精确完成，50,001 拒绝 | 49,999 测点＋1 设施；预览与运行均检查，非截断 |

查询计划补充证据：五个实际领域／生命周期场景共捕获 32 种 SQL 形态，覆盖项目可见集合、设备／输出授权、来源、空间／默认版本、固定成员、查询分页／缓存／过期与后台领取。受测服务连接共记录 16,239 次 SQL 调用，包含授权、管理写入、轮询及错误路径，不代表单次读取成本。每个场景结束后仅在其隔离 schema 内 ANALYZE，再执行 EXPLAIN；参数不导出，合成对象 UUID 脱敏。小夹具中的顺序扫描保留优化器实际选择，计划估算不替代上面的负载实测。

这是可复现的开发验收负载，尚无生产容量、多机故障域、长期吞吐或生产恢复时限结论。完整应用中与本次无关的全量 E2E 未执行；本次大屏、实际领域与终端流程分别留有证据。

## 人工验收操作

1. 按运维说明启动 API、监测 worker、显示 worker 和三个隔离 origin；在已有合法项目中打开「项目大屏」，创建实例。上传 `.artifacts/display/monitoring-read-example.zip`，程序声明应显示 Manifest 2 的资源／设备／原始／指定派生／空间视图。
2. 进入「管理授权」，按当前项目业务资格选择 capabilities 和相应范围，预览完整匹配量；配置草稿映射这些范围。授权、配置各自预览应显示字段、时间边界、未来策略和默认空间版本。测试字段缺失、越界 ID 或 required 未满足时应定位字段并阻止保存／发布。
3. 打开真实草稿预览：依次操作「项目统计」「设施树与地图」「设备与来源」「指定指标与时间」「图纸与模型」「状态与诊断」。逐页取全，进入详情后退出；空范围显示 empty，受限项显示限制原因，待计算显示 pending，缺覆盖显示 partial。图纸应显示页、区域和测点；模型应使用对应版本、相机与隐藏元素。
4. 使用规格第 13 节固定夹具时，对照下面各行的精确预期；自动化 C 已通过同一正式入库路径构造该夹具。任意真实项目按其实际资源和观测显示，不冒充固定测试计数。统计的总计不能随着翻组页变化，按样本／在用／派生逻辑下钻应与对应数值逐页对账。
5. 调整明确时间范围／全部历史／跟随窗口，确认互斥；原始与派生独立显示。只保留汇总字段时，不应取得明细数值；仅输出授权设备不应出现完整档案或序列号。
6. 核验后发布，签发终端配对码，在独立浏览器上下文配对；发布第二配置再回退。配置切换后旧结果与文件 URL 应释放。撤权或撤回发布后运行壳应卸载应用，回退不能恢复已经撤销的资格。
7. 用 devtools 的九种明确标注「模拟数据」场景检查异常提示。真实预览应标注真实草稿，并通过登录／配对流程；不向 ZIP 或外部工程复制令牌。
8. 单独执行联合恢复命令。该命令会暂停测试源 CH／S3，因此必须等其他 monitoring-contract 检查结束；不对运行中的业务环境执行。按 R 的恢复记录验证旧主体失效和重新配对后的读取。

## SDK-001—SDK-100 逐项追溯

每行保留规格的可观察验收预期。代号定位上表中的实现、可执行命令和实际断言；同一集成夹具覆盖多项时不重复复制测试。人工确认单独记录，自动自验不能将状态改成 Accepted。

| 编号 | 验收动作与预期 | 实现／复现证据 | 自验 | 人工验收 |
| --- | --- | --- | --- | --- |
| SDK-001 | Manifest 1 旧包及配置继续可运行，旧接口快照和数量／时间规则不变 | W、D、P | 通过 | 未开始 |
| SDK-002 | Manifest 2 识别新视图；未知版本、字段、重复 JSON 键和错误 domain 被拒绝并定位字段 | W、D、P | 通过 | 未开始 |
| SDK-003 | 读取范围为项目、运行身份、实例、配置及查询的交集；F4/F5 的 ID、名称、计数均不泄露 | W、D、P | 通过 | 未开始 |
| SDK-004 | include_future=false 后新增对象不进入固定视图；撤权对象立即退出有效集合 | W、D | 通过 | 未开始 |
| SDK-005 | 只有授权和配置都允许未来成员才纳入符合条件的新对象；更宽未来谓词在 prepare 被拒绝 | W、D | 通过 | 未开始 |
| SDK-006 | 空 target、未知 view、跨 domain 使用和越界显式 ID 不变成全项目查询 | W、D | 通过 | 未开始 |
| SDK-007 | capability 与底层业务资格分别校验；summary-only 可读汇总但不能读目录、设备档案或监测明细 | W、D、P、N | 通过 | 未开始 |
| SDK-008 | 可选字段省略有 included_field_groups；明确索取未授权字段返回 field_not_granted，不伪造空值 | W、D | 通过 | 未开始 |
| SDK-009 | 普通人员、预览人员、固定终端及委托人资格各自验证；管理员身份不跳过项目业务授权 | P、L、I、H | 通过 | 未开始 |
| SDK-010 | 派生输入资格递归检查；失去输入资格后所选派生数据不可继续读取 | P、C | 通过 | 未开始 |
| SDK-011 | 查询执行中撤销设备／资源／通道／空间授权，交付前复验阻止结果、缓存或文件块继续返回 | P、L、I、H | 通过 | 未开始 |
| SDK-012 | 旧 sources.read 资格保持；新当前来源目录与历史能力分开，历史字段不可随目录权限获得 | P、E | 通过 | 未开始 |
| SDK-013 | 类型目录只有可读设施涉及类型；F1/F2 同定义不同版本归为一个桥梁类型 | D、C、W | 通过 | 未开始 |
| SDK-014 | 设施按类型、状态、关键词筛选，先授权和筛选再分页，total 与逐页唯一对象数一致 | D、C、W | 通过 | 未开始 |
| SDK-015 | 树支持多层构件及设施直属测点；root/children/path 返回约定范围，不一次透出全项目 | D、C、W | 通过 | 未开始 |
| SDK-016 | context 祖先只返回最小信息，不计入资源总数，不泄露隐藏兄弟及其数量 | D、C、W | 通过 | 未开始 |
| SDK-017 | 设施、构件、测点、通道详情包含固定定义依据、允许属性、位置和层级，精确数值不丢精度 | D、C、W | 通过 | 未开始 |
| SDK-018 | 派生通道可挂设施／构件／测点，owner_ref 正确，无测点时不伪造 point_id | D、C、W | 通过 | 未开始 |
| SDK-019 | 删除、退役、正常状态依领域规则筛选；默认非删除范围及状态分项一致 | D、C、W | 通过 | 未开始 |
| SDK-020 | 600 个通道和超过 500 个资源完整分页，无重复／遗漏，不被旧绑定上限截断 | F | 通过 | 未开始 |
| SDK-021 | 翻页期间新建或改名不改写原查询；新查询可见新状态，删除／撤权使受影响查询失效 | D、L、U | 通过 | 未开始 |
| SDK-022 | cursor 改签、换筛选、换用户／终端／实例或过期均被拒绝 | D、L、U | 通过 | 未开始 |
| SDK-023 | 新 views.describe 报告范围策略、字段和预算，不能返回未获准全量 ID 或管理身份信息 | W、D、H | 通过 | 未开始 |
| SDK-024 | 合法空范围为 empty；依赖错误和权限不足不是 empty；没有断面统计或推断传感器类别 | D、X、C | 通过 | 未开始 |
| SDK-025 | devices.list/get 返回 V1—V6 的允许档案，固定产品版本／厂商／型号正确 | E、C | 通过 | 未开始 |
| SDK-026 | V7 只有输出 context；不能读完整档案、序列号、其他输出，也不计入 authorized 总数 | E、C | 通过 | 未开始 |
| SDK-027 | V8 的受限来源不返回设备／输出 ID、厂商、型号及历史快照，搜索不能推断其身份 | E、C | 通过 | 未开始 |
| SDK-028 | devices.outputs 按独立输出资格返回，代次、结束状态、单位／方向／时间语义正确 | E、C | 通过 | 未开始 |
| SDK-029 | 产品发布新版本后，既有设备仍使用其固定采用型号；不同厂商同名型号不合并 | E、C | 通过 | 未开始 |
| SDK-030 | installation、device_change、output_generation 历史分别分页、排序且字段安全，不透传原始审计 JSON | E、P | 通过 | 未开始 |
| SDK-031 | source 和 installation 关系不同的设备，按 relation 得到各自正确设施集合 | E、C | 通过 | 未开始 |
| SDK-032 | 批量测点来源返回每个可读通道，不只取首个；第 101 个测点／第 501 个显式通道按预算拒绝 | E、W | 通过 | 未开始 |
| SDK-033 | bound、unbound、restricted、ended、attention、not_applicable 按实际关系区分；派生无物理来源 | E、C | 通过 | 未开始 |
| SDK-034 | 来源切换后当前关系与历史区间正确，旧输出／通道语义依据保留且不泄露受限快照 | E、P | 通过 | 未开始 |
| SDK-035 | 多输出、多测点、跨设施来源按 device_id 去重；来源有效不被称为设备在线 | E、C | 通过 | 未开始 |
| SDK-036 | 项目外设备／历史工程目标不可越权；context 输出仍可用于允许的来源说明 | E、P | 通过 | 未开始 |
| SDK-037 | 地图 F1/F2 可定位、F3 missing；合法 (0,0) 保留，缺坐标不生成零坐标 | X、X2、C、W | 通过 | 未开始 |
| SDK-038 | CRS 转换、未知 CRS、局部坐标、只有桩号均有准确状态与依据，不继承虚假父坐标 | X、X2、C、W | 通过 | 未开始 |
| SDK-039 | bbox 过滤和分页正确，位置状态计数与命中数分开；非法／跨边界 bbox 按确定规则处理 | X、X2、C、W | 通过 | 未开始 |
| SDK-040 | GeoJSON 要素通过获准固定版本分页，空间授权不额外开放关联设施详情 | X、X2、C、W | 通过 | 未开始 |
| SDK-041 | blank/local 底图在隔离应用中可用，local 保留合成示例标识；tile 不接受任意 URL 或非法 z/x/y | X、X2、C、W | 通过 | 未开始 |
| SDK-042 | 按设施取得布设图、模型、GIS 资料及获准已发布版本；BIM 依据格式说明而非文件名猜测 | X、X2、C、W | 通过 | 未开始 |
| SDK-043 | 使用项目默认版本；unset/unavailable 区分，不回落租户最新版本或泄露隐藏默认 ID | X、X2 | 通过 | 未开始 |
| SDK-044 | 查询固定版本后默认版本变化不改变其 manifest／文件；新查询按未来成员策略处理 | X、X2 | 通过 | 未开始 |
| SDK-045 | 图纸页／区域、模型元素／片段／属性 DTO 可用于真实查看，geometry 与 properties 授权独立生效 | X、X2、C、W | 通过 | 未开始 |
| SDK-046 | 构件关联、测点挂载和坐标依据正确；未授权业务对象不因模型可读而公开 | X、X2、C、W | 通过 | 未开始 |
| SDK-047 | 101 份位置候选全部可分页取得；相机／默认页／隐藏元素／主通道设置按各自授权投影 | X、X2 | 通过 | 未开始 |
| SDK-048 | 分块文件摘要、取消、授权复验和 Blob URL 释放正确；SDK 不返回原件存储地址／任意下载许可 | X、U、L | 通过 | 未开始 |
| SDK-049 | 基础资源计数为 3 设施、6 构件、8 测点、14 个所选通道，direct/derived=12/2 | C | 通过 | 未开始 |
| SDK-050 | 桥梁类型汇总为 2／5／6／11；各设施及类型分组可与项目同范围总计对账 | C | 通过 | 未开始 |
| SDK-051 | 无 channel_target 或通道资格时不伪造 channel_count=0，返回明确未提供状态 | C、W | 通过 | 未开始 |
| SDK-052 | authorized 设备=6；monitoring_bound=5；后者设施分组为 3/2/1，additivity=overlapping | C | 通过 | 未开始 |
| SDK-053 | 厂商 authorized=3/3、bound=3/2；产品／型号互斥分组及 unassigned 正确 | C | 通过 | 未开始 |
| SDK-054 | 多个视图／输出重复命中同一设备只计一台，组分页不改变全局 totals | C | 通过 | 未开始 |
| SDK-055 | raw ever/active/sample=120/117/114；quality_counts 合计=117，bad=2、suspect=1 | C | 通过 | 未开始 |
| SDK-056 | 各设施 raw active=49/39/29、sample=48/38/28；桥梁=88/86，隧道=29/28 | C | 通过 | 未开始 |
| SDK-057 | 派生只含 X01/X02，logical/valid/no_result=20/16/4；X03 有数据也不进入 | C | 通过 | 未开始 |
| SDK-058 | 原始与派生分别返回，未请求种类不是零；无结果不填零，不输出含混合计和跨单位均值 | C | 通过 | 未开始 |
| SDK-059 | `[T0,T0＋5分钟)` raw=60/57/54；from 包含、to 排除，时区换算不改变同一绝对时段计数 | C | 通过 | 未开始 |
| SDK-060 | 同观测重复来源、重试、重新采用及物理存储副本不重复增加 ever；撤销正确影响 active | C、N | 通过 | 未开始 |
| SDK-061 | 同通道多视图时间窗口并集去重，时间空隙不填满；超声明硬时间边界被拒绝 | C、N | 通过 | 未开始 |
| SDK-062 | 有资源无观测返回精确零；无资源为 empty；依赖不可用、汇总未就绪不伪造零 | C、N | 通过 | 未开始 |
| SDK-063 | 派生覆盖不足显示 partial／原因／已有计数；统计不会触发派生重算或隐瞒缺口 | C、N、M | 通过 | 未开始 |
| SDK-064 | 服务端完整计算 totals，任何 group 页都相同；超预算明确失败或 pending，绝无前 500 截断总量 | C、F、L | 通过 | 未开始 |
| SDK-065 | 原始及指定派生通道最新值、明细、趋势、逐通道统计和 overview 均通过真实 Provider 读取 | C、N、W、U | 通过 | 未开始 |
| SDK-066 | range/all_history/follow_seconds 互斥，显式时段及实际授权覆盖可核验，空时间请求不悄悄默认最近一天 | C、N、W、U | 通过 | 未开始 |
| SDK-067 | 派生选择使用 ID／固定 metric_ref；名称重复、指标新版本和新增通道不改变固定选择 | C、W、P | 通过 | 未开始 |
| SDK-068 | view latest 分页完整，明细每页≤1,000、趋势≤50 通道；超限不会静默删除通道 | C、N、W、U | 通过 | 未开始 |
| SDK-069 | 明细游标稳定，读取中新增观测不污染旧快照，刷新后新查询包含新数据 | N、C | 通过 | 未开始 |
| SDK-070 | raw_samples 下钻逐页明细数=114；raw_active=117，设施分组下钻与相应计数一致 | C、N、W、U | 通过 | 未开始 |
| SDK-071 | derived_samples 下钻=16；derived_logical=20，4 条无结果状态可辨认且无伪数值 | C、N、W、U | 通过 | 未开始 |
| SDK-072 | 新数据或新派生头到达后，旧统计下钻继续使用原快照；独立新查询报告新的读取依据 | N、C | 通过 | 未开始 |
| SDK-073 | 伪造 group_key、改写选择、跨主体 token、拿 query_id 直接当 selection 均失败 | W、N、L、P、C | 通过 | 未开始 |
| SDK-074 | summary-only 下钻被拒绝；有统计和 details 能力但缺数值字段资格仍不能取得数值 | W、N、L、P、C | 通过 | 未开始 |
| SDK-075 | 撤销观测历史另验 history 操作／窗口／字段资格，ever 汇总不会绕过历史明细权限 | W、N、L、P、C | 通过 | 未开始 |
| SDK-076 | 大于 JS 安全整数的计数、修订和值保持 string，不经 Number 中转丢精度 | W、U、D | 通过 | 未开始 |
| SDK-077 | 超过同步预算返回一个 pending query，轮询同一 ID 得到完成结果，不每次创建新任务 | N、L、U | 通过 | 未开始 |
| SDK-078 | queries.cancel/release 和 signal 取消幂等；子选择随父查询／主体到期，其他消费者不被误取消 | N、L、U | 通过 | 未开始 |
| SDK-079 | 三类统计订阅以新结果替换，groups 与 totals 属于同一 query，重复事件不重复累计 | D、E、N、U | 通过 | 未开始 |
| SDK-080 | 新旧 generation、sequence、切换视图和慢请求竞争正确，旧响应不覆盖新视图 | L、N、U | 通过 | 未开始 |
| SDK-081 | SSE 断线后在保留窗口内恢复；超窗／环境代次变化 reset，重新取得当前合法结果 | E、N、P、U | 通过 | 未开始 |
| SDK-082 | 最后消费者释放后停止无用计算；同条件多消费者共享不会交叉读取不同字段／授权结果 | L、N、U | 通过 | 未开始 |
| SDK-083 | worker 重启／超时／重复领取时凭租约与 fencing 完成一次有效结果，旧 worker 不能覆盖 | L、N、U | 通过 | 未开始 |
| SDK-084 | 缓存、分页、后台结果、SSE、readFile 均在设备／空间／来源或授权修订变化时正确失效 | D、E、X、N、L | 通过 | 未开始 |
| SDK-085 | PostgreSQL／ClickHouse／对象存储故障区分 dependency_unavailable、pending、失败，不伪造 empty | X、N、L | 通过 | 未开始 |
| SDK-086 | 50,000 成员可按预算解析；50,001 明确拒绝；结果／字段／文件超预算不截断伪成功 | B、W、X | 通过 | 未开始 |
| SDK-087 | 第 9.5 节负载达到规定延迟／截止目标，附固定环境、冷／热分布、扫描量及内存证据 | F、J | 通过 | 未开始 |
| SDK-088 | Redis 丢失、查询清理和环境恢复不扩大权限、不删除正式数据；旧游标和主体不可继续使用 | R、L、P | 通过 | 未开始 |
| SDK-089 | 管理端服务端搜索／分页选择项目范围与指定派生，展示匹配量、字段、时间和未来成员策略 | D、H、B | 通过 | 未开始 |
| SDK-090 | 授权／配置预览分别检查操作权限，候选和样本不因 prepare 或操作者身份扩权 | D、H、B | 通过 | 未开始 |
| SDK-091 | prepare→publish 之间授权／默认资料修订变化会重校验；required 能力不可用阻止发布 | P、X、H | 通过 | 未开始 |
| SDK-092 | 真实预览、发布、固定终端、撤权、withdraw、rollback 全流程可验证；回退不恢复旧权限 | H、P | 通过 | 未开始 |
| SDK-093 | 项目归档可合法只读和撤权，禁止新授权扩大、配置和发布；全链路不绕过归档检查 | P、X、H | 通过 | 未开始 |
| SDK-094 | 独立目录安装 SDK／工具 tgz 并构建验收工程，校验／打包／上传／真实运行成功，无仓库内部依赖 | A、H | 通过 | 未开始 |
| SDK-095 | 模拟场景标识明确且覆盖约定状态；真实验收程序可查看树、地图、来源、图纸模型及时间筛选 | M、H、X2、C、H2 | 通过 | 未开始 |
| SDK-096 | 旧 SDK＋新运行时、旧包＋新工具通过回归；新 SDK＋旧运行时明确报告新能力不可用 | A、U、W、H | 通过 | 未开始 |
| SDK-097 | 新旧数据库升级／格式缺省／混合部署防护／二进制降级流程有证据，未知格式绝不扩大范围 | G | 通过 | 未开始 |
| SDK-098 | 管理与运行协议生成一致，pnpm check、相关领域集成／race／浏览器和负载检查全部通过 | Q、W、A、H、H2、F、J | 通过 | 未开始 |
| SDK-099 | 文档、可安装包、样例 ZIP、启动入口、权限步骤、计数口径、恢复说明及安全证据全部交付 | A、R、DOC | 通过 | 未开始 |
| SDK-100 | SDK-001—SDK-099 逐项可追溯，无未解决必需项；自验通过后才提交用户，用户确认前不记 Accepted | 全部证据与本账本 | 通过 | 未开始 |

## 验证边界

记录的是当前工作区、隔离本机实验环境的自验，不是生产部署或生产容量认证。用户现有环境未迁移；没有执行无关模块的完整端到端回归。大模型／超大原件受明确文件预算限制，局部坐标和未知 CRS 不伪造地理坐标，local 底图保持合成示例声明。用户人工验收完成前不记 Accepted。
