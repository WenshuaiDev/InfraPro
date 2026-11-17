# 部署前 B1：数据库版本检查一致性

状态：**Awaiting acceptance（自验通过，待人工验收）**。本轮仅处理[部署准备清单](../operations/production-deployment-readiness-2026-11-17.md)的 B1；用户人工接受与生产部署分别记录。

## 问题与修复

2026-11-17 在当前 main 工作副本复现：业务库迁移到54后，API使用的 `migrations.Check` 成功，接入运维 `OperatorManifest` 却返回 `schema_incompatible`。最小测试仅创建隔离 schema、执行真实迁移并导出清单，约3秒可复验；原联合恢复测试也在首次导出清单处失败。根因是接入运维仍比较和报告旧常量45，后续迁移已经推进全平台版本。

接入检查改用 [`migrations.Version`](../../packages/go/platform/migrations/migrations.go)，继续精确匹配发行版本；清单 `schema_version` 来自已核验的数据库值，发行报告 `schema` 来自同一清单。不改历史 SQL、不增加迁移，不自动升级或回退数据库。CLI只对已知版本错误输出 `schema_incompatible` 和发行要求的版本，其他错误保留受保护的通用提示。

技术沿用 Go、pgx、Goose、PostgreSQL；测试通过实际 Go 可执行程序、PostgreSQL dump/restore、私有 S3 与解析发行包检查部署链路，无需前端改动或新运行依赖。

接入常规回归还暴露了原有历史迁移夹具的版本依赖：在 v44 数据库上调用当前产品接口，访问了 v51 才存在的 `product_classification_records`。已改为通过历史 schema 的真实约束准备产品、设备及固定解析输入，不调用新版产品写接口，不增加临时兼容表、不跳过升级。真实 parser 在升级前执行原输入，升级后再通过当前服务重放；v43／v44 到当前版本的迁移检查均保留，并增加产品默认解析配置的摘要对比。

## 自验记录

2026-11-17 在本机专用测试库和私有测试对象存储完成以下检查；全部通过。数据库与对象均为隔离模拟数据，未连接生产配置。

| 检查 | 断言 | 状态 |
|---|---|---|
| 版本回归 | 同一测试库的运行时与运维判断一致；当前54通过，旧45、前一版53、未来版55和未迁移状态拒绝 | 已通过 |
| 空库首装 CLI | 真实迁移且未 bootstrap，`manifest`／`release`／`objects` 退出0；版本正确，清单与报告权限0600 | 已通过，0对象／0接收事实 |
| 不支持版本 CLI | 三个命令退出1，显示 `schema_incompatible` 和期望版本，不生成成功报告 | 已通过，45／53／55 |
| 联合恢复 CLI | 实际恢复业务PG、relay PG、对象及解析包，三个命令在恢复目标成功 | 已通过，6对象／2接收事实，未完成计划恢复后共4结果 |
| 完整性拒绝 | 缺对象、同长度篡改、缺历史解析可执行产物均退出1；可信对象修复后恢复通过 | 已通过 |
| 历史增量升级 | v43产品／设备／输出保持，v44方法／配置／默认采用／样本／运行／对象摘要不变，原方法升级前后均解析出12.34 | 已通过，升级目标54 |
| 并发迁移 | 四个并发迁移执行后版本一致、初始化不重复，未知版本拒绝 | 已通过 |
| 工程检查 | `pnpm check`：Go格式／vet、ESLint、TypeScript、契约、权限目录 | 已通过 |
| 接入常规回归 | `pnpm test:ingestion`：协议、接收、权限、故障、历史兼容、诊断、首装和恢复 | 已通过，退出0 |

自动测试入口：[版本回归](../../packages/go/platform/ingestion/operator_test.go)、[空库CLI](../../packages/go/platform/parsing/ingestion_operator_test.go)、[联合恢复](../../packages/go/platform/parsing/ingestion_restore_test.go)。

本机证据目录：[`.artifacts/deployment-schema-20261117/`](../../.artifacts/deployment-schema-20261117/)。`before-fix-joint-restore.log` 保留原问题；`targeted-tests.log` 和 `targeted-joint-restore.json` 记录空库／恢复命令核验；`legacy-upgrade-tests.log` 记录历史夹具修复后的迁移；`before-fixture-fix-ingestion.log` 保留发现夹具问题时的回归失败，最终回归另记为 `ingestion.log`／`ingestion.json`，最终恢复统计为 `joint-restore.json`，静态检查为 `check.log`／`check.json`，并发迁移为 `migrations.log`／`migrations.json`，源码摘要和完成范围为 `result.json`。私有配置、数据库连接、证书和样本正文不附在本报告中。

## 人工复验

使用同一发行包的迁移、API、运维工具和对应平台的完整解析产物；首装／恢复配置均指向隔离目标，按[接入手册](../operations/device-ingestion-and-raw-messages.md)准备数据库和对象。

```sh
umask 077
export INFRAPRO_CONFIG=/etc/infrapro/runtime.yaml
build/bin/ingestion-operator manifest /srv/backup/inner-manifest-b1.json
build/bin/ingestion-operator release /srv/infrapro/releases/parser-current
build/bin/ingestion-operator objects /srv/backup/objects-b1.json
```

预期三个命令均退出0，清单 `schema_version` 与发行报告 `schema` 一致并匹配冻结发行，`missing_or_corrupt=0`；空库应有0个接收事实与对象。清单路径必须尚不存在。按联合恢复流程还原到独立空目标，切换到恢复目标的私有配置后以新输出路径重复，核对版本、能力、对象和接收事实。

不兼容发行／数据库组合应退出1并提示 `schema_incompatible`；缺解析产物或对象完整性失败同样退出1。故障注入仅在专用测试库／桶／包副本进行，不修改生产版本记录或产物；自动测试已提供该流程。

本轮未运行30分钟持续负载或全站浏览器 E2E；常规接入脚本按设计显式跳过持续负载用例。本轮不安装或迁移生产服务器，不改变 HTTPS、Nginx 或其他部署项。真实四服务器部署、全平台联合恢复与人工接受不由本项局部自验代替。
