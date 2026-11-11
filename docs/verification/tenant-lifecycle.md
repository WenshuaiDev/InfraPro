# 租户启停与恢复验证记录

日期：2026-11-10。对应[确认方案](../requirements/tenant-lifecycle-plan.md)及[运行与人工验收手册](../operations/tenant-lifecycle.md)。

- 方案：用户已确认，包括 D-01–D-12。
- 功能状态：**Awaiting acceptance**；实现及自验完成，等待用户人工验收。
- 用户人工验收：**待执行、待确认**。自动验证通过不代表 Accepted。
- 代码基线：分支 `feature/account-access-console`，HEAD `776663fc125223a02df6f46a24e5d0cf7663a8da`，加当前未提交工作区。接续原有成员和角色实现，未覆盖或回退原修改。
- 本轮已执行本机 v5→v6 迁移、权限目录同步及开发服务启动；未提交、推送或部署生产。

## 1 实现范围

1. v6 状态及元数据约束，新旧租户默认 active；独立停用/恢复权限，预览、近期密码、版本及影响哈希校验，事务内状态、授权修订和审计一起提交。
2. 统一关闭全部 31 个现有 tenant 策略入口的业务读写；独立的本人最小状态接口和平台治理继续按权限工作。领域服务先复验 Actor，再返回范围授权结果。
3. 临时停用保留邀请及下层身份历史；真实撤权、固定授予权限覆盖丢失和目录发布仍永久作废邀请。恢复重新计算当前资格，不复活终态。
4. 平台状态及确认交互、我的租户卡片、统一受限页面、邀请检查、30 秒轮询与聚焦复验。按账号、租户、成员关系及页面代次隔离旧响应，纯依赖故障保留草稿。
5. 内部原因与公开说明分离，租户审计字段和搜索均使用公开投影；契约、生成客户端、迁移与回归用例、运行及验收文档同步更新。

## 2 命令与环境

所有日志位于本机忽略目录 `.artifacts/tenant-lifecycle/`。该目录包含敏感的备份和匹配环境文件，不进入版本库；不能分享整个目录。

| 检查 | 命令 / 环境 | 结果与证据 |
| --- | --- | --- |
| 静态检查 | `pnpm check`；Go 格式/vet、ESLint、TypeScript、契约及权限生成一致性 | 通过；`check.log` |
| 生产构建 | `pnpm build`；API、CLI、console | 通过；`build.log` |
| Go 全量及竞态检测 | `pnpm test:identity`，各 Go 模块 `go test -race -count=1 ./...`；独立 PostgreSQL/Redis、隔离 schema | 所有模块通过；平台模块含 tenancy 378.188 秒；API/CLI 因测试容器中断补跑通过。`go-full-final.log` + `go-api-cli-final.log` |
| 开发模式浏览器 | `pnpm test:e2e`；Chromium 桌面/移动端及既有平板展示回归，重试 0 | 63 通过，2 个仅生产 chunk 用例按配置跳过；`e2e-full-final.log` |
| 生产构建浏览器 | `pnpm test:e2e:preview`；生产静态构建，同一浏览器矩阵，重试 0 | 65/65 通过，无跳过；`preview-recovered-final.log` |
| 成员重新加入 | `pnpm test:e2e:preview tests/e2e/tenant-membership.spec.ts -g rejoining` | 2/2 通过；`rejoin-final.log` |
| 启停定向浏览器 | `pnpm test:e2e tests/e2e/tenant-lifecycle.spec.ts` | 8/8 通过；`e2e-targeted-final.log`；最终以整套回归为准 |
| 普通 HTTP 部署 | `pnpm test:http`；独立 Compose、Nginx `/console/`、真实 API/数据库/Redis | 当前代码重新构建后 5/5 通过；`http-current-final.log` |
| 本机备份恢复 | v5 的 `pg_dump` 恢复到新建临时数据库，核对 13 张业务表后删除临时库 | 通过；`local-backup-check.json` |
| 本机增量升级 | `database migrate`、`access sync --dry-run`、`access sync`、`access check` | v6、47 项权限、8 个内置角色；`local-upgrade-result.json`、`local-sync.log`、`local-check.log` |
| 重复升级 | 再次 migrate/sync/check，比对修订、角色版本、审计数和状态 | 无额外变化；`local-repeat-result.json` |

浏览器测试使用独立数据和 15173/18080 端口，不用人工验收账号。HTTP 测试使用另外的 Compose 环境；凭据恢复撤销演练只在隔离测试库执行。

启停确认框的 375/768/1440px 明暗主题共 6 张截图保存在 `screenshots/tenant-disable-{width}-{theme}.png`。已检查桌面、平板及窄屏截图：无横向溢出，窄屏对话框可纵向滚动，表单及取消/确认动作可达。测试完成时的未提交文件摘要保存在 `source-manifest.json`，用于定位当前工作区版本。

## 3 专项证据索引

以下简称用于逐项映射。Go 专项均在[tenant_lifecycle_test.go](../../packages/go/platform/tenancy/tenant_lifecycle_test.go)。

| 简称 | 具体用例/来源 | 重点断言 |
| --- | --- | --- |
| G1 | `TestTenantLifecycleIsolationPreservationAndPrivacy` | 全 tenant 策略拒绝、领域调用、Actor 优先、A/B 隔离、下层记录及会话不变、公开审计与搜索 |
| G2 | `TestTenantLifecyclePreviewRollbackAndValidation` | 空白/Unicode、版本/修订冲突、注入审计失败回滚、并发仅一次成功、数据库约束 |
| G3 | `TestTenantLifecycleInvitationRetentionExpiryAndTransfer` | 邀请整行不变、停用接受拒绝、到期使预览失效、交接/账号失效、原链接恢复与 accepted 结果 |
| G4 | `TestTenantLifecycleRestoreAdministratorAndPreservedRestrictions` | 管理员账号保护、不可用阻断、不能隐式恢复暂停成员、交接后单独恢复 |
| G5 | `TestTenantLifecycleDisabledCatalogLossIsPermanent` | 创建权/固定授予覆盖丢失、目录预览及同步永久作废、授回不复活、重复同步无变化 |
| G6 | `TestTenantLifecycleAcceptSerialization` | 受控锁顺序：停用先提交则接受拒绝；接受先提交则旧停用预览冲突 |
| G7 | `TestTenantLifecycleIndependentPermissionsAndTransactionGuards` | read/disable/enable 分离，真实近期密码过期、锁超时和目录未同步 |
| G8 | `TestTenantLifecycleDoesNotReviveRestrictedGrants` | 停用角色、撤销绑定、结束成员和停用账号恢复前后仍无业务权限 |
| U | [membership_upgrade_test.go](../../packages/go/platform/migrations/membership_upgrade_test.go) 的 v3/v4/v5 升级 | 旧字段快照、凭据、编号、邀请、暂停/结束成员、停用角色及不可用管理员保留；默认 active |
| B1 | [tenant-lifecycle.spec.ts](../../tests/e2e/tenant-lifecycle.spec.ts) `lifecycle UI` | 预览/取消焦点、真实影响、三视口明暗截图、旧表单退出、A/B、原链接恢复、原成员 ID、审计 |
| B2 | 同文件 `all tenant HTTP operations` | 从策略提取全部 31 个 tenant HTTP 操作逐个断言 403；严格 body、CSRF、过滤、隐私 |
| B3 | 同文件 `disabled boundary` | 503 保留草稿/禁提交、真实写响应延迟、35 秒内轮询受限、旧回调不回填、恢复不带旧草稿 |
| B4 | 同文件 `status confirmation` | 预览 503 禁提交、原因保留、复验不自动重发；前端复验提示注入，真实服务端复验由 G7 覆盖 |
| R | 全量既有 Go/浏览器回归 | 开户/登录/恢复、范围授权、交接、邀请未知凭据/错误目标/终态、重新加入及跨账号/租户迟到响应、权限退役 |
| H | [HTTP 启停用例](../../tests/http/tenant-lifecycle.spec.ts) 及既有 HTTP 用例 | 普通 HTTP `/console/` 深链接、真实启停页面、Origin/CSRF、JSON 错误及凭据恢复撤销 |
| L | 本机升级/恢复 JSON 及迁移/目录日志 | 实际存量数据、备份可恢复、v6、重复执行、启动就绪 |

G3 的到期及 G4 的管理员不可用在隔离 fixture 中设置数据库时间/状态以稳定验证边界，不是人工验收库操作。B3/B4 的网络故障和延迟使用 Playwright 拦截，成功业务及状态变更由真实 API 执行。

## 4 TLA-01–TLA-55 对照

下表记录已完成的自验覆盖。`M+A` 中的 M 均待用户按运行手册确认；自动证据不能替代人工接受。

| 标准 | 实际证据 | 结论/边界 |
| --- | --- | --- |
| TLA-01 | G1、B1、U | 新建默认 active，平台/个人入口一致 |
| TLA-02 | U、L | 旧业务字段及凭据保留，不造恢复事件 |
| TLA-03 | G2、U | 非法状态/元数据和管理员关系约束拒绝 |
| TLA-04 | G1、B2 | 状态+搜索+管理员可用性组合及分页验证 |
| TLA-05 | G4、U、状态组件 | 两种状态独立；不可用管理员不修改租户状态 |
| TLA-06 | G7 | read+disable 才能停用，enable 不能替代 |
| TLA-07 | G7 | read+enable 才能恢复，disable 不能替代 |
| TLA-08 | G7、B2、R | 直接 API 校验；普通租户身份不能启停 |
| TLA-09 | G7、B4 | 真实密码时限和前端无自动重放分别验证 |
| TLA-10 | G2、B2 | 空白、501 个 Unicode 字符、未知字段及恢复提交 notice 拒绝 |
| TLA-11 | G1、B1、B4 | 真实影响与对象、预览错误无法提交 |
| TLA-12 | G1、G2 | 单次版本推进、授权变更和单条成功事件 |
| TLA-13 | G1、G2、B1 | 旧版本/同状态拒绝，并发仅一成功；状态及审计可核对 |
| TLA-14 | G2、G3、B1/B4 | 资料/交接/状态变化需新版本或预览，原因保留 |
| TLA-15 | G2、G3、G4、G6 | 授权图、邀请到期及管理员条件参与重新计算 |
| TLA-16 | G2 | 审计触发器注入失败，状态/版本/修订无部分提交 |
| TLA-17 | G1、B2、B3 | 旧会话直接读写拒绝，旧表单不能绕过 |
| TLA-18 | G1、B2 | 全成员策略及 HTTP 操作受限 |
| TLA-19 | G1、B2 | 全角色/目录/授权解释/预览操作受限 |
| TLA-20 | G1、B2 | 邀请管理全部操作受限 |
| TLA-21 | G1、B2 | 完整 context 和租户审计受限 |
| TLA-22 | G1、B1、B2 | B 和个人保持，平台身份未被全局登出 |
| TLA-23 | G1、G3、G4 | 平台读/编辑/交接允许，仍维持 disabled |
| TLA-24 | G1、B2 | 平台完整审计可查；治理身份直接读普通业务为 404 |
| TLA-25 | G4 | 唯一租户停用后管理员账号保护仍在，平台仍能恢复 |
| TLA-26 | G1、G4、G8、B2 | 摘要要求正常账号及 active 关系；无关/暂停/结束不获业务访问 |
| TLA-27 | G3、R | 错误目标/未知凭据统一不可用，不先暴露租户状态 |
| TLA-28 | G3、B1 | 邀请整行一致，含哈希/版本/到期时间 |
| TLA-29 | G3、G6、B1 | 接受拒绝，无成员或 accepted 状态产生 |
| TLA-30 | G3/expire | 时间继续流逝，恢复后仍 expired |
| TLA-31 | G3/retain、B1 | 原链接恢复可用，由本人点击接受 |
| TLA-32 | G3、G5、R | 账号/交接失效和两类真实权限丢失分别覆盖 |
| TLA-33 | G5 | 停用期目录撤权再授回不复活，重复同步无变化 |
| TLA-34 | G3、G5、R | accepted/expired/invalidated 跨启停验证；revoked/replaced 沿用终态校验，启停不写邀请表 |
| TLA-35 | G6 | 两种提交顺序一致，接受先提交须重新预览停用 |
| TLA-36 | G1、G3、B1、B2 | 领域服务、HTTP 及非租户 URL 的接受入口均覆盖 |
| TLA-37 | G1、G3、G8 | 成员/绑定/邀请逐行或授权快照保留 |
| TLA-38 | G3、B1、U | 同租户及原 active 成员 ID，编号历史保留 |
| TLA-39 | G4、G8、R | 暂停/结束/停用账号角色/撤销绑定逐项覆盖；退役权限仍由有效目录排除 |
| TLA-40 | G4 | 管理员不可用阻断；交接后独立恢复；旧管理员 ended |
| TLA-41 | G1、G3、B1、B3、B4、H | 启停不写会话凭据，不自动接受/提交；备份凭据撤销另有演练 |
| TLA-42 | B1、B2、H | “查看状态”、旧页面及邀请一致显示限制 |
| TLA-43 | B1、B3 | 聚焦/请求拒绝和健康前台 35 秒目标；不承诺后台冻结/离线时限 |
| TLA-44 | B1、B3、R | 子树/草稿卸载，迟到读写不回填 |
| TLA-45 | R 跨账号/租户及 rejoining | 新 context 返回前推进范围代次，旧关系响应失效 |
| TLA-46 | B3、B4 | 503 与停用/失权分开，故障保留草稿但禁写 |
| TLA-47 | G4、G8、B3、R | 恢复读取当前授权，不用旧草稿，结束关系不能进入 |
| TLA-48 | G1、G3、B1、B2 | DTO、公开事件与搜索不泄露，其他事件快照也清理私有状态字段 |
| TLA-49 | G1、B1、R | 恢复后公开历史可查，平台身份/内部原因不下发 |
| TLA-50 | B1、R | 375/768/1440 明暗截图、取消焦点、无横向溢出；辅助技术全量审计未做 |
| TLA-51 | `pnpm check`、G1、B2 | OpenAPI/生成文件/47 项权限一致，新路由全部登记 |
| TLA-52 | H | `/console/`、Origin/CSRF 和 JSON 错误通过 |
| TLA-53 | U、L、H | 新库、v3/v4/v5 升级、重复命令、实际隔离恢复 |
| TLA-54 | Go、开发/生产浏览器、HTTP 全套 | 最终结果见第 2 节，不沿用上一轮结果 |
| TLA-55 | 本记录、运行手册、代码基线及本地证据 | 用户接受单独记录，未验证范围与启动步骤齐全 |

## 5 本机升级事实

升级前已停止旧 API 写入，保存权限 0600 的备份及匹配环境，旧 API 二进制为 0700。备份恢复到独立临时库后，对 13 张业务表逐行摘要核对一致；临时库已删除。

升级后，原账号 1、密码记录 1、会话 2、账号角色 3、租户 1、成员 1、成员角色 1 均保留，原业务字段摘要一致。原邀请数为 0，因此邀请存量升级另外用 U 的 v4/v5 fixture 验证。审计 14→15，仅新增目录同步事件，没有制造租户启用事件。再次 migrate/sync/check 后修订、角色版本和审计数无变化。

当前开发入口 `http://127.0.0.1:5173/`，API `http://127.0.0.1:8080/readyz` 就绪。无需 bootstrap，原账号密码继续使用；没有创建演示账号或对本机库全局撤销凭据。

## 6 修复记录与限制

- 新增两项平台权限后更新旧测试固定权限总数；通用总数由实际目录读取。
- 并发浏览器测试共享全局授权修订。测试显式核对 `preview_changed`，重新预览并点击确认；没有开启自动重试，也没有让产品自动重放写入。
- 生产构建捕获重新加入后旧关系清理误取消新请求的问题，已把范围代次推进放到新 context 返回前。桌面及移动端定向和开发/生产整套回归均通过。
- 最后一轮 Go API 与生产浏览器执行时，测试 PostgreSQL/Redis 在 10:22:54 收到 SIGTERM 并正常退出；同刻本机依赖也被停止。未确定停止信号来源。恢复原容器后，受影响的 API/CLI 与生产浏览器补跑通过；此前已通过的 Go 平台模块和开发浏览器结果保留。主命令中断日志仍保留，不将它标为整条命令成功。
- 仅验证 Chromium；Safari/Firefox、真实手机、辅助技术全量审计、大规模性能、异机灾备及生产环境未验证。
- 影响预览使用全局授权修订，其他租户的授权变化也可能要求重新预览，这是保守的一致性检查。计数不意味着恢复暂停/停用对象。
- 已接收的历史内容不能撤回；35 秒只针对健康、可见、未节流的前台页。服务端每次新事务实时校验状态。
- 项目、采集、后台任务、导出、实时订阅、删除/归档、计费及审批不在本轮范围。

人工验收按运行手册 16 步进行；用户明确确认后才改为 Accepted，当前不对人工结果作预设。
