# 租户资料 B 方案改造与验收

日期：2026-11-15。状态：**Awaiting acceptance（实施与自验完成，待用户人工验收）**。用户已选定 B「租户名片」并授权改造与验收；尚未记为人工 Accepted。

## 已确认方案与实现

- 沿用原租户资料路由。页头突出组织名称，编号降为辅助信息，编辑入口放在页头右侧。
- 主栏显示「关于本租户」和紧凑附件列表；右栏显示管理负责人、资料更新时间、查看成员和变更记录。窄屏按阅读顺序排列。
- 从资料页移除当前账号权限清单、成员关系 ID 和权限来源列表。未获资料查看权时只显示访问提示和工作空间选择入口。
- 正常管理员不再单独展示「可用性」字段；管理员异常时显示联系平台管理员处理的提示。没有新增租户内的管理员交接入口。
- 附件列表保留类型、大小、版本、更新时间与原操作菜单。详情、真实预览、上传处理结果在当前资料页打开，URL 保存附件、修订、上传或任务代次，刷新仍可恢复详情。
- 继续使用已有 React、TypeScript、TanStack Router/Query、共享组件、Lucide 与主题变量；无新增依赖。资料编辑、变更原因、版本冲突、审计、附件读／下载／管理权限均沿用已有接口与规则。
- 资料刷新返回 401/403/404 时隐藏已缓存的资料与附件；临时读取失败保留内容并阻止未核实的资料提交。权限变化继续由现有租户边界清理页面与附件缓存。
- 本次自验发现 320px 顶栏搜索文字导致横向溢出，将搜索文字显示阈值从 320px 调整为 384px；搜索图标与键盘入口保留。

原型只作为设计依据，未将模拟数据、原型切换条或演示逻辑引入项目。当前分支与工作区实施，没有创建分支或工作树，没有生产部署或数据库迁移。

## 自验记录

以下区分真实后端测试、前端响应注入和现有验收环境只读核对，不将模拟响应等同于真实授权变更。

| 检查 | 结果与证据 |
| --- | --- |
| Console 类型检查、生产构建 | 已通过。最终完整构建含 `tsc -b`，日志：`.artifacts/tenant-profile/build-final.log`；独立产物位于 `.artifacts/tenant-profile/console-dist`。构建仍提示已有大包与工具弃用警告。 |
| 改动文件 ESLint、差异格式检查 | 已通过。日志：`.artifacts/tenant-profile/lint-final.log`、`browser-script-lint.log`；`git diff --check` 无错误。 |
| B 方案真实后端浏览器流程 | **9 条通过**。真实编辑、上传、检查、下载摘要、原位预览与深链接刷新、改名、键盘焦点、真实授权矩阵，以及空态／超长内容／管理员异常／失权／403 恢复。1440、1024、720、375、320px 和深色模式通过，无浏览器运行时错误。脚本：`node scripts/tenant-profile-browser-check.mjs`；结果：`.artifacts/tenant-profile/browser/result.json`。 |
| 现有文件与附件浏览器回归 | **22 条通过**。覆盖真实上传、下载摘要、修订、回收恢复、项目隔离、通知、权限、Drawer 焦点与手机布局；日志：`.artifacts/tenant-profile/files-browser.log`。 |
| 租户与成员桌面／手机回归 | **22 个唯一用例均有通过证据**：首轮 16 项通过，修正旧测试后针对失败项复测 4 项及 2 项通过；日志分别为 `.artifacts/tenant-profile/tenancy-e2e.log`、`tenancy-e2e-rerun.log`、`tenancy-http-final.log`。覆盖管理员交接、成员暂停／移除／重新加入、跨租户迟到响应、冲突和临时故障。 |
| 现有 25173 验收环境 | **8 项只读核对通过**：真实 B 布局、无权限清单、原 5 个附件、PNG 原位预览、键盘焦点、浅／深色、375／320px 无溢出、核对前后资料与附件 JSON 一致。结果：`.artifacts/tenant-profile/acceptance/result.json`，同目录保存截图；未发现浏览器运行时错误。 |

旧测试同步更新了资料页展示断言、统一审计表格与筛选方式、身份命令的 `X-Command-ID`，以及邀请多步流程的超时预算；未删减服务端授权断言。初次发现的 320px 顶栏溢出已修复，后续布局检查通过。

核心复核命令（仓库根目录）：

```sh
pnpm --filter @infrapro/console typecheck
pnpm --filter @infrapro/console build --outDir /Users/hijacker/Documents/ChatGPT/InfraPro/.artifacts/tenant-profile/console-dist
INFRAPRO_FILES_SKIP_BUILD=1 INFRAPRO_PROFILE_CONSOLE_DIST=/Users/hijacker/Documents/ChatGPT/InfraPro/.artifacts/tenant-profile/console-dist node scripts/tenant-profile-browser-check.mjs
INFRAPRO_FILES_SKIP_BUILD=1 node scripts/files-browser-check.mjs
INFRAPRO_E2E_PREVIEW=1 node scripts/e2e.mjs tests/e2e/tenancy.spec.ts tests/e2e/tenant-membership.spec.ts --project=desktop --project=mobile --reporter=list
```

最后两条使用 `apps/console/dist`；本轮回归使用首次完整构建的产物，B 方案专项使用最终独立产物。重新运行前可用常规 `pnpm --filter @infrapro/console build` 更新默认产物。未为已通过且未受后续修改影响的用例重复执行。

测试写入使用独立的后端测试数据；现有验收租户仅作只读核对。B 方案专项检查中的管理员异常、超长内容、读取失败与前端权限变化采用明确的响应注入；读／下载／管理权限矩阵使用真实独立账号和真实服务端授权。

403 检查等待资料查询的实际 30 秒轮询响应，再验证受保护内容清除与恢复；`permission_denied` 优先触发现有租户边界重新核验，不要求出现页面局部错误文案。9 项专项、22 项文件回归和 22 个租户／成员用例的结果分别保留，合计 53 项不含现有环境的 8 项只读核对。

本轮不验证生产部署、迁移、Safari、物理手机或压力与长时间运行。

## 人工验收

入口：<http://localhost:25173/t/a78cdb12-b224-4869-9cb7-c7c1f76d0c8b/profile>。沿用现有验收账号。当前前端为本工作区 Vite 服务，刷新页面即可查看更新。

| 步骤 | 预期 |
| --- | --- |
| 打开租户资料页 | 组织名称为视觉主体；左侧说明与紧凑附件列表，右侧负责人和维护信息；没有权限清单。 |
| 点击编辑资料 | 打开原编辑表单；名称、编号、说明可维护，变更原因仍必填。取消不保存，提交后刷新可见结果。 |
| 点击附件名称 | 在当前页面打开真实附件详情或预览；关闭后回到原列表。 |
| 打开附件后刷新，或复制当前页面地址重新打开 | 恢复同一附件／修订的详情，不跳到无关工作空间。 |
| 通过附件更多操作查看管理入口 | 依照当前授权显示下载、编辑、回收等操作；操作仍要求原有原因和确认。 |
| 点击「查看成员」和「变更记录」 | 打开本租户成员页；历史入口预先限定当前租户对象。无对应权限时不显示入口。 |
| 查看空说明、无附件、只读、管理员异常状态 | 有明确空状态；只读隐藏编辑和添加；管理员异常给出平台处理提示，不显示正常状态占位字段。 |
| 缩小到手机宽度，使用 Tab/Enter/Escape | 内容纵向排列，无横向溢出；附件可用键盘打开、关闭并恢复焦点。 |

人工验收：**待用户确认**。
