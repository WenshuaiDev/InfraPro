# 项目资料 A 方案改造与验收

日期：2026-11-15。状态：**Awaiting acceptance（实施与自验完成，待用户人工验收）**。用户选定 A「项目简报」，本次交付是正式页面改造，不包含新的监测业务或项目统计。尚未记为人工 Accepted。

## 已确认方案与实现

- 保持原项目资料路由。项目名称、运行状态和编号集中在页头，编辑为主操作；归档、恢复、交接等管理操作收进「更多」，责任人旁保留就近操作。
- 按业务目的与说明、计划周期、责任分工、项目附件、更新时间与变更记录排列。所属租户与切换项目入口压缩为顶部辅助信息，不再重复展示完整项目标题。
- 计划使用真实日历日期，不虚构完成率、阶段或监测指标；到期不会自动归档。业务负责人和项目管理员并列呈现，各自的业务职责及管理权限保持原有区别。
- 归档状态显示原因、时间和受限范围；管理员空缺／不可履职及负责人不可用时显示针对性处理提示。正常状态不单独占据可用性字段。
- 移除资料页的当前项目授权、原始权限及来源列表。权限说明仍在成员、角色等对应管理页面维护；无资料查看权时只提供访问提示和重新选择项目入口。
- 项目附件使用紧凑列表；详情、真实预览和上传处理结果在原页面的系统遮罩抽屉打开，URL 保留附件、修订、上传或任务身份，刷新可恢复。附件读、下载、管理资格仍分别校验。
- 编辑、负责人变更、管理员交接、归档和恢复复用现有表单及真实接口，保留原因、影响预览、版本冲突检查和密码复验。归档后停止编辑、负责人变更及新增附件，继续按授权查阅、交接与恢复。
- 临时上下文失败保留编辑草稿并禁止提交；资料读取被拒绝或上下文失权时清除受保护的资料和已打开的附件。后台刷新期间仍可把键盘焦点返回管理入口，实际写操作继续被阻止。
- 修复管理员交接后上下文拒绝读取引发的重复取消／重查：失权事件继续清理项目业务缓存，但允许当前上下文请求落定，页面能明确显示不可访问并提供项目切换入口。

技术：React、TypeScript、TanStack Router/Query、现有共享组件与 Lucide 图标。布局和主题使用项目现有 Tailwind/CSS 变量；未新增依赖、数据库结构或后端接口。测试使用 Playwright 与独立后端实验环境。

原型依据：`/Users/hijacker/.codex/visualizations/2026/11/15/01a1de56-1cfe-7bf1-8827-778c15b7c504/project-profile-prototype` 中的 A。原型模拟逻辑未引入产品代码。沿用当前分支及工作区，没有创建分支／工作树，没有提交、推送、部署或迁移。

## 自验记录

证据统一保存于 `.artifacts/project-profile/`。只将完成且有结果证据的检查记为通过；真实后端写入与前端响应注入分别记录。

| 检查 | 状态与证据 |
| --- | --- |
| Console 类型检查与生产构建 | 已通过；`typecheck.log`、`build-final.log`。最终独立产物 `console-dist/`，构建仍提示已有大包与工具弃用警告。 |
| 改动文件 ESLint 与差异检查 | 已通过；`lint.log`、`lint-final.log`、`restriction-lint.log`，`git diff --check` 无错误。 |
| A 方案专项浏览器流程 | **10 条通过**；`browser.log`、`browser/result.json`。覆盖真实编辑及日期约束、负责人变更、附件上传／预览／下载摘要／深链接、四组独立项目角色、归档恢复、管理员交接、主题／窄屏、异常与失权清理，无浏览器运行时错误。后台刷新焦点丢失与交接后重复查询均已修复并复测。 |
| 文件与附件回归 | 22 条通过；`files-browser.log`，原结果 `.artifacts/files/browser/result.json`。 |
| 项目与工作空间桌面／手机回归 | **27 个唯一用例具有通过证据**。首轮 25 通过、2 项旧审计断言失败；改为统一审计页与详情抽屉中的真实归档原因后，最终项目完整流程及故障／迟到响应／复验／撤权共 4 项复测通过。日志：`project-e2e.log`、`project-e2e-serial.log`。另 1 项仅桌面的 768px 断点测试在手机按设计跳过，不计通过数。 |
| 关联监测数据回归 | **桌面、手机 2 项通过**，已包含在最终 `project-e2e-serial.log` 的 **6/6** 结果中。真实归档仍可按授权读取及导出，恢复、撤销数据授权和迟到响应清理均通过。 |
| 现有 25173 环境只读核对 | **7 项通过**；`acceptance/result.json`、`acceptance-final.log`。真实资料、3 个附件、PDF 预览与刷新、抽屉取消和焦点、成员／历史链接、主题与窄屏；前后资料及附件 JSON 一致。最终一轮没有 HTTP 失败或浏览器运行时错误；桌面、320／375px、深色及抽屉截图已目视核对。 |

复核命令（仓库根目录）：

```sh
pnpm --filter @infrapro/console typecheck
pnpm --filter @infrapro/console build --outDir /Users/hijacker/Documents/ChatGPT/InfraPro/.artifacts/project-profile/console-dist
INFRAPRO_FILES_SKIP_BUILD=1 INFRAPRO_PROFILE_CONSOLE_DIST=/Users/hijacker/Documents/ChatGPT/InfraPro/.artifacts/project-profile/console-dist node scripts/project-profile-browser-check.mjs
INFRAPRO_FILES_SKIP_BUILD=1 node scripts/files-browser-check.mjs
INFRAPRO_E2E_PREVIEW=1 node scripts/e2e.mjs tests/e2e/projects.spec.ts tests/e2e/workspace-navigation.spec.ts tests/e2e/workspace-switcher.spec.ts --project=desktop --project=mobile --reporter=list
INFRAPRO_MONITORING_E2E=1 INFRAPRO_E2E_PREVIEW=1 node scripts/e2e.mjs tests/e2e/projects.spec.ts tests/e2e/monitoring-data.spec.ts --grep 'project complete UI|project stale responses|project data grants' --project=desktop --project=mobile --workers=1 --reporter=list
```

文件回归与 E2E 使用默认 `apps/console/dist`，重跑前可用常规 console build 更新。专项检查使用独立产物，避免与同工作区其他构建相互覆盖。最后一轮默认构建见 `build-default-final.log`，与专项产物均包含交接后的失权处理修复。

中间一次双 worker 复测为 5/6，手机用例在 5 秒内未显示审计详情原因；最终使用单 worker 完成相同 6 项，全部通过。中间日志 `project-e2e-final.log` 保留，没有将其记为整批通过，也未改写审计模块。最终结果按唯一用例统计为：10 条页面专项、22 条文件回归、29 项项目／工作空间／关联监测 E2E，另列现有环境 7 项只读核对。

现有环境核对过程中曾遇到文件正文 GET 503，错误面板保留「重试预览」，未改写文件或绕过错误。证据保存在 `acceptance-transient-result.json`、`acceptance-transient.log`；后续最终完整只读复测正常，PDF 连续打开、刷新及再次打开均成功。该记录不等同于长期后端稳定性证明。

写入仅发生在独立测试后端；现有验收项目只读核对。专项中的长文本、管理空缺、临时故障和资料拒绝响应使用明确注入；资料编辑、责任人变更、四组项目角色、归档、恢复及管理员交接使用真实接口。未验证生产部署、Safari、物理手机或压力／长时间运行。

## 人工验收

入口：<http://localhost:25173/t/a78cdb12-b224-4869-9cb7-c7c1f76d0c8b/p/2381620c-bd82-43aa-9ba3-09891aedb746/profile>。沿用现有验收账号。当前前端为本工作区 Vite 服务，刷新可查看更新。

| 步骤 | 预期 |
| --- | --- |
| 打开项目资料 | 项目名称只作为一个主体标题，依次显示业务目的、计划周期、责任分工和紧凑附件；无权限清单、虚构统计或完成率。 |
| 编辑资料后取消，或填写原因保存 | 系统遮罩抽屉承载原有字段；取消不写入，成功保存后刷新仍可见。开始晚于结束不能提交。 |
| 变更业务负责人 | 搜索本租户合格候选人，先预览再填写原因确认；仅改变业务责任，不自动赋予项目成员或角色。 |
| 查看附件，打开后刷新，再关闭 | 当前页面打开实际内容与详情，刷新保留同一附件／修订，关闭返回列表。下载和管理操作按独立资格显示。 |
| 从「更多」归档项目（建议使用临时项目） | 显示真实原因与时间；资料编辑、负责人变更、新增附件不再提供；仍可按资格查阅、交接和恢复。 |
| 恢复项目或交接管理员（建议使用临时项目） | 均要求真实影响预览及原因；恢复不补回已结束关系；交接使原管理员退出项目，保留租户关系且不改变业务负责人。 |
| 点击项目成员、变更记录 | 跳转本项目；变更记录预先限定当前项目对象。无对应读取资格时隐藏入口。 |
| 使用只读／无附件下载资格账号 | 隐藏不获准操作，不能通过资料读取资格获得文件正文；资料失权后不继续展示缓存。 |
| 320／375px、深色主题及 Tab/Enter/Escape | 内容可顺序阅读，无整页横向溢出；操作抽屉在手机从底部打开，关闭恢复焦点。 |

人工验收：**待用户确认**。
