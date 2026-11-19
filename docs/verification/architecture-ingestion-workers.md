# 接入、解析和后台调度自验

本页对应 ARC-018 至 ARC-021、ARC-026、ARC-028 和 ARC-037。人工接受状态沿用[架构自验总账](system-architecture-refactor.md)。本轮隔离产物位于 `.artifacts/architecture-refactor/run-20261118/`。测试账号、机器令牌、证书和数据库配置保存在权限受控的私有夹具目录。

2026 年 11 月 19 日，用户将本轮验收收窄至功能与正确性。发行 `59a985d1` 的 18,000 帧原接入压测和 ARC-037 九轮量化均按用户要求跳过，取消时发送帧数为零。此后的候选沿用 `scope/functional-acceptance.json`。此前候选的量化日期、版本、九份历史收据摘要及取消时的进程检查见 `worker-checks/measurement-scope-59a985d1.json`。

## 可重放的功能用例

在仓库根目录执行以下命令。冻结源码和二进制目录由运行账本的 `baseline.json` 提供。A 使用 Go 测试覆盖层装入同一份断言和隔离对象地址，原领域代码、迁移和机器契约均来自冻结源码。两端各自新建业务空间、数据库 schema、入口和对象桶。

```sh
run=.artifacts/architecture-refactor/run-20261118
node scripts/architecture-worker-checks.mjs "$run" all
node scripts/architecture-worker-checks.mjs "$run" summary
```

| 编号 | 本轮核验 | 原始结果 |
| --- | --- | --- |
| ARC-018 | 真实 mTLS DDP 拆包、粘包，厂家 HTTPS 多帧，同键重交、同内容再次上报，来源关联水位和恢复 epoch 隔离 | 两端同一领域用例通过，详细用例名与日志位置见 `worker-checks/summary.json` |
| ARC-019 | 四次分别在写前、鉴权回读失败、写入收据事务提交前、提交后且云端确认前结束接管进程 | 两端都能重启完成原确认意图，每张已确认收据都有匹配原文、摘要和 PG 证据 |
| ARC-020 | 识别、网关解复用、固定方法版本、人工和自动解析、权限撤销、恢复代次，多实例占用执行器及丢失成功响应 | 两端业务用例通过。B 的三个独立数据库连接池共同遵守容量，重试复用机器执行身份，两次技术尝试只选中一个结果 |
| ARC-021 | 全部 12 类任务的 21 组输入或模式、错误角色、按 ID 领取、短试算和后台接续、取消和优先级、非法输入退避终止 | A/B 既有效果相同。B 的每个错误角色成功领取数为零，不能再领活动中的同一租约 |
| ARC-026 | 停留中的兼容进程跨代提交，PG 补扫先领取后再收到真实 JetStream 同键投递，多个接收者同时处理，响应抵达后的撤权 | 有效代次只形成一个识别决定，产生预期的两个单元，旧票据无法提交，正式观测为零 |
| ARC-028 | 无新消息下的派生到期、邀请过期、头像恢复、文件清理保留、租约续领、监测日核查与空闲重启 | 两端公共用例通过，B 两个调度者对同一到期时钟只有一个回调实际执行 |

每个执行目录保留 `go-test.jsonl`、可读的 `domain.log` 和标明命令、耗时、退出码的 `result.json`。汇总按具体测试用例核对。曾有一次 broker 测试进程在生成临时配置时使用错误的工作目录，失败记录和修复后的定向重试均保留。

冻结 S0 的近期重放通过 14 项接入及解析用例、11 项调度与维护用例，分别用时 117.242 和 84.793 秒，退出码均为零。两次运行首尾的提交为 `220aee11`、原包 SHA-256 为 `cf757341…`。逐项结果见 `worker-checks/a-yBLVEI/result.json` 和 `worker-checks/a-domain-1XW1UB/result.json`。

发行 `c0583da0` 的接入组 21 项全部通过，用时 305.291 秒。调度与维护组第二次完整运行 34 项全部通过，用时 149.295 秒。两组首尾源码摘要均与发行归档一致。逐条核对 S0 与本次发行的原生 Go 事件后，六个 ARC 的 61 条断言全部通过。四份原始记录、测试入口、程序和源归档的摘要见 `worker-checks/functional-workers-c0583da0.json`。

调度组首次完整运行有 33 项通过，1 项失败。原试算用例要求空输入在 3 秒内同步返回零输出，当时任务在创建约 3.072 秒后转为后台排队，实际响应为 202。保留相同预算、数据库路径及原断言，第二次整组运行的空输入与三行实际输入分别在 1.238 秒和 1.104 秒内同步完成，长范围任务仍按预期入队并可取消。两次原始记录及同一发行审计入口内独立通过的实例见 `worker-checks/domain-trial-c0583da0.json`。

发行源码 `59a985d1` 的对应两组分别通过 21 项和 34 项，包含命名子用例，用时 244.540 和 172.036 秒，均退出 0。执行前和结束时的源码摘要都匹配该发行清单。响应证据重试、丢失解析响应、PG 与 broker 竞争、取消恢复任务及 v44 升级均已在本次接入组实跑。原命令和逐项结果见 `worker-checks/b-bQVUMs/result.json`、`worker-checks/b-domain-hYjkKg/result.json`。该版的六项 ARC 对照保存在 `worker-checks/summary-59a985d1a8ad19a11a570ae82d9f92b720636c2e95b410a9478a570172f140c5.json`。

同一发行的公开接入与解析回放在两侧各执行 171 次请求，覆盖 123 个操作，逐个比对完整响应体后差异为零，用时 134.441 秒。读取新来源前先等中继报告空监听，发布后再等上报的监听进入 `active`，核对所属来源与应用修订号。前一候选的两条观测时点差异、当前两侧的原始响应及修复验证保存在 `worker-checks/http-source-observation-8ee06505456f/`。

发行源码 `b4a1f272` 的接管、解析及迁移命令在 `worker-checks/b-QsnT5Y` 通过 20 项断言，耗时 235.468 秒。调度、派生与维护命令在 `worker-checks/b-domain-vMrvkT` 通过 34 项断言，耗时 146.578 秒。两者均正常退出，用例开始与结束时的源码摘要与选定发行一致。单连接事务用例覆盖成功与回滚后的连接复用和超时恢复。两次执行使用原领域测试的隔离 schema 和夹具连接。四份部署授权 SQL 的摘要和两个命令的原始日志摘要保存在 `worker-checks/b4a1f272-command-evidence.json`。拆分角色的真实登录权限由各负载轮的 PostgreSQL 会话记录核验。

前一候选的 `worker-checks/b-bXSSnW` 与 `worker-checks/b-domain-XAR2FH` 分别保留 19 和 34 项断言。已保留计划的实库复查逐次使来源资格、终端资格和租户授权失效，证据见 `worker-checks/retained-authority-pg-qualified1.jsonl`。

v44 迁移用例在旧数据库上实际运行固定方法，将结果写入对象存储，并通过所属用户读出位移 12.34 mm。A 升至 v58，核对了 9 张历史表。B 升至 v59，另核对技术尝试表，10 张表的行摘要相同，并由使用四个共享执行槽的现行 worker 重放原方法。两侧结果与日志路径见 `worker-checks/v44-upgrade.json`。

恢复任务在进度没有变化时保留 revision 和任务事件，在取消、重试和代次更替时继续产生对应的业务变更。拆分后的两个专项均已通过真实 parser、relay、PG 和对象存储执行，分别耗时 217.31 秒和 86.22 秒，逐测试退出状态见 `worker-checks/b-ejlpM9/result.json`。

在前一候选期间，`pnpm test:ingestion` 和 `pnpm test:parsing` 分别耗时 546.549 秒和 332.163 秒，退出码均为零。v44 迁移在两个原始入口分别耗时 14.50 秒和 19.81 秒并通过。解析入口也覆盖 parser 服务、模糊测试、平台和 HTTP。最终发行的源码及各命令版本对应关系另见 `worker-checks/original-regressions.json`。

冻结 S0 的 `pnpm test:mfa` 重放用时 500.694 秒，5 个原阶段均通过，桌面及窄屏共 6 个浏览器用例通过。随后 `pnpm test:parsing` 用时 164.083 秒，服务、模糊测试、平台和 HTTP 四阶段均通过。平台阶段的原用例有 10 个要求启动接入 relay，另有 1 个要求显式启动持续负载，这 11 个在该命令中跳过。两个原命令均退出 0，结束时重新核对 S0 归档摘要。原命令、阶段日志和摘要分别见 `regression/baseline/original-phase-evidence/test-mfa/2026-11-18T22-19-13.820Z/` 和 `regression/baseline/original-phase-evidence/test-parsing/2026-11-18T22-27-37.252Z/`。

S0 的原 `pnpm test:identity` 用时 4,635.104 秒，124 个包的 100 批 Go 测试全部结束。发现的 522 个测试中，497 个通过，25 个因原测试的前置条件跳过。对照此前失败的 92 个同包同名用例，本次全部通过。批次、用例和原日志摘要位于 `regression/baseline/original-phase-evidence/test-identity/2026-11-19T00-16-46.245Z/checks.json`。

B10 `b09feb94` 的会话持久化专项已通过。真实 HTTP 入口在原两小时空闲时限前接受最后一次活动并返回 204。原期限经过后，热缓存读取和清缓存后的读取都返回 200。已空闲超过两小时、已达十二小时绝对上限、已撤销三种状态的活动请求均返回 401，Redis 未新增活动，随后写回也未改变 PostgreSQL 中的活动或期限。两版共用同一 HTTP 断言。冻结源码还单独复现了原问题，原始活动到达数据库时间之前清除了待写标志。B10 使用真实 PG 和 Redis 核验 12 种时钟与并发场景，包含缺少新字段的旧缓存，以及相同毫秒下版本变化的确认。两份收据见 `identity/session-activity/baseline/result.json` 和 `identity/session-activity/candidate/result.json`。独立重放可执行 `node scripts/architecture-session-activity.mjs "$run" baseline`，或将角色改为 `candidate`。源码归档、五个源文件和身份二进制的对应关系见 `worker-checks/identity-approved-session/release-binding.json`。

候选 `59a985d1` 的原身份命令在第 82 批的 `TestArchitectureColdStartOutages` 发现并发读写。同一监测配置传给多个角色，各自构造存储时改写了共享的副本地址切片。该版完整 106 批中 530 项通过、46 项按前置条件跳过、1 项失败。发行 `c0583da0` 让每个构造实例先复制副本切片。原冷启动用例和新增的共享配置用例在同一隔离 PostgreSQL、Redis 和 Go `-race` 下各连续通过三次。修复前后的原生竞态日志与两份发行归档的实物核对见 `worker-checks/monitoring-storage-race/verified.json` 和 `b9-release.json`。

`c0583da0` 的原 `test:mfa`、`test:parsing`、`test:ingestion` 和 `test:parsing:regression` 都已完整通过，用时分别为 313.869、148.462、203.021 和 452.930 秒。原 `test:identity` 用时 6,081.300 秒，完整 106 批中 530 项通过，46 项按原条件跳过，2 项失败。第 47 批新增共享配置用例和第 82 批原冷启动用例均通过竞态检测，前一轮曾失败的 26 项也全部通过。本次失败的用例分别要求有界试算在 3 秒内返回结果、真实会话活动落入 PostgreSQL。完整原结果、两项独立复验及所有批次的日志摘要见 `worker-checks/monitoring-storage-race/b9-original-full.json` 和 `worker-checks/identity-b9-final-originals/`。

B10 `b09feb94` 首次执行原 `pnpm test:identity` 时，第 20 批的配置试算和控制收据用例分别在 2,853 和 2,949 毫秒返回 202，两项要求同步返回的原断言均失败。两次业务跟踪的输入行数均为零。测试计划列出 145 个包、107 批、582 个顶层用例。已完成前 30 批后，为释放共同测试机资源，在第 31 批开始时按调度要求中断。原命令运行 1,664.161 秒，退出 1，Go 原生结果记录 `interrupted=true`。本次得到 164 项通过、2 项失败，第 31 批的 5 项没有终态，第 32 至 107 批尚未执行。源摘要、原断言、逐次试算、31 份日志摘要与四个容器的退出时间见 `worker-checks/identity-approved-session/b10-initial-batch-20/interrupted.json`。

两端的 `regression/<侧>/identity-batches/<原命令开始时间>/` 保存 `command.log`、原生 `plan.json`、`result.json` 和逐批 Go JSONL。`scripts/architecture-identity-regression-evidence.mjs` 从这些副本重新核对发现的测试、竞态检测开关、原跳过原因、每批退出和未开始的批次。冻结 S0 的完整 100 批与 B10 首次中断的 31 批分别得到完整通过和中断，两个归档的摘要见 `worker-checks/identity-approved-session/original-batch-receipts.json`。

B10 的原 `test:mfa` 用时 590.659 秒，5 个阶段和 6 个桌面或窄屏浏览器用例通过。原 `test:parsing` 用时 308.201 秒，服务、模糊测试、平台、HTTP 四阶段都通过，平台阶段包括 17 个通过和 19 个按原夹具条件跳过的顶层用例。原 `test:ingestion` 用时 365.545 秒，13 个接入用例通过，覆盖真实中继和 v44 旧解析证据回读，持续负载按原开关跳过。三个原命令均退出 0。逐阶段日志见 `regression/candidate/original-phase-evidence/` 下的对应命令目录，接入结束时的存储状态见 `worker-checks/identity-approved-session/b10-general-handoff.json`。

用户于 11 月 19 日要求结束本轮验收。B10 的原 `test:parsing:regression` 在首个业务阶段按要求中止，运行 80.680 秒，退出 1。审计边界、诊断和客户端三阶段尚未开始。第二轮完整身份回归和 B10 的六项工作器专项均未启动。本次采用已记录的会话时钟、身份原用例和 HTTP 边界专项结果。14:22:33 核实时，接入组和身份组的 13 个专属容器均已退出，12 个回环端口均已关闭。停机记录见 `worker-checks/identity-approved-session/final-user-stop.json`。

原身份用例在服务产生真实活动后调用 `Flush`，然后立刻从 PostgreSQL 读取最后活动时间。冻结基线一次原生服务观测中，数据库比应用时钟慢 7.020 至 7.374 毫秒。提交前活动时间领先数据库 5.643 毫秒，Redis 清除了待写标志，数据库仍保留约 119 分钟前的活动。S0 与 `c0583da0` 的这两条持久化函数逐字相同。将数据库与 Go 进程放在同一 Linux 内核后，原用例各运行一次，分别在 25.93 秒和 28.84 秒通过。两次时钟测量的不确定区间均包含零。原始记录及源码摘要见 `worker-checks/identity-b9-final-originals/clock-handoff.json` 和 `linux-clock-conclusion.json`。

在授权登录后构造比数据库领先 850 毫秒的独立样本，两版的会话列表读取 Redis 时均显示最近一秒内有活动。等数据库到点，清空测试会话缓存，再用新的 PG 和 Redis 连接读取，两版均显示最后活动约在 119 分 52 秒前，原两小时截止点后要求重新登录。隔离副本的定时写回实验保留待写标志，到点后再次检查会话，只有仍有效且权限版本相符的活动落库。本次发行的原始行为与隔离实验分别保存在 `worker-checks/identity-b9-final-originals/cache-loss-visible.json` 和 `real-scheduled-activity.json`。

显式写回的隔离对照令 PostgreSQL 事务时钟在首次条件写时比服务生成的真实活动慢 9 毫秒。仅保留待写标志时，两版在 `Flush` 返回后均读到旧值。仅对服务生成且在本机已到时的活动作一次有界复核后，S0 和 `c0583da0` 分别用 28.286 和 19.286 毫秒返回，两边都已能立即从 PostgreSQL 读到原活动时间。等待预算为整个扫描共用 25 毫秒，总耗时还包括 Redis 读取、SQL 和调度。850 毫秒时差在首轮仍保留待写，数据库到点后写入。撤销、认证版本变化及绝对过期的样本均未延长会话。受控时差只作用于每个用例的私有 schema。四组原始结果见 `worker-checks/identity-b9-final-originals/synchronous-clock/anchored/results.json`，服务自身生成的 850 毫秒时差样本见同目录的 `trusted-large-results/results.json`。用户随后批准将补救纳入本轮。正式源码在原身份、撤销和回滚三个用例分别通过，耗时 19.93、22.32 和 15.29 秒。正式修复及新发行完整原命令另行按实际结果记录，见 `worker-checks/identity-approved-session/`。

同一冻结基线的原 `pnpm test:ingestion` 用时 313.867 秒，退出 0。接入领域 12 个顶层用例通过，包含 v44 迁移后回读原结果（12.00 秒）。持续负载用例按原开关跳过。原接入会话、日志和首尾版本记录见 `regression/baseline/original-phase-evidence/test-ingestion/2026-11-18T23-53-15.890Z/`。

冻结基线的原 `pnpm test:parsing:regression` 用时 580.764 秒。业务回归、5 万行审计边界与中途撤权、诊断、客户端四个阶段全部退出 0。四份原日志及摘要见 `regression/baseline/original-phase-evidence/test-parsing-regression/2026-11-18T23-58-33.335Z/`。

发行源码 `59a985d1` 的原命令 `test:mfa`、`test:parsing`、`test:ingestion` 顺序运行，分别用时 380.350 秒、174.375 秒和 219.371 秒，均退出 0。MFA 完成原 5 个阶段和 6 个真实浏览器用例。解析完成原 4 个阶段，其中平台阶段通过 17 个顶层用例，19 个按各自夹具条件跳过。接入命令启动真实中继后通过 13 个顶层领域用例，v44 迁移回读用时 8.91 秒；另行启用的持续负载用例在日常接入命令中跳过。另一次原 `test:parsing:regression` 用时 454.234 秒，业务、5 万行审计边界、诊断与 UI 四个阶段全部通过。各命令的原报告、原日志和首尾版本摘要保存在 `regression/candidate/original-phase-evidence/` 的对应目录。

原入口 `INFRAPRO_INGESTION_LOAD_SECONDS=180 pnpm test:ingestion:load` 在独立 schema 和对象桶写入 18,000 个新帧。每秒 50 次 DDP 上报和 25 个双帧 HTTPS 批次，在两个租户间共形成 13,501 张含注册报文的收据。第 120 秒停止兼容工作器，第 165 秒重新启动，原程序记录停稳与重启的实际偏移。两侧命令分别保留证据 `ingestionLoadEvidence`，可从中找到原会话的 `load.jsonl`、`tests.log`、`result.json` 及 SHA-256。此处端到端 P95 取解析计划完成时刻。下述 ARC-037 单独验证转发链路断开和所属用户读回。

在并行功能窗口，`b4a1f272` 的原入口用 `415.899` 秒送完 18,000 帧，速率 `43.280` 帧／秒。13,501 张收据全部确认。17,999 个结果完成后，一张收据停在 `awaiting_identity`。它的 81 字节原文与同来源另外 8,999 张正常报文具有相同的 SHA-256。用原执行身份和同一解析器直接回放，完整响应与原决定中保存的响应一致，均识别出一个测量单元。当时 PostgreSQL 在预留响应对象的 SQL 中触发语句超时，旧代码将存储故障记为 `invalid_response` 并拒绝重试。原库状态、原始字节、解析响应与数据库日志见 `worker-checks/original-identification-test-Sf8g1L.json`、`original-direct-response-test-Sf8g1L.json` 和 `original-pg-20261118T202530Z.json`。

旧候选的原命令在发送结束后持续观察 90 分钟，最后 150 次观测均保留同一条未完成识别。接管 P95 为 88.615 秒，已完成计划的端到端 P95 为 887.438 秒。命令自然退出 1，逐帧计数和原日志摘要保存在 `worker-checks/original-b4-18k-closure.json`。原报告的失败计划数为零。这张收据的识别决定已被拒绝，尚未生成解析计划。

冻结 S0 `220aee11` 在独占时段的新会话 `test-RGyfX0` 耗时 180.033 秒送完 18,000 帧，达到 99.982 帧／秒。原工作器在第 120.005 秒停稳、第 165.002 秒恢复。13,501 张收据全部确认、18,000 个结果全部完成，失败计划、重复计划、跨租户单元及待确认数均为零。接管 P95 为 43.656 秒，端到端 P95 为 357.152 秒，首帧至业务核对共 546.064 秒。原命令连同准备和清理共 593.015 秒，退出 0。20 次宿主采样的外部验收作业数均为零。原始收据位于 `regression/baseline/original-phase-evidence/test-ingestion-load/2026-11-18T21-52-34.475Z/`，云端两条接入序列的独立核对见 `worker-checks/original-arrivals-test-RGyfX0.json`。

原 S0 命令退出后，七个空闲且有数据卷的专属组件已停止，两台使用内存文件系统的 PostgreSQL 继续运行。此前旧候选的两台通用测试 PostgreSQL 已停止，其临时数据目录使用 `tmpfs`，停止会释放其中的内容。原始收据和解析回放证据另存于主机，接入专用 spool 和业务 PostgreSQL 继续运行。逐实例核对见 `worker-checks/original-baseline-18k-volume-parking.json` 和 `original-b4-parked-storage-scope.json`。

`b6a60e55` 修复后，响应证据写入暂时失败会进入原有退避，后续尝试重新核对租户授权、代次和执行槽。专项在真实 PostgreSQL 响应对象插入中触发 10 秒语句超时，并分别在网关解复用和测量解析时中断对象写入。三个业务目标的第二次尝试都用新的 fence 完成，两项结果分别通过所属用户读回，正式观测增加数为零。三项真实测试和 12 个故障分类用例的日志索引见 `worker-checks/response-staging-fix/verified.json`。

## 历史九轮接入负载与复现方式

按下列命令从账本取出当前发行目录后，单次启动可使用 `node scripts/architecture-ingestion-load.mjs "$run"`。与其他量化作业共用机器时，在第一个终端冻结、验证并预热，看到首个 `warm.json` 后在独占时段由第二个终端放行。各轮都会独立预热。独占期间可用 `node scripts/fixtures/architecture-benchmark-window.mjs "$run" "$campaign" watch` 每 10 秒记录其他实测进程类别和 PID，随后用 `summary` 对照每轮的计时和授权读回窗口。常驻验收服务单独留痕。

```sh
run=.artifacts/architecture-refactor/run-20261118
campaign=repeat01
INFRAPRO_ARCHITECTURE_HTTP_RELEASE=$(node -e 'const fs=require("node:fs"); console.log(JSON.parse(fs.readFileSync(process.argv[1],"utf8")).directory)' "$run/candidate.evaluated.json")
export INFRAPRO_ARCHITECTURE_HTTP_RELEASE
docker compose -f deploy/ingestion-lab/compose.yaml up -d --wait
docker compose -f deploy/compose/test.yaml up -d --wait redis
node scripts/architecture-ingestion-load.mjs "$run" freeze "$campaign" --fresh
node scripts/architecture-ingestion-load.mjs "$run" probe-campaign "$campaign" pg mq
node scripts/architecture-ingestion-load.mjs "$run" warm "$campaign"
```

第二个终端执行 `node scripts/architecture-ingestion-load.mjs .artifacts/architecture-refactor/run-20261118 start repeat01`。九轮授权回查完成后，第一个终端可以执行 `node scripts/architecture-ingestion-load.mjs "$run" report "$campaign"`，从原始数据重算门槛。

`--fresh` 先在 `registration.json` 登记九个全新目录和 A、B 的实物摘要。每轮使用自己的业务数据库、接入中继 schema、对象容器和两个租户。PG 和 JetStream 的常驻进程使用正式签发器安装的四份 DCL。三个数据库登录分别只继承一个领域组，事件中继、接管和解析调度的 SQL 连接上限依次为 1、3、8。预热前与授权回查后均从持槽的 PostgreSQL 会话核对成员、数据库所有权、RLS 绕过权限和配额。计时期间按登录记录连接数。`probe-campaign` 使用同一批冻结文件和独立目录，PG 与 JetStream 各读回三个预热结果。

需要在轮次间让出机器时，执行 `node scripts/architecture-ingestion-load.mjs "$run" pause-after-current "$campaign"`。当前轮授权回查完成后，`stopped.json` 记录下一轮名称。在下一个独占时段执行 `node scripts/architecture-ingestion-load.mjs "$run" resume "$campaign"`。程序重新匹配已完成轮的名称、模式、驱动和 B 可执行文件，检查下一个目录尚未使用。若某轮已开始且遇到业务错误，原始样本会保存在该轮目录。

新冻结的 A 和 B 测试驱动均使用同一份计时覆写，在隔断内网连接后记录 Go 单调时刻，等待 45 秒并恢复后保存单调历时和两个独立 UTC 时刻。数据写入 `outage.json` 的 `duration_seconds`、`duration_clock` 和 `utc_seconds`。两边的待测应用沿用发行物原始字节。 两份新驱动均已编译，冻结实物中的隔断、等待、恢复、单调相减指令，以及修改发行物或被测角色时的拒绝结果见 `ingestion/campaigns/clockaudit01/verification.json`。

- 各轮先获得三个暖身结果，再以两个原所属用户读回。放行后重新建立 DDP 连接，并再次从对象服务读回三个暖身结果。性能调优时可在 `warm` 命令后附 `--gate-first`。驱动会对首个 PG 和 JetStream 轮各自计算相对门槛，达标后才启动下一轮。常规完整运行按三轮中位数作正式判定。
- 固定 DDP、厂家 HTTPS 批次和两个租户产生 6,000 个核心帧。两个具有不同投递键的相同批次另产生 4 帧。同键重交、坏报文和外租户终端各有独立断言。每轮应有 4,504 张确认收据和 6,004 个可由所属用户实际读出的结果。
- 按 100 帧／秒的开放式时钟发送。DDP 的写与逐帧 ACK 分开计时，HTTPS 有界并发。第 10 秒起仅断开内网交接 45 秒，公开协议监听和持久 spool 继续服务。追平时间从恢复交接算到队尾清零。完成速率以 6,004 个正确结果除以首帧至队尾清零的总用时。
- 云端收帧时间用于端到端起点。独立 PG 会话每 250 毫秒观察已提交结果，记录首次可见时点。领域执行器在提交选中结果之前已经用 S3 的实际回读校验该不可变响应的字节数和摘要，测试期间两个主体的读取资格固定。每个选中的不可变对象都通过原用户的 `Results` 入口读回，校验版本、测量值、单位、映射和质量。负载追平时刻早于读回审计，以免审计的排队影响逐帧计时。
- 三组的 parser、runner 和外部 relay 使用相同冻结二进制。每组的工作进程合计允许 12 个业务 PG 连接，新版 B 将 3 个连接给接管、8 个给调度、1 个给事件转发。PG 与 JetStream 使用相同配置。每轮新建 2 核、768 MiB、64 个卷位的 SeaweedFS，读回后停止并保留数据。运行目录中的 `storage.json` 记录对象容器与镜像摘要，活动容器、Docker 分配和样本摘要见该组的 `environment.json`。
- 每轮 `inputs.json`、`cloud.json`、`receipts.json` 和 `frames.json` 可逐键连接。`start.json`、`outage.json`、`drained.json` 和 `resources.jsonl` 给出边界时刻及进程、数据库、broker、容器资源。`stage-attempts.json` 保留 4,504 次识别和 6,004 次解析的首次真实领取，以及 6,004 个单元从就绪到规划事务产生首个计划的时刻。B 的 `event-deliveries.json` 含完整耐久事件、接收时间、重复数、消费终态和 broker 序号。B 的事件计数含空扫描、实际 PG／broker 接受数、重复数及完成的三个阶段工作数。MQ 轮另检查三个真实耐久消费者的等待及未确认数量均为零。

已运行批次的 ARC-037 量化判定取自各自的 `ingestion/campaigns/<批次>/comparison.json`。文件保留各轮 P50、P95、P99、最大值、完成速率、追平、逐键业务摘要和净收益。每种模式使用三轮指标的中位数。

本轮统一入口 `pnpm test:architecture load --run-dir <证据目录> --release-dir <发行目录>` 对 ARC-037 和 ARC-038 返回“按用户要求跳过”。独立接入驱动在冻结时核对 `candidate.json`、三只物理角色文件及本地编译的 SHA-256，运行各 B 轮和生成比较报告时再次核对启动文件的摘要。任一角色不同会指出角色并退出。已测发行清单、本轮源码摘要和三只角色的摘要保存在相应批次的 `release-binding.json` 和报告的 `candidate_release` 中。

迭代候选时，可在首轮开始前固定三个已经独立预热、测量完毕的 A 轮。先执行 `node scripts/architecture-ingestion-load.mjs "$run" freeze "$campaign"`，再执行 `node scripts/architecture-ingestion-load.mjs "$run" reuse-baseline "$campaign" a-qualified1 a-full2 a-full3`。程序校验 A 源码、所有进程及驱动摘要、输入样本、业务断言和机器配置，并为每轮封存输入与结果的 SHA-256。复用时还需核对各窗口的宿主常驻进程和容器预算。若背景已经改变，在新环境中执行三轮新的 A。随后按上述 `warm` 和 `start` 命令运行新候选的六轮 B，每个模式仍对应三次独立测量。每个候选的完整对照也留在 `ingestion/campaigns/<候选名>/comparison.json`。

### b3159e62 发行物的九轮结果

这批三只拆分角色使用同一测试维护登录，每个角色有独立应用连接池及运行中槽锁记录。随后加入部署签发流程，每个常驻角色只获本域组权限。后续批次的四份 SQL 和每轮实际登录单独登记。

正式批次 `sealed` 使用冻结 A 源码 `220aee11` 和 B 发行源码 `b3159e62`。六个 B 轮实际运行的接管、解析调度、事件中继文件分别与发行清单逐字节一致。前一组 `directclean` 的 A1 和本批 A 使用相同 S0、驱动、parser、runner、外部 relay、样本与 PG／S3 配额。A1 首尾与本批开始及结束时的 70 个容器、镜像、CPU／内存／swap 和本机角色计数均一致。该锚点在 B 计时前登记到首位，其余八轮按固定顺序新测。全部九轮计时与授权回查期间共记录 378 次宿主快照。377 次未命中其他作业。PG2 的 00:33:35.753 快照匹配到两个 Node 进程，经工具回执确认是四个文件的 ESLint 和 Git 空白检查，耗时 1.242 秒，另有 0.380 秒只读源码摘要。该静态检查没有启动业务、容器或编译。原始命中记录和两项回执保存在 `ingestion/pg2-external-process-attribution.json`。

| 模式与轮次 | 端到端 P50（秒） | P95（秒） | P99（秒） | 最大（秒） | 追平（秒） | 完成（帧／秒） | 整体就绪至领取 P95（秒） | PG 扫描／千项 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `a-directclean1` | 121.112 | 175.241 | 177.730 | 179.063 | 183.545 | 25.169 | 87.121 | — |
| `pg-sealed1` | 118.480 | 161.159 | 163.560 | 164.661 | 169.123 | 26.789 | 64.317 | 1,031.97 |
| `mq-sealed1` | 123.243 | 173.507 | 175.783 | 177.063 | 180.661 | 25.477 | 68.393 | 24.77 |
| `a-sealed2` | 119.059 | 174.130 | 176.729 | 177.620 | 182.474 | 25.283 | 87.016 | — |
| `pg-sealed2` | 118.866 | 163.857 | 166.046 | 166.977 | 171.152 | 26.549 | 64.749 | 1,031.19 |
| `mq-sealed2` | 123.989 | 173.170 | 175.699 | 176.970 | 181.472 | 25.390 | 69.471 | 26.46 |
| `a-sealed3` | 119.473 | 173.011 | 175.683 | 177.202 | 181.316 | 25.406 | 86.219 | — |
| `pg-sealed3` | 121.989 | 164.171 | 166.251 | 167.621 | 171.293 | 26.532 | 64.365 | 1,029.49 |
| `mq-sealed3` | 124.200 | 173.749 | 176.636 | 177.998 | 181.826 | 25.352 | 69.366 | 18.35 |

按各模式三轮中位数，PG 和 JetStream 的端到端 P95 分别为 A 的 94.10% 和 99.64%，追平时间分别为 A 的 93.80% 和 99.45%。JetStream 的 PG 扫描由每千项 1,031.19 次降至 24.77 次，减少 97.60%。整体就绪至首次有效领取 P95 为 PG 64.365 秒、JetStream 69.366 秒，相对变化 +7.77%。

实际注入为 100.030–100.033 帧／秒。每轮确认 4,504 张收据、逐项授权读取 6,004 个结果，九轮共 40,536 张收据和 54,036 个结果。SHA 与逐键分类全部相符，已确认丢失、额外正式观测和跨租户效果均为零。九次外租户终端均隔离且未形成计划或可见结果。

| 三轮中位数 | 总 CPU（核秒） | 峰值内存（MiB） | 磁盘读（MiB） | 磁盘写（GiB） | 采样时最大 PG 锁等待总量（毫秒） | 最老积压（秒） | 重复提示占比 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| A | 696.06 | 874.05 | 36.20 | 1.258 | 38.01 | 97.51 | — |
| PG | 652.82 | 1,095.10 | 318.58 | 1.533 | 6.52 | 71.33 | 0.000% |
| JetStream | 699.76 | 1,148.21 | 257.53 | 1.792 | 6.31 | 77.27 | 0.350% |

主报告保留预排 PG2。另取其余两轮中各指标对门槛较不利的实值做敏感性对照，PG 端到端 P95 和追平分别为 A 的 94.28% 和 93.87%，JetStream 领取 P95 相对 PG 增加 7.85%，扫描减少 97.59%。三轮原值、两端点及比率见 `ingestion/campaigns/sealed/host-observations.json`。

九轮业务 PG 连接峰值均为 12。六个 B 轮都持有中继 1、接管 3、解析调度 8 的配额，原始 CPU、锁、磁盘和连接样本均保留。三轮 JetStream 解析通道分别收到 30、30、30 次重复提示。按接收键和真实领域尝试的完整关联见 `ingestion/campaigns/sealed/stage-capacity.json`。

本批冻结驱动在互斥锁下隔断唯一内网交接代理，关闭活动连接并拒绝新连接。pg-sealed2 原始 UTC 为 `2026-11-18T16:32:53.104372Z` 到 `2026-11-18T16:33:38.102316Z`，相减 44.997944 秒，比 45 秒短 2.056 毫秒。原始 UTC 字段记录到微秒，墙钟绝对准确度没有独立校准。Go `UTC()` 去除单调读数。冻结 A／B 驱动的 ARM64 指令均先执行隔断，再以 45,000,000,000 纳秒调用 `time.Sleep`，返回后才恢复连接。Go 1.27.0 的 timer 使用 `nanotime`，由此可证实际隔断至少 45 秒。历时的精确单调值没有写入旧记录。每轮 `outage-clock.json` 和 `outage-clock.asm` 绑定原始 UTC、驱动和指令摘要。夹具原文及 Go 实现摘录在 `ingestion/campaigns/sealed/clock-proof/`。全部原预排轮次保留。

主结论和九轮原始目录索引见 `ingestion/campaigns/sealed/comparison.json`。冻结 B 发行物绑定见同目录 `release-binding.json`，运行隔离见 `competing-jobs-summary.json`，时钟聚合复核见 `aggregation-audit.json`。用户交互与大屏的并发性能在 ARC-038 单列。

### 首个完整九轮对照

`full` 候选让收到的识别消息在登记时领取对应的耐久工作。三种模式每轮均接管 4,504 张收据并授权读回 6,004 个测量结果，九轮共 54,036 个。原文和业务分类逐键一致，已确认丢失、跨租户效果和额外正式观测均为零。下表列出本次候选的全部轮次。有效领取包括每轮 4,504 个识别工作和 6,004 个解析工作。

| 原始轮次 | 端到端 P95（秒） | 恢复至零待办（秒） | 完成速率（帧／秒） | 耐久就绪至有效领取 P95（秒） | 每千项工作 PG 扫描 |
| --- | ---: | ---: | ---: | ---: | ---: |
| A 1 · `a-qualified1` | 110.82 | 116.96 | 34.91 | 52.33 | — |
| PG 1 · `pg-full1` | 153.79 | 160.95 | 27.80 | 61.08 | 1,031.07 |
| MQ 1 · `mq-full1` | 178.16 | 187.41 | 24.77 | 63.13 | 352.15 |
| A 2 · `a-full2` | 160.57 | 168.12 | 26.91 | 78.01 | — |
| PG 2 · `pg-full2` | 163.30 | 170.39 | 26.64 | 68.06 | 1,030.95 |
| MQ 2 · `mq-full2` | 169.35 | 177.24 | 25.85 | 62.18 | 339.67 |
| A 3 · `a-full3` | 160.39 | 168.07 | 26.91 | 79.55 | — |
| PG 3 · `pg-full3` | 150.23 | 158.09 | 28.18 | 58.10 | 1,031.43 |
| MQ 3 · `mq-full3` | 182.37 | 191.46 | 24.36 | 63.25 | 349.79 |

以三轮中位数计算，PG 的端到端及追平用时分别为 A 的 95.89% 和 95.76%。MQ 的两项用时为 A 的 111.08% 和 111.50%，均超出 110% 门槛。MQ 的每千项 PG 扫描比 PG 保底路径减少 66.08%，整体领取 P95 增加 3.36%。单看解析计划，PG 和 MQ 从就绪到真实执行器首次领取的 P95 分别是 27.83 秒和 44.21 秒。后续 `direct` 候选让可用的解析执行槽直接领取消息对应的工作，并在同一 PG 事务中记录接收和领取。

原始分位数、每轮的进程与存储资源、锁等待、重复计数和最老积压见 `ingestion/campaigns/full/comparison.json`。首轮 A 的冻结输入和构建沿用 `a-qualified1`。随后八个实测窗口共采集 322 次外部作业快照，均无竞争测试，详见同目录 `competing-jobs-summary.json`。这组九轮保留了两项超限的原始结果。

| 三轮中位数 | 总 CPU（核秒） | 峰值内存（MiB） | 磁盘读（MiB） | 磁盘写（GiB） | 采样时最大 PG 锁等待总量（毫秒） | 最老业务积压（秒） | 重复提示占比 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| A | 642.24 | 990.9 | 20.97 | 1.155 | 35.68 | 93.97 | — |
| PG | 632.49 | 1,086.6 | 9.55 | 1.538 | 14.91 | 67.51 | 0% |
| MQ | 704.41 | 1,231.1 | 25.31 | 1.786 | 1,647.05 | 69.69 | 17.45% |

各轮业务 PG 连接峰值均为 12。资源从首帧到追平采样，包含业务进程、broker、PG 和对象存储。Docker 的 CPU 和磁盘采用采样值估计。每次采样会合计当前仍在等锁的业务会话已经等待的时长。

用 `node scripts/fixtures/architecture-ingestion-profile.mjs "$run/ingestion/campaigns/full"` 可离线按业务键连接全部耐久交接与实际执行。三轮 MQ 每轮 6,004 个解析计划中，分别有 3,607、3,464 和 3,384 个由 PG 先领取再收到相同的 broker 投递。相应各轮的 4,504 次识别中分别出现 27、26 和 25 次。只看解析，PG 从首次交接到领域开工的 P95 为 72–76 毫秒，MQ 为 2.30–2.39 秒。每轮连接结果保存在 `ingestion/campaigns/full/stage-capacity.json`。新候选的真实交接专项另核对首次执行、退避、执行端丢失、PG 补领和重复提示，三项均通过，见 `events/parse-direct-candidate/checks.json`。

每轮还有一个发往另一租户终端的隔离样本。九个原数据库均保留其收据和未分配设备的单元，原因为 `terminal_unregistered`，产生的计划和选中结果均为零。六个 B 轮的 6,005 个规划提示中，多出的一个分别关联到该轮的这个隔离单元。按云端幂等键追到实际 PG 主键的记录见 `ingestion/campaigns/full/isolated-unit-proof.json`。

上述 `full` 整个观测期间的 379 次宿主快照都出现五个常驻验收栈。为了给后续独占测量腾出资源，这五个栈已退出。新批次 `directclean` 使用同一套已冻结的解析直接领取程序。该批次按 A、PG、MQ 交错安排九次独立测量。基线选择与构建对应关系见 `ingestion/campaigns/directclean/pairing.json` 和 `release-roles.json`。

### 已归档的容量诊断

首版候选将三个角色的 PG 连接分为 3、7、2。旧版首轮追平 118.92 秒，B 的 PG 首轮为 217.82 秒、第二轮为 211.40 秒，JetStream 首轮为 199.78 秒。首组每个业务键及结果一致，但两种 B 模式均超出相对时限。A 第二轮与另一次功能回归重叠，已记录干扰并排除。完整样本、各进程资源和旧候选摘要保存在 `ingestion/campaigns/measured/diagnostic-comparison.json` 及其指向的轮次目录。另有一次 67 帧／秒的历史容量诊断，保留在 `ingestion/pg-1/diagnostic.json`。

第二个候选保留了每种模式一轮完整回读。A、PG、JetStream 的全部端到端 P95 分别为 110.82、127.22、132.54 秒，恢复至零待办为 116.96、134.18、146.43 秒。云收至本地存储 P95 均在 61.8–62.4 秒，差距主要出现在本地存储后。分阶段统计和三份实际构建摘要见 `ingestion/campaigns/qualified/stage-capacity.json`、`source-digests.json` 和 `builds.json`。可用 `node scripts/architecture-worker-checks.mjs "$run" authority pg-qualified1` 对保留计划重新核验当前权限和失效的授权代次。

事务读取进一步合并后，`pg-final1` 以 100.033 帧／秒接入、断联 45.000 秒。6,004 个结果均授权读回，端到端 P95 为 110.19 秒，追平为 116.01 秒。冷升级的独立功能用例在同一宿主运行至 19:00:27，此轮保留为容量诊断，干扰时段见该轮的 `interference.json`。随后落盘的进程副本守卫改变了启动时的 PG 准入，正式 B 组使用包含这项变更的构建。每轮 `replica-pools.json` 记录了三个角色实际持有的数据库锁及预算内连接数。

包含副本守卫的首组三种模式各读回了 6,004 个有效结果。PG 路径端到端 P95 为 118.68 秒、追平 124.26 秒。JetStream 路径每千项候选扫描由 1,028.83 降至 317.81，但识别等待 P95 从 55.06 秒增至 75.68 秒，全链路 P95 为 125.18 秒、追平 132.34 秒，超过这组基线的 110%。这组的 2,953 次 PG 先接收后重复投递均来自识别通道。样本和分段统计保存在 `ingestion/campaigns/rounds/stage-capacity.json`，PG 及 JetStream 的 61 次外部作业抽查均无干扰记录。

一次只完成预热的 PG 夹具在宿主繁忙时因闲时探活超时，三个工作进程均退出，实际输入为零，记录在 `ingestion/pg-handoff1/warm-fault.json`。随后将闲时探活改为无请求写出的短读取，真实 PG 测试确认暂停后仍持有原后端，管理员终止后端仍会触发失权。负载驱动跟踪角色退出，在放行前重验进程 PID 与 PG 副本槽，并在首个正式收帧时钟前重新从所属用户接口读回暖身对象。第一轮放行时的证据见 `ingestion/pg-full1/replica-pools-at-release.json`。

## 人工复核

1. 在原控制台的接入管理中，以对应合成租户的负责人打开一张成功收据。查看原文摘要、入口、epoch、来源序号以及一个或两个单元，核对计划固定的方法版本。
2. 在解析结果中核查位移为 12.34 mm、质量未评价，随后查看跨租户终端和坏包的原有错误分类。相关测点的正式观测只有执行原有显式发布动作才会增加。
3. 在测试时段暂停内网交接并恢复。运维方可以按 `cloud.json` 和 `receipts.json` 的收据键、摘要与确认核对，再按 `frames.json` 逐帧复核可见结果与队尾。
