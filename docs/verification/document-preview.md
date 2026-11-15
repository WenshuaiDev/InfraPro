# 文档在线预览自验

状态：Awaiting acceptance（文档预览功能及预览/版本合并，2026-11-15）。专项自验与本轮完整控制台构建通过；此前并行改动导致的类型检查阻塞已在后续复验中解除，记录见下文。用户人工接受：未完成。

需求：PDF 多页/翻页/缩放/全屏，以及 Word、CSV、TXT、Excel 相应的在线阅读功能。[范围和设计](../requirements/document-preview-proposal.md)，[使用与人工验收](../operations/document-preview.md)。

## 功能证据

| 范围 | 实测结果 |
| --- | --- |
| PDF | 三页 PDF 连续浏览、前后翻页/跳页、缩放/适合宽度/旋转、文字层搜索高亮、全屏通过；用户截图中的既有三页图纸亦在当前验收环境实际渲染 |
| DOCX | 真实三页文件的中文、表格、图片、页眉页脚、翻页、搜索、缩放和全屏通过 |
| XLSX | 两工作表切换、分页、原始行号、公式已保存结果与公式文本、搜索、合并单元格及跨页合并内容、方向键移动、缩放和全屏通过 |
| CSV/TXT | BOM、中文编码、引号内逗号/换行/双引号、前导零和等号文本；UTF-8/UTF-16/GB18030；跨页搜索、行号、换行设置通过 |
| 权限/历史 | 真正的仅查看/仅管理/下载账号验证；已打开正文在真实角色撤权后移除，后续读取返回 403；替换后的历史修订显示旧内容，原生下载 SHA-256 一致 |
| 内容隔离 | Word 外部链接被移除、HTML altChunk 不呈现、iframe 无法访问 parent.document；表格 HTML/公式作为文本；本轮无外部请求或浏览器运行时异常 |
| 边界 | 损坏 PDF 显示错误并保留下载；文字/表格超限明确提示；跨页合并单元格及异常重叠区域有界处理 |
| 布局 | 375px、深色和 720×500 CSS 视口（200% 等效空间）通过；原生全屏及全窗口 fallback、Esc 返回附件通过；截图已目视核对 |

## 检查记录

- 12 项 `pnpm test:files:client` 全通过，日志 `.artifacts/files/document-preview/unit.log`。
- 13 条新增真实浏览器旅程全通过，结果 `.artifacts/files/document-preview/result.json`。包含真实上传、检查器、附件绑定和上述阅读交互，隔离数据库/bucket 随测试结束清理。
- 初轮原有 21 条 `pnpm files:browser` 旅程全通过。历史列表请求的计数匹配收窄至列表端点，新增文档 content 请求不再被误计成提前读取历史；本轮更新为 22 条，最新结果见下节。
- `pnpm test:files:http` 通过（race 模式）：真实上传、严格输入、受控读取、在途权限复核，`.artifacts/files/document-preview/http.log`。
- 本次文件模块、客户端及脚本 ESLint 无警告通过；`git diff --check` 通过。
- Vite 生产资源打包通过，`.artifacts/files/document-preview/bundle.log`。PDF/Word/表格按阅读需要分块；PDF 模块仍有超过 500 kB 的打包体积提示，字体/CMap/WASM 已本地随包发布。
- 初轮最终 `typecheck` 因并行修改的监测导入路由/过滤类型报错（`import-search.ts`、`import-table.tsx`、批次路由及 API client 的 `MonitoringImportFilter` 默认值），没有预览相关文件诊断。旧日志 `.artifacts/files/document-preview/typecheck.log` 保留为历史记录，未覆盖其他聊天的改动。本轮完整构建已通过，见下节。

## 附件预览与版本合并复验（2026-11-15）

- 移除概览/版本记录页签、独立历史面板及重复菜单入口。预览上方版本选择器保留时间、原因、当前标识与还原来源；选择后正文、下载、还原对象随同一 revision 切换。
- 22 条文件浏览器旅程全部通过，`.artifacts/files/browser/result.json`、`.artifacts/files/document-preview/merged-files-browser.log`。包含版本列表按需加载、跨页直达、分页失败重试保留记录、键盘选择和 Esc 焦点返回、直接还原生成新版本，以及原有上传/回收/权限等路径。
- 13 条文档阅读浏览器旅程全部通过，`.artifacts/files/document-preview/result.json`、`merged-preview-browser.log`。新增断言无详情页签、选旧版本及返回最新版、375px 下菜单可达；正文 SHA-256、五类阅读器、全屏、权限与撤权回归保持通过。两组均无浏览器运行时异常。
- `pnpm --filter @infrapro/console build` 通过（含路由生成、TypeScript、Vite），`.artifacts/files/document-preview/merged-build.log`。仅有既有大块体积提示；相关 TSX/脚本 ESLint 无警告通过。
- 本轮为前端组织和交互调整，没有修改数据库、正文、权限或服务端接口；无需重新上传附件。人工接受仍由用户另行确认。
- 当前 `localhost:25173` 环境已实测既有 TXT 历史正文切换、下载版本同步、还原入口、返回最新版、375px 版本菜单与 Esc，以及既有三页 PDF；截图和结果见 `.artifacts/files/document-preview/merged-manual-result.json`。核对期间验收 API 曾间歇返回 503，运行日志同时出现数据库 statement timeout / audit_persist_failure，后续历史正文与 PDF 复验成功；该环境观察不代表修改了服务端或完成压力测试。

## 当前人工验收环境

`http://localhost:25173` 保持原账号和既有数据，未重新初始化。既有“青澜大桥多页图纸.pdf”已验证；另外添加“在线预览验收-Word.docx”和“在线预览验收-Excel.xlsx”两份合成附件并实际打开验证，记录 `.artifacts/files/document-preview/manual-result.json`。刷新文件页即可预览，不需重新上传原有文件。没有生产发布、数据库迁移或 Git 提交。

人工验收尚未执行。未声称覆盖全部 Office 复杂排版、桌面 Office 逐像素一致性、所有移动浏览器、屏幕阅读器或超大真实业务文件；格式与规模边界见[验收说明](../operations/document-preview.md)。
