# 审计与操作追踪运维


## 启动与组件

Go `auditrecord` 在来源事务中保存事实、命令收据、显式关联和下载收据；`audit` 负责三空间授权投影、固定快照、私有 CSV、任务来源和维护。PostgreSQL 是事实及执行状态的唯一真相；S3 只保存已登记 CSV。React/TypeScript 控制台三个原审计入口共用同一页面和服务端裁剪 DTO。

开发启动 `pnpm dev` 同时启动 `audit-worker`；`--no-audit` 可单独停用该进程。单独启动：

```sh
go run ./services/api/cmd/audit-worker
```

数据库未迁移至 43 时 worker 明确拒绝启动。迁移、已有权限目录同步按仓库常规发布流程执行；不得对已有环境重复 bootstrap。`go run ./tools/infrapro/cmd/infrapro audit status` 检查协议、来源覆盖、积压和 worker 心跳。未配置 S3 时，API 和 worker 的元数据、持久交接和未结束下载核对仍能运行；创建／读取成果明确报依赖不可用。审计不要求 ClickHouse、文件检查器、大屏运行域或解析器启动。

`deploy/audit/worker.example.yaml` 列出预算。`INFRAPRO_AUDIT_` 加 YAML 字段大写为对应环境变量，例如 `INFRAPRO_AUDIT_EXPORT_MAX_ROWS`。四项 S3 配置必须全空或全有：`OBJECT_ENDPOINT`、`ACCESS_KEY`、`SECRET_KEY`、`BUCKET`；终端 URL 不得包含凭据和查询参数。配置只允许下调规定预算，不允许无限行数、无限候选、关闭授权或 force 清理。每个实例使用唯一 `NODE_ID` 和私有临时目录。

容器：`deploy/docker/audit.Dockerfile` 与 `deploy/audit/compose.yaml`；systemd：`deploy/audit/audit-worker.service`。进程内存上限 512 MiB，临时目录预算 512 MiB、单结果 100 MiB；快照物化 128 MiB／15 秒。独立 S3 桶必须禁止公开访问；仅允许 `audit-exports/*` 前缀，不能给业务包、附件或原始监测文件权限。策略样例中的 `AUDIT_BUCKET` 由维护人员替换为实际桶名。不要用部署示例代替实际权限核验。

## 凭据分工

业务页面中的人员权限与数据库维护凭据没有转换关系。角色为 NOLOGIN，实际服务 LOGIN 仅继承所需角色，迁移账号与日常进程分离。

| 数据库角色 | 能力 | 对象权限 |
| --- | --- | --- |
| `infrapro_audit_writer` | 来源事务追加最小事实及回执 | 不读取 CSV |
| `infrapro_audit_api` | 审计 API 表；主应用还需要原业务权限 | 对私有审计前缀 Get |
| `infrapro_audit_worker` | 固定快照、当前授权核验、任务事实、通知事实、display 交接 | 对私有审计前缀 Put/Get |
| `infrapro_audit_observer` | 维护状态和元数据校验视图 | 不配置 S3 密钥 |
| `infrapro_audit_maintenance` | 有限 plan/apply、保留策略和保护项 | 物理清理专用 Delete；不赋予 Get |
| `infrapro_audit_content` | 已登记对象元数据和内容校验 | 专用 Get |
| `infrapro_audit_restore` | 审计／任务／附件 epoch 协调恢复 | 恢复复制及校验凭据单独保管 |

运行验证见 `TestAuditNamedRolesRunAndDenyUnrelatedData`：实际用 worker 角色完成平台、租户、项目导出；观察者不能读取正文或创建清理计划；普通角色不能改写或删除历史。测试使用隔离 schema，不修改正式账号授权。

## 保留、回填和清理

升级不覆盖实际旧 `audit_retention`。维护凭据读取原配置后先生成计划；只有明确 `--config-present` 才把配置值导入策略。平台与租户默认 90 天，项目无自动 90 天删除。旧身份维护不再删除审计事实，其他会话／邀请清理照常。

```sh
infrapro audit policy plan --config-present --reason '导入已核对的现有保留配置'
infrapro audit policy apply --plan-id PLAN_UUID --revision REVISION --request-id REQUEST_UUID --reason '核对后执行'
infrapro audit backfill plan --reason '分类旧历史，不改原始字段'
infrapro audit backfill apply --plan-id PLAN_UUID --revision REVISION --request-id REQUEST_UUID --reason '执行有界分类'
infrapro audit cleanup plan --kind facts --reason '预览已到保留期记录'
infrapro audit cleanup apply --plan-id PLAN_UUID --revision REVISION --request-id REQUEST_UUID --reason '按当前保护状态执行'
```

每次最多 500 条事实／100 个对象，回填保留 ID、原时间、原因和原 details，不猜测关联。未知事件保持不可披露并计入维护统计。apply 重新核验策略、修订、hold、执行快照、恢复模式与读取租约；不存在任意 event_id 删除、force 参数或可绕过触发器的会话变量。CSV 到期只影响结果资格，任务历史保持成功。删除只使用登记过的对象键；超时保留可重试的决定与执行记录。

使用 `infrapro audit help` 核对当前命令参数；计划内容、修订、请求编号和原因须保留在实际操作记录。命令输出不包含数据库 URL、S3 密钥或任意正文。`verify --mode metadata` 使用观察凭据；`verify --mode content` 必须使用独立内容校验凭据。内容校验检查实际字节数和 SHA-256，不将不存在的正文替换为空 CSV。

## 备份与独立恢复

1. 创建有到期时间的 backup hold，停止业务写入并进入 `read_only`，等待在途事务和 worker 退出。协调同一部署中的原监测／附件／任务恢复流程；不能只复制其中一张表。
2. 在冻结点执行 PostgreSQL 一致性备份，用相同冻结点编号分页获取 `audit backup manifest --snapshot-ref SNAPSHOT_REF`；保存所有 manifest 页、保留策略和 active holds。按 manifest 精确复制私有对象并核对 size/SHA-256，不枚举未知键作删除依据。
3. 恢复到独立数据库和独立桶，保持 `reconciling`，先运行 metadata／content verify；缺失或损坏内容明确列出，不能开放下载。
4. 以 `infrapro_audit_restore` 运行 `audit restore reconcile --request-id REQUEST_UUID --reason '独立恢复核对'`。同一请求重放不再旋转 epoch。该步骤协调审计、任务、附件协议，关闭旧读取租约、废止旧 fence 和委托，给未完成导出记恢复中断；历史成功结果继续保留，但旧授权向量不再下载，必须重新生成。
5. 核对旧游标拒绝、旧 worker 不能发布、旧通知未补发、业务数据和成果校验后，显式进入 `ready` 并重启所需 worker。释放 hold 前再次核对复制范围及保留期。

`pnpm test:audit:restore` 实际运行 pg_dump/pg_restore，创建独立 PostgreSQL 数据库和 S3 桶，验证恢复中断、旧游标／epoch／fence／租约、通知不重放及逐字节 CSV 校验；不是仅修改一个 epoch 的模拟。

## 故障处理

显示“暂不可核实”时先查请求排障号、`audit status` 和来源 worker。未知命令结果必须使用原命令号核对／重放，不换号盲目重做；快照失败或超限不会发布部分文件。自动重试保留本代快照，最多三次；人工 retry 产生新代和新快照。创建快照、写 S3、发布、下载每个块均核对当前权限，恢复权限也不自动恢复旧 CSV。

受限大屏只写不可变域内 handoff；可信审计 worker 翻译并核对原归属、generation 和 fence。待翻译时查询覆盖标记为 partial；应先排除来源／审计持久化故障，不重跑包业务代替翻译。worker 异常退出的下载收据在 20 分钟后补记 `interrupted_unknown`，不猜测成功或发送字节数。

自动翻译最多尝试 8 次。修复故障后，维护凭据根据已观察的确切条目执行：

```sh
infrapro audit handoff retry --id HANDOFF_UUID --attempts 8 --request-id REQUEST_UUID --reason '已修复审计持久化并核对原任务'
```

此命令只重置翻译待办，不能重新检查／发布包；相同请求与载荷重放返回原维护收据，载荷改变冲突。核对 `audit status` 待翻译数量下降及原业务 job 状态不变。

内容校验除独立数据库角色外，还必须设置 `INFRAPRO_AUDIT_CONTENT_ACCESS_KEY`、`INFRAPRO_AUDIT_CONTENT_SECRET_KEY`；不会把日常删除凭据自动当成正文读取凭据。观察者只用 metadata 模式。

本地实验库的 `audit-volume` 是额外的隔离 SeaweedFS 容量节点，用于同时保留来源与恢复目标桶，不替换现有 S3 容器。数据库迁移及大批量测试按手册顺序执行，避免并行创建多个大 schema 耗尽测试 PostgreSQL 的锁内存。

当前工程环境、账号入口和人工步骤见[本机工程环境与人工验收](local-environment.md)。

## 工程配置操作记录查询修复（2026-11-16）

状态：**Awaiting acceptance（待人工验收）**。本节记录本次软件自检，尚未获得用户人工确认。

复现时，“位移”指标只有 2 条操作记录，平台近 7 天已有 2353 条记录。共用审计查询先扫描整个可见范围，再按对象筛选，导致无关记录消耗 2000 条候选预算，旧操作记录接口返回 `503 audit_query_budget_exceeded`。

已在 Go 审计查询的 PostgreSQL 候选读取阶段应用对象类型、对象 ID 和事件类型条件，使用现有对象索引；随后仍执行原权限投影、脱敏及最终筛选。租户收到的平台治理通知继续按租户视角的对象匹配。候选预算和文本搜索保护保持有效，无需迁移数据。本机 API 已重新构建并更新；其他应用进程及工程数据保留。

自检结果：

- 新增回归先复现相同报错，再验证超过 2000 条无关记录时，设施类型、指标、单位、监测模板仍可读取完整对象历史；覆盖新旧接口、逐条分页、独立和组合筛选、不存在对象及空搜索。
- 6 项 Go 数据库／HTTP 测试通过（含 `-race`），覆盖上述回归、租户通知显示对象、三个工作空间的权限与游标、旧历史、脱敏和严格 HTTP 筛选。16 个新增子场景均通过。
- `go vet`（审计与 HTTP 包）、7 项审计客户端单元测试和 API 构建通过。
- 本机 12 个现有工程定义全部通过浏览器检查：默认操作记录、事件筛选、无结果搜索、顶部“操作历史”入口均正常；“位移”的时间筛选及新旧 API 每页 1 条的分页核对通过，显示创建和发布两条记录。

回归测试位于 `packages/go/platform/audit/object_history_test.go`；本次日志、浏览器结果和截图位于 `.artifacts/debug-engineering-history/`。数据库测试使用隔离测试库，没有向本机工程数据插入合成记录。本次未运行全仓端到端套件或生产环境压测。

人工验收：刷新[位移指标](http://localhost:25173/platform/engineering/metrics?definition=34cfd72f-3db2-4de0-8861-a5e45bc663e7)，打开“操作记录”，不填写时间条件也应显示“创建工程定义”和“发布工程版本”；切换事件类型后只显示对应记录。再查看设施类型、单位和公共监测模板，并点击顶部“操作历史”，应正常显示该对象记录，不再出现“候选记录过多”。本机服务已经运行；后续如需启动，执行 `pnpm local:start`。
