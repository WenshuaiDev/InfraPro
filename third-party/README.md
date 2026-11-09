# 前端来源与本地适配

参考源码固定版本，不跟随上游 main 自动变化。使用原始 MIT 许可，完整文本保存在相邻目录。

| 来源 | 固定版本 | 本地用途 |
| --- | --- | --- |
| [satnaing/shadcn-admin](https://github.com/satnaing/shadcn-admin/tree/e16c87f213a5ba5e45964e9b67c792105ec74d26) | commit `e16c87f213a5ba5e45964e9b67c792105ec74d26`，package 2.2.1 | 基础组件、移动端 hook、主题；后台/认证/设置的视觉和交互基准 |
| [shadcn/ui Chart](https://github.com/shadcn-ui/ui/blob/8ab27715521cbd4f87ae6886cd989cbb678f6de7/apps/v4/registry/new-york-v4/ui/chart.tsx) | commit `8ab27715521cbd4f87ae6886cd989cbb678f6de7` | Recharts 容器、Tooltip 和 Legend 的基础实现 |

许可证：[shadcn-admin](shadcn-admin/LICENSE)、[shadcn/ui](shadcn-ui/LICENSE)。npm 库保留各自随包分发的许可证和版本，锁文件记录安装来源。

## 文件映射

- shadcn-admin `src/components/ui/{alert-dialog,avatar,badge,button,card,checkbox,collapsible,command,dialog,dropdown-menu,form,input,label,scroll-area,select,separator,sheet,sidebar,skeleton,table,tabs,textarea,tooltip}.tsx` → `packages/ts/ui/src/components/` 同名文件。
- shadcn-admin `src/components/ui/sonner.tsx` → ui 的 Sonner 适配，保留共享主题变量并加入中文可访问名称。
- shadcn-admin `src/hooks/use-mobile.tsx` → `packages/ts/ui/src/hooks/use-mobile.tsx`。
- shadcn-admin `src/styles/theme.css` → `packages/ts/ui/src/styles/theme.css`。
- shadcn/ui 上述 `chart.tsx` → `packages/ts/ui/src/charts/chart.tsx`。
- 布局参考 shadcn-admin `src/components/layout`、主题菜单、用户菜单、导航搜索，以及 `src/features/auth/{auth-layout,sign-in,forgot-password,sign-up}` 和 settings 的导航排列；在 `packages/ts/layouts` 重新组织为业务无关接口。

## 本地改动

1. 所有复制组件的 `@/` 引用改为 ui 包相对引用，不依赖应用 alias；新增显式包 exports 与 peer dependency。
2. Sidebar 由 layouts 的 Zustand store 受控管理，移除上游 cookie 状态；骨架长度改为固定值。移动端/开关与 Dialog、Sheet 的关闭文字中文化。
3. Table 的横向滚动容器提供可聚焦的命名区域。CommandDialog 增加真实 trigger，并将可访问标题放入 Content，确保键盘打开/关闭后归还焦点。表单组件通过新 FormInput 关联 label、描述、字段错误，密码显示按钮复用共享 PasswordInput。
4. 主题变量保留上游 slate/new-york 配置；Inter 字体同源托管并添加中文系统字体回退。应用 CSS 显式扫描共享包，支持 reduced motion；未采用上游覆盖滚动锁的 body 样式。
5. Chart 不再注入动态 style 标签，主题色通过容器 CSS 变量设置，并验证配置 key。新增 SeriesChart 负责五类图形、状态、格式化和可访问数据表。
6. Admin/Auth/Settings 布局接受品牌、导航、链接渲染和操作回调；不包含身份请求、团队数据或 Router。Sidebar 宽度、header 高度、inset 外框、卡片、间距和主题沿用参考。
7. 认证页复用默认居中 sign-in 的品牌/卡片结构。用户名替换邮箱，邀请开户和维护恢复接真实接口；不使用上游模拟登录、社交入口、Clerk、邮件或 OTP。重置页按同套风格补齐。
8. 保留 InfraPro 原品牌标识和中文文案；菜单对应实际账号能力。上游演示任务、聊天、收入等不进入产品页面。共享图表夹具在单独测试入口验证。

后续升级应以本表为检查清单，逐项比较固定来源与新版本，不直接覆盖本地状态、可访问性或业务适配。视觉对照与截图位置见[验证记录](../docs/verification/frontend-refactor.md)。
