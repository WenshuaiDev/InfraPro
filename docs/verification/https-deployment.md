# B2 应用 HTTPS 适配验证

状态：**Awaiting acceptance（应用 HTTPS 适配范围）**。本项专项与相关回归通过。首次扩大回归发现的问题已按用户后续授权修复，并完成全量遍历、修复后复验与专项补验，见[全仓回归修复报告](repository-regression-2026-11-17.md)。生产安装和用户人工验收未执行。

## 变更与边界

- `config/config.go`、`display.go`、`web.go`：人员地址、可信来源和大屏三入口接受 HTTP／HTTPS；校验协议、Secure Cookie、精确 origin 和规范主机／端口。支持带端口的 IP 入口、DNS 与 IPv6；拒绝浏览器会归一成相同 IP 的别名，禁止把大屏入口加入人员可信来源。
- `display-devtools/src/server.mjs`：真实预览／上传支持 HTTPS，保留 loopback 回调与正常 CA 验证；拒绝带凭据／查询／片段及混合协议地址，拒绝运行接口重定向。
- `deploy/display/https.example.yaml`、`https.nginx.conf`：152 原生部署配置；Nginx 负责 TLS，应用监听本机 HTTP，代理覆盖协议与来源、保留端口、过滤运行 Cookie，资源剥离 Cookie／Authorization 并屏蔽 Set-Cookie。现有 HTTP 模板的转发协议改用 `$scheme`。
- Cookie 写入、登录服务、CSRF 中间件和业务权限沿用原实现，用新增集成验证确认 HTTPS 行为。数据库 migration、业务 DTO 和前端页面未作功能变更。
- 首次扩大回归发现既有测试夹具／页面交互与当前实现不一致。当时诊断改动未纳入 HTTPS 交付，已按快照恢复；后续修复属于上述全仓回归任务，分别记录。

工作在当前 `main`，没有创建分支／worktree；原工作区存在大量先前变更，仅针对本项修改。操作前相关文件快照见 `.artifacts/https-20261117/baseline/`，用于区别本轮与既有修改。

## 验证环境

macOS 本机 Nginx（本轮通过 Homebrew 安装供隔离自检，未注册开机服务）、OpenSSL、Go、Node／pnpm、Playwright Chromium；Docker 中的测试 PostgreSQL、Redis、S3 和 ClickHouse。业务测试使用独立 schema／Redis namespace 与合成对象。

真实 Nginx HTTPS 为 `127.0.0.1:28443/28444/28445`，后端为 HTTP。使用两天临时 CA／IP SAN 证书；Node 和 Playwright API 按 CA 链、IP SAN、有效期正常验证。Chromium 使用仅匹配本次证书的 SPKI 固定，不修改系统信任库，不全局忽略证书错误。未使用真实生产身份或生产服务器。

## 首次 HTTPS 验证证据（历史）

以下保留首次验证的原始结果；2026-11-17 后续修复和最新 HTTPS 复验以[全仓回归修复报告](repository-regression-2026-11-17.md)及 `.artifacts/regression-20261117/` 为准。

| 检查 | 结果／证据 |
| --- | --- |
| HTTPS 配置与来源／CSRF／Cookie Go race 定向测试 | 通过，`https-go.log`；实际隔离 PG／Redis。 |
| Nginx HTTPS 完整业务链路 | 通过，`result.json`、`browser/result.json`、`browser/https-identity.json`。 |
| JS 单元测试 | 100 项通过，`unit.log`，含工具 HTTP／HTTPS 地址与拒绝携带配对凭据重定向。 |
| 全仓静态检查 | 通过，`check-final.log`；最终配置 race 另见 `config-final.log`。 |
| 全功能构建 | 通过，`build.log`，包含全部 Go 程序、前端与独立诊断；最终 Go 产物再构建见 `go-build-final.log`。 |
| 修改涉及的 Go 配置／HTTP／身份／大屏 handler 完整包 | 完整普通模式通过，`http-package-tests-final.log`；HTTPS 定向 race 通过。整包 race 触及 10 分钟总时限（`http-package-tests.log`），剩余任务／租户 HTTP 两项 race 补验均通过（`http-race-remainder.log`）。不把超时的整包运行记为通过。 |
| 大屏独立包构建与打包 | 通过，`pack.log`。 |
| 补充全仓 Go／大屏领域回归 | **未全绿**，详见下节，`identity.log`、`display.log`。 |
| HTTP 大屏浏览器回归 | 44 项通过，`http-display.log`、`http-display-result.json`。 |
| 身份页面桌面／平板／移动回归 | 33 项全部通过，`identity-browser.log`；会话撤销／密码变更、账号状态的先前超时独立复跑通过。 |
| 补充全站浏览器回归 | **未全绿并已中止**：39 通过、19 失败、12 跳过、1 中断、198 未运行；`e2e.log`、`broad-regressions.json`。 |

普通 Go 测试中的 `TestE2EServer` 是常驻浏览器夹具，按设计跳过；已由本轮 HTTPS、大屏 HTTP 和 33 项身份浏览器流程实际启动验证。identity handler 子包没有独立单测，其身份边界由上述 HTTP 和浏览器测试覆盖。

证据根目录为 [`.artifacts/https-20261117/`](../../.artifacts/https-20261117/)。

HTTPS 业务链路覆盖：登录、深层页面刷新、资料写入、邀请与开户、重置与新密码登录、退出及会话清除；四类 Cookie 的 Secure／HttpOnly 与人员 SameSite；错误协议／端口／Origin／Host、缺失或伪造 CSRF、不可信 CA 和错误证书名称均拒绝；大屏上传、草稿、授权、发布、沙箱 iframe、SDK 导航、刷新、发布切换、终端配对、本机模拟、真实预览、工具续传、离线租约与撤权停供、WASM／Worker／WebGL、移动布局；代理观察器实际检查上游凭据剥离和来源头覆盖。

## 首次扩大回归发现项（后续已修复）

以下保留首次扩大检查时的判定，不能把当时失败的运行记作通过。各项已在后续回归任务中完成修复与补验；这仍不代表生产安装或人工验收完成。

| 范围 | 已观察到的失败与判定 |
| --- | --- |
| 租户迁移旧测试 | `TestTenantUpgradePreservesLegacyAndRollsBackIncompatibility` 把最新 `Down()` 当作租户历史回退边界；当前最新迁移可以回退空的新列。诊断时直接验证租户迁移的拒绝行为通过，但该测试修订未纳入 HTTPS 交付。 |
| 解析集成环境 | 全仓 `test:identity` 脚本没有启动真实 parser，解析用例报 `real parser URL required`。本轮相关 HTTP 包复验另行准备隔离 parser；全仓启动脚本未改。 |
| 大屏读取契约 | `TestDisplayReadSpatialRepresentationsAndRelationships` 在已创建测点后仍提交旧结构版本 1；`TestDisplayReadCanonicalCountsAndOpaqueDrilldowns` 在并发负载下出现 `conn closed`。后一项原因尚未完全核实，不归因于 HTTPS，也不标为通过。 |
| 设备／绑定旧 E2E | 产品发布夹具没有按现有接口显式确认分类，返回 400。 |
| 工程／设施旧 E2E | 页面标签、测点归属编辑流程与当前工程结构改版不一致；项目授权摘要新增断面／孔位计数，旧对象全等断言不匹配。 |
| 其他浏览器超时 | 身份会话、账号状态、审计分页出现超时；身份相关用例在本轮 33 项独立回归中通过，审计分页未单独复验，不声称其已解决。 |

首次全仓 Go 和全站 E2E 在确认这些独立失败后停止。失败上下文保存在 `broad-e2e/`，当时的诊断修订保存在 `diagnostic-test-changes-not-applied.patch`。后续用户已授权完成回归修复，最新源码与验证结论见独立报告；此历史记录不替代生产验收。

## 人工验收与未验证项

人工状态：**尚未验收**。按[HTTPS 运维手册](../operations/https-deployment.md)合并配置、安装正式证书，使用安装 CA 的正常浏览器及工具检查三个真实入口。预期为证书验证成功，登录／写入／退出与大屏流程正常，错误来源被拒绝，资源后端不接收人员凭据。

本项未安装四台生产服务器，未签发生产证书，未修改生产信任库／防火墙／时钟；无数据库迁移。真实 IP／DNS／证书续期、目标服务器端口和终端信任需正式安装时验证。B3 全模块代理请求体限额、systemd 发行和联合备份仍由部署清单独立跟踪。本轮不执行生产容量或长时间稳定性认证。
