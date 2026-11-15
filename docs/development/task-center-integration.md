# 任务来源接入指南

本指南对应[已确认方案](../requirements/task-center-development-plan.md)和当前 v39 数据库。运行、迁移与恢复见[任务运维](../operations/task-center.md)，功能证据见[自验记录](../verification/task-center.md)。

## 职责与登记

`taskrecord` 保存中性事实、来源身份、执行代次、尝试和不可变事件；`tasks` 提供当前授权后的检索、控制分发、目标解析及索引诊断。业务模块继续拥有队列、输入、租约、执行身份、成果和通知。任务索引没有业务执行能力。

稳定来源身份为 `(source_module, source_kind, source_root_id)`；由 `SourceRef.ID()` 生成稳定 UUID。同一业务运行只有一个任务，跨项目可见性不复制任务。`TaskRef` 还包含十进制字符串 `generation`，以及可选 `task_id`。不能用文件名、页面 ID、上传进度或通知 ID 代替实际运行身份。

| 来源 | 任务类型 | 执行拥有者 |
| --- | --- | --- |
| monitoring:job | monitoring.import.validate、monitoring.import.adopt、monitoring.export、monitoring.query | monitoring_jobs；query 仅实际持久化的耗时查询 |
| derived:run | derived.trial、derived.backfill、derived.replay、analysis.trial、analysis.run、analysis.compare | derived_runs 与共享 monitoring_jobs |
| derived:export | analysis.export、derived.evidence.export | monitoring_exports 与共享 monitoring_jobs |
| spatial:job | spatial.process | spatial_processing_jobs |
| display:job | display.package | display_package_jobs |
| derived:group | derived.backfill_group | 实际回算命令收据的展示分组；不执行、不计入执行任务总数 |

自动派生、维护、ReadSubject 子查询及逐收件人通知投递不登记成人员任务。自动工作通过当前有权空间的 `operations` 返回有限运行概况。

### 同事务路径

当前既有 monitoring／derived／spatial 路径采用来源触发器形成白名单 `Change` 快照，再写 `task_transaction_facts`。`access.Service.Transact` 在提交前调用 `taskrecord.FlushTx`，把该事务的事实依次交给 `ApplyTx`。原业务写、历史和终态通知共同提交或回滚。事务内的中间状态也有证据，不能只在提交后扫描最终 job。

这是方案中“所有原业务事务调用 ApplyTx”的具体收口方式：覆盖既有直接 SQL、领取、维护回收及内联成功出口，减少逐个调用点漏记。正常业务请求在提交时已经登记；恢复路径如直接数据库事务留下已提交 journal，由独立索引进程有界补登记。journal 不是另一个执行队列，不采用自增最大值跳过晚提交。

新模块可在自己事务内直接 `taskrecord.ApplyTx(ctx, tx, change)`，或按上述来源快照／FlushTx 机制接入。失败必须回滚，不能忽略登记错误。新增类型同时修改有限 `Catalog()`、校验、runtime 组合和真实契约测试；不可只创建一条演示投影就宣称支持。

### 受限进程路径

大屏包 worker 只写大屏自有 `display_task_handoffs`。不可变快照与源状态同事务提交；`task-index-worker` 消费后同事务写公共事实并确认交接。重复交接幂等，单条错误通过 savepoint 隔离、退避后进入诊断；人工重放保持原身份。包 worker 没有公共任务表、通知表或账号表访问权限。

## Change 必须满足的不变量

- 类型、scope、phase、来源、动作及目标均为有限注册值；不接受任意 URL、存储路径、SQL、原始参数或报告正文。
- `source_revision`、`control_revision`、`generation`、`fence` 和计数使用规范十进制字符串。`Change` 最大 32 KiB，引用键为白名单；不得依赖 JavaScript Number 保存大整数。
- 初始 `originator_account_id` 不变；每代单独记录 `requested_by`、`executor_kind/ref`、固定输入引用及 hash。空间人工重试按原规则由当前操作者执行；计算重试仍核验原执行身份，不借管理员身份替代。
- 内部尝试更新 fence／attempt，仍属同一代；人工重试才增加 generation。开始新代前封存旧代及旧错误证据。关闭的历史不能被迟到事件重写。
- 进度仅使用来源真实 completed/total；未知 total 为 null。任务状态与成果 `available/expired/invalidated/restricted/missing` 独立。
- 已清理的源保留安全历史，当前相应管理资格可查 `source_missing`，不能执行控制。未保存的历史标 `history_incomplete`，不得凭 attempt 计数伪造记录。

旧空间任务仅在 `spatial_changes` 中能证明唯一创建人的情况下恢复初始发起人；其余是未知归属，不添加到任意人的 mine 列表。旧多运行回算收据只有在成员、租户、项目、发起人和 mode 一致时才重建展示分组。

## 来源 Adapter

`tasks.Source` 的五个方法分别负责：

1. `ExistsTx`：通过白名单身份视图判断源是否保留，不把“无权限”当作“源已删除”。
2. `ResolveTx`：批量重新核验当前成员、空间、来源资格，返回安全摘要与能力；管理摘要和完整结果读取分开。未授权项省略，依赖错误返回错误，不能伪装成空结果。
3. `Control`：进入原业务事务，先按原顺序锁定授权与来源，核验当前资格和幂等回执，再比较预期代次／修订并执行。相同请求重放返回原结果；不同载荷同键冲突。
4. `TargetTx`：再次按当前结果权限返回有限目标 kind/id/scope/generation。下载仍由原来源检查。旧代不能自动跳到新代成果。
5. `OperationsTx`：仅返回当前有权空间的受控运行概况。

索引使用 `IndexSource` 和可选 `HandoffSource`。索引 Reader 不初始化 ClickHouse、S3、转换器、Redis或大屏 runtime。权限拒绝与来源不可用必须区分。`ExistsTx` 是必需接缝，不能在测试 Adapter 中省略。

`tasks.SetCommandTx` 把安全操作上下文写入事务局部配置，来源触发器记录操作者／理由，原 `access.Audit` 事实附带任务 ID、来源、代次、前后状态、原发起人、操作人、request/trace 引用。不要追加第二条“业务成功”审计。

## API 与控制台

公共路由前缀为 `/api/v1/tasks`：列表、`summary`、`catalog`、`resolve`、`operations`、`/:id`、`/:id/executions`、`/:id/events`、`/:id/target`、`POST /:id/commands`。仅会话人员使用，响应 `private, no-store`，命令经过现有 CSRF 检查和严格 JSON 校验。不存在通用创建任务或下载任意对象接口。

列表先按当前权限组成页，再返回签名游标。每批至多核验当前页尚缺的条数加一条前瞻，无权候选继续推进有界批次；工作空间资格只在同一账号、同一持锁事务内复用，固定输入和结果仍逐任务核验。游标绑定账号、筛选、授权版本、恢复代次及首屏上界；新任务通过刷新首页出现。撤权、切换账号和恢复后丢弃旧游标／缓存。任务页不并发重复扫描顶栏全范围计数；由当前列表的授权向量继续驱动缓存清理，其他页面保留顶栏计数轮询。超过候选预算返回可收窄筛选的错误，不能给假末页。索引不完整时 coverage 明确为 partial，计数未知不写 0。

业务页使用 `TaskLink`／`TaskSourcePanel`，专业配置和成果仍留在原页面。`TaskActions` 复用取消／重试、理由和幂等请求；丢失响应时重放同一个 request_id。URL `/tasks/:id?execution=N` 固定旧代。原监测 jobs URL 继续解析到共享详情。

终态通知继续由来源模块发送，索引不发通知。新通知使用 v2 可选 TaskRef 固定代次；已保存 v1 的来源键、正文指纹、接收集和已读不变。缺少代次的旧通知显示历史限制。通知权限和任务权限均重新核验。

## 最低接入验证

必须有真实来源命令，覆盖登记回滚、内联／后台成功、取消、人工与内部重试、租约丢失、重复／乱序事实、管理摘要分权、撤权、结果过期／失效、无业务依赖的元数据读取、迁移和恢复。受限进程必须用真实受限 SQL 角色测试。协议纯函数用单元测试，不能用投影插入替代真实来源验收。

执行 `pnpm tasks:contracts` 更新任务和通知契约；若改动来源 DTO，还需对应来源生成器。完成后运行 `pnpm check`、`pnpm build`、`pnpm test:tasks` 和 `pnpm tasks:browser`，保留日志及对应源码 manifest。
