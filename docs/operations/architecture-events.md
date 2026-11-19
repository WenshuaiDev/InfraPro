# 内部事件投递与恢复

PG 迁移 59 为已提交的接管收据、单元、解析计划、正式监测变化、公共任务快照、通知和审计辅助作业写出事务 outbox。原领域作业仍负责授权、租约、结果唯一性和正式清单。每条消息只含契约列出的 UUID、范围、版本、代次或位置和时间，最大 16 KiB。消息字段见 [v1 契约](../../contracts/events/v1.schema.json)。

## 安装与启动

1. 用业务 schema 的维护账号执行 `infrapro database migrate`。首次安装或从 `pg_dump --no-acl` 恢复时，在同一数据库和 `search_path` 下按顺序应用发行包中的 `deploy/architecture/restore-base-grants.sql`、`deploy/display/package-worker-role.sql`、`deploy/architecture/postgres-roles.sql` 和 `deploy/events/postgres-grants.sql`。每个部署登录继承其唯一职责组。发布进程使用 `infrapro_event_relay`，管理命令使用单独的 `infrapro_event_admin`。SQL 文件无登录口令。
2. 按目标架构运行 `node scripts/architecture-broker-build.mjs linux amd64` 或 `linux arm64`，分发 `build/events/<系统>-<架构>/` 下的二进制、`broker.json` 和 `SHA256SUMS`。在目标目录用 `sha256sum -c SHA256SUMS` 校验。断网构建主机需先导入锁定的 Go 模块缓存。交付的二进制可直接离线运行。server v2.14.6、client v1.55.0 与源模块摘要在 [版本清单](../../deploy/events/versions.json)。
3. 为管理进程挂载只含业务 PG 配置的 `INFRAPRO_CONFIG`，用 `event-relay status` 读取 `protocol.restore_epoch`。配置中的数据库连接凭证仅进入相应运行账号的私有文件。为每个 broker 准备私有监听地址、本地持久卷及独立服务器证书。集群路由使用单独的节点 CA、节点证书和三个私网路由地址。文件模板见 [broker.example.json](../../deploy/events/broker.example.json)。
4. 用 `node deploy/events/render.mjs node-profile.json <restore-epoch> nats.conf` 生成各节点配置，执行 `nats-server -t -c nats.conf` 校验并运行 `nats-server -c nats.conf`。开发单节点省略 routes，部署目标为三个故障域。健康检查使用每台本地的 `127.0.0.1:8222/healthz?js-enabled-only=true`。
5. 以 [events.example.json](../../deploy/events/events.example.json) 为每个进程填写本环境 broker 地址、环境名、CA 和该角色证书／私钥路径，通过 `INFRAPRO_EVENTS_FILE` 指定。证书 DNS SAN 与渲染器内的 `<role>.infrapro.internal` 对应。发布账号只可写固定事实主题和查询本环境流容量；消费账号只可操作本角色的持久消费组与监督游标。管理证书仅供 provision。管理身份运行 `event-relay provision` 建立／更新三个文件流，再执行 `event-relay enable --reason '首次启用事件捕获'`。它在 PG 同一事务保留各租户已有的监测提交位置，后续事实才进新 outbox。
6. 启动 `event-relay run` 及应用职责进程。同一用途的多个实例共享其 durable 和有 fence 的领域领取。接入的 `identify`、`plan`、`parse` 由 parsing-dispatch-worker 承担；监测 `summary`、`derived` 由 monitoring-incremental-worker 承担，`query-cache`、`display-cache` 由 monitoring-read-worker 承担；`task-index`、`notification`、`audit` 分别由任务索引、通知、审计 worker 承担。

独立本机验收可执行 `node scripts/architecture-event-lab.mjs <新目录> <环境名> <恢复代号>`。该命令生成 0700 权限的临时目录、短期测试 CA、角色证书、固定主题授权与未占用的环回端口。broker 使用生成的 `broker.conf`，每个角色用各自子目录的 `events.json`。目录内有测试私钥，仅在隔离验收机使用。

## 切换与回退

转发器收到 JetStream 的持久化确认后，最多合并 32 条确认写入业务 PG。首条确认等待合并的时间上限为 5 毫秒。每行独立核验恢复代号、租约和 fence，并记录 broker 流名及序号。迟到的消费提示仍按原事件 ID 记入重复次数，只有可领取的工作才唤醒执行器。

识别与解析消费者有空闲执行配额时，分别按 broker 交付的收据 ID、解析计划 ID 核对原事实，在同一业务 PG 事务中保存交付记录并领取工作。事务成功后确认消息。执行器中途退出时，PG 对账会在租约到期后领取同一事件。正常投递沿用原工作到期时间。订阅持续有进展时，每个对账周期另核查一项遗漏或到期工作。订阅中断后按有界批次排空积压。

`event-relay status` 返回每个角色 mode/generation、当前 epoch、隔离和待处理量、存储与队列水位。初始六个角色归属 `compat`。记录要切换角色的当前代次，使用管理身份依次执行。下面的两个数字是示例，操作时取实际返回值。

```sh
event-relay switch --role parsing-dispatch-worker --mode draining --expected 1 --reason '停止旧拓扑领取'
event-relay switch --role parsing-dispatch-worker --mode split --expected 2 --reason '启用独立解析调度'
```

draining 拒绝新领取；已经进入提交点的领域事务可提交，迟到结果由 PG 代次和租约拒绝。回退同样先 draining，再以新的预期代次切回 compat。旧二进制必须按升级手册先停妥。运维操作保存在 `event_operations`，包含数据库登录身份、时间、原因及前后代号。

同版本兼容接入进程和监测进程使用各自的数据库登录，归属行与租约均受兼容模式限制。旧监测进程执行人工解析时，只能占用空闲或已到期的解析配额，使用人工清理调度项。兼容模式切为拆分模式后，数据库同时撤销这两个旧进程对该角色的领取资格。

多次切换后，原兼容进程可沿用实例 ID。切回 `compat` 时，该进程重新核验当前角色、分区和恢复代号，用现行代次更新同一实例的旧租约。执行中的业务提交仍须持有当前有效的租约。回退演练在运行目录的 `deploy/compat-lease-check.json` 逐项记录旧实例重新领取、迟到提交、跨角色访问和冻结期间的数据库结果。

启用捕获后，`mode=jetstream` 同时保留定键唤醒和有界 PG 兜底扫描，`mode=pg` 只扫描尚未终结的工作。将受影响角色的配置切换为 pg 后重启，即可验证或使用 PG 路径。转发器停发期间 PG 保留待发引用。再切回 jetstream 后，先前已被 PG 完成的消费记录阻止重复执行。没有 events 配置的历史兼容进程继续原领域巡检。

## 容量、隔离与存储

默认三流字节预算为接入 1 GiB、监测 512 MiB、辅助 256 MiB，保留 7 日，传输层去重 120 秒。每个持久消费组默认单次最多 32、在途最多 64、确认期限 30 秒，业务工作并发 4、最长 120 秒。业务失败或传输交付的上限各为 8 次。退避从 500 ms 增至 60 s，PG 对账周期 5 秒。转发器以查询单批 32 个租约及事务提交的空通知唤醒。具体部署应按实测吞吐、延时和故障窗口定额。

`event-relay status` 中各 family 的 `pending`、`oldest_at`、`broker_bytes`、`broker_limit_bytes` 和 `storage_level` 分别给出 PG 队列与文件流压力。文件流默认使用 80% 软水位和 95% 硬水位，每 5 秒采样一次。PG 默认每类待发 250000 / 500000，管理命令 `event-relay limits --soft 250000 --hard 500000 --reason '记录容量基线'` 调整。达到软水位时，接管单批上限降至 10 条，所有副本共享的监测正式发布领取上限降至每 100 毫秒一次。达到硬水位时暂停新接管与正式发布领取，保留已排队事项。直接产生新正式变化的事务返回可重试的存储背压。已接管后续解析、交接、普通登录和既有成果读取仍按原路径服务。流满时使用 DiscardNew 并保留 PG 待发事实。

提交时先保存领域事务与对应投递意图，再在锁外发布。PubAck 后才更新 outbox。消费组将引用与持久作业在同一事务交接，随后 ACK；业务完成后更新该用途的唯一工作及连续位置。未获确认则重查原件。每个消费组另有一条持久的末次投递通知游标，按 broker 的流序号回查原事件，保存隔离证据后确认。旧位置、版本不支持、确定性坏引用、持续执行错误都可在库中定位，各用途互不共用水位。

从隔离记录保存的事实键纠正来源后执行 `event-relay redrive --consumer <用途> --event <事件 UUID> --reason '已核对原始事实'`。该命令只恢复当前 epoch 对应的未完成工作，完成后保留 resolved 隔离记录。监测连续位置有缺口时，先补齐缺失事实，再重驱后继；不能通过手改游标跳过缺口。清理每次最多 256 项，要求当前恢复窗口已过、所有目标已完成、没有开放的隔离项、监测水位覆盖且来源仍可查。隔离证据与旧 epoch 留存供审计。

## 联合备份和恢复

在发布写入和副作用停止后执行 `event-relay freeze --epoch <旧代号> --reason '联合备份封存'`，记录 status、业务 PG、既有对象／ClickHouse 清单及 broker 持久卷的封存范围。联合恢复时旧域所有客户端及旧卷仍需隔离。恢复守卫检测业务审计 restore epoch 变化后会要求事件代号同步轮换。用管理账号执行 `event-relay rotate-epoch --epoch <旧代号> --reason '恢复到指定备份'`。仅仍未完成的工作按原事实键和代次生成新引用。重新渲染角色 ACL、使用新卷/新流、新 epoch 运行 provision 后，`event-relay resume --epoch <新代号> --reason '完成恢复核对'`。对账全部未完成工作、监测位置及业务唯一键。常规冻结后继续原数据可用 resume，业务恢复守卫尚未轮换时拒绝恢复执行。

`node scripts/test-architecture-events.mjs` 使用独立测试库 schema 和临时本地 broker 进程执行源事务回滚、重复确认、租约退役、连续缺口、角色权限、mTLS、三流容量、多数派中断、末次投递退出与重驱。可设置 `INFRAPRO_ARCHITECTURE_RUN_DIR` 归档各动作的结构化回读结果。
