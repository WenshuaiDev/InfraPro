# 租户成员与邀请界面统一

日期：2026-11-15。状态：Awaiting acceptance（自检通过，等待人工验收）。

## 目标与方案

按平台「账号管理」「账号开户邀请」的正式实现重构租户「成员管理」「成员邀请」，统一组件、布局密度、状态表达和操作入口。租户成员关系、历史关系及邀请业务语义继续使用现有租户 API。

技术：React / TypeScript 编写页面；TanStack Query 管理租户和成员关系隔离的查询、刷新及游标；TanStack Table 与 `@infrapro/ui` 提供表格；现有 Tailwind 主题令牌和 Lucide 图标统一样式。没有增加依赖、数据库结构或后端接口。

| 页面 | 平台参考及复用 | 租户保留的行为 |
| --- | --- | --- |
| 成员管理 | PageHeader、Tabs、Table（DirectoryTable 封装）、AccountAvatar、Badge、DropdownMenu、ListFilter、ListPagination、ResponsivePanel | 当前/有效/暂停/历史/全部关系；用户名或显示名称、内置角色筛选；历史角色记录、授权配置、审计；自身、管理员及权限范围保护 |
| 成员邀请 | PageHeader、下划线 Tabs、DataTable、InvitationStatus、InvitationEmpty、DropdownMenu、Dialog、AlertDialog、Sheet | 精确定位已有账号、24 小时本人接受邀请；受邀账号/状态/上海时间范围筛选；撤销、重新生成及关联记录 |

列表中的查看详情保留筛选上下文；旧详情 URL 可直接访问和刷新，关闭后返回对应列表。成员和邀请仍每 30 秒刷新，所有写操作继续校验版本、权限及操作原因。失去资格和离开页面时仍清除一次性邀请凭证；不会把生成的链接加入列表或详情接口。

## 自检记录

| 检查 | 结果 |
| --- | --- |
| Console TypeScript 与路由生成 | 通过（`pnpm --filter @infrapro/console typecheck`，最终构建再次执行类型检查） |
| 变更文件 ESLint / 测试工具 TypeScript | 通过（`eslint … --max-warnings 0`、`tsc -p tsconfig.tools.json`） |
| Console 生产构建 | 通过（`pnpm --filter @infrapro/console build`） |
| 租户成员及平台邀请回归 | 桌面 / 手机 18 项全部通过，覆盖成员加入/暂停/恢复/移除/重新加入、邀请撤销/替换、作用域、权限撤回、刷新草稿、重新认证、迟到响应隔离、筛选和深链接 |
| 最终详情布局及角色授权复验 | 桌面 / 手机 6 项全部通过，覆盖成员完整流程、详情/筛选/焦点，以及多角色配置、影响预览、角色停用与清空授权 |
| 视觉检查 | 已检查 1440px 桌面、375px 手机、浅深主题截图；横屏 667px + 24px 根字号断言通过，页面无横向溢出 |
| 当前本地验收服务 | `localhost:25173` 运行于当前 checkout，成员页与邀请页源码均返回 HTTP 200 且包含新界面；刷新即可使用 |

自动测试在隔离的 E2E 数据库和浏览器中执行，使用构建后的 preview。首轮发现公共 E2E 夹具尚未给现有角色写接口发送 `X-Command-ID`，已补齐并重跑通过。最后复验使用最终详情布局；没有未解决的测试失败。

复现：

```sh
INFRAPRO_E2E_PREVIEW=1 pnpm test:e2e tests/e2e/tenant-membership.spec.ts tests/e2e/invitations-page.spec.ts
INFRAPRO_E2E_PREVIEW=1 pnpm test:e2e tests/e2e/tenant-membership.spec.ts tests/e2e/tenant-role-authorization.spec.ts --grep 'member journey:|tenant directories preserve|role UI:'
```

日志位于 `.artifacts/tenant-members-ui/{build,e2e,final-details-e2e}.log`。最终截图：

- [桌面成员管理](../../.artifacts/tenant-members-ui/desktop-members.png)、[桌面成员邀请](../../.artifacts/tenant-members-ui/desktop-invitations.png)。
- [成员 Drawer](../../.artifacts/tenant-members-ui/desktop-member-details.png)、[邀请侧边详情](../../.artifacts/tenant-members-ui/desktop-invitation-details.png)。
- [手机成员管理](../../.artifacts/tenant-members-ui/mobile-members.png)、[手机成员邀请（深色）](../../.artifacts/tenant-members-ui/mobile-invitations-dark.png)。

本次执行相关前端和业务回归，没有执行全仓库全量测试。邀请表在窄屏内横向滚动，与平台邀请表一致；状态 Tabs 不显示服务端未提供的总数。人工验收与自检结果分开记录。

## 人工验收

在已有本地验收环境 `http://localhost:25173` 登录后，选择租户工作空间：

1. 打开「成员管理」，对照平台「账号管理」查看标题、状态 Tabs、搜索、角色筛选、头像、徽标、行操作、显示列和分页。长名称不应撑开页面。
2. 切换当前/历史/全部关系，搜索用户名，选择角色并重置；数据应匹配条件。切换每页条数、前后翻页后，不应混入其他筛选的记录。
3. 点击成员名称或菜单中的「查看详情」；应打开有遮罩的 Drawer。通过「绑定角色」「授权详情」「状态记录」查看角色授予历史、成员授权和审计，关闭后应回到原触发按钮。直接访问旧成员详情地址应打开同一详情。
4. 使用专用测试成员验证暂停、恢复及移除；填写原因并确认后，列表应更新。取消应保留原数据；本人和当前管理员不应出现可用的暂停/移除入口。
5. 打开「成员邀请」，对照平台「账号开户邀请」查看规则说明、下划线状态 Tabs、搜索框、状态徽标、表格与菜单。展开「创建时间」可继续按上海时间筛选；无结果时可清除筛选。
6. 点击受邀账号或菜单详情，侧边详情应显示受邀人、发起人、时间及关联记录，不显示旧明文链接；直接访问旧邀请详情地址和刷新均可使用。
7. 使用专用受邀账号定位、创建邀请，再取消/撤销/重新生成；新链接仅在生成结果中展示，关闭后不可从详情取回。受邀人确认后才成为成员。
8. 检查 375px 手机视口、浅色和深色主题；成员表自动精简次要列，邀请表可在表格内横向滚动，页面本身不应溢出。菜单、弹窗关闭和键盘焦点应正常。

人工验收尚未完成。当前工作仅更新本地代码与测试；未提交、推送或部署生产环境。
