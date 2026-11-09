# 前端重构实施与验证记录

日期：2026-11-09。状态：**Awaiting acceptance（等待用户验收）**。已按[更新后的方案](../requirements/frontend-refactor-plan.md)完成实现与自验；用户验收尚未进行。

## 实施基线

- 保留当前未提交的账号、HTTP 部署及维护 CLI 实现，在其上迁移。
- 基线源码与文件摘要保存在本机临时目录 `infrapro-frontend-baseline-7_zcrs_b`，未重置或覆盖工作区。
- shadcn-admin 固定为 `e16c87f213a5ba5e45964e9b67c792105ec74d26`；已下载固定源码，许可证保存在 `third-party/shadcn-admin/LICENSE`。
- shadcn Chart 来源固定为 shadcn/ui `8ab27715521cbd4f87ae6886cd989cbb678f6de7`，许可证保存在 `third-party/shadcn-ui/LICENSE`。

## 迭代状态

| 迭代 | 状态 | 结果 |
| --- | --- | --- |
| 0 基线 | 已记录 | 当前源码备份、工具链、方案要求和现有账号测试已核对 |
| 1 UI 与图表 | 自验通过 | 23 个参考组件、通用表单/表格/反馈、5 类图表；类型检查、lint 与组件浏览器检查通过 |
| 2 布局与 Zustand | 自验通过 | Admin/Auth/Settings 布局、导航搜索、主题、Zustand 侧栏；三视口共 6 项测试通过 |
| 3 路由与认证 | 自验通过 | TanStack 文件路由、身份适配、登录/找回/邀请/重置均接真实接口；fragment 在 Router 初始化前清除 |
| 4 账号页面 | 自验通过 | 资料、邀请、密码、会话和安全记录使用共享组件；真实游标分页及延迟响应隔离检查通过 |
| 5 单应用与部署 | 自验通过 | screen 目录、依赖、启动进程、镜像产物及契约已移除；旧地址兼容、CSP 与本地启动已验证 |

## 实现结果

- `packages/ts/ui`：23 个参考基础组件、共享 Sonner、RHF 字段/密码控件、反馈状态、确认弹窗、TanStack DataTable、主题和 Recharts 五类图表。旧 AppFrame、账号专用表单和手写样式已清除。
- `packages/ts/layouts`：AdminLayout、AuthLayout/AuthCard、SettingsLayout、PageHeader/PageContent；链接渲染由应用适配。Zustand 已受控管理侧栏，持久化仅含非敏感偏好。
- `apps/console`：唯一应用；新 Router/Query/RHF/Zod 装配，保留用户名密码、邀请开户与维护恢复。资料成功提示使用 Sonner，字段错误就近显示。
- 启动、配置、OpenAPI、客户端类型、Compose、Docker、Nginx、测试与文档统一收敛。旧 screen 配置键兼容读取并提示弃用；旧路径固定站内跳转，不再部署第二个页面或监听 5174。
- 既有工作区改动保留；无数据库迁移、提交或推送。本机 `.env` 只清理退役 screen 的相关项，数据库、Redis 和 CSRF 材料未重建。

开发约定见[共享包用法](../frontend-development.md)，来源与全部本地适配见[来源记录](../../third-party/README.md)，部署迁移见[运行手册](../operations/account-management.md)。

## 命令与运行证据

| 检查 | 实际结果 | 覆盖范围 |
| --- | --- | --- |
| `pnpm check` | 通过 | 全工作区 Go 格式/vet、ESLint、TypeScript、生成契约同步；包括测试文件 |
| `pnpm build`，后续 `pnpm --filter @infrapro/console build` | 通过 | Go API/CLI 与唯一前端；最终前端修改已重建 |
| `pnpm test:identity` | 通过 | foundation/platform/API/CLI 的 Go 测试与 race 检测，含退役配置兼容、真实隔离 PostgreSQL/Redis |
| `pnpm test:ui` | 6 项通过 | 三视口；布局、主题/系统变化/持久化、Zustand reset、五类图表、键盘 Tooltip/Legend、数据状态、弹窗/搜索焦点、密码显示 |
| `pnpm test:e2e` | 25 项通过 | 桌面/手机完整账号与数据测试，三视口真实页面展示检查 |
| `pnpm test:e2e:preview` | 27 项通过 | 构建产物下复跑账号测试，并增加路由 chunk 失败、保留身份布局与重载恢复检查 |
| `pnpm test:http` | 3 项通过 | Docker 内构建 `/console/`，LAN 普通 HTTP、真实 CLI/DB/Redis、旧地址/外部端口、CSP、依赖故障与日志脱敏 |
| 产物与依赖核对 | 通过 | apps 仅 console；dist 仅 index.html 入口；无测试预览/固定图表数据的 JS；React/ReactDOM 均为 19.3.1 |
| 本地运行 | 通过 | 已重启本仓库 `pnpm dev`，8080 API + 5173 console，5174 无监听；readyz 200，公开帮助响应不再含 screen_url，登录页浏览器无未捕获异常 |
| `git diff --check`、文档本地链接检查 | 通过 | 无空白错误，当前文档链接可解析 |

配置解析及后端测试保持严格字段检查；遗留 `public_screen_url` 是有意保留的弃用兼容项。HTTP 测试栈运行后自动删除，只保留本地开发环境与专用账号测试依赖。测试产生的账号和数据位于隔离 schema/Redis 前缀，未写入本地业务账号库。

本轮发现并修复：Nginx 默认绝对跳转丢失外部发布端口，改为相对跳转；Radix/Sonner 的动态 style 元素被 CSP 拦截，仅为 style-src-elem 增加对应支持；Zod JIT 探测触发 eval，改为统一 jitless 入口，脚本策略继续 self-only。命令搜索与确认弹窗都以真实 trigger 归还焦点，路由资源失败提供保留身份场景的错误页及完整重载恢复。

工具链仍输出上游非阻断提示：Node 26 的 module.register 弃用、TanStack CLI 的循环依赖属性提示，以及 pnpm 的可选 agent skills 待批准提示。未批准依赖包提供的 agent skills；安装、构建与测试不依赖它们。

## 视觉对照与合理差异

参考图：[上游后台](../../.artifacts/frontend-refactor/reference/dashboard.png)、[上游默认登录](../../.artifacts/frontend-refactor/reference/sign-in.png)。固定源码参数核对：桌面侧栏 16rem、折叠 3rem、手机抽屉 18rem、顶部栏 4rem、根圆角 0.625rem；同套 slate/new-york 变量、Inter、本地中文字体回退、卡片和表格。

实现截图保存在 `.artifacts/frontend-refactor/`：`dev/`、`preview/`、`ui/`、`http/`，索引为 `screenshots.json`；`local-login.png` 是当前本地运行入口。这里的浏览器视口为 1440×1000、1024×768、375×812，覆盖浅/深色登录、验证错误、资料、安全、邀请、侧栏和共享图表。截图由实际渲染检查取得；并非批准像素差异的自动基线。

已人工检查参考与实现截图：后台 inset 框架、品牌、侧栏/顶部栏位置、标题层级、卡片、控件和主题保持同一视觉基准；表格在自己的容器内滚动，手机没有整页横向溢出。登录卡片桌面最大宽度 384px，按原结构居中。以下差异按已确认业务适配保留：

- 品牌和文案使用 InfraPro/中文，登录字段是用户名；删除未接通的社交按钮、公开注册、邮件/OTP 和虚构协议。卡片高度随实际内容缩短，仍居中。
- 团队位置显示组织品牌；导航只显示首页、资料、安全、邀请。顶部搜索保留原位置层级，未添加演示客户/产品菜单或布局配置面板。
- 首页显示真实账号摘要和功能入口，不显示模拟收入、设备指标或演示用户。统计图表由独立 UI 测试入口证明共享能力。
- 邀请/安全记录用真实游标加载更多；不显示上游演示总页数、全量筛选或排序。密码恢复显示实际维护联系方式。

## 要求与证据对应

| 验收项 | 本次自验证据 |
| --- | --- |
| FE-01 单应用 | workspace/锁文件/产物检查；本地进程与端口；HTTP 镜像只复制 console |
| FE-02 框架复刻 | 固定源码尺寸/变量/来源记录；参考与实现三视口截图对照 |
| FE-03 主题与布局偏好 | components 测试的持久化、系统主题、折叠/展开/reset；presentation 的真实页面刷新 |
| FE-04 认证页面 | account 测试的用户名登录、邀请、找回/重置、一次性凭证和刷新丢失；认证截图 |
| FE-05 安全回跳 | account 的受保护资料页回跳、外部 returnTo 拒绝；HTTP 深层刷新 |
| FE-06 邀请 | 创建/复制/认领/复验/重新生成/撤销；HTTP 手动复制；account-data 的 51 条真实游标记录与重试 |
| FE-07 资料/密码 | 资料保存、Unicode 码点校验、修改/恢复密码、字段错误，真实接口与维护 CLI |
| FE-08 会话与身份隔离 | account-state 的跨标签页草稿/链接/记录清理、当前 Cookie 更替、迟到写响应；account-data 的迟到读响应 |
| FE-09 响应式与键盘 | 三视口 UI/presentation，无整页横向溢出；命令搜索/确认弹窗焦点归还、Escape、RHF 首错焦点、手机抽屉 |
| FE-10 故障 | 503 保留身份与草稿、手动恢复、CSRF 不重放写请求；游标下一页失败保留已有行；HTTP 真实依赖停止/恢复 |
| FE-11 部署路径 | `/console/profile` 深层刷新、旧 screen 路径固定目标、API 404/503 JSON、相对跳转保留发布端口 |
| FE-12 共享复用 | apps 的公开包导入、ESLint 依赖方向、业务配置与组件分离；产物无 UI 预览演示数据 |
| FE-13 图表 | 5 类共享图形导出，三视口、Tooltip/Legend 键盘、主题、尺寸与数据状态；文字图例和可访问数据表 |
| FE-14 Zustand | store 单一持有侧栏状态；持久化白名单/reset；账号状态子树与 Query 退出清理约定 |

## 手工验收入口

本地服务已启动：[console](http://127.0.0.1:5173/)；使用现有账号登录。以后重启在仓库根目录运行 `pnpm dev`。新环境按[运行手册](../operations/account-management.md)执行安装、setup 和交互 bootstrap，没有默认密码。

1. 登录后检查侧栏、顶部栏、首页和个人资料；切换主题、收起侧栏并刷新，应保持所选偏好。
2. 修改显示名称，应显示成功通知；新建邀请，在另一浏览器完成开户，再检查邀请使用状态、撤销与重新生成。
3. 打开账号安全，核对会话，尝试退出其他设备；另行验证改密/维护重置会按既有规则退出会话。
4. 将窗口缩到手机宽度，检查抽屉、表单和表格；用 Tab、Escape 和 Ctrl/⌘+K 操作，应看到清晰焦点且能返回入口。
5. 打开[共享能力预览](http://127.0.0.1:5173/ui-preview.html)，查看五类图表、空/加载/错误状态、主题和布局 reset；该入口仅在开发服务器存在。

完整 FE-01–FE-14 预期见[方案验收表](../requirements/frontend-refactor-plan.md#10-用户手工验收表)。用户人工验收尚未进行；自验通过不代表 Accepted。

## 验证边界

自验环境为本机 Chromium、三个模拟视口和本机 Docker 普通 HTTP；未在目标服务器部署，也未替代 Safari/Firefox 或真实手机验收。HTTP 是既有明确的交付范围。此次没有实现新的监测业务、团队/角色模型或业务统计指标。后续组件升级应逐项比较来源记录中的本地适配。
