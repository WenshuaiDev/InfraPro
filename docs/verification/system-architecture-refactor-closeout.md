# 系统架构重构交付记录

2026 年 11 月 19 日，用户将本轮工作收尾为修复已确认的问题、针对修复复验、保留现有业务数据部署，并实测访问入口和基本登录。冻结后端简称 S0，本次选定的重构后端简称 B10，共用控制台简称 C2。下述开发与复验结果均来自隔离环境。四机生产结果单独登记，人工接受待用户确认。[最新规格](../development/system-architecture-refactor-spec.md) · [用户收尾决定与逐命令记录](../../.artifacts/architecture-refactor/run-20261118/scope/user-closeout.json)

生产标签为 `production-2026-11-19`。仓库内的部署记录保留四机实测摘要及各验收编号的状态。文中指向 `.artifacts/` 的原始浏览器、实验和运行收据保存在部署工作区。[版本对照方法](../operations/production-version-2026-11-19.md) · [可随 Git 查阅的记录](../operations/releases/2026-11-19/deployment.json)

## 实现与交付版本

| 范围 | 已交付的改动 |
| --- | --- |
| 管理 API | 保留公开网关和原路径，以双向 TLS 转发给身份、业务控制、监测查询和文件传输四个服务。每个操作有确定的负责服务，事务、权限复核、流式传输和取消沿用对应领域的实现。保留兼容进程供分组切换和回退。[路由清单](../development/system-architecture-refactor-routes.json) · [公开路由实测](../../.artifacts/architecture-refactor/run-20261118/routes/registered.json) |
| 接入、计算与消息 | 将接管和解析调度拆开，将正式发布、增量计算、历史计算和读取维护分为四个后台进程。业务作业与待发事实同事务写入 PostgreSQL；转发器收到 JetStream 持久化确认才标记发布。消费者按引用寻找原作业，利用租约、代次和幂等键限制重复生效。保留 PostgreSQL 对账和补领路径。[迁移 59](../../packages/go/platform/migrations/00059_architecture_events.sql) · [事件及恢复手册](../operations/architecture-events.md) |
| 大屏与运维 | 大屏运行壳和包资产拆成独立服务，沿用两个外部 origin。离线发行物包含角色配置、按角色授权的 SQL、原生、Compose 和混合部署模板，以及诊断、联合备份恢复与回退工具。[部署与恢复手册](../operations/system-architecture-refactor.md) · [B10 离线包校验](../../.artifacts/architecture-refactor/run-20261118/deploy/offline-release.json) |

S0 冻结源码归档摘要为 `cf757341bc3c429d49147611e019c21a643aad36ef54b57aa9ffabb00cedaaa3`，业务数据库为版本 58。B10 应用源码摘要为 `b09feb944c39eabe1ed8ad3de850812bffaa9a646bfdaa693f3dfdd00c43252b`，封存了 1,981 个源码输入和 31 个程序，要求业务数据库版本 59。用于双后端复验的 C2 含 554 份静态文件，清单摘要为 `f419d21edac5379c42263779b36da33e4d91033ab52b03a0b540e59c81c26288`。[S0 冻结清单](../../.artifacts/architecture-refactor/run-20261118/baseline.json) · [B10 发行清单](../../.artifacts/architecture-refactor/run-candidate-20261119-c2-functional/candidate.json) · [共用控制台与获准的源码变更](../../.artifacts/architecture-refactor/run-20261118/shared-console.json)

## 已修缺陷与针对性复验

| 用户可见的问题 | 修复与实际观察 |
| --- | --- |
| 项目治理页在窄容器里无法翻页 | C2 修正分页器宽度。S0 和 B10 各实测 375、768、1024 和 1440 像素、明暗两种主题，共 16 组。鼠标和键盘均能从首页十条项目翻到下一页再返回，浏览器页面无横向溢出。[逐组几何、项目顺序及截图](../../.artifacts/architecture-refactor/run-20261118/console-corrections/pagination.json) |
| 暂停成员、重发邀请时，切回窗口后丢失已填写的原因 | C2 在同一操作者、同一成员关系与资格复核成功后恢复草稿。两种动作在 S0 和 B10 的真实浏览器中均通过。另一管理员实际撤回办理权限后，两端对应请求均返回 403，目标维持原状态；恢复资格重新打开时，原因框为空。[草稿、权限与目标状态](../../.artifacts/architecture-refactor/run-20261118/console-corrections/drafts.json) |
| 监测导入详情关闭时，列表晚返回会把焦点留在搜索框 | 两端各验 10 种情况。原行早到或晚到、键盘或鼠标关闭时均能回到可见的原行；用户主动移动焦点、原行消失、筛选或工作空间更换、列表卸载时，保留用户当前焦点。[修复前的两侧观察](../../.artifacts/architecture-refactor/run-20261118/regression/browser-diagnoses/monitoring-import-escape-focus.json) · [C2 两侧复验](../../.artifacts/architecture-refactor/run-20261118/console-corrections/import-focus.json) |
| 活动中的会话在缓存丢失后提前到期 | S0 可复现数据库最近活动仍停在约 119 分钟前、Redis 却已清除待写活动。B10 保留待写值，等数据库时间到达后复核会话有效性再写回。真实 PostgreSQL 和 Redis 用例覆盖定时写回、并发新活动、缓存丢失与重连、撤销、身份版本失效、2 小时空闲和 12 小时绝对上限。[冻结版原行为](../../.artifacts/architecture-refactor/run-20261118/identity/session-activity/baseline/result.json) · [B10 分项结果](../../.artifacts/architecture-refactor/run-20261118/identity/session-activity/candidate/result.json) |
| 收到租户邀请后，个人通知摘要在受限服务账号下返回 503 | B9 保留了 18 条服务错误。B10 通过受限的邀请查询读取当前资格和所需的历史租户名。最终用 S0 与 B10 在同一初态分别登录待处理邀请的接收人、已接受邀请的接收人和已接受邀请的发起人。每侧 17 次公开 HTTP 调用，摘要、邀请状态和目标可用性逐项相符，访问其他账号的数据均返回 404。复验后已停本次启用的 9 个依赖实例。[原 503](../../.artifacts/architecture-refactor/run-20261118/http/visual-notification-summary-denial.json) · [角色授权与封存源核对](../../.artifacts/architecture-refactor/run-20261118/deploy/notification-invitation-release-binding.json) · [最终三主体及停车记录](../../.artifacts/architecture-refactor/run-20261118/console-corrections/targeted-c2-notifications-2026-11-19T06-25-17.175Z.json) |

租户管理员候选搜索另有一项兼容决定。未指定租户时，保留 S0 实际返回的四个业务字段，以及固定的 `has_avatar: false`、`avatar_revision: ""`。头像读取仍受原作用域授权。规格记录了这一响应与原 OpenAPI 声明的差额。[业务决定及原字段](../development/system-architecture-refactor-spec.md#1-交付目标与等价标准) · [冻结版候选对照](../../.artifacts/architecture-refactor/run-20261118/regression/browser-diagnoses/c2-tenant-candidate-baseline.json)

开发期间还定位并修正了以下运行时缺陷。各条凭证记录了实际运行的版本。

- **取消浏览器请求后，业务控制服务失去数据库连接配额。** 修复前，三条真实事务被取消后服务就绪检查返回 503；修复后的连接池保留数据库会话和副本配额。五个连接池场景各运行三次，B4 三个视口的公开页面另有真实读回。[故障前后、连接 PID 及浏览器记录](../../.artifacts/architecture-refactor/run-20261118/diag-browser-cancellation.json)
- **两个人员读取事务可在账户行相互等待。** S0 对同一合成主体的并发读和注销实测出现 PostgreSQL 死锁错误 `40P01`。人员读取现按共享锁复核资格，注销和撤销仍有排他顺序；修正候选的人员并发读、注销、大屏和已发布成果输入资格专项通过。该记录保留了当时的候选摘要。[两侧数据库专项](../../.artifacts/architecture-refactor/run-20261118/visual/personnel-read-qualification.json)
- **冷恢复后的固定成果读取和历史摘要重建缺少适当的角色授权。** 恢复脚本重建固定成果元数据投影的访问权限；派生与空间授权仍按所属租户、项目和范围校验。B10 的手工摘要重建有十个受限账号正反例，另有空库重建、联合恢复后的正式读回。[冷恢复授权专项](../../.artifacts/architecture-refactor/run-20261118/deploy/fixed-baseline-restored-acl.jsonl) · [摘要重建角色用例](../../.artifacts/architecture-refactor/run-20261118/deploy/summary-repair-check.json) · [B10 恢复回读](../../.artifacts/architecture-refactor/run-20261118/deploy/joint-recovery.json)

## 其他已留证的隔离验证

- 逐方法和路径模板记录了 1,071 个公开操作的两端合法请求，1,066 个操作另有拒绝用例。事件专项 13 项通过，覆盖真实数据库事务、消息重投和三副本 broker 故障。[公开操作记录](../../.artifacts/architecture-refactor/run-20261118/http/coverage.json) · [事件用例及退出码](../../.artifacts/architecture-refactor/run-20261118/events/checks.json)
- B10 隔离部署检查 17 项通过，含迁移、角色权限和联合恢复。另在切流前、部分 HTTP 切流后、后台任务切换后三个时点实做回退，分别回读了 1、2、3 笔已接受的写入。旧会话、已发布历史成果、过期试解析和附件任务亦有从旧库续办的记录。[部署检查](../../.artifacts/architecture-refactor/run-20261118/deploy/deployment.json) · [联合恢复](../../.artifacts/architecture-refactor/run-20261118/deploy/joint-recovery.json) · [切换后写入回读](../../.artifacts/architecture-refactor/run-20261118/deploy/rollback-readback.json) · [旧作业续办](../../.artifacts/architecture-refactor/run-20261118/faults/legacy-work.json)
- 两侧用相同诊断夹具核对了 13 套检查、各 9 张截图、22 个浏览器场景及 15 个故障场景。冻结版原样运行时另有模板、分页文案与测试夹具不一致导致的退出码 1，原始记录一并保留。[诊断原件、共同夹具及可执行文件核对](../../.artifacts/architecture-refactor/run-20261118/diagnostics-comparison.json)

## 已停止和未验证的范围

11 月 19 日 14:30 的功能总账记录 37 个功能编号中 20 项通过、11 项未通过、6 项证据不完整。ARC-014 的原总账仍记未通过，上表另列的邀请三主体验证有独立输入和结果。ARC-016、017 的完整页面、状态和导航对照未完成，ARC-039 全域门槛未通过。最终 C2 的冻结侧浏览器在第 10 个分组停车，B10 的整组浏览器队列已撤销。两侧完整 C2 视觉矩阵和独立人工走查未取得本版全套通过记录。[逐项总账](../../.artifacts/architecture-refactor/run-20261118/acceptance.json) · [浏览器停车记录](../../.artifacts/architecture-refactor/run-20261118/regression/browser-diagnoses/c2-browser-user-closeout.json)

| 原回归命令的收尾状态 | 通过 | 未通过 | 已有失败后中止 | 按指示中止 | 未验证 | 按要求跳过 | 合计 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| S0 | 35 | 3 | 1 | 0 | 1 | 4 | 44 |
| B10 | 17 | 0 | 1 | 1 | 21 | 4 | 44 |

S0 的原派生契约、审计全域回归及 50,000／50,001 人的大屏授权边界均留下失败。其中两次默认 PostgreSQL 规划下的边界查询超过原 10 秒限额；S0 与较早的 B9 使用相同规划参数的补测通过。B10 的原身份全套已出现失败后停车，原解析综合回归按指示中止。监测、派生、空间、大屏、文件等未在 B10 完成的命令，在收尾清单中逐项标记。其他发行物通过的场景均保留原发行编号。[逐命令状态与真实退出记录](../../.artifacts/architecture-refactor/run-20261118/scope/user-closeout.json) · [大屏原默认失败与两侧匹配条件](../../.artifacts/architecture-refactor/run-20261118/regression/display-member-boundary.json)

用户取消了 ARC-037 接入九轮压测、ARC-038 监测与大屏混合负载、ARC-039 的 2 小时混合与 24 小时观察，以及其他纯吞吐和长期稳定性试验。历史量化目录按原版本保留，本版吞吐、P95 和长期稳定性未验证。[取消范围与历史记录索引](../../.artifacts/architecture-refactor/run-20261118/scope/functional-acceptance.json)

## 四机生产部署与基本登录

**状态为 Awaiting acceptance（待用户人工验收）。** 四台生产主机已启用 `20261119-b09feb944c39`。11 月 19 日 14:52，新 API 与 Nginx 在 152 启动。14:54 使用现有账号 `gzzxadmin` 从原 HTTPS 入口登录，进入「账号首页」，当前账号接口返回 200，浏览器加载 73 份静态资源，页面异常数为 0。只退出本次核验会话后，同一接口返回 401。人工验收可从原入口登录，查看既有租户、项目与平台工程目录。[生产控制台](https://10.23.52.152:18443/console/) · [登录收据](../operations/releases/2026-11-19/deployment.json)

37 的业务数据库已由现场 schema 55 迁移至 59。原 328 张表的 9,723 个历史主键全部保留，切换前后的账号、41 条会话、租户、项目、工程定义和解析版本均吻合。151 的中继库 10 张表、ClickHouse 4 张表和三台主机上的 7 个对象桶对账相符。四台主机的 36 个常驻服务已启动，18 个由此次发行提供的进程逐个核对了实际执行文件摘要。[生产数据逐项对账](../operations/releases/2026-11-19/deployment.json) · [四机运行与备份记录](../operations/production-version-2026-11-19.md)

本次按分阶段发布顺序保留生产兼容 API 和六组 `compat` 作业归属，现场事件捕获关闭，JetStream 未启用。隔离环境中已有拆分服务、消息和回退演练证据。生产拆分切流、切换后完整恢复演练及新版首个每日备份列为未验证。[生产调度状态](../operations/releases/2026-11-19/deployment.json)
