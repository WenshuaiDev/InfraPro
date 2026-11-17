# 解析服务逐项验收报告

状态：**Awaiting acceptance（全部开发自验通过，待用户人工验收）**。人工验收：未进行；实机联调：未进行。目标为[开发规格](../development/parsing-service-and-method-binding-spec.md)全部 PAR-001—PAR-076，未缩减范围。

开发自验结果：**PAR-001—PAR-076 共 76 项通过，未缩减范围**。`pnpm check`、`pnpm build`、解析服务／fuzz／真实 PG-S3-HTTP 专项、17 项浏览器场景及相关模块回归均已完成。原始回归中的旧权限数量断言已修复并复验，详情见下文。协议文档样本验证与系统贯通验证通过；真实设备联调未执行。

## 环境和证据类型

自验日期 2026-11-16。当前 checkout `/Users/hijacker/.codex/worktrees/bed0/InfraPro`；Go 1.27、Node 26、pnpm 12.18，macOS 原生 Go 程序，隔离 PostgreSQL 测试 schema／恢复数据库、Redis 测试前缀和 S3 私有桶。浏览器 Chromium 使用真实 API、monitoring-worker、parser HTTP 父进程及每次执行的固定子进程。另在 Linux arm64 Docker 中以非 root、只读根、断网和资源上限运行真实 runner 与内核隔离测试。

测试只使用合成厂家／产品／设备、固定 DDP 单帧和明确标注的模拟 JSON。公共 DDP 封装能力与模拟测量载荷分别判断；不以合成样本声明任何真实型号／固件适配。不依赖 ClickHouse 的试解析链路没有生成监测导入、正式 publication 或 chunk。

## 检查入口

- `pnpm parser:contracts`：业务与机器 API、Go 源模型、生成 TypeScript 和权限同步。
- `pnpm test:parsing`：真实固定执行产物、服务故障／边界、5 秒 fuzz、PG／S3／HTTP、迁移和独立恢复。
- `pnpm parsing:browser`：真实账号与页面、遮罩 Drawer、任务回跳、版本比较、375 px／720 px 布局、停机恢复及租户／项目隔离。
- `pnpm test:parsing:regression`：任务、设备目录、权限、审计、配置、诊断与任务客户端回归。
- `pnpm check`、`pnpm build`：全项目静态检查、契约、类型与构建。

证据均在 `.artifacts/parsing/`；下表中证据号定位到具体测试名。该目录包含本机生成的原始日志，不作为产品或生产数据提交。初次回归发现权限目录数量断言仍为 207，已按新增 12 项更新为 219；设备／任务／审计通过的结果保留，权限与配置单独复验在 `regression-access-recheck.log`，无需重复运行已通过的长回归。

| 证据号 | 文件（相对 `.artifacts/parsing/`） | 实际测试／场景 |
| --- | --- | --- |
| C | `platform-tests.log` | `TestCatalogImmutableAndUnavailable` |
| B | `platform-tests.log` | `TestAdoptionTransactionsAndSemanticBoundaries` |
| F | `platform-tests.log` | `TestFrozenRunMappingRevisionsAndExplicitCopy` |
| M | `platform-tests.log` | `TestMappingRejectsKnownUnitAndShapeConflict` |
| T | `platform-tests.log` | `TestTenantIsolationBudgetsAndRawPrivileges` |
| W | `platform-tests.log` | `TestResponseValidationRetryRevocationAndFences` |
| R | `platform-tests.log` | `TestObjectCommitInterruptionMissingAndRetention` |
| J | `platform-tests.log` | `TestIndependentPostgresS3AndPackageRestore` |
| A | `platform-tests.log` | `TestProjectReadOnlyTaskAtomicityAndAuditPrivacy` |
| Q | `platform-tests.log` | `TestScopeConcurrencyAndFrozenAdoption` |
| V | `platform-tests.log` | `TestAdditiveMigrationPreservesLegacyObjects` |
| P | `platform-tests.log` | `TestRealServiceAdoptionExecutionAndReplay` |
| S | `service-tests.log` | `TestMachineAuthBudgetAndChildReaping / TestRealExecutableContractAndPackageCoverage / TestResponseByteBudget` |
| D | `service-tests.log` | `TestDocumentFixtures / TestMalformedFramesAndLimits / TestStrictJSONValues / TestVersionsAndDeterminism / TestStrictWireAndRepresentations` |
| U | `http-tests.log` | `TestParsingHTTPStrictBoundariesAndDisabledHistory / TestRoutePolicyCoverage` |
| E | `browser/result.json` | `真实浏览器 17 项场景；截图在同目录` |
| L | `linux-limits.log` | `TestLinuxInvocationHardLimits，另见 linux.json 的真实 runner 结果` |
| G | `architecture.json` | `独立依赖图与 8 组原始二进制／HEX／SHA-256 夹具` |
| H | `platform-tests.log` | `TestRemoteEndpointRequiresTLS`，跨主机私网明文同样拒绝 |
| K | `check.log` | `pnpm check；另见 build.log、regression*.log 和 regression.json` |

## PAR-001—PAR-076 对照

每项环境沿用上面的真实隔离环境；L 项专用 Linux 容器。结果是开发自验，与用户人工接受分开。用例中的故障和越权被拒绝是预期成功结果。

| 编号 | 实际操作／核对条件 | 结果 | 证据 |
| --- | --- | --- | --- |
| PAR-001 | 首次启动真实服务并同步清单 | 通过 | C |
| PAR-002 | 重复同步同一清单 | 通过 | C |
| PAR-003 | 同一方法版本返回不同清单或产物摘要 | 通过 | C |
| PAR-004 | 同步时服务不可用／清单不完整／契约不支持 | 通过 | C |
| PAR-005 | 平台启用新版本 | 通过 | C,B |
| PAR-006 | 版本停止新采用 | 通过 | B,P |
| PAR-007 | 平台阻断已使用版本 | 通过 | W,T |
| PAR-008 | 发布遗漏在用／历史所需产物的新服务 | 通过 | S |
| PAR-009 | 更新 HTTP 服务但保留旧方法产物 | 通过 | S,D |
| PAR-010 | 已有设备采用新版本后回滚到缺少该版本的包 | 通过 | S,J |
| PAR-011 | 平台公共产品、租户私有产品分别配置 | 通过 | E,B |
| PAR-012 | 更新配置映射并设默认 | 通过 | F |
| PAR-013 | 用不可见版本、跨产品修订或未知方法建立配置 | 通过 | B,F |
| PAR-014 | 映射重复输出键／重复槽位／不存在槽位 | 通过 | B |
| PAR-015 | 不兼容单位、值形态或已知时间语义 | 通过 | M,F |
| PAR-016 | 有可配置槽位但实际设备输出未配置 | 通过 | B,F |
| PAR-017 | 有默认的新设备在界面预选准确默认修订并明确提交 | 通过 | P,B,E |
| PAR-018 | 无默认或旧客户端未传采用意图 | 通过 | B,V |
| PAR-019 | 提交创建／采用前产品默认发生变化 | 通过 | B |
| PAR-020 | 产品默认从 1.0.0 切至 1.1.0 | 通过 | P,E |
| PAR-021 | 租户只升级三台设备中的一台 | 通过 | B,E |
| PAR-022 | 设备特化为非产品默认的可用方法／版本 | 通过 | B |
| PAR-023 | 特化设备恢复继承 | 通过 | B |
| PAR-024 | 清空采用／删除产品默认 | 通过 | B |
| PAR-025 | 回退到历史可用配置 | 通过 | B |
| PAR-026 | 同时修改同一设备或批量中混入冲突项 | 通过 | B |
| PAR-027 | 产品新版本发布／复制产品／从可读配置显式复制／删除引用对象 | 通过 | F,E |
| PAR-028 | 试解析时切换采用／结束设备输出代次 | 通过 | F |
| PAR-029 | 同一字节以 HEX、文件方式提交 | 通过 | T,G |
| PAR-030 | 非法 HEX、base64、长度或摘要 | 通过 | D,S,U |
| PAR-031 | 大整数、精确小数、零值、空值和溢出夹具 | 通过 | D |
| PAR-032 | 缺少观测时间／无时区时间 | 通过 | D |
| PAR-033 | 响应方法／输入摘要／执行 ID 与请求不符 | 通过 | S,W |
| PAR-034 | 返回未知输出键、越界字节范围或无效步骤引用 | 通过 | S |
| PAR-035 | 在运行详情选一个监测值 | 通过 | E |
| PAR-036 | 正确解析但映射缺失或已变化 | 通过 | F,E |
| PAR-037 | 某个字段非法而其他字段正确 | 通过 | D |
| PAR-038 | 同一固定输入和产物多次执行 | 通过 | D,S |
| PAR-039 | 超输入、输出、记录、步骤、嵌套或执行预算 | 通过 | S,D,L |
| PAR-040 | 对同一原始样本运行两个版本 | 通过 | E |
| PAR-041 | 提交第 7.3 节的 81 字节固定样本 | 通过 | D,G |
| PAR-042 | 用示例方法解析同一帧 | 通过 | D,E |
| PAR-043 | 提交固定注销帧及独立注册／应答夹具 | 通过 | D |
| PAR-044 | 错误帧头／帧尾／长度／截断 | 通过 | D |
| PAR-045 | 载荷内出现 0x7B、刚好 1024 字节与超过上限 | 通过 | D |
| PAR-046 | 一次提交两帧、部分 TCP 流或 UDP 布局 | 通过 | D |
| PAR-047 | DDP 帧合法但包型不在首批范围 | 通过 | D |
| PAR-048 | 用户载荷为未知二进制 | 通过 | D |
| PAR-049 | JSON 重复键、未知字段、非法 UTF-8 或数值 | 通过 | D |
| PAR-050 | 含 source_status 的样本分别跑两个示例版本 | 通过 | D,E |
| PAR-051 | 在产品／设备页使用示例方法 | 通过 | E |
| PAR-052 | 浏览器从选方法到查看原文和结果 | 通过 | A,E |
| PAR-053 | 租户尝试创建方法、上传代码或设置服务 URL | 通过 | U |
| PAR-054 | 租户更改公共产品默认／其他租户配置 | 通过 | B,T |
| PAR-055 | 平台方法管理员访问租户原始样本或升级其设备 | 通过 | T,E |
| PAR-056 | 项目成员仅有设备读取／输出使用授权 | 通过 | A,E |
| PAR-057 | 有运行读取权但无样本读取权 | 通过 | T |
| PAR-058 | 排队后或执行期间撤销本代发起人权限 | 通过 | W |
| PAR-059 | 撤权后访问旧任务、详情、下载 URL 或浏览器缓存 | 通过 | W,E |
| PAR-060 | 相同样本摘要分属两个租户 | 通过 | T |
| PAR-061 | 伪造 DTU 身份与其他设备序列号相同 | 通过 | D,A |
| PAR-062 | 解析服务无机器令牌、错令牌、浏览器直接访问及跨主机明文 | 通过 | S,H |
| PAR-063 | 写操作丢失响应后用原 request_id 重放 | 通过 | P,A,R |
| PAR-064 | 同键不同载荷重放 | 通过 | A |
| PAR-065 | 服务完成后网络中断导致内部重试 | 通过 | W |
| PAR-066 | 取消、租约丢失或新代建立后旧响应迟到 | 通过 | W |
| PAR-067 | 人工重试失败运行／改版本重新解析 | 通过 | W,P |
| PAR-068 | 原始或结果对象写入与 PG 提交之间中断 | 通过 | R |
| PAR-069 | 关闭解析服务再恢复 | 通过 | E |
| PAR-070 | 触发队列、范围存储预算和并发限制 | 通过 | T,Q,S |
| PAR-071 | 平台／租户真实任务登记、伪造项目任务及登记故障 | 通过 | A |
| PAR-072 | 检查审计、日志和诊断快照 | 通过 | A,E |
| PAR-073 | 旧数据库执行新增迁移 | 通过 | V |
| PAR-074 | PG、S3、清单与旧执行产物联合恢复 | 通过 | J |
| PAR-075 | 按交付启动说明运行并执行全部相关检查 | 通过 | K,E,L |
| PAR-076 | 核对最终验收报告 | 通过 | K |

## 人工验收入口与预期

启动、停止、配置、方法发布／兼容回滚、联合备份恢复的完整命令见[运行手册](../operations/parsing-service.md)。`pnpm parsing:lab` 建立可重复的独立环境，页面 `http://127.0.0.1:15578`；私有账号清单和对象索引分别位于 `.artifacts/parsing/browser-lab/accounts.json`、`acceptance.json`。当前保留的独立环境已有一条设备 1.0.0 的成功基线运行；`readiness.json` 记录其入口、原样本与结果摘要，结果为 `12.34 mm`。

1. 平台账号打开“试解析与运行”→方法目录，看到 DDP 封装与示例 JSON 的 1.0.0／1.1.0、最近同步成功；筛选方法和版本状态，查看完整版本和产物摘要。
2. 公共产品“DDP 监测载荷模拟器”→解析配置，将默认由 1.0.0 改成 1.1.0；切换租户账号，旧设备应仍显示固定 1.0.0。只升级一台，确认遮罩预览及未选设备不变。新设备预选已显示的默认修订；可明确改为未配置。
3. 建立设备特化配置，明确映射或忽略；采用、恢复继承、回退与清空都填写理由，历史各区间和来源默认修订可查。私有产品页只允许所属租户维护。
4. 提交 `samples/parsing/measurement.hex` 或 `.bin`，应得到 `2026-11-16T00:00:00Z`、`12.34 mm`，点击值定位 `[71,79)`，看到原文 `"12.340"`、步骤规则及固定输出代次；下载原输入表示应与提交的 HEX 文件逐字节相同。业务质量为未评定、正式采用为否。
5. 使用控制帧、坏长度和 `source-status.hex`：控制帧零条测量，错误帧有诊断；1.0.0 拒绝新字段、1.1.0 保留 `suspect`。两次运行独立，旧结果不覆盖。任务链接可取消／重试并回跳对应代。
6. 第二租户不可读取第一租户运行或原文；项目只读账号只能看到获准设备采用摘要，不能维护、试解析或下载原文。重新启动隔离环境可复现实验初始状态。

## 未声称完成的事项

用户人工接受、真实厂家／型号／固件报文联调、生产部署和生产容灾演练尚未进行。长连接采集、TCP 分帧重组、UDP DDP、质量评定、告警及正式观测采用属于规格明确排除的后续范围。Linux 内核限制已有实测；systemd 单元交付为部署模板，本次没有修改本机或远程主机 systemd。诊断原有 Docker／DBus／多主机实环境全套没有重跑，本次完成解析相关进程报告、coverage、诊断 Go 回归和 Linux 子进程隔离。

## 主分支集成复验

本次只提取解析服务、固定方法采用及必要的权限／任务／审计／诊断接入。基于主分支已提交基线导出的独立源码快照通过 `pnpm check`、`pnpm build`、`pnpm test:parsing` 与 17 项浏览器场景；没有依赖其他未提交的项目治理、空间或界面改造。集成日志在原开发 checkout 的 `.artifacts/parsing/integration/`，浏览器验证使用独立端口 15579／18579，原人工验收入口保持 15578。临时源码目录是普通文件导出，没有新建 Git 分支或 worktree。


## 设备接入实现的兼容复验（2026-11-16）

本次在 `/Users/hijacker/.codex/worktrees/6934/InfraPro` 复验现有协议管理，未改变上面的历史自验和人工接受结论。新接入目录、生产流水和人工解析记录独立；测量目录新增 Reading 方法后，相关数量断言随实际目录更新。

- `pnpm test:parsing` 的服务、fuzz、平台与 HTTP 均通过，记录 `.artifacts/parsing/tests.json`。
- `pnpm test:parsing:regression` 的业务竞态、审计量级、诊断和任务客户端回归通过，记录 `.artifacts/parsing/regression.json`。
- `pnpm parsing:browser` 的原浏览器路径通过，记录 `.artifacts/parsing/browser/result.json`。
- 原三份方法 entry、manifest 与可执行文件保持字节一致，经契约1实际重放，记录 `.artifacts/ingestion/compatibility/result.json`。
- v44已有方法、配置、采用、样本、run和对象记录迁移到v45后逐表哈希不变；旧方法仍可执行，接入不自动启用。
- 当前权限目录为233项并通过生成一致性检查；接入管理权限不替代原设备解析管理权限。

新功能的持续负载、完整 ING-001—ING-136 及环境边界单独见[设备接入验证报告](device-ingestion-and-raw-messages.md)。用户人工接受仍待进行。
