# 工作空间导航重构自验记录

日期：2026-11-11。状态：**Awaiting acceptance**。自验结论：NW-01～NW-10 已实现，SV-01～SV-21 覆盖完成。人工验收：未开始。

## 基线与边界

用户授权实现[完整方案](../requirements/workspace-navigation-refactor-plan.md)。工作树原为 `e2620cd`，缺少方案所需前端；已从正式已提交基线 `4e92f95` 创建 `codex/workspace-navigation-refactor`，带入指定方案及其历史文档检查。独立实现时没有复制主工作区的其他未提交迭代；后续按用户指令合回主工作树，整合记录见末节。

实现与回归测试提交：`5d42bbc`；原交付文档提交：`6ff48cd`。2026-11-11 已通过 `eca8826` 合入主工作树，随后与其他实现统一进入 `main`；最新验证和远端同步状态见[主分支整合记录](main-integration-20261111.md)。

本轮仅修改前端、浏览器测试和文档。HomePage 源码保持不变；Go API、OpenAPI、数据库迁移和依赖版本均无改动。未采用原型分支。

## 独立实现验证结果

| 命令 / 范围 | 实际结果 |
| --- | --- |
| `pnpm check` | 通过：Go 格式 / 静态检查、ESLint、workspace / 测试 TypeScript、OpenAPI 与权限目录同步。 |
| `pnpm --filter @infrapro/console build` | 通过；生产路由拆包正常。 |
| `pnpm test:ui` | 最终 9 通过，覆盖 plain / workspace、桌面 / 1024px / 375px、主题、顶部账号与键盘。 |
| `pnpm test:e2e workspace-navigation.spec.ts workspace-switcher.spec.ts presentation.spec.ts account.spec.ts account-state.spec.ts route-error.spec.ts tenant-management.spec.ts tenant-membership.spec.ts tenant-lifecycle.spec.ts tenant-role-authorization.spec.ts projects.spec.ts tenancy.spec.ts access.spec.ts account-data.spec.ts resources.spec.ts` | 首次 121 项：113 通过、5 失败、3 条件跳过。失败均已定位并在下述复验中通过，未改变业务断言。 |
| 导航、租户创建、成员邀请和角色完整 UI 专项复验 | 23 通过、1 条件跳过；剩余 2 个租户 UI 用例由下行最终复验关闭。新增验证邀请取消凭证清理、治理全部筛选、迟到身份请求。 |
| `pnpm test:e2e tenancy.spec.ts -g 'tenant UI creates' --workers=1` | 2 通过；桌面 / 手机创建、编辑、交接、旧会话失权和管理员停用保护。 |
| `pnpm test:e2e:preview workspace-navigation.spec.ts workspace-switcher.spec.ts route-error.spec.ts` | 最终 27 通过、1 条件跳过；包含生产 chunk 加载失败恢复。跳过的手机项目条目仅避免重复执行 768px 专项，768 / 1024 / 1440 已在桌面项目内逐一运行。 |
| `pnpm test:http` | 9 通过；真实 Nginx HTTP `/console/` 构建、旧别名与业务深链接、邀请、项目归档与租户限制。测试容器及专用卷已按脚本回收。 |
| `git diff --check` | 通过。 |

共享 helper 的额外调用方 access、account-data、tenancy、resources 已包含在集成回归中。开发模式的 2 个 route-error 跳过已由生产构建用例实际补验。

修复及复验记录：

- 弹层的 aria-labelledby 曾覆盖显式名称；现在提供独立的可访问菜单名称。
- Portal 尚未建立焦点时执行普通 effect 会丢失搜索框焦点；改为实际焦点事件后的 microtask，手机先等待外层 Sheet 打开焦点事件。
- 旧租户用例仍点击列表链接，已改为选择器；刷新后的 helper 先等待身份外壳就绪，再判断是否需要手机抽屉，并兼容抽屉使外层账号入口 aria-hidden 的情况。
- 迟到请求测试在退出完成前重新登录导致竞态；现在等待真实退出完成。迟到搜索及身份切换后的旧请求均被验证不能回填。
- 完整角色 UI 在首次并行回归中达到 30 秒总时限；保持业务断言与时限不变，专项桌面 / 手机复跑通过。
- UI preview 遇到一次 Vite 页面重载；停止同时重生成路由文件后，最终 9 项通过。共享演示的链接改用接收 Slot 事件的组件，保留原生链接与菜单关闭语义。

原运行日志、8 张亮暗 / 桌面手机截图、浏览器报告及 HTTP 证据，已随主分支整合保存至主目录 `.artifacts/main-integration-20261111/5695-InfraPro/local-artifacts.tar.gz`（本地产物，不纳入 Git）。归档保留原工作树内 `build/verification/workspace-navigation/`、`playwright-report/` 和 `test-results/` 路径，并已完整读取核验。

## 实现与 SV 覆盖位置

| 检查 | 实现 / 浏览器证据 |
| --- | --- |
| SV-01～02 | `plain account pages`：普通登录、首页卡片、无侧栏 DOM、顶部工具、无偏好读取及快捷键写入；HomePage 文件零差异。 |
| SV-03～04 | `workspace hierarchy`、`business deep links`：租户 / 项目 / 平台菜单隔离、侧栏位置、刷新、折叠偏好回到业务空间恢复。 |
| SV-05 | plain 及共享 UI：唯一顶部账号入口，资料 / 安全 / 首页、退出失败后重试。 |
| SV-06～07 | `picker searches the server`：名称 / 编号经服务端查询命中未加载项，独立游标，去重，类型与归档条件重置首游标，项目状态变化保留租户页。 |
| SV-08～09 | `independent retry`、`unavailable sources`、分页用例：来源失败不展示旧交互结果，不伪装为空；父租户仅标签，独立项目仍可进入。 |
| SV-10 | 生命周期回归与选择器状态动作：真实停用状态只读最小信息；归档、暂停和失权继续受原路由边界限制。 |
| SV-11～12 | `legacy aliases`、两个 `anonymous alias`、账号安全回跳用例：replace、消费参数、历史、跨登录选择类型、非法外部回跳。 |
| SV-13 | `business deep links`、`workspace hierarchy`、tenancy：登录深链接、刷新、前进后退、不同标签页独立范围。 |
| SV-14 | `late searches and an identity change`、account-state / account-data / access：延迟搜索及旧身份请求、退出 / 权限失效清理。 |
| SV-15 | `unavailable sources`、生命周期 / tenancy、presentation、生产 route-error：受限页原地重新选择，404 返回，异常页顶部账号可用。 |
| SV-16 | `tenant governance`、projects、tenant-management：创建、负责人 / 失管 / 状态筛选、治理摘要、平台列表及详情。 |
| SV-17 | tenant-membership：登录不自动接受；取消后选择器可达、凭证清除，重新打开原邀请后才可明确确认。 |
| SV-18 | 手机 `business deep links`：正文打开抽屉→焦点准备→选择器；取消回到真实触发器，选择关闭两层且主区域可操作。 |
| SV-19 | desktop / tablet / mobile + 独立 768px 用例；亮暗截图、Space / Enter / 方向键 / Escape、输入左右键、无横向溢出。 |
| SV-20 | 实际 Docker / Nginx `/console/` HTTP 构建：两个旧别名、首页、个人资料和业务深链接、邀请。 |
| SV-21 | `git diff` 确认 HomePage、Go、契约及 lockfile 零差异；UI preview 仍为开发独立入口，HTTP 测试验证未进入产品 HTML。 |

## 验证边界

搜索 / 分页 / 单侧故障 / 迟到响应测试使用真实账号及明确的响应模拟；业务资格、租户停用、归档和成员授权由现有真实 API 回归验证。两个来源分别传 AbortSignal，按账号、筛选隔离，并复用既有资格失效前缀。没有新增后端权限语义。

HTTP 测试使用独立测试库和容器；导航功能本身没有数据库迁移。已启动的人工预览则使用原有本地库及账号，入口和停止方式见[操作手册](../operations/workspace-navigation.md)。

人工 MA-01～MA-15 尚未由用户执行 / 确认；生产未发布，未验证其他浏览器内核及真实手机硬件（手机测试使用 Chromium 375px 仿真）。构建保留约 522 kB 主 chunk 的 Vite 体积提示，构建成功；此轮不扩展到打包性能重构。

## 合并回主工作树

2026-11-11，按用户指令将 `5d42bbc`、`6ff48cd` 以合并提交 `eca8826` 合入 `/Users/hijacker/Documents/ChatGPT/InfraPro` 的 `feature/tenant-project-engineering`。保留此前已合入的项目资源功能，手动合并项目治理列表并保留“资源授权”入口；README 保留当前资源版本说明和导航说明。

合并前的 48 个修改 / 新增文件均已恢复：43 个逐字节一致；README、单账号验收手册、租户浏览器测试、HTTP 部署测试及导航方案按两边内容整合。两个原未跟踪的导航方案文档现已纳入导航提交，其余原修改继续保留未提交状态。原始补丁、未跟踪文件归档及逐文件核对清单保存在主目录 `.artifacts/workspace-navigation-integration/`。

整合后的 `pnpm check`、console 生产构建及 HTTP 部署测试（9 项）已通过。追加的生产预览浏览器回归执行期间，用户明确要求“无需验收，合并完成即可”，因此停止剩余验证；不将本次部分浏览器结果计为整套通过，也不据此将功能状态标记 Accepted。未推送、未部署生产、未迁移原业务数据库，现有服务与数据保留。
