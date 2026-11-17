# ClickHouse 单节点部署支持验证

日期：2026-11-17（Asia/Shanghai）。状态：**Awaiting acceptance**。用户要求本轮移除至少两个 ClickHouse 副本的限制，首期采用单实例试运行。代码已修改并完成下列自检，生产安装与用户人工验收尚未执行。

## 实现范围

- 默认监测存储接受一个或多个有效 endpoint；附加物理分片也接受单节点。未配置监测存储的可选行为保持；维护 CLI 和直接存储构造仍拒绝零节点、无效地址和重复节点。
- 沿用 `ReplicatedMergeTree`、Keeper、热层／在线 S3 层和现有原生备份协议。单节点可部署一个同机 Keeper；不建立第二份数据，也不宣称自动接管。
- 写入仍按实际配置的节点数确认；布局、类型、约束、TTL、对象存储、发布清单与载荷摘要检查继续执行。多节点配置仍逐节点核验身份与完整性，不能重复填写同一实例冒充多副本。
- 配置样例、维护 CLI 帮助、监测／派生运行手册、架构说明和首期部署方案同步更新。

主要实现：[配置读取](../../packages/go/platform/config/monitoring.go)、[存储构造与读写](../../packages/go/platform/monitoring/storage.go)。本轮没有更换表引擎、迁移已有表、改变数据库版本或修改生产服务器。

## 自检结果

| 检查 | 实际结果 |
|---|---|
| 配置测试 | 单节点、双节点、单节点附加分片均接受；空／空白／非法／重复 endpoint、零节点附加分片拒绝 |
| 真实单节点存储 | ClickHouse `system.replicas` 确认 total/active 均为1；重复初始化、布局检查、写入、完整载荷封存和读取通过 |
| 附加物理分片 | 单节点默认分片及双节点默认分片均可搭配单节点附加分片，实际写入与核验通过 |
| 完整性拒绝 | 损坏的发布清单哈希无法通过读取就绪检查 |
| 单节点备份恢复 | 使用原生备份写入独立备份 S3，恢复到独立空数据库；布局检查及原载荷摘要一致 |
| 单节点业务发布 | 真实 PG／CH／S3 上完成 CSV 上传、校验、发布、查询；1900年在线历史与2262年纳秒端点、精确大数均保留 |
| 双节点回归 | 两个真实副本的初始化、写入、封存与读取；CSV 发布／重复集、历史分层、布局篡改拒绝、原生 checkpoint 备份均通过 |
| 单元与 CLI | config、monitoring 及 observation 子包、维护 CLI 测试通过 |

证据在本机 [`.artifacts/single-clickhouse-20261117/`](../../.artifacts/single-clickhouse-20261117/)：`storage-topology.log`、`publication-regression.log`、`unit-cli.log`。真实服务使用现有隔离实验室；每次创建唯一数据库／schema／对象前缀，结束后清理自己创建的数据，不停止或删除共享测试服务。

执行命令：

```sh
go test -count=1 ./packages/go/platform/config
go test -count=1 ./packages/go/platform/monitoring/... ./tools/infrapro/internal/command
INFRAPRO_MONITORING_CONTRACT=1 go test -tags monitoringintegration -count=1 -v -timeout=5m ./packages/go/platform/monitoring -run '^TestStorageOneOrMoreReplicas$'
```

业务回归使用 `scripts/test-environment.mjs` 中的隔离测试 PG／Redis 环境变量，并设置 `INFRAPRO_MONITORING_CONTRACT=1`，执行：

```sh
go test -tags monitoringintegration -count=1 -v -timeout=10m ./packages/go/platform/resources -run '^TestMonitoring(SingleNodePublicationAndOnlineHistory|StorageLayoutAndOnlineHistory|CSVPublicationAndFixedDuplicateSet|NativeBackupCheckpoint)$'
```

未运行全项目 E2E、生产压测或整个平台联合恢复；单节点原生恢复测试仅证明此次改动涉及的 ClickHouse 存储路径，不替代正式备份演练。本地实验室仍保留两个 CH 服务用于双节点回归，但单节点用例只配置／初始化一个真实副本，实际 total/active 为1，不靠另一个数据副本通过测试。

## 人工验收

在备齐 CH／Keeper／S3 的空验收环境中，将 `monitoring_clickhouse_replicas` 仅配置一个地址，通过统一 CLI 执行 `monitoring storage init` 与 `monitoring storage check`，应成功；完成一批 CSV 发布并查询，应能看到精确结果。配置空地址、重复地址或破坏发布证据，应明确失败。生产实例尚未安装，本文件不提供已上线地址。
