# 平台审计原始内容 Code Block

状态：**Awaiting acceptance**。2026-11-10。自验证通过，待用户人工验收。

## 范围与实现

用户要求使用 [shadcn/ui Base Code Block](https://ui.shadcn.com/docs/components/base/code-block) 优化审计抽屉内的原始内容。将官方组件及高亮 hook 适配至共享 UI 包，使用现有 React、TypeScript、Tailwind 和 Button；Shiki 采用按需加载的 JavaScript 引擎，仅注册 JSON。

保留“技术信息 → 显示原始内容”入口。代码块显示文件标题、JSON 语法高亮、行号、复制反馈、自动换行开关；限制高度并内部滚动，兼容桌面右侧抽屉、手机底部抽屉和明暗主题。复制始终使用完整格式化 JSON，不含行号。高亮未加载或失败时仍可查看、选择和复制原始内容。

## 自验证

| 检查 | 结果 |
| --- | --- |
| `pnpm typecheck` | 通过 |
| `pnpm lint` | 通过 |
| `pnpm --filter @infrapro/console build` | 通过；高亮器、JSON 语法与主题拆为动态加载资源 |
| `pnpm test:e2e platform-audit.spec.ts --project=desktop --project=mobile` | 4 项通过；包括原始内容显示、详情、筛选、分页、焦点归还和失败登录语义 |
| 本地 Chrome 真实“同步权限目录”记录 | 457 行、14,323 字符，复制结果与渲染的完整 JSON 逐字一致；显示“已复制”反馈 |
| 真实长记录布局 | 明暗主题和 375 × 812 窄屏实查；换行后代码区 `scrollWidth === clientWidth`，长内容保持内部滚动 |

Shiki 固定为 `4.6.0`。构建仍有既有主包超过 500 kB 的提示，不影响构建通过。高亮器独立资源约 164.51 kB（gzip 54.36 kB），打开原始内容时才加载。

日志：`.artifacts/audit-code-block-{typecheck,lint,build,e2e}.log`。截图：`.artifacts/audit-code-block/{desktop,mobile}-{light,dark}.jpg`；最终预览 `preview.jpg`。本地服务已运行，Vite 已加载修改；其他环境使用 `make dev` 启动。

## 人工验收

1. 打开 `http://127.0.0.1:5173/access/audit`，进入一条审计记录的“技术信息”，打开“显示原始内容”。预期显示带行号和 JSON 高亮的代码块。
2. 点击复制，预期出现“已复制”，粘贴得到完整 JSON，未混入行号或标题。
3. 对“同步权限目录”等长记录切换“自动换行”；预期长行可在代码块内横向滚动，开启换行后适应内容区宽度，页面和抽屉不会被撑宽。
4. 在明暗主题及手机窄屏查看，预期代码清晰，长内容在代码块内部滚动，标题和操作可用。

自验证通过不代表用户已验收；无数据库或接口变更。
