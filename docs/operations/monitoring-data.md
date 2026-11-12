# 监测数据使用与维护

状态：**Awaiting acceptance（本机功能待验收）**；功能自验与人工验收分别记录在[验证记录](../verification/monitoring-data.md)。当前使用明确标记的合成样本，本轮只验证本机功能。没有实测业务样本或目标部署，规模与生产容量结论不在本轮交付范围。

## 使用入口与授权

租户和项目工作空间均有“监测浏览”“导入与批次”“监测任务”。工程资源的测点／通道详情可直接跳到监测浏览。无需绑定设备才能导入历史观测。

项目读取须同时具备成员资格、操作角色、有效通道资源授权和明确的监测数据授权。在租户“项目治理 → 项目详情 → 监测数据授权”选择已授予的通道，填写包含起点、不包含终点的时间范围及能力。留空端点表示该端不限；全部历史及未来须单独勾选。资源授权结束后重新授予资源，不会复活旧数据授权。

无数据授权时显示零个获准通道。归档后仍按当前权限读取、查询和导出，导入、批次撤销和授权变更不可用。撤权后旧页面的选择、缓存和迟到响应失效；后续下载块重新检查资格。

## 导入与核对

1. 创建 CSV 或 XLSX 批次，填写来源说明。浏览器 worker 按 32 MiB 片计算 SHA-256，进度为实际确认字节。暂停或离开后，从批次页重新选择同一文件续传；服务端核对已有片。CSV 上限 32 GiB、XLSX 上限 512 MiB，每人最多同时三个上传会话。
2. 明确编码／分隔符、表头行、时间列、固定时区、值列及现有业务通道。支持单通道、长表、宽表；XLSX 一次一个工作表。宽表空格单元格是否为明确缺测须显式选择。映射区域公式不执行，也不把缓存值当原始值。
3. 保存映射版本并全量校验。任务中心显示读取字节、解析行、候选数、准备块数。问题报告提供来源位置和对应观测依据。任一格式错误、缺值、无授权或同时间异值冲突均阻止整批生效。
4. 同观测重复须核对完整固定集合、勾选确认并填写理由；只忽略新的重复候选，保留来源及核对记录。没有新增时显式显示“完成，无新增数据”。
5. 核对拟采用、忽略、可疑和错误数后发布。双副本全部核验，PG 清单一次提交后整批可见。处理中可离开；不确定的物理写入重试使用新 attempt。

未标注和正常质量默认参与数学统计；可疑、无效保留原值但默认排除。可疑判断依据冻结规则版本。源文件不可就地替换，修正源文件应建立新批次。

## 浏览与精度

页面区分累计曾发布唯一观测、当前在用观测、当前质量筛选样本；撤销不减少累计唯一数量。最新覆盖与最新可用值分列。

默认以最近实际数据为锚点查看前 24 小时；可返回最新、相对当前时间、输入带时区的纳秒范围、按键平移／缩放。多个测点的兼容通道可比较；不同指标、单位、时间含义、方向或基准分组，每图最多六条曲线。

趋势只加载固定边界的完整时间块及至多左右各一个邻块，并发三、约 150 ms 合并快速操作，缓存上限 32 MiB／80,000 点。绘图总量不超过 20,000 点、每曲线不超过 2,000 点。边缘时间块可含窗口之外的数据，其样本数与当前窗口精确概况分开标注。

原始点或摘要的首／最小／最大／末点都对应真实观测。枚举显示事件或状态样本计数，不推断持续时间。坐标采用相对精确基准的近似值，提示、明细、统计和导出保留精确文本。不把未加载区间视为缺测，不补零或插值。

快照固定选择、基础范围、质量、水位、路由和相关撤销修订；`view_range` 只能取基础范围内的子窗口。相邻块复用快照，明细游标进一步绑定子窗口。追加数据只提示刷新，不改变正在浏览的快照。

明细默认 200 条，可选 500／1,000 条，以完整纳秒和稳定身份翻页，最多缓存十页。统计按通道或 IANA 时区自然日／月／年计算，采用总体标准差、精确整数矩和 half-even 舍入；部分授权／窗口标注实际纳入范围。

## 后台导出

从当前明细／统计创建任务。Parquet 2.6 保留原始类型及 UTC 纳秒；CSV 提供精确交换与表格阅览模式。阅览模式对公式样式文本作可逆转义，清单附规则。统计结果附精度及舍入规则。

默认每卷 100 万行或 256 MiB，全部结果校验后开放下载，保留七天。Parquet 可跨目标大小一个有界行组。后续追加不改变固定结果；相关撤销或撤权使结果失效。每卷提供实际行数、字节数及 SHA-256。

## 撤销与再次采用

先预览完整影响，再填写原因确认。仅撤销该批次实际创建的采用关系；此前忽略的重复候选不会自动生效，原文件和原观测保留。

从已撤销批次建立关联新批次，选择原批次全部实际采用观测或明确子范围。重新校验并发布后，原 `observation_id` 保持不变，`adoption_id` 更新；累计唯一数量不增加。

## 本机自验命令

```sh
pnpm test:monitoring:contract --keep
pnpm test:monitoring --reuse-contract --keep
pnpm test:monitoring:client
INFRAPRO_MONITORING_E2E=1 pnpm test:e2e tests/e2e/monitoring-data.spec.ts --project=desktop --project=mobile --workers=1
pnpm check
```

这些命令使用隔离测试 schema／CH 数据库和合成文件。契约实验室是 `deploy/monitoring-contract/compose.yaml`，不应作为生产部署模板。已有 CLI 和独立 worker 入口：

```sh
pnpm infrapro monitoring storage init
pnpm infrapro monitoring storage check
go run ./services/api/cmd/monitoring-worker
```

配置由 `INFRAPRO_CONFIG` 与 `INFRAPRO_MONITORING_*` 注入，参照 `configs/infrapro.example.yaml`。API、worker、CLI 必须指向同一套控制库和存储。初始化是显式维护操作。维护／恢复命令及持久本机入口见下文。

## 持久本机验收环境

在本 checkout 根目录执行 `pnpm monitoring:local setup`，首次创建专用 Docker 项目 `infrapro-monitoring-local`、命名卷、v12 控制库及明确的合成管理员。再次 setup 仅检查／初始化缺失结构，不重置已有账号、数据或卷。运行 `pnpm monitoring:local start` 启动真实 API、独立 worker 与前端；另一个终端执行 `pnpm monitoring:local seed` 经实际 HTTP 接口创建合成资源与导入批次。

入口为 <http://127.0.0.1:15273/>。用户名与随机密码位于当前 checkout 的 `.artifacts/monitoring-local/login.json`（0600），不在文档或日志复制密码。`acceptance.json` 保存工作空间链接、资源 ID、批次与初始期望数量；`samples/` 保存标明合成的验收文件。详情见[人工验收清单](monitoring-data-acceptance.md)。完成 seed 后重跑不会覆盖人工验收改动。

应用用 Ctrl+C 停止，存储卷保留。Docker 重启后重新 `setup`、`start` 即可；不要用 `down --volumes`，它会删除验收数据。该本机环境只绑定 loopback，S3 身份为公开的本机测试凭据，不能用于真实数据。PG／Redis、源文件、CH 两副本和 Keeper 均有独立命名卷；备份暂存使用另一个 S3 服务。它们仍处于同一台电脑，不能代表生产故障域。

`pnpm monitoring:local cli <命令>` 为本机包装器，只负责注入本机配置，内部执行统一 `infrapro`。例如 `pnpm monitoring:local cli monitoring storage status`。普通部署使用下文 `pnpm infrapro ...`，从配置文件或环境变量注入自己的设置。

## 检查与对账

- `monitoring storage check` 检查布局版本、实际类型／排序／分区／约束、双独立副本、路由配置、在线 S3 卷和有界文件缓存。`init` 是显式操作，先初始化再执行同样检查；不自动迁移已有不兼容原始表。
- `monitoring storage status` 只读输出任务等待／批次停留、attempt、dirty 年龄、过期租约、隔离通道、临时盘字节／inode 余量，以及各副本 part、字节、复制队列／延迟、合并、最近一小时 CH 查询分位数／扫描量／缓存事件。不可用项标为不可用；物理行数包含工作副本，不是当前在用观测数。CH query_log 有刷新延迟。
- API 响应 `X-Monitoring-Trace-ID`、持久 job.trace_id、审计 trace、PG 事务本地 trace 和 CH query_id 关联同一链路。`monitoring.operation` 日志按操作输出时长、响应字节、对象读写量／错误及取消返回时间；可按操作和时间区间聚合吞吐与 API 分位数，不能将 CH 查询分位数冒充整个 API 延迟。日志不包含原始文件正文或连接凭据。
- 独立 worker 每分钟选取一个到期租户，首次发布满 24 小时后排队每日对账。任务持久保存，多个 worker 不会重复建立同一天相同水位的检查。应监看失败任务、长期 dirty、隔离通道、复制与空间余量；本轮不新增业务告警执行系统。

只读全面检查：

```sh
pnpm infrapro monitoring verify --tenant TENANT_ID
```

检查源文件 SHA-256、PG 清单与原始规范依据、所有目标副本的完整载荷和 ordinal、当前同通道同时间唯一性、历史首次采用与稳定身份、贡献及摘要精确状态／树结构、固定路由。对账期间有关版本变化会显式失败，重试时重新固定；临时空间达到每任务 8 GiB 时中止，不把未检查的范围记为通过。

需要持久对账并隔离／修复发现的问题时，先从当前查询或检查结果获取实际发布水位，再执行：

```sh
pnpm infrapro monitoring verify --tenant TENANT_ID --apply --expected-watermark N --reason '本次维护原因' --wait
pnpm infrapro monitoring jobs inspect --tenant TENANT_ID --id JOB_ID --report
pnpm infrapro monitoring summaries rebuild --tenant TENANT_ID --channel CHANNEL_ID
pnpm infrapro monitoring summaries rebuild --tenant TENANT_ID --channel CHANNEL_ID --apply --plan-hash HASH --reason '按预览重建' --wait
```

`verify` job 的 succeeded 表示检查执行完成，报告 `state`／progress.phase 才表示通过或发现问题。发现受影响通道后进入 quarantine，查询明确不可用，修复派生对象使用新不可变路径并保留旧依据。原始块、身份或源文件损坏不能靠重建摘要消除；保持隔离，从已核验备份恢复原依据后重新检查。源数据与相关版本核验通过后才安装新摘要并释放隔离。旧查询快照失效。

任务最多自动尝试五次，租约／fence 防止旧 worker 提交。`jobs inspect` 查看当前修订；修复原因后用 `jobs retry --tenant ... --id ... --expected-revision ... --reason ...` 重试。已终止导入需在原工作空间重新全量校验，不借操作员身份绕过原操作者资格。

## 在线分层与逻辑分片迁移

原始载荷按观测月份分区，90 天是事件时间的热层放置目标，绝不是删除期限。1900 年等负 Unix 时间在 TTL 放置计算中截到纪元后 1 秒，仍立即进入在线历史层；原始 DateTime64(9) 不改。2262 年纳秒上端点不在 TTL 算术中溢出。历史经 S3 及有界本地缓存读取，无需人工恢复。

附加物理分片通过 `monitoring_physical_shards` 配置；每个有独立数据库和两个独立副本。先显式初始化并 check；再预览固定清单：

```sh
pnpm infrapro monitoring routes plan --tenant TENANT_ID --logical 0 --target second
pnpm infrapro monitoring routes move --tenant TENANT_ID --logical 0 --target second --plan-hash HASH --reason '按预览迁移' --wait
pnpm infrapro monitoring routes status --tenant TENANT_ID --id MOVE_ID
pnpm infrapro monitoring routes pause --tenant TENANT_ID --id MOVE_ID --expected-revision N --reason '暂缓切换'
pnpm infrapro monitoring routes continue --tenant TENANT_ID --id MOVE_ID --expected-revision N --phase switch --reason '核对后切换' --wait
pnpm infrapro monitoring routes rollback --tenant TENANT_ID --id MOVE_ID --expected-revision N --reason '反向复制后回退' --wait
```

第一阶段复制时原路由继续服务；switch 设置短暂发布屏障、补齐尾部、逐副本完整核验，再通过 PG 短事务安装新全局路由版本。copy／switch 可暂停和恢复，所有命令核对当前 revision。旧快照保持原路由，跨分片明细按完整排序键有界合并。不确定写入用新 attempt，不能重发同一物理身份。回退先反向复制切换后新增数据，再切换。当前保留全部旧物理副本；尚无自动 GC，避免在快照、导出、备份或对账仍引用时回收。

## 一致性备份与独立恢复

本轮提供**应用 checkpoint 的完整备份与恢复**，不是任意时间点 PITR。配置独立备份 endpoint、bucket、身份以及 CH 服务器 named collection `monitoring_backups`；CH 原生备份 SQL 只引用该名称，不放入访问密钥。源文件、CH 磁盘、备份三个身份互相分开。PG 客户端需与服务器主版本相符；本机可通过 `--pg-tools-container` 使用 PG 17 容器内客户端，密码经进程环境传递。

```sh
pnpm infrapro monitoring backup --directory /ABS/NEW_BACKUP_DIRECTORY --reason '生成应用一致性备份' --pg-tools-container PG_CONTAINER
pnpm infrapro monitoring restore --directory /ABS/BACKUP_DIRECTORY --manifest-sha256 MANIFEST_SHA256
# 改为独立空目标的配置后执行；不会覆盖已有目标库或源文件桶。
pnpm infrapro monitoring restore --directory /ABS/BACKUP_DIRECTORY --manifest-sha256 MANIFEST_SHA256 --apply --reason '独立环境恢复核验' --pg-tools-container TARGET_PG_CONTAINER
pnpm infrapro monitoring restore open --backup-id BACKUP_ID --reason '恢复核验完成，开放写入'
```

备份目录必须不存在；创建为 0700，成员文件 0600。导出 PG repeatable-read snapshot 并让 pg_dump 使用同一快照，冻结租户发布水位／路由、原始及保留的 selected attempts；原生 CH BACKUP 保存 part／必要元数据，再复制源文件及全部依据到本机归档，记录每文件大小／SHA-256、索引和清单 seal。较新物理数据可以存在但不会加入控制 checkpoint。最终清单完成之前不算成功，失败 receipt 留存用于定位。

恢复先验证整份归档，再写入独立空 PG／CH／源对象环境。备份不包含 ready guard 行，PG 恢复事务一旦可见即默认禁止正常监测读写；随后进入 restoring，撤销旧会话和访问凭证、使旧快照／导出失效。恢复原始载荷／文件、核验所有副本／身份／撤销／授权／数学状态，必要时重建派生摘要；全部通过后只读开放，再显式 open 写入。失败目标保持不可用，不对非空半恢复环境盲目重放；保留报告并在另一个空目标重新恢复。已有环境的应急恢复仍应由维护人员控制配置切换。

`pnpm test:monitoring:restore` 自动建立独立临时目标，恢复期间暂停源 CH、源对象存储和备份暂存，验证撤销及重复核对依据、项目局部授权、纳秒与大数值、旧凭证撤销和恢复后新发布，随后清理自己创建的临时目标。运行它会短暂暂停契约实验室，勿与其他该实验室测试并行。不会暂停持久验收环境。

当前备份完整归档在同一主机；没有 WAL 连续归档、PITR、异地主机故障或生产 RPO/RTO 证据。归档含账号／业务控制信息，应按敏感备份保管；密钥配置由外部配置恢复，不写入 manifest。生产上线前须明确独立故障域、WAL/PITR 和密钥恢复方案，再实测容量、恢复点及耗时，不能以本机秒级结果作 SLA。
