# MFA 验证账本

状态：Awaiting acceptance。R1–R6 已实现，MFA-001–060 自验及交付复核通过；人工接受待用户确认。完成日期：2026-11-17。

依据：[开发规格](../development/mfa-development-spec.md)。实施自验阶段保留原 checkout 的既有修改，未创建分支或 worktree。本次按用户要求在主工作区创建 `develop` 并集成 MFA；`main` 保持生产基线提交 `e0699959819823fc6f1af5b355982ef222f5643e`，既有生产部署资料保留且不纳入 MFA 提交。未执行生产部署。本账本将自动自验与用户人工接受分开。

## 实现交付

- Go identity 统一拥有全局策略、加密因素、短时挑战、恢复码、管理员恢复和维护入口；PostgreSQL 串行化版本与一次性消费，Redis 提供共享请求预算及会话缓存。schema 56 默认 optional，回填存量账号及会话。
- React 控制台和 CLI 按 200/202 判别登录。正式人员入口及人员大屏都复验当前 MFA 证明；机器权限独立。登录后的管理操作只有既有密码复验，更换只验证新因素。
- 新增三项平台权限、17 个 HTTP 端点、九类审计事件及安全通知；收据、日志和测试产物不保存可用秘密。
- [运行手册](../operations/mfa.md)包含升级、密钥生成/分发/轮换、维护恢复、备份恢复、隔离验收启动和失败处置。

## 自验证据索引

以下简称用于逐项映射，均指真实 PostgreSQL/Redis 或真实 HTTP/浏览器测试；算法测试采用固定向量。证据文件中的秘密步骤禁用 trace、视频及自动截图，仅在结果已关闭后保存安全截图。

| 简称 | 实际来源 |
| --- | --- |
| CRYPTO / LIFE / POLICY / RECOVERY / CONCURRENCY | [mfa_test.go](../../packages/go/platform/identity/mfa_test.go)：算法、完整个人生命周期、全局代次、恢复、双实例竞争和持久冷却 |
| ROTATION / CACHE / PASSWORD / DISPLAY-RACE | [mfa_faults_test.go](../../packages/go/platform/identity/mfa_faults_test.go)：轮换、故障、密码恢复、停用与人员资格并发 |
| MIG | [mfa_migration_test.go](../../packages/go/platform/identity/mfa_migration_test.go)：真实 55→56 升级与受限角色 |
| ATOMIC / GRANTS / REGEN-RACE | [mfa_atomic_test.go](../../packages/go/platform/identity/mfa_atomic_test.go)：注入审计失败回滚、收据、授权和码组竞争 |
| SNAPSHOT | [mfa_restore_test.go](../../packages/go/platform/identity/mfa_restore_test.go)：实际 pg_dump/restore 后安全状态撤销 |
| RATE / EDGES | [mfa_rate_test.go](../../packages/go/platform/identity/mfa_rate_test.go)、[mfa_edges_test.go](../../packages/go/platform/identity/mfa_edges_test.go)：跨 IP 请求预算、取消/过期/错误新码及丢失确认恢复 |
| HTTP / CLI | [HTTP 边界](../../services/api/internal/transport/http/mfa_test.go)、[CLI 集成](../../services/api/internal/transport/http/mfa_cli_test.go)、[PTY 驱动](../../tests/mfa/cli.py) |
| FRAGMENT | [内存凭证单测](../../tests/unit/mfa-credential.test.mjs)：MFA、邀请、重置及租户 fragment 用途隔离，匿名初始化保留，身份变化清理 |
| BROWSER / POLICY-UI | [个人与多标签](../../tests/e2e/mfa.spec.ts)、[平台策略与管理员恢复](../../tests/e2e/mfa-policy.spec.ts)，桌面与 375px 各执行 |
| WORKERS | [运行装配验证](../../packages/go/platform/runtime/mfa_test.go)：机器进程无需人员 keyring |
| TLS | [HTTPS 身份检查](../../scripts/https-identity-checks.mjs)、[人员持续连接检查](../../scripts/mfa-display-checks.mjs)，实际 Nginx/TLS 三源 |
| LAB | [隔离环境](../../scripts/mfa-lab.mjs)：两 API、两租户、一项目、P1/P2/P3/TA/U1–U6、真实人员大屏、U3 跨空间自验 |
| CHECK / GO / HTTP-DEPLOY / CATALOG | 下方命令账本；GO 为广泛 Go 回归，不能解释为仅身份单元测试 |

## MFA-001–060 对照

表内列出可重复的自验断言，60 项均已实现并通过对应自验。汇总见[交付结果](../../.artifacts/mfa/delivery/result.json)，逐项状态见[用例结果](../../.artifacts/mfa/delivery/case-results.json)。人工使用真实验证器扫码和最终业务确认仍由用户执行，不因为自动测试通过而记为 Accepted。

| 用例 | 场景 | 实现与自验依据 | 人工接受 |
| --- | --- | --- | --- |
| MFA-001 | 新安装及存量升级 | MIG：55→56 实际升级、默认 optional、存量因素行和会话回填；HTTP 新开户 | 待用户确认 |
| MFA-002 | U1 在 optional 模式登录 | LIFE、HTTP、BROWSER：密码 200 正式会话 | 待用户确认 |
| MFA-003 | U2 在 optional 模式登录 | LIFE、HTTP、BROWSER：密码仅 202，TOTP/恢复码后正式会话 | 待用户确认 |
| MFA-004 | TA、普通用户直接修改全局策略 | HTTP、LAB：普通用户及租户资格没有策略权限；目录权限检查 | 待用户确认 |
| MFA-005 | P3 只具备策略查看 | POLICY-UI：P3 只读开关，保存入口不可用；三项独立权限目录 | 待用户确认 |
| MFA-006 | P1 未启用 MFA，开启强制 | POLICY、POLICY-UI：未绑定管理员提交强制后返回 sign_in_again，旧会话失效 | 待用户确认 |
| MFA-007 | 已绑定且完整登录的 P2 开关策略 | POLICY、POLICY-UI：MFA 管理员开关策略保留会话，无额外 TOTP | 待用户确认 |
| MFA-008 | U1、TA、U4 强制模式下登录 | POLICY、POLICY-UI：全局账号统一进入 enrollment；HTTP 挑战业务隔离 | 待用户确认 |
| MFA-009 | U3 在两个租户及项目间切换 | LAB：U3 同一 MFA 会话访问 A/B 及项目、两个 API；平台管理仍 403 | 待用户确认 |
| MFA-010 | 关闭平台强制 | POLICY、LIFE、POLICY-UI：关闭强制不清因素，个人关闭后重新密码登录 | 待用户确认 |
| MFA-011 | 开启再关闭强制，持有开启前旧密码会话 | POLICY、POLICY-UI、TLS：开启前密码会话在关闭后仍 401 | 待用户确认 |
| MFA-012 | Redis 命中、另一个 API 节点处理旧会话 | POLICY：两个 Service 共用 PG/Redis，预热缓存后立即检查当前代次 | 待用户确认 |
| MFA-013 | 已有合法 MFA 会话经历开关变化 | POLICY：旧合法 MFA 证明保持；mfa_verified_at 一小时前仍可认证 | 待用户确认 |
| MFA-014 | 开始绑定后关闭页面、过期或扫码未确认 | EDGES：开始、取消、过期不启用因素，不撤销原正式会话 | 待用户确认 |
| MFA-015 | U6 无个人资料或会话列表权限 | HTTP：删除可选角色后本人 MFA 可用、profile 为 403 | 待用户确认 |
| MFA-016 | 新验证器动态码错误后重试正确码 | HTTP、BROWSER：错误新码后正确码完成，恰有 10 个恢复码 | 待用户确认 |
| MFA-017 | 首次启用成功 | LIFE、HTTP、BROWSER：轮换会话、旧会话撤销、关闭结果后秘密移除 | 待用户确认 |
| MFA-018 | 登录后更换验证器 | LIFE、BROWSER：只验证新因素；更换没有旧 TOTP/恢复码字段 | 待用户确认 |
| MFA-019 | 更换过程中取消或新码错误 | EDGES、BROWSER：错误新码及取消保留旧因素和码组，旧 TOTP 仍能登录 | 待用户确认 |
| MFA-020 | 更换成功后使用旧因素、旧恢复码和旧会话 | LIFE、EDGES：成功更换撤销旧会话、旧恢复码及挑战；新组可登录 | 待用户确认 |
| MFA-021 | optional 模式登录后关闭 MFA | LIFE、BROWSER：密码复验规则关闭，当前 Cookie 轮换为 password | 待用户确认 |
| MFA-022 | 强制模式下直接调用关闭接口 | POLICY：强制状态下关闭返回 mfa_required_by_policy | 待用户确认 |
| MFA-023 | 关闭与开启强制并发 | DISPLAY-RACE：真实并发强制提交/个人关闭，共享策略与账号锁 | 待用户确认 |
| MFA-024 | 登录后重建恢复码 | LIFE、ATOMIC、BROWSER：重建一次、旧组撤销、当前会话保留，无 TOTP | 待用户确认 |
| MFA-025 | 同一 TOTP 在窗口内重复或跨实例并发提交 | CONCURRENCY：两实例同 TOTP 竞争，只有一次成功；CRYPTO 持久步判定 | 待用户确认 |
| MFA-026 | 同一恢复码跨实例并发提交 | CONCURRENCY：两实例同恢复码竞争，只有一次成功 | 待用户确认 |
| MFA-027 | TOTP 前导零、允许时间步边缘、超窗及回拨 | CRYPTO：RFC 6238 六位向量含前导零、超窗、已消费时间步和 AAD；输入采用字符串 | 待用户确认 |
| MFA-028 | 连续错码、新建挑战、切换浏览器继续猜码 | CONCURRENCY、RATE、HTTP：换挑战/实例/IP 不清失败计数，共享账号请求预算 | 待用户确认 |
| MFA-029 | MFA 校验失败后检查数据库并重启 | CONCURRENCY、HTTP：失败事务落库，新实例仍冷却，Retry-After 核对 1–31 秒 | 待用户确认 |
| MFA-030 | 持有挑战调用 me、租户、文件、通知、平台接口 | HTTP：挑战不能访问 me、个人、租户、通知、平台；统一业务 Actor 门禁 | 待用户确认 |
| MFA-031 | 伪造 mfa_verified、账号 ID、用途或因素版本 | HTTP、CRYPTO：额外 mfa_verified 拒绝，错 ID/用途不消费，AAD 防账号/版本替换 | 待用户确认 |
| MFA-032 | 密码通过后另处修改密码、停用账号或重置 MFA | PASSWORD、RECOVERY：密码恢复、停用、管理员重置均撤销未完成资格 | 待用户确认 |
| MFA-033 | 密码重置后用新密码登录 U2 | PASSWORD：重置密码后新密码仍得到 mfa_required | 待用户确认 |
| MFA-034 | 用恢复码登录 U2 | RECOVERY、BROWSER：密码后恢复码成功一次；可继续更换新因素 | 待用户确认 |
| MFA-035 | 恢复码使用与码组重建并发 | REGEN-RACE：消费/重建并发，旧代次全部撤销，新代次恰 10 个 | 待用户确认 |
| MFA-036 | P1/P2 已完整登录后重置 U2 | RECOVERY、POLICY-UI：权限及密码规则重置，目标旧凭据撤销，不要求操作者 TOTP | 待用户确认 |
| MFA-037 | 管理员试图查看目标密钥/恢复码或重置自己 | RECOVERY、HTTP：管理自重置拒绝；状态结果无因素或恢复码读取字段 | 待用户确认 |
| MFA-038 | 平台不强制，U2 已被管理员重置 | RECOVERY、HTTP、POLICY-UI：optional 下 reset_required 仍需授权和新因素 | 待用户确认 |
| MFA-039 | 恢复凭证过期、重复认领、错误账号认领 | GRANTS：错账号、过期、重复、被新授权替代均拒绝 | 待用户确认 |
| MFA-040 | 恢复链接 fragment 与其他凭证页面混用 | POLICY-UI、FRAGMENT：fragment 立即从地址栏移除并可认领；用途隔离，匿名初始化不误清，切换身份清理 | 待用户确认 |
| MFA-041 | 停用账号被重置 MFA | PASSWORD：停用目标重置后仍拒绝登录，未隐式恢复 | 待用户确认 |
| MFA-042 | 唯一管理员失去验证器及恢复码 | ROTATION、CLI：维护 CLI 发一次性授权，不直接建立会话；需密码与新绑定 | 待用户确认 |
| MFA-043 | 创建、读取、修改或重放秘密响应 | ATOMIC、RECOVERY、EDGES：审计/通知/收据无秘密，重放不返回恢复码或链接 | 待用户确认 |
| MFA-044 | 平台保存、绑定、重建或重置响应丢失 | ATOMIC、EDGES、POLICY-UI：一次性提交与安全收据；前端写入无自动重试 | 待用户确认 |
| MFA-045 | 绑定成功后缓存写失败 | CACHE：注入 Redis SET 故障，因素已启用但未交付会话全部撤销；重新登录仍需 MFA | 待用户确认 |
| MFA-046 | 数据库/Redis/密钥故障、策略行缺失 | CACHE、RATE、CRYPTO、ROTATION：缺策略、Redis 故障、公开权限 keyring、缺 key_id 均拒绝；POLICY-UI 查询失败显示不可核实而非未启用 | 待用户确认 |
| MFA-047 | HTTP 开发与 HTTPS 代理下登录、绑定、退出 | HTTP、TLS、HTTP-DEPLOY：两种 Cookie 模式、真实 HTTPS 三源代理、登录/绑定/退出 | 待用户确认 |
| MFA-048 | 伪造来源、缺 CSRF、额外字段和超大请求 | HTTP、TLS：无 CSRF、伪 Origin、额外字段及超过 16 KiB 请求被拒绝 | 待用户确认 |
| MFA-049 | 页面刷新、后退、过期、取消、切换账号 | BROWSER、POLICY-UI、EDGES：刷新恢复挑战、取消清理、过期不完成；账号切换清理 | 待用户确认 |
| MFA-050 | 两标签页同时开始不同账号登录 | BROWSER：双标签页 Cookie/ID 不匹配拒绝，迟到响应不覆盖下一账号 | 待用户确认 |
| MFA-051 | MFA 已登录超过 5 分钟后操作角色、密码或平台管理 | POLICY、BROWSER：一小时 MFA 证明有效、六分钟前密码需要复验；维护界面无旧 TOTP | 待用户确认 |
| MFA-052 | CLI 收到 mfa_required/enrollment_required | CLI：真实二进制和 PTY，TOTP/恢复码交互，202 绑定不写伪会话文件 | 待用户确认 |
| MFA-053 | 旧 CLI 会话、人员大屏、持续连接遇到强制切换 | DISPLAY-RACE、TLS：统一人员门禁；真实人员 SSE 30 秒内停止并卸载 iframe | 待用户确认 |
| MFA-054 | 机器终端、采集与后台任务正常运行 | WORKERS、TLS、GO：无人员 keyring 的机器进程装配通过，机器终端在强制切换后保持授权 | 待用户确认 |
| MFA-055 | 查看审计、安全记录及通知 | ATOMIC、PASSWORD、BROWSER：成功事实数量、中文事件、安全记录和通知内容脱敏 | 待用户确认 |
| MFA-056 | 密钥轮换中断、不同 key_id 及受限数据库角色 | ROTATION、MIG：双 key_id 重加密可重入、缺旧 key readiness 拒绝、受限数据库角色不可读因素 | 待用户确认 |
| MFA-057 | 数据库恢复及备份后三种安全变化 | SNAPSHOT、HTTP-DEPLOY：真实 pg_dump/restore；消费码、旧因素、较弱策略均不能复活 | 待用户确认 |
| MFA-058 | 升级目录、旧节点、旧缓存和契约检查 | MIG、CHECK、CATALOG：旧数据回填、严格证明、生成契约/权限/审计目录；停旧节点顺序见运维手册 | 待用户确认 |
| MFA-059 | 浅深色、窄屏、键盘、粘贴、恢复码保存 | BROWSER、POLICY-UI：桌面/375px、深浅主题、键盘、字符串输入、主动复制与下载；安全截图 | 待用户确认 |
| MFA-060 | 全部五项交付复核 | CHECK、GO、BROWSER、HTTP-DEPLOY、TLS、CATALOG 均通过；运维及恢复手册一致，隔离验收环境和私有凭证文件可用，60 项证据齐全 | 待用户确认 |

## 命令与结果

运行环境：macOS arm64，Go 1.27.0，Node 26.10.0，pnpm 12.18.0，Playwright 1.65.0；隔离 PostgreSQL、Redis、Docker 和本机 Chromium。所有因素及密码均为合成夹具，不接触现有业务账号。

| 命令 | 范围和当前结果 | 证据 |
| --- | --- | --- |
| `pnpm check` | 通过：Go 检查、ESLint、TypeScript、OpenAPI/领域契约、权限目录 | [.artifacts/mfa/delivery/check.log](../../.artifacts/mfa/delivery/check.log) |
| `INFRAPRO_GO_TEST_JOBS=4 pnpm test:identity` | 通过，99 批次、486 项通过、25 项条件跳过；0 失败 | [Go 结果](../../.artifacts/go-tests/2026-11-17T10-01-19.678Z/result.json) |
| `pnpm test:mfa` | 通过，五组：内存凭证、真实数据库与快照、HTTP/TLS/PTY、机器进程装配、6 个桌面/手机浏览器流程 | [MFA 结果](../../.artifacts/mfa/2026-11-17T10-33-50.041Z/result.json) |
| `pnpm test:http` | 通过，11 个真实 Docker HTTP/代理/恢复用例 | [HTTP 日志](../../.artifacts/mfa/delivery/http.log) |
| `pnpm test:https` | 通过，可信 TLS、Cookie/CSRF/Origin、真实 MFA 流程、人员流在 30 秒内停止、机器终端仍获授权 | [HTTPS 结果](../../.artifacts/https-2026-11-17T10-05-19.545Z/result.json) |
| `pnpm audit:catalog` | 通过，422 项契约、146 处来源 | [目录结果](../../.artifacts/mfa/delivery/audit-catalog.log) |
| `pnpm test:e2e` 与 `pnpm test:e2e --last-failed` | 全量 273 项：首轮 189 通过、28 条件跳过、56 失败；全部失败项复跑为 55 通过、1 条件跳过，退出码 0；合并覆盖 244 通过、29 条件跳过、0 未解决失败 | [首轮日志](../../.artifacts/mfa/delivery/e2e-full.log)、[复跑日志](../../.artifacts/mfa/delivery/e2e-retry.log)、[最终状态](../../.artifacts/mfa/delivery/e2e-final-last-run.json) |
| `pnpm mfa:lab` 及内置自验 | 两 API、跨空间权限、通知实际送达、P1/U5/U6 页面与状态核验 | [当前索引](../../.artifacts/mfa/manual/2026-11-17T10-39-20.728Z/session.json)、[页面核验](../../.artifacts/mfa/manual/2026-11-17T10-39-20.728Z/ui-check.json) |

全量 Go 的 25 项跳过包含浏览器/人工服务器、专用快照/CLI 及另行开启的领域环境用例；本次 MFA 的快照、CLI、跨实例和浏览器已由 `test:mfa`、HTTPS 与 lab 实际运行。浏览器全量的 29 项为条件跳过，其中复跑的 1 项仅适用于桌面断点；未将跳过记为执行通过。初次全量收集后新增的 MFA 多标签用例由最终专项覆盖。

首轮失败涉及旧夹具重复登录、角色选择分页及审计查询时间范围，已修正并复测；构建期间的移动端资源竞争超时也在复跑中消除。首轮失败记录保留，未将首轮命令改记为成功。最终 `pnpm check`、审计目录同步及 `git diff --check` 均通过。

## 主工作区分支集成

2026-11-17 按用户要求，以主工作区现有 `main` 的生产基线为起点创建 `develop`，集成 155 个 MFA 相关文件。保留原生产版本说明、部署目录及其未提交资料；不修改 `main` 引用，不执行生产部署。功能状态继续为 Awaiting acceptance。

主工作区再次通过 `pnpm check`、`pnpm audit:catalog` 和 `pnpm test:mfa`：五组专项检查全部通过，含 6 个桌面／手机浏览器流程。功能源码与前述完整自验版本一致，路由生成顺序差异已核对语义并由浏览器验证；完整 Go、全站浏览器和 Docker HTTP／HTTPS 沿用上方证据。集成检查见[分支集成记录](../../.artifacts/mfa/develop-integration/result.json)，新专项结果见[MFA 复测](../../.artifacts/mfa/2026-11-17T10-55-02.330Z/result.json)。

## 人工验收

当前验收入口：[本机控制台](http://127.0.0.1:58558)。账号取得方式及两个 API、人员大屏入口见上方当前索引；两 API 的 readiness 和控制台已核验可用，P1/U5/U6 页面自验通过。

重新启动入口：在本 checkout 执行 `pnpm mfa:lab`，等待输出入口及 `session.json` 路径，以新索引为准。索引中的 `credentials_file` 指向仓库外 `0600` 随机凭证文件。停止脚本会删除该次私有资料和隔离 schema。不要重复 bootstrap 实际环境。

1. 用 U1 登录 `/security`，扫码真实验证器并保存恢复码；退出再登录应出现第二步。错误新码不会完成绑定。
2. 保持登录，重建恢复码、更换验证器、关闭 MFA。应只提示当前密码复验及新绑定码，无旧动态码验证；旧码组和旧会话按操作规则失效。
3. P1 未绑定时开启全局强制，应保存成功后退出；原 U1/TA 密码会话也失效。重新登录须绑定。P3 只可查看，TA 不能修改策略。
4. P1 重置 P2/U2，目标通过专用链接、本人密码和新因素完成恢复。P2 完整 MFA 登录后开关策略不重复验证 TOTP；关闭强制不清 U2 因素，也不恢复旧密码会话。
5. U3 同一因素跨 A/B 和项目切换；U4 无租户也受强制；U5 重置后仍停用；U6 可管理本人 MFA，但没有其他未授权个人/平台功能。
6. 逐项演练恢复码一次消费、旧组失效、管理员恢复、唯一管理员维护 CLI、人员 CLI、人员大屏撤销和机器终端保持。隔离环境做故障与数据库恢复后所有人员须受控重新绑定。
7. 核对安全记录、审计与通知中的操作、对象、原因及成功事实，确认没有密钥、验证码、恢复码和恢复链接。

生产密钥分发、真实手机扫码、跨物理主机时间同步、生产负载/长期运行和生产发布尚未执行。本机两实例并发及真实代理检查不等同这些条件；不作性能或生产可用性承诺。
