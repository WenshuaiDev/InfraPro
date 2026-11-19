# 自有进程报告接入

当前运行目录包含 26 个会产生进程报告的 Go 入口，以及按需程序、代理和外部依赖，共 44 项。目录源是 `services/diagnostics/settings/catalog.go`，生成到 `contracts/diagnostics/runtime-catalog.json`。新增 Go 常驻入口应登记期望和 lane，并接入 `diagnosticreport.FromEnv`。`pnpm diagnostics:coverage` 会报告未登记的入口。

`packages/go/foundation/diagnosticreport` 不依赖 platform 或诊断 server。入口在解析业务配置、连接数据库前建立 reporter，报告 starting；真实初始化完成后 Ready，错误退出前 Failed，原有 defer/errgroup/context/退出码保持不变。没有诊断环境变量时返回 noop。

启用变量：`INFRAPRO_DIAGNOSTICS_INSTANCE_ID`、`INFRAPRO_DIAGNOSTICS_NODE_ID`、`INFRAPRO_DIAGNOSTICS_REPLICA_ID`、`INFRAPRO_DIAGNOSTICS_REPORT_FILE`。report 文件必须绝对路径，私有目录由运行账号拥有。build 通过 `-X github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport.BuildID=<id>` 标识。不要给业务进程 observer key。

业务线程只更新有界内存快照，不同步写盘、不发网络。独立 writer 定时原子落盘，文件 0600，序号递增，拒绝 symlink。写入失败或挂住不改变业务成功／失败；Close 等待最多 100ms。报告不携带 argv、env、数据库 URL、原始错误、业务正文或任务参数。

lane 的 Start/Working/Success/Failure/Idle 跟随真实调用。有限任务使用实际 context deadline；长间隔维护提供真实 next_attempt_at；事件驱动 lane 没有虚假的周期。并发 lane 使用 group/track 记录实际活跃调用，不以 reporter writer 心跳伪造业务循环。依赖状态跟随实际 PG 等调用成功／失败，不能用诊断自己 PING 的结果替代。

兼容 monitoring-worker 保留 scheduler、foreground、historical、parsing、parsing-maintenance 五条 lane。拆分后的 monitoring-publish-worker、monitoring-incremental-worker、monitoring-history-worker、monitoring-read-worker 分别报告各自执行的发布、增量、历史和读取工作。spatial 的 converter 预检与实际转换分别报告，Python 是按需子进程。兼容大屏进程分别检查 runtime/assets 两个端点，拆分模式下 display-runtime 和 display-assets 分别报告各自的 PID 和监听。files-worker 的 prepare/cleanup/heartbeat 沿用原 errgroup 退出语义，存储启用状态决定 lane 是否执行。files-checker 只在父进程持有诊断配置并生成报告。

ProcessReport schema 是严格输入契约。进程身份用 PID、OS start time、UID、可执行文件共同核对；复制旧 report 不能继承 fresh。sample age 基于原始 emitted_at 和 report_seq，同一来源报告反复读取不产生新 attempt。deadline 超期、错过 next_attempt 加宽限、持续依赖错误分别形成明确原因；只启动 writer 不能掩盖 stalled lane。

parser 是独立常驻服务，报告 catalog 清单验证与 parse 有界子进程执行；解析子进程按需启动，不计入常驻分母，也不继承机器令牌或诊断配置。

ingest-relay 报告 control、receivers、spool-maintenance，依赖单独的云端 PostgreSQL；兼容 ingest-worker 报告 pull、relay-status、confirmation、identify、planning、parse、source-publication、association-watermark、recovery、object-maintenance。拆分模式下 ingest-custody-worker 持有原文接管与确认，parsing-dispatch-worker 单独持有识别、计划、解析和恢复。PG、S3、parser 状态来自实际调用；识别或测量调用失败后即使成功保存退避记录，lane 仍报告失败。解析服务故障不阻断健康的原文接管和确认。空闲对象维护每分钟验证存储与业务 schema，未启用接入时报告 opt-in idle。

接入自验包括 reporter 阻塞／退出边界、真实 owning process 的空闲／工作／失败、业务任务生命周期与原沙箱测试。调用 `node scripts/diagnostics-business-check.mjs` 使用隔离 lab 的真实数据库和独立测试 schema，最终保留脱敏的 ProcessReport 快照。


## 拆分拓扑的度量

从原有独立诊断页面依次打开组件、副本、进程报告检查，可以查看最后一次健康证据、各条真实执行循环和度量。每条 MQ 消费循环使用对应消费者的 `events-<consumer>`，转发器使用 `publish`。依赖状态和角色心跳由相应进程写入，observer 离线或报告陈旧沿用原有过期规则。

| 度量 | 含义和单位 |
| --- | --- |
| `event.outbox_pending`、`event.outbox_oldest_ms`、`event.publish_acks` | 待发数量、最老待发毫秒数、发布确认计数 |
| `event.delivery_lag_ms`、`event.redeliveries`、`event.quarantined` | 投递延迟毫秒数、重投次数、隔离计数 |
| `event.consumer_position`、`event.position_gaps`、`event.pg_reconciliations` | 消费位置、位置缺口和 PG 补偿次数。对外 JSON 中位置使用无符号十进制字符串 |
| `dispatch.rejected_fences`、`parser.duration_ms` | 被拒绝的过期代号次数和实际解析耗时 |
| `quota.pg_connections`、`quota.ch_connections`、`quota.s3_inflight`、`quota.parser_slots`、`quota.http_inflight` | 角色已配置的 PG、CH 连接，S3 在途和 parser、HTTP 额度 |
| `tenant.queue_depth` | event-relay 每 30 秒沿租户索引轮换至多 12 个有积压的租户，统计当前恢复代号内待发和租用中的条数。维度为租户 UUID 的 96 位摘要 |

每份报告至多 48 条度量。租户查询使用部分索引 `event_outbox_tenant_pressure`，只在发布批次之间执行，时限 250 毫秒。读数失败会清除上一组租户维度。共享报告只包含汇总数值和有限枚举。请求、命令、收据、作业和连续位置由持有相应权限的操作员在原审计和业务事实中核对。大屏任务的受限交接角色沿用获准的翻译路径。

拆分角色使用运行配置中实际生效的连接和并发上限。PG 的数值来自已打开的连接池。ClickHouse 按所有物理分片及副本平分角色额度，每个端点至多 6 个连接，报告各端点上限之和。S3 的多个对象库客户端共用本进程一个额度，响应体读完或关闭时归还槽位。未启用相应客户端时省略该度量。

角色配置中的 `pg_connections` 和 `max_inflight` 是角色总量。`replica_slots` 按声明顺序平分总量，前面的槽位分配除法余数。各进程使用自己的 `replica_id` 或 `INFRAPRO_REPLICA_ID`。业务连接建立时，在 PostgreSQL 中检查并持有该槽的会话锁。进程内的其他连接凭本次启动的随机身份共享锁；同一配置再次启动时因槽位被占而失败。所有连接消失后 PostgreSQL 归还槽位。实现复用普通业务连接，单连接池仍然可以处理工作。网关继续独立启动，各 HTTP owner 同时限制自己接收的在途请求。

`pnpm diagnostics:coverage` 核对 Go 入口、部署角色和三个 profile 的所有预期副本。运行 `node scripts/diagnostics-architecture-check.mjs <本地证据路径>` 会启动 13 个新增或兼容角色和实际 JetStream，在配置读取、停止和启动失败各阶段验证独立探针。它还在隔离容器内运行发布用 files-checker 二进制，核实 PID 1、UID、可执行路径和停止后的结果。测试结束仅清理本次创建的目录、进程和有匹配部署标签的容器。
