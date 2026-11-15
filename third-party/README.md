# 第三方来源与本地适配

前端参考源码固定版本，不跟随上游 main 自动变化。以下复制组件使用原始 MIT 许可，完整文本保存在相邻目录；其他依赖各自遵循下文及其发行包中的许可。

| 来源 | 固定版本 | 本地用途 |
| --- | --- | --- |
| [satnaing/shadcn-admin](https://github.com/satnaing/shadcn-admin/tree/e16c87f213a5ba5e45964e9b67c792105ec74d26) | commit `e16c87f213a5ba5e45964e9b67c792105ec74d26`，package 2.2.1 | 基础组件、移动端 hook、主题；后台/认证/设置的视觉和交互基准 |
| [shadcn/ui 组件与 Chart](https://github.com/shadcn-ui/ui/blob/8ab27715521cbd4f87ae6886cd989cbb678f6de7/apps/v4/registry/new-york-v4/ui/chart.tsx) | commit `8ab27715521cbd4f87ae6886cd989cbb678f6de7` | Recharts 容器、Tooltip、Legend，以及 Pagination、Switch、Toggle |
| [shadcn/ui Base UI Drawer](https://github.com/shadcn-ui/ui/blob/8ab27715521cbd4f87ae6886cd989cbb678f6de7/apps/v4/registry/bases/base/ui/drawer.tsx) | 同一 commit；2026-11-10 获取 base-nova registry 样式展开版本 | 角色详情和操作的桌面右侧／手机底部抽屉，使用 `@base-ui/react` 1.9.0 |
| [shadcn/ui Base Code Block](https://github.com/shadcn-ui/ui/blob/8ab27715521cbd4f87ae6886cd989cbb678f6de7/apps/v4/registry/bases/base/ui/code-block.tsx) | 同一 commit；2026-11-10 获取 base-nova registry 样式展开版本 | 审计原始 JSON 的标题、行号、复制、换行和 Shiki 明暗主题高亮 |

许可证：[shadcn-admin](shadcn-admin/LICENSE)、[shadcn/ui](shadcn-ui/LICENSE)。npm 库保留各自随包分发的许可证和版本，锁文件记录安装来源。

## 文档阅读器（2026-11-15）

| 依赖 | 固定版本/来源 | 本地用途与许可 |
| --- | --- | --- |
| [PDF.js](https://github.com/mozilla/pdf.js) / pdfjs-dist | 6.5.438，npm | PDF 页面、文字层、搜索；Apache-2.0 |
| [docx-preview](https://github.com/VolodymyrBaydalka/docxjs) | 0.4.1，npm | 隔离 iframe 内的 Word 排版；Apache-2.0 |
| [JSZip](https://github.com/Stuk/jszip) | 3.10.1，npm | docx-preview 的 ZIP 读取；采用 MIT 许可选项 |
| [Papa Parse](https://github.com/mholt/PapaParse) | 5.7.1，npm | Worker 内 CSV 解析；MIT |
| [SheetJS CE](https://docs.sheetjs.com/docs/getting-started/installation/nodejs/) | 0.20.3，官方 `cdn.sheetjs.com/xlsx-0.20.3/xlsx-0.20.3.tgz`，完整性记录在锁文件 | Worker 内 XLSX 只读解析；Apache-2.0 |

许可副本存放于 `document-preview/`。依赖随应用本地构建，运行时不连接上述发行站点；PDF 配套字体和 WASM 按发行包复制，保留其目录内附带的许可。

## 文件映射

- shadcn-admin `src/components/ui/{alert-dialog,avatar,badge,button,card,checkbox,collapsible,command,dialog,dropdown-menu,form,input,label,scroll-area,select,separator,sheet,sidebar,skeleton,table,tabs,textarea,tooltip}.tsx` → `packages/ts/ui/src/components/` 同名文件。
- shadcn-admin `src/components/ui/sonner.tsx` → ui 的 Sonner 适配，保留共享主题变量并加入中文可访问名称。
- shadcn-admin `src/hooks/use-mobile.tsx` → `packages/ts/ui/src/hooks/use-mobile.tsx`。
- shadcn-admin `src/styles/theme.css` → `packages/ts/ui/src/styles/theme.css`。
- shadcn/ui 同一固定版本的 `ui/{pagination,switch,toggle}.tsx` → ui 包同名组件；工具函数改为本地引用，Radix 从既有独立包导入，分页文案中文化。
- shadcn/ui `registry/bases/base/ui/drawer.tsx` 的 [base-nova registry 输出](https://ui.shadcn.com/r/styles/base-nova/drawer.json) → ui 包 `drawer.tsx`；工具函数使用本地引用，保留 Base UI 的 portal／backdrop／viewport／popup／content 组合。遮罩改为 25% 黑色与 8px 模糊，深色模式使用 40% 黑色。`ResponsivePanel` 配置桌面 560px 宽、12px 留白和圆角，手机底部 92dvh；body 增加 relative 定位以符合上游 iOS 遮罩要求。
- shadcn/ui 上述 `chart.tsx` → `packages/ts/ui/src/charts/chart.tsx`。
- shadcn/ui [code-block registry](https://ui.shadcn.com/r/styles/base-nova/code-block.json) 与 [use-highlighted-code registry](https://ui.shadcn.com/r/styles/base-nova/use-highlighted-code.json) → ui 包 `components/code-block.tsx`、`hooks/use-highlighted-code.tsx`、`lib/highlighter.tsx`。使用本地 Button、工具函数和 Lucide 图标；复制反馈中文化并提示失败。保留行号、换行、键盘滚动和组合式 API；沿用项目滚动条，不加入上游 scroll-fade 样式。高亮器动态加载，仅注册当前使用的 JSON 语言，使用 Shiki JavaScript 引擎和 GitHub light/dark 主题。
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

后续升级应以本表为检查清单，逐项比较固定来源与新版本，不直接覆盖本地状态、可访问性或业务适配。

2026-11-10 角色界面重设计：以 Tasks 页的紧凑表格、工具栏、行操作及分页为视觉参考；按用户追加要求，`ResponsivePanel` 统一使用 Base UI Drawer，并加入背景模糊。业务表格使用真实游标分页，不复制演示数据或虚构总数。主导航与角色链接显式指定对应主题文字颜色，保证主题切换后的可读性。

## 工程定义校验

`github.com/santhosh-tekuri/jsonschema/v6` 锁定 v6.0.3（Apache-2.0），用于 Go 端编译和执行系统生成的 JSON Schema 2020-12。来源和许可证见[上游仓库](https://github.com/santhosh-tekuri/jsonschema/tree/v6.0.3)；作为 Go module 依赖引入，没有复制修改上游源码。API 独立模块的依赖与校验和同步，容器以 `-mod=readonly` 构建。

## 空间可视化

2026-11-13 按实际安装包元数据核对。浏览器依赖由 `pnpm-lock.yaml` 锁定；Python 完整依赖及分发哈希由 `tools/spatial-converter/requirements.lock` 锁定，容器按 `--require-hashes` 安装，保留发行包内许可证。这里没有复制或修改这些库的源码。

| 组件 | 固定版本 | 许可与用途 |
| --- | --- | --- |
| [OpenLayers](https://github.com/openlayers/openlayers) | 10.11.0 | BSD-2-Clause；二维地图、GeoJSON、图纸浏览及投影转换。 |
| [Three.js](https://github.com/mrdoob/three.js) | 0.187.0 | MIT；GLB、相机、选择、高亮及剖切。 |
| [IfcOpenShell](https://github.com/IfcOpenShell/IfcOpenShell) | 0.9.0.post1 | LGPL-3.0-or-later；独立 Python 进程解析 IFC 几何与语义。发行包中的原生依赖继续保留各自许可。 |
| [pypdfium2](https://github.com/pypdfium2-team/pypdfium2) | 5.15.2 | BSD-3-Clause／Apache-2.0 及随包列出的依赖许可；PDFium 原生页渲染。 |
| [Pillow](https://github.com/python-pillow/Pillow) | 13.0.0 | MIT-CMU；图像方向、尺寸校验及多级浏览图片。 |
| [NumPy](https://github.com/numpy/numpy) | 2.5.4 | 发行元数据为 BSD-3-Clause AND 0BSD AND MIT AND Zlib AND CC0-1.0；IFC 几何数组。 |
| [ReportLab](https://hg.reportlab.com/hg-public/reportlab/) | 5.1.0 | BSD，具体以随包 license.txt 为准；仅生成合成 PDF 样本。 |

合成桥梁、边坡、图纸、模型和本地示意底图由本仓库脚本原创生成，标为 CC0-1.0，见 [samples/spatial](../samples/spatial/README.md)。样本许可不替代转换库及地图服务许可。可配置外部 XYZ 时需提供来源署名；默认本地示意底图无需商业地图密钥。

## 大屏 SDK 与开发工具

`@infrapro/display-sdk`、`@infrapro/display-devtools` 1.0.0 和最小接入样例为本仓库实现，随 npm 安装包携带 MIT LICENSE。SDK 核心没有第三方运行依赖；开发工具使用 Node 内置 HTTP、文件、密码和压缩 API，包内可信壳来自本仓库 Vite 构建。构建继续使用锁文件中的 TypeScript、Vite、React 与已有 UI；服务器沿用 Go ZIP、pgx、BlobStore，不引入新的包执行或脚本引擎。部署镜像沿用现有 Go/Alpine/Nginx 基础版本。
