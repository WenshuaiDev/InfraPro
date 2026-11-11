# 租户成员管理验证与交付记录

日期：2026-11-10。工作区：`feature/account-access-console`。方案基线：`776663fc`。

- 方案：**Accepted**，用户确认按方案完成本轮，包含 D-01–D-12。
- 功能：**Awaiting acceptance**，开发与自验完成，待用户人工验收。
- 用户人工验收：**尚未进行**；自动自验不代替用户确认。
- 本轮工作尚未提交、推送或生产部署。

启动、入口及人工验收见[运行手册](../operations/tenant-membership.md)，完整范围见[已确认方案](../requirements/tenant-membership-plan.md)。

## 1 完成内容

TM-01–TM-10 均已实现：本租户成员列表/详情和历史、完整用户名定向定位、24 小时账号绑定邀请、本人明确确认、固定只读普通角色、暂停/恢复/移除、新关系再次加入、管理员交接保护及复用、邀请资格永久撤销、租户治理审计与隔离页面。OpenAPI、生成客户端、权限目录和实际路由同步。

数据库迁移 v4 保留原始 ID，允许一名管理员及多名普通成员；当前关系唯一、组合归属、固定角色和接受结果由约束保护。接受时成员、角色、邀请消费及两类成功审计同事务；锁后用数据库当前时间再次检查到期。令牌仅保存摘要；明文只在首次响应展示，不能通过重放请求重新获取。

## 2 实测结果

| 检查 | 结果 | 证据及范围 |
| --- | --- | --- |
| `pnpm check` | 通过 | gofmt、Go vet、ESLint、TypeScript、OpenAPI、权限目录及精确路由映射 |
| `pnpm build` | 通过 | Go 服务/CLI、console 生产构建 |
| `pnpm test:identity` | 通过 | 全部 Go module，真实 PostgreSQL/Redis，`-race -count=1`；成员包 202.556s |
| 凭据用途及跨租户绑定专项 | 通过 | 实际开户/重置凭据不能进入租户领取；数据库拒绝跨租户角色绑定，补充 Go `-race` 专项 7.176s |
| `pnpm test:e2e` 全量 | 49 通过、2 按配置跳过 | desktop / 375px mobile，全量账号、授权、租户和成员回归；tablet presentation |
| 成员/邀请草稿回归 | 4 通过 | 重新聚焦触发身份核验后，桌面和手机操作弹窗与原因保留；修复前的 2 项桌面失败已稳定复现并修复 |
| `pnpm test:e2e:preview` | 51 通过 | 最终构建全量回归（1.5m），包含开发模式跳过的 2 项构建块加载失败恢复 |
| `pnpm test:http` | 4 通过 | 最终代码实际 Nginx/Compose 普通 HTTP `/console/`、登录确认、Cookie/CSRF、备份恢复和旧凭据撤销（22.2s）；专用测试栈已清理 |
| 本机 v3→v4 升级 | 通过 | 原有 1 账号、1 密码记录、2 会话、3 平台绑定和历史内容保留；没有自动生成租户、成员或邀请 |
| 本机备份恢复与重复升级 | 通过 | 备份恢复至独立库，12 个业务表原始列指纹一致；重复 migrate/sync/check 无重复记录或审计 |
| 本机服务 | 通过 | `make dev`；readyz 返回 ok、console HTTP 200，既有会话访问 auth/me 200 |
| UI 视觉与可访问性 | 通过 | 桌面/375px 明暗主题截图复核，键盘确认/取消与焦点返回，667px 横屏、150% 根字号；测试均启用 reduced-motion |

共享 UI/布局包源码未修改，未额外运行 `test:ui`。现有 presentation 回归与成员页面专项覆盖使用场景。Node/Vite deprecation 和 TanStack generator 提示未造成检查失败。

自验中发现并修复：真实旧成员回填触发延迟事件、阻止后续 ALTER；同一路径连续打开不同 fragment 时保留上次邀请结果；路由上下文边界错误导致切换中短暂缺少租户上下文；桌面表格 cell 函数不稳定，身份复核时重挂载并丢失操作弹窗。最后一项先用重新聚焦的成员/邀请草稿测试稳定复现，再固定列定义及回调。对应升级与浏览器回归用于确认修复。

## 3 证据索引

Go 成员测试位于 `packages/go/platform/tenancy/membership_test.go`：

| 简称 | 用例 |
| --- | --- |
| L | `TestMembershipLifecycleAndMinimalAuthority`：完整生命周期、最小权限、多租户、历史分页、审计、交接复用 |
| I | `TestTenantInvitationIntentExpiryAndReplacement`：请求去重、到期、撤销、替换和终态保留 |
| V | `TestTenantInvitationRevocationCoupledToIdentity`：发起/目标停用、受控重置、交接、直接任命、统一撤销 |
| A | `TestTenantAcceptRollbackAndConstraints`：四类注入失败原子回滚和非法结构拒绝 |
| C | `TestTenantControlledRaceOrders`：接受与撤销/替换/交接/停用/直接任命/重复接受，实际锁队列控制两种先后 |
| E | `TestTenantExpiryAfterLockWait`：请求等待锁后跨过到期时间仍拒绝 |
| G | `TestMemberGovernanceRaceAndAccountAvailability`：暂停/移除与交接竞争两种顺序；账号状态不混淆关系状态 |
| R | `TestTenantInvitationRateLimitsAndFailClosed`：真实 Redis 限流及依赖不可用时无写入 |
| Q | `TestConcurrentInvitationCreationAndRecentPassword`：密码复验过期无写入、复验后同目标并发最多一邀请 |
| N | `TestNormalPasswordAndLogoutPreserveInvitation`：正常改密/退出保留邀请，已接受成员不因交接删除 |
| P | `TestInvitationPurposeAndCrossTenantBinding`：开户/重置凭据用途隔离、真实跨租户绑定拒绝 |

其余证据：

- **M**：`packages/go/platform/migrations/membership_upgrade_test.go`，真实 v3 原成员/已结束成员、角色绑定、凭据/会话/审计升级保持，重复同步无新增历史。
- **B1–B5**：`tests/e2e/tenant-membership.spec.ts`，依次为完整成员流程、HTTP 权限与严格字段矩阵、替换撤销/刷新、复验及故障草稿与迟到链接、重新加入旧关系迟到读取；均执行 desktop/mobile。
- **D**：`tests/http/tenant-membership.spec.ts`，真实 `/console/` 部署和停 API 后 pg_dump/pg_restore、CLI 统一撤销及重启。
- **F**：既有 `tenancy_test.go`、identity/access/HTTP/CLI、`tests/e2e/tenancy.spec.ts`、账号/授权/presentation 回归及 `tests/http/deployment.spec.ts`。
- **S**：静态交叉核查 API DTO/严格 JSON、路由策略、迁移约束、日志过滤、内存凭据及查询键；仅作为运行测试的补充，不代表单独运行过故障场景。

## 4 MA-01–MA-52 对照

下表为开发自验证据映射，**全部人工验收仍待用户执行/确认**。“运行 + 静态”表示组合证据，不能解释成每个浏览器分支都做过独立故障注入。

| 编号 | 证据 | 核验要点 |
| --- | --- | --- |
| MA-01 | I、B1 | 精确目标、固定角色、24h、一次性链接，pending 不是成员 |
| MA-02 | I、B1、S | 输入校验、大小写归一、无目录枚举，账号不可用统一响应 |
| MA-03 | B1、D | 匿名→登录→返回，必须再次确认 |
| MA-04 | L、B1、B2 | 错账号拒绝且无目标/租户预览 |
| MA-05 | L、B2、A | 成员、角色、消费与两类审计完整 |
| MA-06 | C、B2 | 重复/并发最多一关系；原接受结果可查 |
| MA-07 | I、C、B3 | 撤销后不可领取，原因和关闭人保留 |
| MA-08 | I、B3 | 旧链失效、新链限定本人、替换链和原终态保留 |
| MA-09 | I、E | 当前有效过期与锁后到期均拒绝 |
| MA-10 | I、B4、S | 创建/再生成幂等，不返回旧 token；丢响应先查记录 |
| MA-11 | I、Q | 同目标已有 pending 拒绝；并发只一份 |
| MA-12 | P、B2 | 开户/重置 token 不能领取；角色等未知字段拒绝 |
| MA-13 | L、B1、B2 | 最小资料 DTO、仅查看资料，成员/邀请/审计接口拒绝 |
| MA-14 | L、B1 | 暂停 A 后原会话新请求拒绝 A，B 不变 |
| MA-15 | L、B1 | 恢复保留同一成员 ID 和角色 |
| MA-16 | L | 暂停成员不能再邀请，须走恢复 |
| MA-17 | L、G | active/suspended 可移除，结束后不能 resume |
| MA-18 | L、B1、B5 | 再次加入使用新 ID，旧角色撤销 |
| MA-19 | L | 旧接受结果只读，不新建/恢复旧关系 |
| MA-20 | L、I、S | 版本及状态冲突不做成功写入，页面要求刷新核对 |
| MA-21 | G | 禁用账号关系可治理，不能恢复不可用账号成员 |
| MA-22 | G、V、F | 账号启用不修改 suspended/ended，旧会话失效 |
| MA-23 | L、B1 | 搜索、分页、历史关系分开且限制当前租户 |
| MA-24 | L、B2 | 纯平台身份不能走租户成员/邀请/资料入口 |
| MA-25 | L、B1、B2 | 当前管理员 UI/API 保护 |
| MA-26 | L、B2 | 复用普通成员关系，保留加入时间，旧角色撤销并升任 |
| MA-27 | G、F、S | 暂停接任拒绝；ended/无关系新建；候选预览区别提示 |
| MA-28 | V、C、I | 失权邀请永久终止，再生成使用当前发起来源 |
| MA-29 | V、N | 账号停用/受控重置永久失效，常规改密/退出不误撤销 |
| MA-30 | V、C | 直接任命目标终止未领取普通邀请，不重复关系 |
| MA-31 | V、G、F、D | 常规管理员停用保护、应急停用、恢复/交接和可用性审计 |
| MA-32 | N、C | 已接受成员在发起人退出/交接后仍保持 |
| MA-33 | L、B2、P | 路径归属、额外正文拒绝、组合 FK 隔离 |
| MA-34 | L | 去掉所有平台角色，仍能本人确认并以普通成员进入 |
| MA-35 | C | 每类竞争都控制两边先后提交，不依赖随机竞争 |
| MA-36 | G | 暂停/移除与升任两顺序均保持唯一有效管理员 |
| MA-37 | Q、B2、B4、F | CSRF/Origin、真实复验过期拒绝，复验后显式再提交 |
| MA-38 | B2、A、S | 严格字段、固定动作、目标版本及管理员保护 |
| MA-39 | A | 成员/绑定/邀请/审计失败全部回滚，授权修订也不变 |
| MA-40 | A、P、F | 重复当前关系、暂停管理员、第二管理员、跨租户绑定拒绝 |
| MA-41 | M、本机升级 | v3 老数据真实升级、原 ID 和历史保留，重复同步无扩增 |
| MA-42 | V、D | 实际恢复备份后统一撤销，旧会话/邀请失效、关系不变 |
| MA-43 | B4、F | 双租户/双标签及迟到读写，A 结果不污染 B |
| MA-44 | B5 | 旧成员请求延迟，重新加入后仅显示新成员 ID/资料 |
| MA-45 | B1、B3、F、S | 首次匿名登录保留凭据，刷新/身份变化清理；错账号无预览 |
| MA-46 | B1、B4、B5、F、S | 服务端新请求立即拒绝，焦点复核；30 秒可见轮询配置 |
| MA-47 | R、B1、B3、B4、F、S | Redis 限流/失败关闭、DB 故障回归、成员/邀请操作弹窗复核后草稿保留；结果未知不自动重放 |
| MA-48 | B1、B2、D、S | DTO/持久存储/URL/日志/审计不暴露明文或摘要，敏感测试关闭 trace |
| MA-49 | B1、B3、F | 桌面/375px、667px 横屏及 150% 根字号、键盘确认/取消焦点、卡片无水平滚动 |
| MA-50 | L、I、B2、F | 来源、前后状态、事件筛选和平台/租户审计边界 |
| MA-51 | D | 实际普通 HTTP `/console/` 生成链接、fragment、登录返回和接受 |
| MA-52 | F、全部回归 | 开户/角色/建档/编辑/编号保留/旧管理员交接和启停保持 |

## 5 运行产物与限制

日志在 `.artifacts/tenant-membership-{check,build,go,purpose,isolation,preview,http,draft-red,draft-green}.log`；升级、备份和本机启动产物在 `.artifacts/tenant-membership/`。Playwright 报告在 `playwright-report/e2e/`，复核后的明暗主题及放大文字截图保存在 `.artifacts/tenant-membership/screenshots/`。这些路径均 gitignored；含本机数据的备份和行指纹文件限制为 0600，不提交版本库。

本机原库当前无租户，旧成员迁移验证使用 M 的真实 v3 测试数据；不能把本机零租户升级单独当成旧成员迁移证据。HTTP 恢复使用专用可丢弃 Compose，未在用户正式本机库撤销会话或邀请。

尚未进行用户人工验收、生产部署、生产规模容量/吞吐、长期稳定性、异机灾备、完整屏幕阅读器及所有浏览器兼容性测试。没有已知未处理的自验失败；授权变化仍使用全局锁串行化。具体人工步骤和恢复边界见运行手册。
