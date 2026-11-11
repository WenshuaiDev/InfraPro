# 主分支整合与验收记录

日期：2026-11-11。功能状态：**Awaiting acceptance**。本轮代码整合与自验通过；用户授权自验后推送远端并清理其他工作树和分支，远端同步及清理结果另行记录。自验不代替用户人工验收。

## 整合方案

- **What**：保留当前已实现的账号、邀请、角色、租户、项目、工程定义、资源授权及正式导航，纳入所有尚未提交的源码、测试和文档。主目录最终只使用 `main`，本地与 `origin/main` 一致。
- **How**：先核对项目当前及归档会话，保存各工作树源码快照、补丁、文件摘要和 Git bundle；分别提交未提交实现，逐支合并并按最新确认的功能处理重叠；完成整体验证后推送，检查所有待删分支均为 `main` 的祖先，再清理其他分支和工作树。
- **Technology**：Git 保留提交历史并核验分支包含关系；现有 Go/Gin、React/TypeScript/pnpm 工程执行静态检查和生产构建；Go race、Playwright Chromium 与 Docker Compose 在专用 PostgreSQL/Redis 测试环境中验证业务、UI 和 Nginx HTTP 部署。不对生产环境执行部署或迁移。
- **Acceptance**：无未解决冲突或未提交源码；原有有效实现及增量迁移可构建、测试通过；仅一个工作树和本地/远端 `main`；远端主分支提交与本地一致。人工体验验收仍由用户确认。

## 来源核对

已查询本机当前和归档会话列表，并读取本次仓库重新初始化后与该工作空间有关的 40 个会话的最近交付记录；对独立工作树、未提交内容及原型取舍进一步读取用户指令。本机索引为恢复目录中的 `session-inventory.json`。早于 2026-11-09 仓库重新初始化的已废弃实现不重新引入。

| 来源 | 开始时状态 | 整合原则 |
| --- | --- | --- |
| 主工作树 `feature/tenant-project-engineering` | `e555ace`，保留账号、开户邀请和单账号验收数据等未提交改动 | 完整提交并保留所有当前变更 |
| `codex/account-management-redesign`，工作树 `1864/InfraPro` | 基于 `4e92f95`，14 个未提交文件 | 12 个文件与主工作树逐字节一致；两个回归测试保留主目录后续邀请与导航适配，同时纳入该分支历史 |
| `codex/project-resource-management` | `7a5c859`，原 `e49a` 工作树已删除 | 已经包含于主工作树历史；复核 v10 迁移、101 项权限与项目资源实现 |
| `codex/workspace-navigation-refactor`，工作树 `5695/InfraPro` | `6ff48cd`，已通过 `eca8826` 合入，随后 `e555ace` 收尾 | 保留正式导航和主目录的项目资源授权入口；等原会话结束后再整合 |
| `feature/account-access-console` | `776663f` | 已包含于功能分支历史，随主分支整合保留 |
| `feature/prototype-workspace-navigation` | `11f020b`，另有 7 个未提交路径 | 原型会话中用户已否定布局并停止原型，随后确认正式方案；保存全部源码历史，最终代码采用已实现的正式导航，不恢复原型开关、旧布局或对比控件 |
| `main` / `origin/main` | `e2620cd` | 作为汇总后的唯一主分支，保留现有历史 |

基础账号、角色权限、租户基础/成员/角色/生命周期、项目管理、类型定义、工程资源、代码审查修复及既有界面迭代，均已包含于 `6997829` → `776663f` → `4e92f95` 的提交链。后续独立资源、正式导航、账号管理、邀请和验收数据作为本次增量整合。

恢复材料保存在本机 Git 忽略目录 `.artifacts/main-integration-20261111/`；不推送账号凭据、环境配置、数据库或验收截图到远端。

## 合并与保护结果

- 账号管理未提交实现保存为 `318dbfe`；主目录的邀请、账号、单账号验收脚本与文档保存为 `34ab8d2`。
- `1dfa3f3` 合并账号分支：唯一冲突为邀请创建用例，保留主目录已适配的“创建邀请 → 生成链接”流程；其他相同文件无重复改写。
- 原型未提交代码保存为 `fa5144d`；`4c5bade` 合并其历史。四处正式导航/旧原型冲突与其余原型入口，按用户已确认的正式方案解决。原型完整源码可从该提交恢复；最终源码树不引入原型切换器。
- `main` 从 `e2620cd` 快进到整合提交。`branch-containment.json` 记录七个本地分支的提交均是 `main` 的祖先，后续删除分支不会丢失这些提交。
- 初次整合后的 563 个原主目录源码路径与备份逐一核对，无遗漏、无意外改写。之后仅追加本次验收修正和交付记录。
- 三个待删工作树分别归档了 74、63、22 个本机产物，并逐文件完整读取 SHA-256 校验；依赖、可重建 dist 和临时缓存未纳入本机产物归档。源码快照和 Git bundle 另外保存并验证。

## 自验结果

所有日志位于 `.artifacts/main-integration-20261111/`。本表记录本次主目录整合后的实际检查，不复用旧会话的通过结论。

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `pnpm check` | 通过：Go 格式/vet、ESLint、TypeScript、OpenAPI 和权限目录一致 | `check.log` |
| `pnpm build` | 全部 Go module、API/CLI 和 console 生产构建通过 | `build.log` |
| `pnpm test:identity` | 四个 Go workspace module 全部通过，启用 `-race -count=1`，包含真实 PostgreSQL/Redis 集成 | `go-race.log` |
| `pnpm test:ui` | 9/9，通过桌面/平板/手机共享布局、主题、图表及焦点检查 | `ui.log` |
| `pnpm test:http` | 9/9，实际 Nginx `/console/` 部署、CLI、深链接、CSP、业务隔离及恢复流程通过；专用容器和卷已自动清理 | `http.log` |
| `pnpm test:e2e:preview` | 首轮 152 通过、2 个旧测试定位器失败、1 个条件跳过；失败修正与下行复验后，154 个有效场景均有通过证据 | `e2e-preview.log`、`test-results/e2e/` |
| `pnpm test:e2e:preview account-state.spec.ts account.spec.ts --reporter=list --output=.artifacts/main-integration-20261111/account-recheck-results` | 修正后两个相关文件的全部 18 项通过，涵盖桌面/手机账号切换、邀请一次性领取、密码、会话撤销及重认证 | `e2e-account-recheck.log`、`account-recheck-results/` |
| 修正后的测试 ESLint、`tsc -p tsconfig.tools.json` | 通过 | `test-fix-static.log`（成功时无输出） |
| `git diff --check`、文档链接与本机专有文件检查 | 通过；无凭据、`.env` 或 `.artifacts` 文件被跟踪 | 本轮命令记录 |

首轮两个桌面失败均发生在旧用例的 `getByLabel('用途备注')`：它同时命中邀请表单与新增的“搜索用途备注”。失败 trace 和最小实际页面探针均复现严格匹配错误（模糊匹配 2 个，精确匹配 1 个）。已将三处测试定位收敛到当前 dialog 内的精确标签，保留账号切换、凭证清理与邀请领取的全部业务断言，没有修改产品行为或放宽测试超时。产品构建未变化，因此复跑全部受影响的账号文件；其余已通过场景不重复运行。

唯一跳过项为手机项目内重复的 `768px breakpoint` 用例；桌面项目中的该用例已实际检查 768/1024/1440px 和明暗主题，手机其余场景全部执行。另人工查看了桌面邀请列表、768px 深色选择器、375px 首页及权限抽屉截图，无裁切或整页横向溢出。

## 人工验收

统一源码目录为 `/Users/hijacker/Documents/ChatGPT/InfraPro`，后续从 `main` 开发。已有业务环境和验收数据保留；本轮自动测试使用专用测试 schema/容器，没有迁移原开发库或部署生产。

已有项目资源 v10 验收入口为 <http://localhost:35173/>，原账号/邀请单账号验收入口为 <http://localhost:25173/login>。本轮核验了对应 API 的 `/readyz` 均返回 200；这只确认运行中的环境就绪，不将它们冒充本轮整合构建的完整验收结果。最新整合代码的完整验证使用上表的隔离测试环境。

服务停止后，从主目录按[项目资源运行手册](../operations/project-resource-management.md#3-启动和恢复本机验收服务)或[单账号验收手册](../operations/manual-acceptance.md)更新构建并启动，原凭据保留在各自本机文件中。普通开发环境使用 `make dev`，默认 <http://127.0.0.1:5173/>；旧库需先按项目资源手册备份、升级至 v10 并同步权限，再启动匹配的新 API，不重复初始化账号。

| 手工检查 | 预期 |
| --- | --- |
| 在主目录执行 `git status --short --branch`、`git worktree list`、`git branch -a` | 源码无未提交改动；仅主工作树、本地 `main`、`origin/main` 及其 HEAD 别名 |
| 比较 `git rev-parse main` 与 `git ls-remote origin refs/heads/main` | 提交 ID 相同 |
| 登录后查看首页，进入个人资料/账号安全，再切换平台、租户、项目 | 首页无业务侧栏，右上账号入口唯一，业务菜单和数据随当前空间切换 |
| 在平台检查账号列表、角色/权限抽屉、开户邀请搜索与创建弹窗 | 筛选与分页正常，取消不写入；账号切换清空前一账号草稿与邀请凭据 |
| 在项目资源专用环境按授权打开同一设施、测点和通道 | 复用同一资源身份，不越权显示对象；完整写入与撤权步骤见项目资源手册 |

用户人工验收尚未确认；各业务功能保留原有状态，不因 Git 合并自动变更为 Accepted。浏览器自动验收范围为 Chromium 的桌面、平板和手机仿真，未覆盖 Safari/Firefox、真实手机及生产规模负载。构建保留现有约 526 kB 主包体积提示及 Node/Router 工具警告，均不影响本轮检查成功。
