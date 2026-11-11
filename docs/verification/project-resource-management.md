# 租户统一资源与项目授权重构：功能自验

日期：2026-11-11。方案：**Accepted**（用户确认并授权实施）。实现状态：**Awaiting acceptance**（实现与自验完成，等待用户人工验收）。用户人工验收：**未开始，未标 Accepted**。

对应[确认方案](../requirements/project-resource-management-refactor-plan.md)，运行入口、身份和 MA-01–MA-26 的操作/预期见[验收手册](../operations/project-resource-management.md)。本记录是开发自验，不替代用户确认。

## 1 实施基线与范围

初始实现工作区为 `/Users/hijacker/.codex/worktrees/e49a/InfraPro`，分支 `codex/project-resource-management`，基线 `4e92f95e24eb14274d24deef2a926a56d1141c24`。首次交付后，用户要求合并回主工作树 `/Users/hijacker/Documents/ChatGPT/InfraPro` 并清理临时工作树；原有账号、邀请及验收修改保留。迁移只作用于隔离测试库及专用人工验收库，未推送或部署生产。

完成 v10 增量迁移、明确 Scope 资源服务、租户治理接口、九项项目/两项租户权限、预置角色、归档保护、SQL 授权投影、资源/授权历史、v1/v2 回执及双工作空间共用页面。原四类资源身份、固定定义、字段规则、编号保留和直接保存方式延续。

前端采用资源 Provider 内层修订边界，而非重挂整个 ProjectBoundary：资源授权变化清理资源查询和草稿，仍有权访问的项目成员/角色页面继续保留。该细化落实方案 18.3 的分级清理要求，已回写 18.2。另新增租户 `project-usage` 分页接口支持设施真实使用项目列表，使用相同治理权限组合。

## 2 检查结果

| 检查 | 实际结果 |
| --- | --- |
| `pnpm check` | 通过：Go 格式/vet、ESLint、全部 TS 类型、OpenAPI 生成一致性、权限目录及实际 HTTP 路由覆盖。 |
| `pnpm build` | 通过：Go API/CLI 与 console 产物。Vite 仍提示公共主 chunk 超过 500 kB；生成器有依赖循环提示，无构建失败。 |
| `pnpm test:identity` | 通过：foundation、platform、API、CLI 全部模块的 `-race -count=1` 回归。最终通道候选归属过滤及租户结构变更/旧祖先历史投影另经定向 race 复验通过。 |
| `pnpm test:http` | 9 项全部通过：Docker/Nginx、普通 HTTP、base path、CSP 与原功能部署回归。 |
| 开发浏览器全量及复验 | 首轮 113 通过，2 条长流程在默认 30 秒总时限下超时，2 条生产 chunk 用例按设计跳过；保留断言和动作期限，将批量邀请/角色长流程总时限设为 90 秒后，资源及相关完整文件 28 项复验全部通过。 |
| `pnpm test:e2e:preview --timeout=90000` | 首轮 116 项通过，1 条首批登录流程因构建产物更新时启动预览而请求旧 chunk（404）失败。等待构建完成并固定产物后，access 与 project-resources 两个文件桌面/手机共 10 项全部复验通过，包含该失败用例；117 个全量用例均有通过证据。原断言期限保持，零自动重试。 |
| v9 → v10 备份恢复演练 | 通过：实际 pg_dump/pg_restore，15 张旧表数据核对，原账号登录、原资源读取、v1 回执重放；重复迁移/权限同步可用。 |
| 大集合查询计划 | 通过：2000 临时测点，明确授权 1000 项，单次查询返回 26 条（25+1），EXPLAIN ANALYZE 8.258 ms，临时数据全部回滚。该值是本机结构检查，不是生产容量承诺。 |
| 独立人工环境就绪自查 | 通过：六类真实身份登录，A/B 相同资源 ID 与精确通道范围、项目写入能力、无租户回退、平台/T2 隔离、按构件定位通道候选；实际桌面/手机截图复核无页面横向溢出。 |

路由检查发现的 usage 重复注册已通过明确 namespace 分流修复，并重新跑过完整 API race 与路由覆盖。浏览器首轮失败和复验如上保留，不把首次全量命令描述为一次全绿。

开发浏览器使用 API 48080、console 45173，避免与另一工作区的 18080/15173 冲突；配置支持 `INFRAPRO_E2E_API_URL` 与 `INFRAPRO_E2E_CONSOLE_URL`。测试依赖使用独立 schema/Redis 前缀。Docker 镜像元数据拉取曾因去掉本机代理而超时，恢复正常本机网络环境后重新执行；不是业务测试通过的替代证据。

## 3 自验场景到证据

以下列出方案 SV-01–SV-44 的运行证据及对应实现核对。业务规则经正式 service、HTTP 或页面入口验证；数据库直接写入仅用于准备较大集合、故障注入与非法约束负向测试。

| 场景 | 主要证据与观察 |
| --- | --- |
| SV-01–SV-04 同一身份、明确授权、祖先最小摘要、不继承 | `project_access_test.go` 的 `TestProjectResourceExplicitGrantsContextAndCreation`；A/B 相同 UUID，单通道上下文无属性/位置/定义/历史泄漏，未来新增不自动开放。浏览器真实 B 只读角色无租户资源权。 |
| SV-05–SV-08 角色、双身份、跨范围和目录 | `project_boundaries_test.go` 的双身份场景，项目角色与对象授权相交；同账号拥有租户权仍不能在无项目授权时回退读取。项目公共版本目录在实际新通道流程使用。 |
| SV-09–SV-12 创建政策与原子性 | 新建政策默认关闭、开启/关闭不撤销既有对象；项目创建来源与标准能力正确。分别在资源授权、授权历史、资源历史、收据写入注入失败，整个事务回滚。 |
| SV-13–SV-16 类型、分页、当前集合、缩权 | 130 点/65 明确可见点的跨页查询，候选与当前授权独立有界分页；1001 项当前集合明确拒绝；错误能力/跨设施目标拒绝；预览后新增返回 preview_changed；结束父读取保留必要上下文。 |
| SV-17 关联身份 | 撤销后重新关联采用新 link/grant 身份；旧范围不可读，当前原成员/关联检查约束新收据。 |
| SV-18 共享资料与修订 | 两项目预览同修订后，先写成功、后写冲突；共享修改须原因和确认，其他项目历史只得公共投影。 |
| SV-19–SV-20 结构 | 项目构件换父级拒绝；测点归属纠正需要目标 create_point 能力；租户结构调整后上下文按当前 ID 路径重算。 |
| SV-21–SV-25 生命周期与归档 | 当前及归档项目引用阻止项目生命周期/租户删除；独用且完整能力可操作；归档保留读、拒绝写和扩权；归档期撤权恢复后不复活。原资源生命周期与下级保护回归继续通过。 |
| SV-26–SV-27 两种提交次序 | `project_races_test.go` 用 Actor 会话复验屏障控制事务顺序；撤权/项目写入、归档/项目创建、租户删除/新增引用均验证先后两种结果，没有半提交或悬空引用。 |
| SV-28 上层资格 | 原账号启停、租户启停、成员暂停/结束/重加入的 service 和浏览器回归；项目资源每次使用同一 RequireProjectTx 资格链，新收据绑定两级成员 ID。 |
| SV-29–SV-32 查询、历史、来源及审计 | 授权在分页前过滤；历史隐藏其他项目身份/原因和当前不可见旧祖先；平台审计排除资源/授权对象；独立授权历史与最小项目审计分开；异范围游标拒绝。 |
| SV-33–SV-34 幂等与旧回执 | 相同意图重放保持同对象，不同指纹拒绝；原政策关闭后只读回执可核对，POST 重放仍需当前执行资格。v9 恢复后实际 v1 回执成功重放。 |
| SV-35 原规则 | 原资源 invariants、concurrency、history_definitions、浏览器及 Go/Zod 位置共同样例：固定定义、通道语义、零坐标、数量、编号和结构保护。 |
| SV-36–SV-38 数据和目录 | v10 空库/旧库/重复迁移、101 项目录及 AllowedOperations；组合外键、能力类型、不可变来源、当前唯一及关联生命周期负向检查；归档直接 service 写入拒绝。 |
| SV-39–SV-41 浏览器隔离和交互 | `project-resources.spec.ts` 真实租户授予→无租户权项目创建→共享编辑→503 保留输入→同角色撤权；现有账号/租户/项目迟到响应、双标签页、重加入、手机和键盘用例。资源树和选择按钮不隐式保存。 |
| SV-42 真实 HTTP/原功能 | Docker/Nginx 普通 HTTP、CSRF/Origin、256 KiB API 限制、反向代理 base path、CSP、原账号/租户/项目/工程资源回归。 |
| SV-43 恢复 | `scripts/verification/project-resource-migration.mjs` 使用真实基线源码、备份与全新恢复数据库，未用同库导出文件可读代替恢复。 |
| SV-44 查询规模 | 授权过滤在 SQL 分页前执行，可见祖先与定义集合批量解析、候选页授权集合一次查询；2000/1000 临时集合执行计划及 130/65 跨页行为验证。没有逐对象读取 API 的 N+1 调用。 |

新增核心测试：[授权与创建](../../packages/go/platform/resources/project_access_test.go)、[共享与边界](../../packages/go/platform/resources/project_boundaries_test.go)、[竞争次序](../../packages/go/platform/resources/project_races_test.go)、[项目页面闭环](../../tests/e2e/project-resources.spec.ts)。

## 4 恢复演练证据

[演练脚本](../../scripts/verification/project-resource-migration.mjs) 从 `4e92f95` 导出源码，在临时目录使用原 v9 业务接口生成账号、租户、项目、定义、四类资源和 v1 回执，执行 pg_dump。随后 pg_restore 到新数据库，使用当前代码升级 v10，并运行两次迁移和权限同步/检查。最后删除演练数据库和临时源码，保留本地报告。

`.artifacts/project-resource-migration/report.json` 的实际结果：

```json
{"from":9,"to":10,"restored":true,"preserved_tables":15,"original_login":true,"original_resource":true,"v1_receipt_replayed":true,"repeat_migrate_sync":true,"default_links":0,"default_grants":0,"default_create_policy":false}
```

备份与私有 fixture 位于忽略的本机 `.artifacts`，未提交凭据。此证据不代表原业务/生产环境已升级。

本机完整命令日志保存在 `.artifacts/project-resource-verification/`；验收环境只读自查报告为 `.artifacts/project-resource-acceptance/verification.local.json`，截图位于同目录 `screenshots/`。82 个相关本地文档链接和差异空白检查通过，私有凭据/备份/构建产物均未进入 Git 交付范围。

## 5 交付与未验证事项

专用手工环境保留 T/T2、G109/野外观测、F109/F214、P1/P2/C1/C2/TEMP 及六种独立身份。完整访问方式、受控凭据位置和全部人工步骤在操作手册。手工环境重新运行 setup 不清库，seed 完成后不重复准备或覆盖验收修改。

尚未由用户进行 MA-01–MA-26 人工验收，尚未部署生产或迁移原业务数据库，也未做生产规模容量、浏览器矩阵或跨租户共享验证。项目页面的权限变化依赖聚焦或 30 秒轮询复验，服务端限制从事务提交后生效，不宣称实时推送。v10 不支持破坏性 down，回退使用备份恢复。上述边界与确认方案一致。

## 6 合并回主工作树与临时目录清理

2026-11-11，按用户指令将资源重构提交 `7a5c859` 快进合并到主工作树 `/Users/hijacker/Documents/ChatGPT/InfraPro` 的 `feature/tenant-project-engineering`。原有账号、邀请及验收修改继续保留为未提交状态；42 个不重叠的修改/新增文件逐字节核对一致，重叠的 OpenAPI 按结构三方合并，API 类型由合并后的契约重新生成。

整合后 `pnpm check`、`pnpm build` 通过；账号、邀请、资源及项目资源的构建预览回归 19 项全部通过；六类验收身份的只读范围及桌面/手机页面自查通过。专用验收服务改从主目录运行，仍访问 `http://localhost:35173/`，配置、账号凭据、数据库及资源 ID 均保留；没有迁移原业务数据库或推送远端。

原临时工作树 `/Users/hijacker/.codex/worktrees/e49a/InfraPro` 已通过 `git worktree remove` 删除。原有修改恢复快照、临时目录的验收报告/截图副本和整合日志保存在主目录 `.artifacts/merge-project-resource-e49a-a2tr_2_t/`；本次临时 stash 已清理，原有 stash 不受影响。功能状态仍为 Awaiting acceptance。
