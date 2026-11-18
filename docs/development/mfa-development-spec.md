# 平台 MFA 开发规格

版本：1.0。日期：2026-11-17。

业务范围已经确认；本规格落实“登录后敏感操作不再要求 TOTP”的修订。当前实现状态为 Awaiting acceptance（R1–R6 已实现，MFA-001–060 自验及交付复核通过，待用户人工接受）。实际实现、自验和人工接受分别记录在[验证账本](../verification/mfa.md)，部署与恢复见[操作手册](../operations/mfa.md)。本节以下保留开发时确认的行为约束。

本规格是本次 MFA 开发、集成和验收的依据；[功能方案](mfa-proposal.md)作为范围摘要。交付范围为平台策略、个人 MFA 管理、登录与会话、恢复与重置、安全存储与审计五项，包括使这五项在现有系统中完整工作所需的迁移、客户端、部署和测试。

## 1 已确认的业务规则

### 1.1 功能范围

| 需求 | 必须交付的行为 |
| --- | --- |
| R1 平台策略 | 平台通过一个默认关闭的开关控制是否强制所有人员账号使用 MFA；有明确平台权限的管理员可以修改 |
| R2 个人管理 | 不强制时用户可以自主启用或关闭；强制时未绑定者必须绑定，已绑定者不能关闭；支持更换验证器、重新生成恢复码 |
| R3 登录与会话 | 密码加 TOTP 或恢复码完成登录；未完成者没有业务会话；全局策略变化约束旧会话及所有人员入口 |
| R4 恢复与重置 | 提供一次性恢复码、平台管理员受控重置、唯一管理员的维护恢复；重置后必须重新绑定 |
| R5 安全与审计 | 密钥加密、一次性凭证、限流、防重放、故障拒绝、操作审计、个人安全通知及部署恢复规则完整接入 |
| R6 登录后的体验约束 | 正式登录后，任何敏感操作均不得因本功能要求再次输入 TOTP 或恢复码；保留既有密码复验 |

全局账号 `accounts.id` 是 MFA 的唯一归属。平台管理员、租户管理员、普通用户及未加入租户的账号执行相同规则；同一账号属于多个租户时不重复绑定。租户和项目没有覆盖开关，也没有人员白名单。

首期采用密码加 TOTP；一个账号同时只有一个有效验证器配置，配套 10 个一次性恢复码。短信、邮件验证码、Passkey、多个独立因素及可信设备免验证不在本期范围。机器凭证沿用原有机制；人员大屏和开发授权不得借机器凭证路径绕过人员认证。独立诊断维护账号不属于平台租户账号体系，其认证不受本开关控制。

### 1.2 登录决策表

| 账号状态 | MFA 状态 | 平台强制 | 密码通过后的下一步 |
| --- | --- | --- | --- |
| disabled | 任意 | 任意 | 拒绝，不能创建挑战或正式会话 |
| active | disabled | false | 创建仅密码的正式会话 |
| active | disabled | true | 进入受限绑定；新验证器确认成功后才创建正式会话 |
| active | enabled | 任意 | 验证 TOTP 或一个恢复码，成功后创建正式会话 |
| active | reset_required | 任意 | 验证专用恢复凭证并重新绑定，完成前不能创建正式会话 |

平台开关由 true 改为 false，只解除强制，不删除用户配置。原来因强制要求而启用的 MFA 仍保持 enabled，用户可以自行关闭。账号恢复、邀请开户、加入新租户和密码恢复均执行当前规则；修改或恢复密码不能清除 MFA。

### 1.3 哪些位置允许输入动态码

| 场景 | 现有密码复验 | TOTP 或恢复码规则 |
| --- | --- | --- |
| 已启用账号的新登录 | 先验证登录密码 | TOTP 或恢复码二选一 |
| 强制模式下未绑定账号的新登录 | 已通过登录密码 | 只校验新验证器动态码，证明绑定成功 |
| 登录后首次启用 | 复用最近 5 分钟的密码复验 | 只校验新验证器动态码，证明绑定成功 |
| 登录后更换验证器 | 复用最近 5 分钟的密码复验 | 不要求旧因素；只校验新验证器动态码 |
| 登录后关闭 MFA | 复用最近 5 分钟的密码复验 | 不要求 TOTP，不要求恢复码 |
| 登录后重建恢复码 | 复用最近 5 分钟的密码复验 | 不要求 TOTP，不要求恢复码 |
| 修改平台开关 | 复用最近 5 分钟的密码复验 | 不要求 TOTP，不要求操作者预先绑定 |
| 管理员重置他人 MFA | 复用最近 5 分钟的密码复验 | 不要求操作者 TOTP，不要求目标用户 TOTP |
| 角色、租户、项目、密码及其他既有操作 | 原有规则保持 | 不增加 TOTP 字段、弹窗或第二因素复验 |

绑定确认中的 `new_totp_code` 仅验证新配置能否工作，不是敏感操作的二次认证。不新增 `mfa_reauthenticated_at`、MFA step-up Interface、敏感操作因素新鲜度或“管理员必须先启用 MFA 才能操作”的限制。

未绑定管理员可以开启强制。保存成功后，其仅密码会话与其他人的同类会话一起失效，再按正常登录规则完成绑定；不能在保存开关前偷偷恢复 TOTP 前置要求。平台策略使会话失效，与登录后的操作复验是两个不同流程。

## 2 当前系统基线与改动地图

以下为实施前的系统基线及改动地图；已完成的实际实现以验证账本和当前代码为准。保留其他正在进行的修改，默认在当前分支和 checkout 工作。

| 位置 | 当前行为 | 必须如何衔接 |
| --- | --- | --- |
| [identity/service.go](../../packages/go/platform/identity/service.go) `Login` | 密码通过后立即拿到会话，再写 Redis；缓存写失败会撤销未交付会话 | 返回带状态的登录结果；密码、挑战、正式会话分开；保留限流、密码工作槽和失败撤销逻辑 |
| [identity/repository_sessions.go](../../packages/go/platform/identity/repository_sessions.go) `Login`、`LoadSession`、`CheckSession` | 持久会话检查账号状态、auth_version、撤销和到期；缓存命中也查询数据库 | 拆出统一会话创建；数据库检查增加 MFA 状态和强制代次，不能只修改缓存模型 |
| [identity/repository.go](../../packages/go/platform/identity/repository.go) `lockPrincipal`、`insertAccount` | 事务内复验身份；recent=true 检查最近 5 分钟密码复验 | 增加 MFA 会话有效性，保留 recent 的密码语义；开户同时创建 MFA 状态行 |
| [identity/access.go](../../packages/go/platform/identity/access.go) `AccessActor`、`AuthContext`、`CheckDisplaySessionTx` | 业务操作通过 Actor.Check 复验；人员大屏有独立的数据库会话查询 | 三条路径必须执行相同会话规则，防止 HTTP 门禁之外漏检 |
| [runtime/business.go](../../packages/go/platform/runtime/business.go) | 用 `WithPersonCheck` 装配人员大屏检查 | 保持这条装配链，确保更新后的统一规则实际被运行壳读取和续租使用 |
| [HTTP identity handler](../../services/api/internal/transport/http/identity/handler.go)、[router.go](../../services/api/internal/transport/http/router.go) | 登录返回 Account；统一路由按 permission/authenticated/credential/public 分流；已有 Origin、CSRF、no-store | 返回 LoginResult，增加有限挑战入口；credential 路由必须自行校验挑战，不得因绕开正式 Guard 就放行 |
| [access/policy.go](../../packages/go/platform/access/policy.go)、[catalog.go](../../packages/go/platform/access/catalog.go)、[builtins.go](../../packages/go/platform/access/builtins.go) | 操作、权限、内置角色和目录校验统一维护 | 登记新增操作与三个平台权限；同步内置管理员及最后管理账号保护；不能只写前端权限常量 |
| [identity/repository_maintenance.go](../../packages/go/platform/identity/repository_maintenance.go) | 密码重置、停用、恢复撤销、定时清理 | 撤销相应挑战和恢复授权，保留因素配置；恢复场景另按第 14 节处理 |
| [identity/notification_receipts.go](../../packages/go/platform/identity/notification_receipts.go)、[auditrecord/catalog.go](../../packages/go/platform/auditrecord/catalog.go) | 安全变更可在事务内写审计与 `security.changed` 通知收据 | 新事件进入白名单和中文名称映射；复用既有通知投递机制 |
| [api-client/index.ts](../../packages/ts/api-client/src/index.ts) | `login` 返回 Account；只对 `unauthenticated` 广播退出；命令 helper 只用于不含秘密的载荷 | 增加登录判别类型及 MFA 错误处理；含 OTP 的请求不得进入基于请求体构造命令 key 的 helper |
| [api-client/identity.tsx](../../packages/ts/api-client/src/identity.tsx) | phase 为 loading/authenticated/anonymous/unavailable；失权时清空查询和回调 | 增加待认证状态；202 不执行 signedIn；刷新先判定正式会话，再恢复受限挑战 |
| [登录页](../../apps/console/src/features/auth/login-page.tsx)、[账号安全页](../../apps/console/src/features/account/security-page.tsx)、[认证路由](../../apps/console/src/routes/_authenticated/route.tsx) | 单步登录；账号安全管理密码、会话和记录；路由只显式处理 anonymous | 新增登录步骤和 MFA 卡片，待认证状态不得渲染业务工作区 |
| [密码复验弹窗](../../apps/console/src/features/access/mutation-dialog.tsx)、[管理操作表单](../../apps/console/src/features/access/role-mutation-form.tsx) | `reauthentication_required` 后确认密码，再由用户重新提交 | 保留这套交互，不添加 TOTP，不在复验成功后自动重发敏感写入 |
| [凭证提取](../../apps/console/src/features/auth/credential.ts) | fragment 分别保存 invitation/reset/tenant；其他页面默认按 invitation 处理 | 显式增加 mfa-recovery 用途，禁止新恢复链接被误判为开户邀请 |
| [CLI 人员登录](../../tools/infrapro/internal/command/derived_client.go) | 任意成功登录响应后直接写会话文件 | 解析状态并完成 MFA；只有正式登录成功才创建 0600 会话文件 |
| [CLI 账号维护](../../tools/infrapro/internal/command/account.go)、[运行配置](../../packages/go/platform/config/config.go)、[迁移](../../packages/go/platform/migrations/) | 已有账号恢复、配置来源和 Goose 迁移 | 增加受控 MFA 恢复、密钥文件配置和独立增量迁移 |

现有默认会话空闲期为 2 小时、绝对有效期为 12 小时、缓存 TTL 为 5 分钟，均可由部署配置改变。本功能沿用实际配置，不把每 30 秒变化的 TOTP 或 5 分钟挑战期限当作正式会话期限。

## 3 实现组织与技术

identity Module 拥有策略、因素、挑战、恢复和会话的全部认证规则。保留现有 Service/Repository 分工；建议增加 `mfa.go`、`mfa_crypto.go`、`repository_mfa.go`、`repository_challenges.go`、`security_policy.go`、`repository_security_policy.go` 等文件，不把新逻辑继续堆入单个登录函数。

HTTP Adapter 只负责输入解析、Cookie、响应及传输元数据；CLI 和 HTTP 调用同一 Module。access Module 维护权限和操作目录，不反向导入 identity；通过现有 Actor.Check 接口复验身份。前端只展示后端下一步，不自行决定 MFA 是否可跳过。

内部 Interface 应覆盖 BeginLogin、CompleteLogin、Start/ConfirmEnrollment、DisableMFA、RegenerateRecoveryCodes、Get/UpdatePolicy、ResetMFA、ClaimRecovery 和 SessionValidity。命名可以依项目风格调整，但数据库事务只能由拥有操作的 Repository 提交一次。时钟、随机数及密钥读取提供内部可替换 Seam，便于验证时间窗、并发和故障；不增加通用认证插件框架。

| 技术 | 使用位置与理由 |
| --- | --- |
| Go、Gin、pgx、Goose | 复用现有认证、HTTP、数据库事务和迁移体系 |
| `github.com/pquerna/otp` | TOTP 计算与本地二维码生成；锁定经过依赖检查的版本，包装为内部 Interface，不自行发明认证算法 |
| Go crypto/rand、AES-256-GCM、SHA-256、crypto/subtle | 随机凭据、密钥加密、高熵令牌摘要及恒定时间比较 |
| PostgreSQL | 平台策略、因素状态、挑战和一次性消费的持久真相 |
| Redis | 既有会话缓存、活动合并、共享短期限流；不保存唯一的启用状态或消费结果 |
| React、TypeScript、现有 UI、TanStack Query、React Hook Form、Zod | 复用主题、路由、身份状态、查询和表单，不另建 UI 体系 |
| OpenAPI 与生成客户端 | 一处定义请求、响应、错误和判别状态，统一控制台和 CLI 的行为 |
| Go 测试、Playwright | 验证真实事务、时间边界、跨实例并发、HTTP/HTTPS 和人员使用路径 |

算法依据见 [RFC 6238](https://www.rfc-editor.org/info/rfc6238/)，候选库能力见[官方仓库](https://github.com/pquerna/otp)。TOTP 不具备抗实时钓鱼能力；本规格不把它描述为硬件密钥等级的认证。

## 4 状态与安全不变量

### 4.1 三种状态分别表达

平台策略只有 `require_mfa` 一个业务开关，另有并发版本和失效代次。账号 MFA 状态为 disabled、enabled、reset_required。登录挑战是短时凭据，绝不能放在正式 `account_sessions` 中用普通会话再加前端限制来模拟。

| 状态迁移 | 发生条件 | 对其他资格的影响 |
| --- | --- | --- |
| disabled → enabled | 新验证器动态码通过 | 认证版本和因素版本递增，旧会话及挑战撤销，产生新恢复码 |
| enabled → enabled | 新验证器确认成功并替换 | 版本递增，旧密钥、恢复码、会话及挑战失效 |
| enabled → disabled | 不强制且密码复验有效 | 版本递增，删除有效密钥和恢复码，撤销旧会话 |
| enabled/reset_required → reset_required | 管理员受控重置或重新发放恢复凭证 | 版本递增，旧因素、一次性凭证、会话及挑战失效 |
| reset_required → enabled | 密码及专用恢复凭证通过，再确认新验证器 | 版本递增，恢复资格结束，新恢复码及正式会话生效 |
| enabled → enabled | 仅重建恢复码 | 仅码组版本递增；旧码全部失效，已有正式会话保留 |

临时绑定不是账号状态，保存在挑战中。开始绑定、扫码、取消或输错新码均不能提前把账号标记为 enabled。更换失败时原验证器保持有效。

### 4.2 必须维持的不变量

1. 只有密码及当前规则要求的后续步骤全部完成，才能签发正式会话并记录登录成功。
2. MFA 证明只由服务端写入；客户端不能声明认证完成、指定目标账号或提升会话认证方式。
3. 同一因素的同一 TOTP 时间步、同一恢复码、同一成功挑战分别最多消费一次。
4. 认证提交时重新核对账号状态、auth_version、factor_version、平台策略和期限；缓存不能替代核对。
5. 策略读取失败、策略行或应有账号状态行缺失均返回不可用，不能默认按未强制、未启用处理。
6. 正式会话满足规则后，操作是否敏感不再触发 TOTP；密码复验仍只验证密码。
7. MFA 重置不改变账号 active/disabled 状态、角色、租户成员关系或平台开关。
8. 业务权限始终来自现有授权图。通过 MFA 不等于获得任何租户、项目或管理权限。

## 5 数据结构与迁移

所有时间存为 timestamptz，期限以服务端时间判断；浏览器倒计时仅展示。UUID 使用现有生成方式。对外暴露的 bigint 版本使用十进制字符串，避免 TypeScript 数值精度问题。下列为必需语义，DDL 应作为新的 Goose 迁移提交，不改已发布迁移；当前目录已有 00055，实施时取实际可用的下一个编号。

### 5.1 platform_security_policy

| 字段 | 类型及约束 | 说明 |
| --- | --- | --- |
| id | smallint 主键，CHECK id=1 | 全平台单例，迁移创建 |
| require_mfa | boolean NOT NULL DEFAULT false | 唯一业务开关 |
| revision | bigint NOT NULL DEFAULT 1，正数 | 真实策略变化递增；用于 expected_version |
| enforcement_generation | bigint NOT NULL DEFAULT 0，非负 | 仅 false → true 时递增，不回退 |
| updated_by | uuid 可空，引用 accounts | 初始迁移为空；正常修改记录操作者 |
| updated_at | timestamptz NOT NULL | 修改时间 |

初始状态只由迁移写入，运行时不得每次启动重置。重复保存相同值返回 changed=false，不增加版本或生成虚假的变更事实。

### 5.2 account_mfa

| 字段 | 类型及约束 | 说明 |
| --- | --- | --- |
| account_id | uuid 主键，引用 accounts | 每个全局账号一行 |
| state | text，disabled/enabled/reset_required | 新账号默认 disabled |
| factor_version | bigint NOT NULL DEFAULT 1 | 因素配置变化或管理员重置递增 |
| secret_ciphertext、secret_nonce、key_id | bytea、bytea、text，可空 | enabled 必须完整，其他状态必须为空 |
| last_used_step | bigint 可空 | 当前因素最后成功消费的时间步，不能因重启清零 |
| recovery_generation | bigint NOT NULL DEFAULT 0 | 每次生成新码组递增 |
| enabled_at、updated_at | timestamptz | 当前绑定生效时间及状态修改时间 |

迁移为所有现有账号回填 disabled 行，不生成 TOTP 密钥；`insertAccount` 为以后开户在同一事务创建状态行。state=enabled 时必须有启用时间及密文；disabled/reset_required 不保留可继续验证的旧密钥。因素版本变化时才重置时间步；单纯加密密钥轮换不改变因素版本。

### 5.3 account_mfa_recovery_codes

字段为 id、account_id、factor_version、generation、code_hash、created_at、used_at、revoked_at。id 为 UUID，code_hash 唯一；按 account_id/generation 建索引。消费使用带 `used_at IS NULL AND revoked_at IS NULL` 的条件更新并核对当前因素及码组版本，受影响行数必须为 1。

每组 10 个，每码 16 字节密码学随机数据，采用大写 Base32 无填充编码并分组显示。校验允许去除分隔短横线及首尾空白，大小写归一；不把不明确字符随意转换为另一字符。摘要输入包含固定用途、account_id、generation 和规范化后的完整码。高随机性使摘要不依赖低熵口令的存储方案。

### 5.4 account_auth_challenges

| 字段组 | 必须包含的内容 |
| --- | --- |
| 身份 | id、account_id；需要持有凭证的用途保存唯一 token_hash，不存令牌明文 |
| 用途 | login_verify、login_enroll、login_recover、self_enroll、self_replace、admin_reset_grant |
| 生命周期 | pending/completed/revoked、created_at、expires_at、completed_at/revoked_at、failed_attempts |
| 版本 | account_auth_version、factor_version；记录创建时策略版本用于诊断，执行时仍读取当前策略 |
| 关联 | origin_session_id 仅用于登录后自助操作；recovery_grant_id 用于受控恢复；issuer_account_id 用于管理员发放 |
| 验证依据 | password_verified_at；待绑定密钥的密文、nonce、key_id；登录后操作仍要检查所属正式会话 |

登录挑战只允许一个浏览器当前使用一个 Cookie 凭据，但一个账号可在不同浏览器同时进行独立登录。创建新挑战不能清空账号级 MFA 失败计数。自助绑定同一账号最多一个未撤销的 enrollment/replace 流程；新建时撤销旧流程并清理临时密钥。admin_reset_grant 每账号最多一个未消费且未撤销的授权。

不使用 `now()` 的部分唯一索引表示“尚未到期”；对 pending 状态建立约束，并在新建事务中先撤销过期或被替代行。建立到期索引及 account_id/state 索引，正式因素和临时密文的 key_id 也应可索引查询。默认每分钟执行一次维护、每批最多 500 行；已到期挑战先撤销并清除临时密钥，终态满 24 小时后可清除无审计依赖的挑战元数据。成功或主动撤销时立即清除临时密钥；不依赖定时删除才阻止过期使用。安全事件由审计保留策略处理，不随挑战清理删除。

### 5.5 扩展会话与失败计数

`account_sessions` 增加 authentication_method（password/password_totp/password_recovery_code）、mfa_verified_at、mfa_factor_version、policy_enforcement_generation。旧会话回填 password、空 MFA 证明和代次 0。密码会话的 MFA 字段必须为空；其他两种必须完整。现有 auth_version、撤销、空闲及绝对期限仍有效。

`Principal`、会话数据库扫描、Redis 序列化与所有会话创建路径同步调整。旧缓存缺少字段时按仅密码处理，并由数据库补全当前状态，不能将缺省值解释为已验证。公开会话列表只增加认证方式标签，不暴露内部秘密。

复用 `account_rate_limits` 建立独立的 MFA key 范围，保存跨挑战、跨重启的失败数及冷却时间。密码成功只清密码计数；不能顺带清除 MFA 失败。

新表不向内容进程、任务 worker、审计导出 worker 或 PUBLIC 授予读取密钥和恢复凭证的权限；按当前部署角色模式仅给身份请求及受控维护链所需权限，并用真实受限角色验证。

## 6 认证流程与事务

### 6.1 BeginLogin

保留用户名规范化、等价的不存在账号密码校验、密码失败冷却和工作槽。先完成昂贵密码校验，随后在事务中复验同一密码摘要及 auth_version，避免长时间持有平台策略锁；密码变化、账号停用或版本变化即拒绝。

按第 1.2 节决策。无需 MFA 时创建正式会话；其余只创建挑战，返回下一步，绝不插入临时正式会话。login_verify/login_recover 默认 5 分钟到期；login_enroll 最多自 password_verified_at 起 10 分钟。开始扫码、刷新或再次读取状态不延长期限。

新的密码登录撤销当前浏览器旧挑战；若已有完整正式会话，要求先退出或显式切换账号，不混用两个账号的正式会话与挑战。用户名或密码失败保持现有不区分账号不存在、密码错误与账号不可用的消息。

### 6.2 CompleteLogin

挑战 Cookie、expected_challenge_id、用途、期限、账号及因素版本全部匹配后，读取当前因素。TOTP 按匹配时间步执行原子消费；恢复码按当前码组执行原子消费。消费、挑战完成、正式会话写入和成功审计必须在同一事务完成。

密码通过但验证码错误只记录认证失败，不能记录登录成功。错误计数应以“事务提交后返回业务错误”的方式持久化，不能在 return error 回滚时一并丢失。成功仅清除对应 MFA 失败计数，不撤销其他浏览器的合法会话。

正式会话生成新的随机令牌，清除挑战 Cookie。缓存写失败沿用当前实现，撤销尚未交付的会话后返回 503。若消费已经提交而客户端没有拿到 Cookie，不能凭已消费挑战再生成第二个会话；客户端先查询 me，未登录则重新开始登录。

### 6.3 绑定与更换

登录前绑定由有效的 login_enroll 挑战授权；登录后绑定/更换由正式会话、所属 enrollment ID 和最近 5 分钟密码复验授权。初次绑定只允许 disabled，更换只允许 enabled。

开始时生成新密钥并加密存入临时挑战。二维码与手工密钥只可由同一资格在挑战未过期时读取；刷新恢复原配置，不自动生成另一把密钥。确认时输入 `new_totp_code`，校验它来自新密钥，并在事务内重新检查资格。

确认成功，原子完成因素切换、factor_version 和 auth_version 递增、旧恢复码及所有旧挑战撤销、旧会话及缓存失效队列写入、新恢复码、审计及安全通知收据。临时密钥解密后按正式账号因素的用途和新版本重新加密，使用新 nonce，不能直接复制绑定挑战的密文。正在完成的挑战被标记 completed，不能被通用撤销循环误改为可重试。新因素的 last_used_step 记录本次绑定验证步。

强制登录绑定成功创建新正式会话。登录后绑定或更换成功轮换当前正式会话，认证方式为 password_totp，其他设备失效；新会话绝对到期时间不超过旧会话剩余绝对期限。`reauthenticated_at` 使用实际最近密码验证时间，不把动态码通过时间冒充新的密码复验时间。

服务端一次性返回新恢复码明文；页面在进入业务区前展示保存步骤。确认“已保存”只是交互确认，不新增可绕过 MFA 的授权步骤。响应丢失时，用户可重新登录后用密码复验重建恢复码，不要求再次输入 TOTP 作为重建条件。

### 6.4 关闭和重建恢复码

关闭要求当前状态 enabled、平台不强制、预期因素版本一致、密码复验有效。事务中清除有效因素、撤销恢复码及挑战、递增版本并撤销旧会话，再轮换当前浏览器为 password 会话。不得要求旧动态码或恢复码。平台在提交期间变为强制时返回 409，不允许先成功关闭再异步修正。

重建恢复码要求 enabled 和密码复验有效；每次成功递增 recovery_generation、作废旧码组并产生新码。它不改变 factor_version、正式会话或平台策略。返回明文只发生一次；并发请求按预期 generation 与命令 ID 避免重复生成。恢复码消费与重建使用相同账号及因素锁，结果只能是先消费再作废旧组，或旧码因新组已生效被拒绝。

### 6.5 管理员重置和恢复凭证认领

重置要求目标账号存在且 MFA 为 enabled/reset_required、操作者有账号查看与 MFA 重置权限、最近密码复验有效、目标因素版本匹配、原因 1–500 字符。可对已停用账号重置，但不能自动启用目标账号。普通管理入口不得重置本人，返回 self_mfa_reset_forbidden；已登录本人可以直接更换验证器。

重置事务递增目标 auth_version/factor_version，设置 reset_required，删除旧密钥，作废恢复码、旧挑战及旧恢复授权，撤销全部会话，生成 30 分钟有效的 admin_reset_grant，只返回一次恢复链接。目标账号状态、角色和租户关系保留。操作者不需要 TOTP，也不需要先启用自己的 MFA。

用户取得恢复链接后先验证密码，得到 login_recover 挑战，再提交专用恢复凭证。服务端校验账号及版本一致，原子消费恢复授权并将当前挑战转入 login_enroll；新到期时间不能晚于 password_verified_at+10 分钟或恢复授权原到期时间，并同步更新挑战 Cookie 的期限。随后刷新不能继续延长期限，按新验证器绑定流程完成恢复。

认领后刷新可以继续当前挑战；若认领后的挑战丢失或过期，已经消费的恢复凭证不重新生效，需要管理员重新发放。密码也丢失时先完成现有密码恢复，再发放 MFA 恢复凭证；不得把密码重置链接当作 MFA 恢复凭证。

### 6.6 敏感响应丢失与重放

普通策略修改使用既有命令收据约定。identity 所有的事务直接使用 auditrecord 的 BeginCommandTx/CompleteCommandTx 等公共 Interface；access 中 mutationCommand 等私有封装保持私有，不为此暴露接收任意请求体的日志接口。带 OTP 的认证提交不把验证码放入通用命令 payload 或前端 pendingCommands 的 key；使用一次性挑战与安全 operation_id 对照结果。含秘密输出的重建恢复码、管理员重置等操作只在首次执行成功时返回秘密，收据只存对象、状态及版本。

同一命令 ID 的重放不得再次产生新恢复码或重置凭证，返回已完成的安全状态和 `secret_unavailable=true`。用户核对结果后，可明确发起新命令重新生成，旧秘密因此失效。未确认提交结果时保留原命令 ID；不得自动换 ID 重做。数据库提交结果不明沿用现有 unknown/收据核查机制，不能宣称失败后直接重执行业务。

## 7 平台策略与统一会话检查

### 7.1 策略修改

策略 GET 返回 require_mfa、revision、修改人和时间。影响统计按全局账号去重，给出活跃账号、已绑定、未绑定及待恢复数量；不能把一个账号的多租户成员关系当成多个人。会话数返回数据库中未撤销且未绝对到期的仅密码会话上限 `potential_password_sessions`，可能包含空闲过期会话，页面明确标注“可能受影响会话”，不宣称在线人数。统计返回 computed_at，只作预览，不是保存授权依据，也不为计数遍历所有 Redis 活动键。

保存要求 read+update 权限、expected_version、变更原因和现有密码复验。false→true 时 revision 和 enforcement_generation 同时递增；true→false 只递增 revision。保存时再次核对权限和当前状态，不能靠页面已显示开关取得修改资格。

保存成功响应包含 `session_action: keep | sign_in_again`。未完成 MFA 的操作者开启强制时返回成功与 sign_in_again，清除其正式 Cookie；前端先告知“设置已生效，请重新登录完成 MFA”，再进入登录，不得把随后的 401 当成开关保存失败。

### 7.2 唯一会话有效性规则

所有人员会话路径执行下列判断，MFA 部分应封装为同一内部 Interface 及同一组契约测试：

```text
先满足现有账号 active、会话未撤销、auth_version 一致、空闲和绝对期限有效
读取当前平台策略及 account_mfa
若 reset_required：拒绝正式会话
若账号 enabled 或平台 require_mfa：
    必须有服务端 MFA 完成证明，且 mfa_factor_version 等于当前 factor_version
若会话没有 MFA 证明且签发代次小于当前 enforcement_generation：拒绝
其余允许进入原有业务权限检查
```

只有 MFA 登录或新验证器绑定成功才写 MFA 证明；恢复码登录证明记为 password_recovery_code。密码复验不能把 password 会话升级为 MFA 会话。proof 没有“5 分钟后敏感操作必须重输动态码”的期限，它随正式会话及因素版本失效。

开启后旧仅密码会话在下一次普通受保护请求时被拒绝，即使 Redis 命中也如此。关闭强制后旧代次不会倒退，旧失效会话不复活；新密码登录可以使用当前代次重新建立会话。已经通过有效 MFA 的会话无需因开关变化重新验证。

### 7.3 事务顺序和其他入口

统一顺序为授权状态锁 → 安全策略行锁 → 账号按 UUID 排序 → 会话及 MFA 状态 → 挑战/恢复码。普通受保护事务读策略用共享锁；修改策略一开始就取排他锁，不能先共享再升级。策略检查应使用事务记忆避免重复查询，并与现有授权锁顺序一致。

`lockPrincipal`、登录确认、管理员重置和人员大屏检查不得形成相反顺序。复验涉及两个账号时先确定完整账号集合，再按固定顺序加锁。昂贵密码计算及外部 I/O 不放在这些锁内；最终提交仍须核对密码摘要未变化。

策略提交前完成的业务事务有效，提交后不得用旧策略开始新的受保护操作。普通读请求以服务端授权检查为准；持续下载、SSE 和人员大屏在发送后续批次或续租时重新检查，最大复验间隔 30 秒。使用当前状态的短事务，不沿用长快照里的旧策略；已交付数据不可能收回。

现有 `WithPersonCheck → CheckDisplaySessionTx` 链必须保留；运行壳不能只检查自己签发的 display 会话。机器终端、采集转发和 worker 按原身份执行，不增加人工验证码。后台任务依照现有任务授权继续执行，人员读取结果或控制任务仍须通过正式会话检查。

## 8 HTTP 契约

所有路径以下均相对 `/api/v1`。新增请求沿用严格 JSON 解码、未知字段拒绝、16 KiB 认证请求上限、Origin/CSRF 校验、X-Request-ID 及 no-store。HTTPS 正式部署继续使用 Secure Cookie，本地 HTTP 测试按既有配置运行。

### 8.1 登录响应及 Cookie

`POST /auth/login` 输入仍为 username/password；响应从 Account 调整为 LoginResult 判别联合：

```json
{
  "status": "authenticated",
  "account": { "id": "<账号 UUID>", "username": "<用户名>", "display_name": "<显示名称>", "status": "active", "created_at": "<RFC3339 时间>" }
}
```

以上返回 200 并设置正式会话 Cookie。需要后续步骤时返回 202，示例：

```json
{
  "status": "mfa_required",
  "challenge_id": "<非秘密的挑战 UUID>",
  "account_label": "<当前账号显示标识>",
  "expires_at": "<RFC3339 时间>",
  "methods": ["totp", "recovery_code"],
  "policy_required": true
}
```

status 还可以为 mfa_enrollment_required 或 mfa_recovery_required；methods 仅在验证步骤返回，恢复码用尽时可只包含 totp。202 不包含业务权限、租户列表、正式会话令牌或恢复凭证明文。

新增 `infrapro_auth_challenge` Cookie：至少 32 字节安全随机令牌，数据库仅保存用途区分摘要；HttpOnly、SameSite=Lax、无 Domain、Path=/api/v1/auth，Secure 随当前可信配置，Expires/MaxAge 不超过挑战期限。正式 Cookie 继续使用现有 `infrapro_session`，不复用挑战令牌。

挑战 UUID 只用于识别页面步骤，不构成授权凭证。每个挑战修改请求同时携带 challenge_id 和 Cookie，二者必须匹配，以防多标签页覆盖 Cookie 后把旧页面的码提交给新账号。成功、取消、退出和切换账号清除挑战 Cookie；恢复码、TOTP 密钥不写 Cookie。

### 8.2 登录前专用入口

这些路由在操作目录中使用 credential 类别，并由 identity 显式验证用途。它们不产生普通业务 Actor，不允许被作为业务路由的公共前缀。

| 方法与路径 | 输入 | 成功结果 |
| --- | --- | --- |
| GET `/auth/mfa/challenge` | 挑战 Cookie | 200：当前 LoginChallenge；没有有效挑战时 status=none，不返回其他账号信息 |
| POST `/auth/mfa/verify` | challenge_id、method=totp/recovery_code、code 字符串 | 200：authenticated；消费挑战并设置正式 Cookie |
| POST `/auth/mfa/enrollment/start` | challenge_id | 200：EnrollmentSetup，含 enrollment_id、issuer、account_label、secret、qr_png_base64、expires_at |
| POST `/auth/mfa/enrollment/confirm` | challenge_id、new_totp_code | 200：authenticated、account、一次性的 recovery_codes；设置正式 Cookie |
| POST `/auth/mfa/recovery/claim` | challenge_id、recovery_token | 202：当前挑战转为 mfa_enrollment_required；不签发正式会话 |
| POST `/auth/mfa/cancel` | challenge_id | 204：仅撤销匹配的当前挑战并清除 Cookie；重复取消安全返回 |

enrollment/start 只有 login_enroll 可用；已有临时密钥时返回原配置，不延长 TTL，不重置失败计数。qr_png_base64 为服务端本地生成的 PNG，响应体设合理上限，建议 128 KiB；不接受任意远程图片 URL。二维码承载标准 otpauth 配置，但不作为网络请求地址打开。

绑定过程中平台解除强制、账号仍 disabled 时，可继续自愿完成绑定，或取消后重新用密码登录；不能自动把现有挑战兑换为正式会话。账号已启用时，解除强制不免除本次验证码步骤。因素或账号版本变化始终使旧挑战失效。

现有 `/auth/logout` 扩展为同时撤销当前正式会话及当前挑战。缺少正式 Cookie 不能导致挑战退出失败，也不能撤销其他浏览器的挑战。

### 8.3 已登录用户入口

本人入口使用 authenticated 类别，不要求可被角色移除的个人权限。每个 Repository 操作仍检查当前会话，只能访问当前账号；下面表格中的“密码复验”必须在操作事务内调用现有 recent 检查，而不只是写 Policy.Recent 元数据。

| 方法与路径 | 输入 | 密码复验 | 成功结果 |
| --- | --- | --- | --- |
| GET `/account/mfa` | 无 | 否 | state、factor_version、recovery_generation、recovery_codes_remaining、enabled_at、policy_required、can_disable |
| POST `/account/mfa/enrollment/start` | mode=enable/replace、expected_factor_version | 是 | EnrollmentSetup，绑定当前正式会话 |
| POST `/account/mfa/enrollment/confirm` | enrollment_id、new_totp_code | 是 | 新状态、一次性 recovery_codes，轮换当前正式 Cookie |
| POST `/account/mfa/enrollment/cancel` | enrollment_id | 否 | 204；只撤销当前会话拥有的未完成流程 |
| POST `/account/mfa/disable` | expected_factor_version | 是 | disabled 状态，轮换仅密码的正式 Cookie |
| POST `/account/mfa/recovery-codes/regenerate` | expected_factor_version、expected_recovery_generation | 是 | 新码组版本及一次性的 recovery_codes |

自助 start 对同一会话、用途和版本的重复请求返回同一个未过期流程；另一个会话开始新流程会撤销旧流程，页面明确提示旧绑定流程失效。提交 confirm 时已超过最近密码复验期限，保留未过期的绑定流程，提示仅确认密码后重新提交；不要求旧动态码。

disable、regenerate 登记为命令收据操作，要求 X-Command-ID。start/confirm/cancel 用 enrollment 状态去重，不能把 new_totp_code 放入命令 payload。除 confirm 的新配置动态码外，上述入口不接受或要求 totp_code、旧因素码、recovery_code 等二次认证字段。

`POST /auth/reauthenticate` 继续只接受 password，执行成功只更新密码复验时间，不升级认证方式。GET `/auth/me` 可增加非秘密的 authentication 摘要供 UI 展示；只有正式会话有效时才返回账号与权限，挑战不能调用它取得完整身份上下文。

### 8.4 平台管理入口

| 方法与路径 | 资格及输入 | 成功结果 |
| --- | --- | --- |
| GET `/platform/security-policy` | security_policy.read | SecurityPolicy：require_mfa、revision、updated_by、updated_by_label 摘要、updated_at |
| GET `/platform/security-policy/mfa-impact` | security_policy.read | active_accounts、enabled_accounts、unenrolled_accounts、reset_required_accounts、potential_password_sessions、computed_at |
| PUT `/platform/security-policy` | read+update、密码复验、X-Command-ID；require_mfa、expected_version、reason | policy、changed、session_action、operation_id；不要求操作者已绑定 |
| GET `/access/accounts/:id/mfa` | platform.account.read | 目标 state、factor_version、enabled_at；不返回秘密或私人会话 |
| POST `/access/accounts/:id/mfa-reset` | platform.account.read+mfa.reset、密码复验、X-Command-ID；expected_factor_version、reason | state、factor_version、operation_id，以及首次响应中的 recovery_url/expires_at |

账号详情使用单独的 MFA 查询，避免为状态展示把密钥结构暴露到 access.Account 或现有账号列表。重放管理员重置时只返回安全状态及 secret_unavailable，不重新返回恢复 URL。

三个权限的完整代码为 `platform.security_policy.read`、`platform.security_policy.update`、`platform.account.mfa.reset`，scope=platform。内置平台管理员自动纳入目录；基础成员无需获得平台权限，租户/项目自定义角色不可分配这些权限。目录同步、校验和、生成 TypeScript 常量和权限说明一起更新。

迁移不向既有自定义平台角色隐式添加新权限。升级前核对至少一个明确的平台管理员能取得新管理权限；如果唯一完整管理账号来自自定义角色，使用现有受控目录同步或管理资格恢复流程处理，避免新的 ManagementCodes 使平台失去可用管理者。此检查属于升级验证，不能给所有用户补发管理权限。

### 8.5 错误契约

| HTTP 与 code | 含义 | 前端动作 |
| --- | --- | --- |
| 400 validation_failed | 格式、字段、原因等不合法 | 当前表单字段反馈 |
| 401 invalid_credentials | 登录密码错误或账号不可用 | 留在密码步骤，不暴露账号存在性 |
| 401 unauthenticated | 正式会话过期、撤销、版本变化等 | 清理身份和业务查询，返回登录 |
| 401 mfa_login_required | 当前正式会话不满足 MFA 策略或强制代次 | 同样清理身份，显示需要重新登录的说明 |
| 401 mfa_challenge_invalid | 缺失、过期、用途不符或版本失效的挑战 | 清理当前挑战，重新验证密码；不误清理其他标签页已建立的有效正式会话 |
| 400 mfa_code_invalid | 动态码或恢复码不正确、超出窗口或不可用 | 当前步骤反馈并计入失败次数，不区分码是否曾经正确 |
| 409 mfa_challenge_completed | 匹配的挑战已成功完成 | 查询 me；存在会话则继续，否则重新登录，不再次发会话 |
| 403 reauthentication_required | 最近密码复验不满足 | 使用现有密码复验，再由用户重新提交；没有 TOTP 步骤 |
| 403 permission_denied | 无管理资格 | 禁止操作，重新读取权限 |
| 403 self_mfa_reset_forbidden | 管理接口尝试重置自己 | 指向个人更换验证器或维护恢复 |
| 409 mfa_required_by_policy | 平台强制时尝试关闭 | 刷新个人状态，说明当前不能关闭 |
| 409 mfa_state_conflict / version_conflict | 状态或预期版本过期 | 刷新并比较，不自动覆盖提交 |
| 429 rate_limited | 挑战或账号/IP 限制 | 展示 Retry-After，不自行高频重试 |
| 503 unavailable | 数据库、Redis、密钥或目录故障 | 展示不可用，不能切回仅密码流程 |
| 现有命令收据错误 | 缺命令 ID、同键异载荷、提交结果不明等 | 沿用既有核对规则，保留原 ID |

所有错误保持现有 message/request_id/fields 结构。新增 MFA 冷却必须返回实际剩余 Retry-After 秒数；修改当前 identity HTTP 对 429 固定写 60 的行为，使其支持具体冷却时间，同时保留其他既有固定限流的兼容默认值。

## 9 前端接入与交互规格

### 9.1 客户端与身份状态

OpenAPI 在 [contracts/openapi/api.yaml](../../contracts/openapi/api.yaml) 中定义 LoginResult、LoginChallenge、EnrollmentSetup、MFAStatus、SecurityPolicy、影响统计和各请求。通过现有生成脚本更新 schema.d.ts；禁止手工让生成文件与契约分叉。

`IdentityProvider` 增加 `authenticating` phase 与 challenge 子状态；只有 authenticated 可以挂载 WorkspaceShell。初始化顺序为读取 CSRF/帮助 → 查询 me；只有确认没有正式会话时才查询当前挑战。数据库不可用必须进入 unavailable，不能误判 anonymous；失效挑战返回密码步骤。

为 MFA 专用请求禁止普通身份失效广播，分别处理挑战错误；对业务请求的 mfa_login_required 与 unauthenticated 使用同一清理机制。保留 identityEpoch、generation、AbortSignal 机制，旧账号或旧认证步骤的迟到响应不能覆盖新状态。正式会话在其他标签页建立后，focus/reload 优先识别正式会话。

`login` 不再直接作为 `signedIn` 的 mutation success 回调。202 设置挑战步骤并清除密码输入；200 才重新读取身份。新验证器确认返回恢复码时，登录页先展示一次性恢复码，再执行目标页面导航；不能被旧 LoginPage 的 authenticated 自动跳转分支提前带走。

登录后轮换 Cookie 的成功响应触发业务查询失效与身份重载；恢复码展示状态只可作为当前账号、当前操作的短时内存数据保留。退出、切换账号或关闭结果面板清理该数据。禁止存入 localStorage、sessionStorage、持久查询缓存或可恢复草稿。

### 9.2 路由及导航

| 位置 | 改动 |
| --- | --- |
| `/login` | 页面内展示 password/verify/enroll/recover/恢复码结果步骤；URL 不携带任何秘密 |
| 新增 `/mfa-recovery` | 使用独立的凭证用途提取 fragment，立即清除地址栏；引导密码登录和恢复凭证认领 |
| `/security` | 新增 MFA 卡片、绑定向导、恢复码结果及事件名称映射 |
| 新增 `/platform/security-policy` | 全局开关、状态统计、影响确认、修改记录入口 |
| `/access/accounts` 的账号详情 | 增加 MFA 状态查询及按权限显示的重置操作 |
| `_authenticated/route.tsx`、workspace-shell、navigation | authenticating 不能进入工作区；新增路径和权限映射；safeReturn 仅接受合法站内目标 |

新增路由文件按 TanStack Router 约定生成 routeTree，不手改生成结果。账号安全入口即使没有可选的个人资料、密码或会话列表权限，也必须允许账号访问自己的 MFA 卡片；其他卡片保留原权限控制。不能通过添加 account.mfa 权限再允许管理员撤销它，破坏所有用户可自主管理的要求。

恢复凭证为 `mfa-recovery` 独立用途，与 invitation、reset、tenant 分开存取和清理。只能绑定到后端确认的目标账号；登录另一个账号后必须丢弃不匹配凭证并提示，不能给错误账号绑定验证器。恢复页初始化查询 me 返回 401 是正常未登录状态，不能因此提前删除刚从 fragment 捕获的凭证；认领成功、明确退出/取消或确认账号不匹配时再清除。

### 9.3 页面行为

动态码使用单一 text 输入框，inputMode=numeric、autoComplete=one-time-code，按字符串保留前导零；允许粘贴、密码管理器及键盘提交。恢复码输入允许分组格式。扫描二维码与手工密钥是并列方式；二维码只在当前绑定步骤显示，关闭后移除。

失败提示放在当前字段或表单附近，并为辅助技术播报；不能仅靠颜色表达错误。提交中有明确状态，阻止重复提交；返回结果不明确时引导核对，不自动重发。加载、验证码错误、过期、冷却、策略变化和服务故障分别展示。

平台开关使用已有主题、Switch、ConfirmDialog 和密码复验交互。用户拨动时仅改变待提交值，明确保存后才生效；保存失败回到服务端真实状态，不做“前端先开、后台慢慢保存”的乐观更新。只读管理员能查看但不能操作。

用户启用后的状态同时展示“本人已启用”和“平台是否强制”。强制模式下关闭按钮不可用，并说明原因；后端仍独立拒绝。验证码页面提供使用恢复码、退出/切换账号，强制绑定页面没有跳过入口。

浅色、深色和窄屏使用现有组件规范，输入、按钮和对话框有可见标签与焦点；不禁用浏览器缩放。绑定码和恢复码可主动复制或下载，不自动复制到系统剪贴板。下载文件由用户触发，明确其为账号恢复凭证，不能上传到普通附件区。

## 10 命令行及其他调用方

`derivedLogin` 解析 LoginResult。mfa_required 时通过受控终端交互选择动态码或恢复码，提交到同一验证 Interface；读取后清理输入缓冲，不接收会进入 shell 历史或进程参数的验证码参数。mfa_enrollment_required/mfa_recovery_required 时提示到控制台完成绑定/恢复，然后重新登录，退出时不写会话文件。

只有收到 authenticated 且 cookie jar 中存在正式会话 Cookie 时，才以既有 0600、O_EXCL 规则创建会话文件；不保存挑战 Cookie、OTP、恢复码或临时密钥。旧会话文件在策略变化后必须被服务端拒绝，CLI 明确提示重新登录。脚本不能通过“无需解析响应体”绕过 MFA。

扩展现有维护 CLI，新增 `infrapro account mfa-reset --username <name> --reason <reason> --operator <operator>`。本命令属于服务器维护信任路径，需要受控交互终端和既有维护数据库访问资格，不能通过普通用户 HTTP 入口调用。复用 ResetMFA 核心事务，不要求丢失的 TOTP，允许恢复唯一管理员。

维护输出只在终端一次性显示恢复链接，不允许重定向到普通日志文件。审计记录 maintenance actor、操作员说明、目标、原因和操作 ID；操作员文字不是额外身份认证手段，实际维护权限来自受控服务器访问。命令不改变全局开关，不启用已停用账号，不创建业务会话。

实施时检索全部 `/auth/login` 调用方，至少覆盖控制台、CLI、Playwright fixtures、HTTP/HTTPS 验证、演示和验收脚本。测试专用未启用账号在 optional 模式可保留单步登录，但必须解析响应状态；禁止增加生产环境 MFA bypass 参数或“测试账户豁免”。

## 11 密码学 限流与密钥配置

### 11.1 固定协议参数

TOTP 固定采用 HMAC-SHA-1、6 位、30 秒时间步、20 字节随机密钥，验证当前时间步及前后各一步。算法参数不做用户可改选项。内部包装需要返回匹配时间步，并用 last_used_step 的条件更新完成防重放；只得到库的 true/false 不足以完成消费控制。

同一时间步在绑定确认或登录成功后均视为已消费。再次使用统一返回 mfa_code_invalid，页面可说明“请使用尚未使用的当前验证码，必要时等待更新”。不因服务器时钟回拨清空防重放记录；校时后仍需满足未消费条件。

主密钥为 32 字节 AES 密钥，使用 AES-GCM，每次加密生成新随机 nonce；附加认证数据包含固定应用域 infrapro/mfa/v1、账号、因素或挑战用途和对应版本，采用固定规范编码。不同部署环境使用独立 keyring，不使用可变的 issuer 显示名或站点地址作为附加数据。不得把一个账号的密文直接移给另一个账号后继续解密。动态码不落库，TOTP 密钥和临时绑定密钥加密保存，恢复码/挑战/恢复授权仅存摘要。

### 11.2 部署配置

新增 `mfa_keyring_file` 和 `mfa_issuer` 配置，分别可由 `INFRAPRO_MFA_KEYRING_FILE`、`INFRAPRO_MFA_ISSUER` 覆盖。keyring 使用绝对路径，指向仓库外的受限文件；issuer 默认 InfraPro，允许部署为环境内固定可识别名称，去除首尾空白后为 1–64 个字符，不包含秘密或控制字符。不可每次启动随机生成新主密钥。

密钥文件结构为 format_version、active_key_id 和 keys 列表；每项包含唯一 id 及 key_base64，解码必须为 32 字节。文件权限只允许受控进程用户读取，多实例配置同一完整 keyring。配置示例只写文件路径及占位说明，不提交真实密钥或可用的默认密钥。

密钥加载及检查属于身份 Module 装配和 readiness，不在所有不涉及身份的 worker 配置加载中强制引入文件依赖。人员 API 发布前必须具有完整 keyring；无法加载或缺少数据库引用的 key_id 时返回不可用，不把因素当作未配置。日志只记录失败类别及非秘密 key_id。

轮换顺序是所有节点先具备新旧解密密钥 → 切换 active_key_id → 有界批量重加密 → 核对没有旧 key_id 引用 → 按备份保留政策移除旧密钥。重加密不改变 TOTP 本身、factor_version 或会话证明；必须测试轮换中断可恢复，不能一开始删除旧密钥。

### 11.3 限流与故障行为

默认每个挑战最多 5 次失败，之后撤销该挑战并要求重新验证密码。账号级连续失败达到 5 次开始 30 秒冷却，随后按现有指数规则增加，最高 15 分钟；冷却剩余时间通过 Retry-After 返回。密码正确但重建挑战不能绕过冷却。动态码、恢复码和新绑定码的错误都进入相应账号 MFA 范围；恢复令牌认领另外受挑战及 IP 限制。

Redis 保留账号/IP 请求速率限制，默认沿用已配置 CredentialRequestLimit/IPRequestLimit；PostgreSQL 保存连续失败冷却与已消费状态。攻击不存在的挑战只能触发有限的匿名/IP 限制，不产生无界账号状态行。失败事件设置合理去重或限速，计数不能因审计降噪而失效。

数据库、Redis 或解密失败均返回 503，不发正式会话；已经存在的正式会话仍必须通过既有 readiness 与当前状态检查，不为故障增加认证旁路。服务异常不得暴露密文、密钥、验证码或原始请求体。

## 12 审计 通知与现有领域衔接

### 12.1 新事件

| 事件代码 | 何时写入 | 安全字段与展示 |
| --- | --- | --- |
| mfa_policy_changed | 平台开关实际变化并提交 | 操作者、原因、前后布尔值、策略版本、代次 |
| mfa_enabled | disabled 首次完成绑定 | 目标账号、因素版本、启用方式 self/required |
| mfa_replaced | 登录后更换成功 | 目标账号、新因素版本，不含旧新密钥 |
| mfa_disabled | 自主关闭成功 | 目标账号、版本和平台当时允许关闭的事实 |
| mfa_recovery_codes_regenerated | 新码组提交 | 账号、码组版本、数量，不含码或摘要 |
| mfa_recovery_code_used | 恢复码作为登录因素消费成功 | 账号、操作关联和剩余数量，不含码或可核验其值的指纹 |
| mfa_reset | 管理员/维护重置并生成恢复授权 | 操作者、目标、原因、因素版本、授权到期时间 |
| mfa_recovered | 受控恢复最终绑定成功 | 账号及与重置事实的安全关联 |
| mfa_verification | 登录或新绑定校验失败/受限 | 阶段、有限错误类别、来源，不含验证码 |

沿用 login 事件，但只在正式会话成功创建时写 success；MFA 失败可以记录 login failure 与明确的验证阶段，不把“密码正确”冒充登录成功。不对每个普通成功业务请求重复写 MFA 事件。

事件注册到 auditrecord 的有限目录、中文名称和允许字段；接入 [审计来源契约](audit-integration.md)及 [事件目录](audit-event-catalog.md)。策略对象可使用 platform_security_policy，个人因素使用 account_mfa，并关联账号；新增对象类型需同时登记审计目标映射。成功事实与业务修改同事务，不能在 handler 返回后补记。

密码、OTP、二维码、原始密钥、恢复码、恢复令牌、完整恢复 URL 均禁止由程序写入 audit details、收据 payload/result、request log、错误上报和测试产物。不得把凭据或原始错误响应自动拼入 reason；原因输入提示只填写业务原因，后端沿用字段白名单与长度限制，不记录完整请求。

本人配置变化和管理员重置复用 securityReceipt/NotificationReceiptTx，投递既有 security.changed 站内通知，目标为账号安全页。通知只描述发生的操作，不能交付恢复凭证。策略修改进入平台审计和操作结果，不为一次开关变化向全平台无界发送个人通知。

### 12.2 其他领域规则

| 既有流程 | 本期必须保证 |
| --- | --- |
| 邀请开户 | 同事务生成 disabled 状态行；首次登录按当前策略，邀请本身不算 MFA |
| 租户/项目加入、角色变更 | 不复制或修改账号因素；MFA 不赋予任何业务权限 |
| 修改密码、密码恢复 | 保留因素及全局策略；现有 auth_version 失效同时使旧登录挑战、恢复授权失效 |
| 停用/恢复账号 | 停用撤销会话和挑战，保留必要因素配置；恢复不得恢复旧凭证 |
| 退出/退出全部设备 | 退出当前清挑战；退出全部设备同时撤销该账号未完成登录及自助挑战 |
| 会话撤销 | 撤销某会话时取消依附该会话的自助绑定；不得撤销其他账号挑战 |
| 文件/下载/通知/任务查询 | 挑战不能取得数据；正式人员请求使用统一门禁 |
| 人员大屏 | 关联原人员会话持续复验；旧会话不满足策略时停止续租和后续授权 |
| 后台 worker/设备采集/机器终端 | 不增加 TOTP，保持原来明确的机器授权；不因此放开人员入口 |

修改密码与因素维护共用账号锁；不得出现旧密码验证通过后，新密码已生效却仍发出旧版本会话。全局撤销和备份恢复必须包含新凭证种类，不能只处理原密码重置表。

## 13 文件改动清单与实施顺序

以下为完整开发任务清单；W1–W7 的实际实现及自验状态见验证账本。

| 工作包 | 新增或调整位置 | 完成证据 |
| --- | --- | --- |
| W1 数据与配置 | migrations 新增 MFA 迁移；config 新增 mfa 配置解析；runtime 身份密钥装配；测试环境临时 keyring | 新安装、存量升级、缺配置拒绝、受限角色和数据约束测试 |
| W2 身份核心 | identity 新增 MFA/挑战/策略文件；调整 service/model/repository/session_cache/access | 状态、原子消费、会话代次、缓存命中、并发与失败持久化测试 |
| W3 HTTP 与权限 | identity handler 新文件；router；access MFA catalog/policies、builtins 和命令目录 | 每条路由已登记；直接 HTTP 越权、CSRF、挑战越界和契约检查 |
| W4 契约与客户端 | OpenAPI；api-client index/identity；生成 schema、permissions；登录调用方 | 200/202 正确分流，新增错误分类，旧脚本无静默绕过 |
| W5 控制台 | auth 登录和恢复页面；account MFA 卡片；platform 安全策略；access 账号详情；navigation、workspace-shell 和 routes | 真实浏览器全部场景，尤其登录后管理动作不要求 TOTP |
| W6 恢复与审计 | 账号维护 CLI；identity maintenance；auditrecord/notificationevent 名称及字段；审计目标 | 恢复码/管理员/唯一管理员路径，单次秘密输出，审计和通知证据 |
| W7 发布与验收 | 部署示例、操作手册、验证账本、相关脚本与测试 | 完整自验通过、入口可访问、人工验收资料齐备 |

按 W1→W2→W3/W4→W5/W6→W7 集成，内部可以分小迭代，但不能以某一个工作包通过代替五项功能完整交付。W2 验证正式会话签发规则之后再接页面，避免 UI 状态暂时替代后端门禁。

实施同时维护 `docs/operations/mfa.md` 和 `docs/verification/mfa.md`，并更新正式需求、架构及文档目录。现有文档中针对旧模块“本轮不增加 MFA”的阶段性表述，不用于覆盖本次已确认范围；现有密码复验规则继续适用。实现交付须同步 OpenAPI、迁移和运行配置；文档中的状态不能替代实际检查。

## 14 发布 运维与恢复

### 14.1 首次发布

1. 在隔离环境完成下面全部检查。清点实际部署 API、控制台、CLI 和代理版本；备份数据库及原配置。
2. 在仓库外生成独立 keyring，按受控文件分发到各身份节点；验证权限、密钥长度、issuer 和时间同步。密钥备份与数据库备份分开保管。
3. 停止或隔离旧身份 API 的流量，执行新 Goose 迁移和权限目录同步。迁移默认 require_mfa=false，不改变现有账号的启用意愿。
4. 部署同一兼容版本的 API、前端和人员 CLI，重建必要缓存并检查 readiness、权限目录与契约；旧代码不得继续接收登录流量。
5. 使用验收账号检查可选模式及全部恢复入口。平台管理员通过页面自行决定何时开启强制，不由部署脚本自动开启。

增量迁移应覆盖当前所有开户路径及测试夹具，不能只回填旧账号却漏掉以后新建账号。更新 Nginx 的必要路由与 Cookie 测试；新认证端点保持小请求上限，恢复 URL 的 fragment 不能进入代理日志。若现有通用代理已覆盖路径，则不为 MFA 单独扩大代理权限。

### 14.2 维护与回滚

提供按批清理挑战和过期密文的维护任务、仅含计数的认证故障指标、密钥引用检查及密钥轮换操作步骤。不能用删除审计或清零失败记录作为日常解锁方式；使用有审计的恢复流程。

不能直接回滚到不认识 MFA 的旧 API 后继续接收人员流量。出现发布故障时先保留拒绝访问或进入维护状态，使用兼容当前认证数据的修复版本；如必须恢复旧系统，需作为单独受控恢复，不能静默去掉已启用的保护。

### 14.3 数据库恢复

暂停全部人员入口及旧节点，恢复数据后先运行扩展后的 RevokeAfterRestore：递增认证版本、撤销所有正式会话、登录/自助挑战、管理员恢复授权和旧恢复码，清理临时秘密并执行缓存失效。旧恢复码即使备份中显示未用也不能再接受。

时间点回滚可能还原已经更换的 TOTP 密钥或较弱的旧策略。重新开放前必须核对可信的最新安全记录及独立密钥备份，恢复已经确认的策略水平；无法确认的账号进入 reset_required 并撤销不可信密钥，按受控恢复重新绑定。不能仅凭旧快照把 disabled 当作用户从未启用 MFA；无法确认影响范围时保持人员入口关闭，不能默认解除全局强制。

恢复演练必须包含“备份后消费恢复码”“备份后更换验证器”“备份后开启强制”三个回滚场景，并证明旧凭证或旧开关不自动恢复访问。唯一管理员用维护 CLI 重新取得恢复授权；重新生成恢复码发生在受控登录或恢复完成之后。

## 15 自动验证与证据要求

### 15.1 验证层次

| 层次 | 必测内容 | 证据要求 |
| --- | --- | --- |
| 算法和状态 | RFC 测试向量、前导零、时间窗边缘、防重放、状态表、加解密和错误密钥 | 使用可控时钟；不只 mock 一次 bool=true |
| 数据库集成 | 升级、开户补行、消费与会话同事务、失败计数提交、重置/禁用竞态、回滚与恢复 | 真实隔离 PostgreSQL；断言最终行和审计事实 |
| 跨实例和缓存 | 两个实例同时消费同一码、Redis 命中时策略切换、重启、缓存故障 | 真实 Redis 命名空间，证明最多一次成功 |
| HTTP 与权限 | 所有新端点、请求体严格性、Cookie/CSRF/Origin、挑战不可越界、三种平台权限 | 直接调用后端；不能只看按钮是否隐藏 |
| 浏览器 | 多步登录、刷新/返回/多标签页、个人维护、平台开关、受控恢复和无 TOTP 操作复验 | Playwright 真实 API；测试合成账号，不写入现有业务账号 |
| 其他调用方 | CLI 200/202、会话文件、人员大屏和长连接复验 | 证明没有密码-only 旁路或伪成功文件 |
| 部署与恢复 | HTTP 开发、HTTPS 代理、keyring、混版拒绝、备份恢复及密钥轮换 | 实际隔离环境；记录未覆盖的真实部署条件 |

补充专项测试文件，可按 `mfa_test.go`、`mfa_concurrency_test.go`、`mfa_migration_test.go`、HTTP `mfa_test.go`、Playwright `mfa.spec.ts` 组织。专项执行入口可新增 `pnpm test:mfa`；如新增，必须真正隔离数据库 schema、Redis 前缀、端口和密钥文件，不把伪造 MFA 凭据注入生产认证代码。

复用并通过现有 `pnpm check`、`pnpm test:identity`、`pnpm test:e2e`、`pnpm test:http`、`pnpm test:https`，以及涉及审计目录的检查。注意当前 test:identity 会启动相关依赖并运行广泛 Go 测试，不能把命令名称当成只覆盖身份包。测试完成后记录命令、版本、范围和结果；未运行的检查明确列出，不能称全部通过。

浏览器 trace、截图、HAR、响应正文和日志可能捕获二维码/恢复码。秘密生成、展示和提交步骤应关闭相关内容捕获或遮罩敏感区域；证据仅保存合成账号标识、布尔结果、错误代码和脱敏截图。不要把密钥或恢复码写进测试失败断言、快照或 CI 产物。

### 15.2 验收环境和账号

准备两个租户 A/B、一个项目及以下独立合成账号：平台管理员 P1（未启用）、P2（已启用）、只读平台审计员 P3、租户管理员 TA、普通用户 U1（未启用）、U2（已启用）、同时属于 A/B 的 U3、无租户账号 U4、停用账号 U5、无可选个人角色权限的 U6。密码和测试因素由隔离夹具生成，验收文档不保存明文秘密。

准备两个浏览器上下文、至少两个 API 实例共享相同测试数据库/Redis/keyring，以及一份可撤销的人员大屏会话。人工验收用真实兼容验证器扫码；自动测试使用合成随机因素和可控时钟，不把生产时钟来回修改。

## 16 验收用例

下表为完整交付检查清单。每项在实施验收账本中分别记录实现状态、自验结果、证据位置和人工接受；执行结果见[逐项验证账本](../verification/mfa.md)，不在规格中重复记录。

| 编号 | 操作或前提 | 必须观察到的结果 | 对应需求 |
| --- | --- | --- | --- |
| MFA-001 | 新安装及存量升级 | 默认不强制；现有和新建账号有 disabled 状态行，没有自动生成因素 | R1 R2 |
| MFA-002 | U1 在 optional 模式登录 | 密码成功直接建立 password 会话 | R3 |
| MFA-003 | U2 在 optional 模式登录 | 密码成功返回 202，第二因素通过前没有正式会话 | R3 |
| MFA-004 | TA、普通用户直接修改全局策略 | 后端拒绝，无策略或审计成功变化 | R1 |
| MFA-005 | P3 只具备策略查看 | 能查看，不能保存或重置账号 | R1 |
| MFA-006 | P1 未启用 MFA，开启强制 | 保存前只需已有密码复验，不要求先绑定；保存成功后旧会话失效 | R1 R6 |
| MFA-007 | 已绑定且完整登录的 P2 开关策略 | 全程不再次要求 TOTP；权限及密码复验仍生效 | R1 R6 |
| MFA-008 | U1、TA、U4 强制模式下登录 | 均进入绑定，不能跳过或访问业务接口 | R1 R3 |
| MFA-009 | U3 在两个租户及项目间切换 | 共用同一因素，业务权限仍按原规则，不再次输入动态码 | R2 R6 |
| MFA-010 | 关闭平台强制 | U2 仍需 MFA；已有因素和恢复码保留，U1 可自主关闭 | R1 R2 |
| MFA-011 | 开启再关闭强制，持有开启前旧密码会话 | 旧会话仍被拒绝，新密码登录可建立当前代次会话 | R3 |
| MFA-012 | Redis 命中、另一个 API 节点处理旧会话 | 策略保存后的下一次请求拒绝，不等待缓存 TTL | R3 |
| MFA-013 | 已有合法 MFA 会话经历开关变化 | 原期限内继续使用，不重复认证 | R3 R6 |
| MFA-014 | 开始绑定后关闭页面、过期或扫码未确认 | 账号未被错误启用；临时资格无法访问业务 | R2 |
| MFA-015 | U6 无个人资料或会话列表权限 | 仍可管理自己的 MFA，不获得其他个人或管理权限 | R2 |
| MFA-016 | 新验证器动态码错误后重试正确码 | 错误保留未完成状态，正确后一次性启用并生成恢复码 | R2 R5 |
| MFA-017 | 首次启用成功 | 原会话撤销，当前会话正确轮换，恢复码只展示一次 | R2 R3 |
| MFA-018 | 登录后更换验证器 | 只确认密码和新验证器，不要求旧 TOTP/恢复码 | R2 R6 |
| MFA-019 | 更换过程中取消或新码错误 | 旧验证器继续有效，旧恢复码未提前失效 | R2 |
| MFA-020 | 更换成功后使用旧因素、旧恢复码和旧会话 | 全部失效，新配置可登录 | R2 R3 |
| MFA-021 | optional 模式登录后关闭 MFA | 只需密码复验；没有动态码字段，关闭后新登录只需密码 | R2 R6 |
| MFA-022 | 强制模式下直接调用关闭接口 | 409 拒绝，不能通过请求参数绕过 | R1 R2 |
| MFA-023 | 关闭与开启强制并发 | 按事务顺序得到一致结果，不存在强制已提交而新关闭仍绕过的情况 | R1 R5 |
| MFA-024 | 登录后重建恢复码 | 只需密码复验；新组生成，旧组失效，现有会话保留 | R2 R6 |
| MFA-025 | 同一 TOTP 在窗口内重复或跨实例并发提交 | 最多一次成功，其他请求拒绝，不发第二份正式会话 | R5 |
| MFA-026 | 同一恢复码跨实例并发提交 | 只有一个消费结果与一个对应成功会话 | R4 R5 |
| MFA-027 | TOTP 前导零、允许时间步边缘、超窗及回拨 | 按固定窗口判定，字符串不丢零，重启不清防重放 | R5 |
| MFA-028 | 连续错码、新建挑战、切换浏览器继续猜码 | 挑战和账号/IP 限制有效，计数不因换挑战重置 | R5 |
| MFA-029 | MFA 校验失败后检查数据库并重启 | 失败数和冷却已提交，返回准确 Retry-After | R5 |
| MFA-030 | 持有挑战调用 me、租户、文件、通知、平台接口 | 全部不能取得业务身份或业务数据 | R3 R5 |
| MFA-031 | 伪造 mfa_verified、账号 ID、用途或因素版本 | 输入拒绝或资格校验失败，不能提升认证级别 | R5 |
| MFA-032 | 密码通过后另处修改密码、停用账号或重置 MFA | 旧挑战无法完成登录 | R3 R5 |
| MFA-033 | 密码重置后用新密码登录 U2 | MFA 保留，仍需第二因素 | R3 R4 |
| MFA-034 | 用恢复码登录 U2 | 密码仍必须正确；码只用一次，登录后可直接按密码规则更换验证器 | R4 R6 |
| MFA-035 | 恢复码使用与码组重建并发 | 仅接受一致的码组版本，不恢复已撤销码 | R4 R5 |
| MFA-036 | P1/P2 已完整登录后重置 U2 | 只按权限和密码复验执行，无 TOTP；目标旧凭据和会话全部失效 | R4 R6 |
| MFA-037 | 管理员试图查看目标密钥/恢复码或重置自己 | 无秘密读取入口；管理自重置被拒绝 | R4 R5 |
| MFA-038 | 平台不强制，U2 已被管理员重置 | 仍必须凭专用恢复凭证及密码重新绑定，不能只密码登录 | R4 |
| MFA-039 | 恢复凭证过期、重复认领、错误账号认领 | 被拒绝；新发放凭证使旧凭证失效 | R4 R5 |
| MFA-040 | 恢复链接 fragment 与其他凭证页面混用 | 地址栏及时清除，用途隔离，不被识别为邀请或密码重置 | R4 R5 |
| MFA-041 | 停用账号被重置 MFA | 仍不能登录；管理重置不隐式启用账号或改变角色 | R4 |
| MFA-042 | 唯一管理员失去验证器及恢复码 | 维护 CLI 可发恢复授权；全局策略不变，需重新绑定才能进入 | R4 |
| MFA-043 | 创建、读取、修改或重放秘密响应 | 不把秘密写入日志/审计/收据；重放只给安全状态 | R5 |
| MFA-044 | 平台保存、绑定、重建或重置响应丢失 | 核对原状态或收据，不自动重做，不重复产生有效秘密 | R1 R4 R5 |
| MFA-045 | 绑定成功后缓存写失败 | 因素状态明确、未交付会话撤销；可重新登录，不降级放行 | R3 R5 |
| MFA-046 | 数据库/Redis/密钥故障、策略行缺失 | 返回不可用，不当作 optional 或 disabled | R5 |
| MFA-047 | HTTP 开发与 HTTPS 代理下登录、绑定、退出 | Cookie/CSRF/Origin 正确，挑战与正式 Cookie 分离 | R3 R5 |
| MFA-048 | 伪造来源、缺 CSRF、额外字段和超大请求 | 请求被拒绝，无认证状态变化 | R5 |
| MFA-049 | 页面刷新、后退、过期、取消、切换账号 | 恢复正确步骤或重启登录；业务区不可提前进入 | R3 |
| MFA-050 | 两标签页同时开始不同账号登录 | challenge_id 与 Cookie 不匹配时拒绝，迟到响应不能污染新账号 | R3 R5 |
| MFA-051 | MFA 已登录超过 5 分钟后操作角色、密码或平台管理 | 仅可能触发现有密码复验，不弹 TOTP；登录本身的 MFA 证明不按 5 分钟到期 | R6 |
| MFA-052 | CLI 收到 mfa_required/enrollment_required | 完成 MFA 才写正式会话文件；绑定未完成无伪成功文件 | R3 |
| MFA-053 | 旧 CLI 会话、人员大屏、持续连接遇到强制切换 | 旧人员资格被拒绝，持续连接在 30 秒复验上限内停止后续授权 | R3 |
| MFA-054 | 机器终端、采集与后台任务正常运行 | 不弹验证码，不借机开放人员入口 | R3 |
| MFA-055 | 查看审计、安全记录及通知 | 只有真实成功事实；操作、目标、原因可追溯，无秘密 | R5 |
| MFA-056 | 密钥轮换中断、不同 key_id 及受限数据库角色 | 可受控恢复，旧密钥仍可解密有效数据，未授权角色不可读取秘密 | R5 |
| MFA-057 | 数据库恢复及备份后三种安全变化 | 旧会话/挑战/恢复码不复活，历史因素和较弱策略不自动恢复访问 | R5 |
| MFA-058 | 升级目录、旧节点、旧缓存和契约检查 | 新旧节点不混跑；缺字段不视为 MFA；客户端及权限目录一致 | R1 R3 R5 |
| MFA-059 | 浅深色、窄屏、键盘、粘贴、恢复码保存 | 无阻断交互，前导零和长恢复码正确，秘密只由用户主动复制/下载 | R2 R3 |
| MFA-060 | 全部五项交付复核 | 相关测试全通过，资料齐全；功能自验和用户接受分开记录 | R1 R2 R3 R4 R5 R6 |

### 16.1 人工验收推荐顺序

1. 从 optional 开始，用 U1 密码登录，进入账号安全页绑定真实验证器，保存恢复码，退出后确认第二步登录出现。
2. 保持 U1 已登录，不输入旧动态码，分别执行重建恢复码、更换验证器、关闭 MFA；只按提示确认密码和新绑定码，验证第 1.3 节的区别。
3. P1 在未绑定状态开启强制，观察保存成功说明及会话退出；重新登录完成绑定。另一浏览器的 U1 旧密码会话也必须失效。
4. U2 正常 MFA 登录，U3 跨租户切换，U4 未加入租户也需绑定；尝试直接进入业务 URL，确认没有跳过入口。
5. 用 TA/P3 验证权限限制；由 P2 关闭强制，检查 U2 的个人 MFA 保留及旧会话不复活。
6. 用一个恢复码登录并检查不能再次使用；重新生成码组后检查旧组失效。
7. 管理员不输入 TOTP 重置 U2，通过专用链接完成恢复；平台不强制时也不能跳过重新绑定。
8. 在隔离环境演练唯一管理员维护恢复、CLI、人员大屏、故障和备份恢复，最后查看审计和安全记录。

人工验收页面入口为现有控制台的 `/login`、`/security`、`/platform/security-policy`、`/access/accounts` 和 `/mfa-recovery`；若部署带 `/console/` 前缀，应使用实际 PublicConsoleURL 拼接，文档不得写一个未经验证的生产地址。

## 17 完成标准和交付记录

实现状态按 In progress → Awaiting acceptance → Accepted 推进。进入 Awaiting acceptance 前必须满足：R1–R6 和 MFA-001–060 均有对应实现及自验结果；契约、权限、审计、CLI、迁移、代理和恢复检查通过；所有发现的失败已解决；启动入口、测试账号取得方式、操作手册和证据路径可用。不能仅凭单元测试通过或页面出现按钮进入该状态。

最终交付说明应包含实际改动、启动/访问方式、已运行检查和结果、部署范围、尚未验证的真实环境条件、已知限制及上述人工验收步骤。不得把本规格中的接口或建议文件称为已经上线，也不得把开发者自验写作用户接受。只有用户明确确认，才能将功能标记 Accepted。

本轮在当前 checkout 完成实现和隔离自验；未部署生产，未改变现有实际业务账号的安全设置。人工接受须由用户明确确认。
