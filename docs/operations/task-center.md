# 任务中心运行、升级与恢复

数据库版本 v39；公共任务协议 schema_version=1，通知协议 v2。完整功能范围见[开发方案](../requirements/task-center-development-plan.md)，实现接缝见[接入指南](../development/task-center-integration.md)，本机操作见[人工验收](task-center-acceptance.md)。

## 进程与配置

API 提供当前授权后的任务摘要、详情、控制和目标；业务执行仍由各来源 worker 承担。独立 `task-index-worker` 只维护元数据、历史纳入和大屏交接，不执行业务，不连 ClickHouse／S3／转换器。通知 worker 独立投递。

`pnpm dev`／`pnpm preview` 会启动索引 worker，`--no-tasks` 可用于本机故障验证。`pnpm build` 生成 `build/bin/task-index-worker`。直接启动使用已有配置文件：

```bash
INFRAPRO_CONFIG=/path/to/operator.yaml build/bin/task-index-worker
pnpm infrapro tasks status
pnpm infrapro tasks backfill --source all
pnpm infrapro tasks reconcile --source display:job
pnpm infrapro tasks verify
pnpm infrapro tasks replay-handoff --id <交接UUID> --reason '修复来源映射后重放元数据'
```

`pnpm infrapro` 会加载仓库 `.env`，环境变量优先于配置文件。使用独立运维配置时，应直接运行构建的 CLI 或 `go run ./tools/infrapro/cmd/infrapro`，并确保进程环境指向同一数据库；不能只设置 INFRAPRO_CONFIG 就假定仓库环境不再生效。

`--source` 接受 all、monitoring:job、derived:run、derived:export、derived:group、spatial:job、display:job。backfill/reconcile 均分批扫描并持久保存游标，worker 中断后可续扫。CLI 显式重扫某来源时从头开始有界分页，事实仍幂等。replay-handoff 仅重排失败大屏元数据交接，不重试源业务，理由保存在 `task_index_repairs`。

配置 `tasks` 字段如下，也可用 `INFRAPRO_TASKS_<大写字段名>` 环境覆盖：

```yaml
tasks:
  list_page_size: '50'
  max_page_size: '100'
  candidate_batch: '200'
  candidate_budget: '2000'
  metadata_timeout: 5s
  handoff_batch: '100'
  handoff_retry_limit: '8'
  poll_interval: 1s
  reconcile_batch: '500'
  reconcile_interval: 5m
  shutdown_grace: 10s
```

这些是请求和维护边界，不代表大规模性能验收。API 候选不足时给明确错误及收窄范围提示。停止进程后不再领取新一轮，当前事务在 grace 内完成或取消回滚。

## 数据库最小权限

迁移建立 `infrapro_task_index` NOLOGIN 角色，授予公共任务写、白名单来源元数据读取和专用交接确认能力。DBA 用密钥管理系统建立独立 LOGIN，授予此角色及部署 schema 的 USAGE；参见 `deploy/tasks/index-worker-role.sql`。不复用 API 数据库账号，不给源任务表 UPDATE、报告正文、账号或通知读取权。

Compose 的 `tasks` profile 启用该进程；先通过受保护环境提供 `TASK_INDEX_DATABASE_URL`，再执行：

```bash
docker compose -f deploy/compose/compose.yaml --profile tasks up -d task-index-worker
```

API 镜像包含该二进制。默认不猜测索引登录凭据；连接串不写入仓库、截图或诊断输出。大屏包 worker 保持原受限角色，它只接触大屏自有交接，不能写全局任务／通知。

## 协调升级到 v39

1. 固定含未提交改动的源码 manifest，备份数据库、原业务对象索引和所引用对象。`HEAD` 不能代替整个工作区快照。
2. 暂停新任务入口，等待原执行 worker 有界退出，停止旧 API、索引与通知等相关 writer。当前 `migrations.Check` 要求精确版本，不支持旧新进程任意混跑。
3. 用既有 `infrapro database migrate` 迁移流程执行 00037–00039；不要改已应用的 00034–00036。同步权限目录并检查，内置角色按规则升级，自定义角色不自动扩权。归档项目／停用租户只允许完整、首次发布的两项任务摘要／控制权限进入原内置管理员角色；已有目录、部分发布和任意扩权继续拒绝，来源冻结写保护不变。
4. 配置索引独立登录，启动一致版本 API／来源 worker／索引 worker。协议先处于 backfilling；原页面仍可使用，公共列表明确历史准备中。
5. 执行 `tasks backfill --source all` 或等待 worker 逐页回填，再执行 `tasks verify`。有新增及晚提交时依靠来源登记加缺项反连接核验，不以最大 UUID／自增值推断完整。
6. `tasks status` 应显示 ready、六个来源 complete、无来源错误、交接无积压；验查 mine、managed、旧链接和执行历史后恢复完整入口。状态、控制和结果不通过索引副本替代来源检查。

旧记录使用 task_revision=0 基线，已存在新事实不会被覆盖。不补发旧通知，不重跑业务，不把缺失旧代／attempt 伪造成完整历史。空间初始发起人证据不足时保持未知归属；旧回算分组只从可证明的实际命令收据恢复。

## 故障定位

`tasks status` 返回协议 mode/restore_epoch、记录数、来源对账时间与安全错误码、worker 心跳及大屏交接统计。用户接口的 partial/unknown 不是空列表或零积压。

| 现象 | 检查与处理 |
| --- | --- |
| 历史准备中／大屏任务同步中 | 检查 index worker 心跳、来源回填和 handoff；修复后 reconcile/verify，不能重新提交业务来补索引。 |
| 来源不可核实 | 检查 PostgreSQL 及对应来源元数据；当前页面隐藏未核实的动作。ClickHouse、对象库或转换器故障不应阻断可授权元数据读取。 |
| 交接重试耗尽 | 定位安全诊断码、修复来源，按原交接 ID replay-handoff；不得修改来源键或新建通知。 |
| 旧游标失效 | 从首页重新加载；撤权、账号变化或恢复代次变化是预期原因。 |
| 控制版本冲突 | 保留操作理由，重新核对来源当前状态；响应丢失先重放原 request_id，不创建第二个命令。 |
| 原执行资格失效 | 原计算任务拒绝重试，在业务页按当前资格重新创建；不得借管理人员身份冒充原执行者。 |
| 结果失效／过期／清理 | 执行成功历史仍保留。原来源决定是否可读、重建或清理，任务中心不绕过下载检查。 |

## 备份恢复与回退

完整备份必须同时包含原队列／收据、task_records/executions/attempts/events/relations、任务协议与同步状态、source evidence、task_transaction_facts、display_task_handoffs、notification 事实／交接／个人状态、权限，以及所引用对象清单和对象。只重扫当前 job 无法恢复人工理由和旧错误历史。

恢复时停止所有相关 writer，协调恢复来源、公共历史、通知与对象，然后按原业务手册推进身份会话、监测／空间／大屏环境恢复资格。任务额外执行：

```bash
pnpm infrapro tasks restore-epoch --reason '完整备份恢复后废弃旧游标与索引租约'
pnpm infrapro tasks backfill --source all
pnpm infrapro tasks verify
pnpm infrapro tasks status
```

restore-epoch 使旧游标、旧索引同步租约失效并进入 backfilling；原执行 worker 的 fence／环境资格仍由原模块管理。先验证历史、回执、交接与通知一致，再恢复业务入口。禁止只恢复公共索引到旧点却继续接受来源新点的控制，也禁止重跑已成功导入／发布来“修复”列表。

无破坏性 Down。功能回退先关闭公共入口和新任务引用展示，保留历史表；回到不登记任务的旧程序须停止新 writer，重新升级后补对账，期间不得宣称索引完整。备份后丢失的记录按恢复点说明，不能承诺凭空恢复。
