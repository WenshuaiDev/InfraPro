# 账号管理界面重设计

日期：2026-11-11。状态：**Awaiting acceptance**。用户授权直接实施，参照角色管理、平台审计和 shadcn-admin 完成重构。实现与自验通过，等待用户人工验收，证据见[验证记录](../verification/account-management-redesign.md)。

## 目标与范围

- 紧凑表格显示名称、用户名、真实账号状态、绑定角色、创建时间及行操作。状态标签、搜索、角色筛选、列显隐、10／20／50 条游标分页统一使用现有组件；不从显示名称推断租户成员状态。
- 详情使用桌面右侧／手机底部响应式抽屉，包含绑定角色、有效权限和有权查看的租户任职。关闭保留条件、页码和触发焦点。
- 有效权限按组显示名称、用途及来源，编码按需显示；无目录读取权限仍能查看接口允许的编码及来源。
- 配置角色及启停在抽屉内展示变更内容和原因，沿用密码复核、版本冲突、最后管理员及租户管理员保护。编辑保留打开时的版本，后台刷新不覆盖草稿和提交版本。
- 各查询具有加载、空态、错误和重试；候选角色可以继续加载，不丢失未加载的已有绑定。

## 技术与数据流

- React／TypeScript 拆分账号列表、表格、详情及角色编辑；复用 shadcn/ui Table、Avatar、Badge、Tabs、Dropdown Menu、Checkbox、Switch、Toggle、Input、Button 和 Base UI Drawer 的 `ResponsivePanel`。
- TanStack Table 管理列与渲染；TanStack Query、`useCursorList` 和 `ListPagination` 管理真实服务端游标。查询键包含调用者及筛选／页大小，改变条件回到第一页；不虚构总数或对单页做全局排序。
- 复用权限说明／分组和 `RoleMutationForm` 的写入校验；写入后刷新身份、列表和详情。保留 Go、PostgreSQL、Redis 与原 API，不增加依赖或迁移。

## 人工验收

1. 搜索、状态与角色筛选、重置、列显隐及分页正确；改变条件回到第一页，下一页故障保留已加载行。
2. 名称和菜单均打开抽屉；三个详情标签清晰，关闭回到原位置及焦点。
3. 角色编辑含已选项和增删摘要；保存后列表与详情一致，再次编辑没有残留草稿。启停需要原因，冲突及管理员保护可见。
4. 无相关权限不显示写入口，不请求无权读取的角色／权限／租户目录。
5. 桌面、375px 手机、明暗主题无整页横向溢出，长名称及多角色不挤压操作入口。

参考：[shadcn-admin Users](https://github.com/satnaing/shadcn-admin/tree/e16c87f213a5ba5e45964e9b67c792105ec74d26/src/features/users)、[shadcn/ui Data Table](https://ui.shadcn.com/docs/components/radix/data-table)。沿用仓库固定组件版本和 TanStack Table v8，不升级到在线文档的 v9。自验另记于 `docs/verification/account-management-redesign.md`，用户确认后才标记 Accepted。
