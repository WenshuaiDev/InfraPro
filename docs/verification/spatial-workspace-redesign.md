# 工程空间 A 方案重构自验

日期：2026-11-15。状态：**Awaiting acceptance（待人工验收）**。正式重构与自验完成；人工验收尚未进行。

## 交付内容

工程空间按六项任务组织：空间总览、GIS 地图、布设图、BIM／三维、测点标注、资料与版本。租户与项目共用浏览器，项目保留固定版本与权限交集。资料详情、发布／撤回预览、删除草稿、项目授权与影响预览使用系统带遮罩的 Drawer。

所有目录、图模、标注、版本、任务、监测、成果及授权均调用原有真实服务。未引入原型数据或模拟成功操作。新建、上传、处理、标注、发布、当前版、撤回、退役恢复与授权结束均已通过浏览器实际执行并由 API 回读核对。

本轮增加与修复：

- `section` 导航与 URL 状态；原 asset、version-only、owner-only、page、view、mode=manage 深链接继续有效。非法或失权固定版本不回落到其他内容。
- 对象详情按选择展开，同一业务身份可在 GIS、图纸、模型之间定位；资源和源模型构件目录均保留。
- 标注工作区明确区分业务对象、画布、标注属性和已挂载列表。取消与离开未保存草稿会提示；模型加载完成前禁用相机记录；尚未读取完整对象前禁用应用操作。
- 迁移候选显示所属图页和中文状态，载入核对同时恢复图页与主通道；采用与清空候选分别执行，未采用项不生成标记。
- 撤回版本的管理状态来自版本记录，浏览仍拒绝访问；创建草稿只从已发布版本复制，其他状态可创建新的空白草稿。
- 项目授权历史筛选进入 URL，在权限修订刷新后保留。查看与原件下载独立，授权预览继续使用服务端修订与预览摘要。
- 总览／资料页不再请求未展示的浏览目录；列表的版本与授权行信息限制为两个并发请求，取消和权限切换时不启动排队的旧请求。

技术及功能映射见 `docs/development/spatial-workspace-redesign.md`。

## 验证环境与结果

变更操作使用 `pnpm spatial:demo` 启动的独立数据库／对象存储命名空间，使用真实 Go API、空间 worker、转换器和权限服务，输入为仓库合成 PNG、PDF、GLB、IFC、GeoJSON。测试不写入现有 25173 人工验收业务数据。独立自验服务已正常停止并清理本次隔离数据；证据文件保留。当前人工验收服务继续运行。

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| Console 类型检查与生产构建 | 通过 | `.artifacts/spatial-redesign/build.log` |
| 空间前端、客户端类型与回归脚本 ESLint | 通过，0 警告 | `.artifacts/spatial-redesign/lint.log` |
| `pnpm test:spatial` | 9 项 Go 集成、7 项转换器、5 项客户端检查通过 | `.artifacts/spatial-redesign/spatial-tests.log` |
| 原浏览器显示回归 | PDF、GLB、IFC、GeoJSON、资源地图、390px；无运行错误、无失败请求、无整页横向溢出 | `.artifacts/spatial-browser/inspection.json` |
| 原浏览器交互回归 | 7 项通过，浏览器错误 0 | `.artifacts/spatial-browser/interactions.json` |
| A 方案完整业务流程 | 9 项通过，浏览器错误 0 | `.artifacts/spatial-redesign/redesign-flows.json` |
| 图纸候选／模型标注／GeoJSON 关联专项 | 3 项通过，临时草稿已清理 | `.artifacts/spatial-redesign/editors.json` |
| 真实权限与数据范围回归 | 11 项通过，授权恢复成功 | `.artifacts/spatial-scope-check.json` |
| 限并发与取消单元检查 | 2 项通过 | `.artifacts/spatial-redesign/metadata-tests.log` |
| 当前 25173 验收入口读取复核 | 8 项通过，运行错误 0、服务失败请求 0、无整页横向溢出 | `.artifacts/spatial-redesign/acceptance-read.json` |

当前验收入口已逐页复核，并读取原有 PDF 深链接的真实监测值和项目固定版本授权列表。截图位于 `.artifacts/spatial-redesign/acceptance-overview.png`、`acceptance-layout.png`、`acceptance-assets.png`、`acceptance-access.png`。

构建保留既有第三方包体积提示，无类型或构建失败。构建不是线上部署；未修改数据库结构。

### A 方案浏览器流程

1. 六项导航、真实设施范围、资料筛选与无匹配状态、刷新恢复筛选。
2. 从界面登记资料、创建草稿、上传 PNG、处理真实原件及查看任务。
3. 引用已有测点，分别保存锚点与标签；取消、跨页面离开保护与保存回读。
4. 草稿校验、带遮罩发布预览、实际发布、设置租户当前版本。
5. 授予项目固定版本但不授予原件下载；进入项目读取；结束授权并显示历史。
6. 撤回当前版本后浏览返回 404，历史仍可管理；退役和恢复。
7. 经确认删除空草稿，既有历史版本保留。
8. 旧固定版本／对象链接、跨图模同身份定位、非法版本不替换。
9. 390px 与暗色主题、键盘打开和关闭、Drawer 关闭后的焦点返回。

### 编辑器与权限重点

- 替换 PDF 后，原标注仅成为候选；逐项载入到对应图页、采用、保存默认页和清空候选。旧发布版保持不变。未处理草稿校验明确失败。
- IFC 草稿中的三维锚点、标签、源属性发布设置与构件解除真实落库，原版本和业务构件归属保持不变。
- GeoJSON 图形关联到设施，不改设施真实经纬度。
- 旧交互检查覆盖多页切换、独立标签编辑、键盘源构件选择、IFC 属性、剖切／隔离、项目相机和显隐、主通道保存、真实悬停值／单位／时刻／质量、趋势时段与分析只预填、触屏。
- 权限检查覆盖祖先仅导航、固定新版需显式授权、原件下载单独控制、设备来源最小信息、设施／构件直接派生通道、成果真实输入与所属项目、递归输入撤权与基线时段缺失后隐藏派生值。
- Go 集成检查另覆盖草稿并发修订、撤回与项目默认、上传取消、处理围栏、下载撤权、工程依据变化、缺失文件、备份恢复及实际模型语义失败路径。

## 手工验收

正式入口：<http://localhost:25173/t/a78cdb12-b224-4869-9cb7-c7c1f76d0c8b/spatial>。使用已有验收账号；账号材料仍在本机原位置，本文件不保存密码。

1. 刷新工程空间，确认六项任务入口。分别进入 GIS、布设图和 BIM／三维，选择实际资料，切换版本／图页并选中测点或源构件。预期：真实图模与监测显示，未选择对象时没有空详情栏。
2. 在“测点标注”选一个已发布版本，创建标注草稿，编辑位置或标签。尝试离开并选择继续编辑，再保存。预期：修改只进入草稿，旧发布版不变；可继续核对与发布。
3. 在“资料与版本”打开资料 Drawer，查看原件、处理任务、历史及变更记录。导入新资料时依次登记、创建草稿、上传处理，再标注和发布。
4. 从“项目空间授权”选择项目，核对明确版本、下载选项和变更预览。预期：项目不会自动跟随租户新版；项目视图可保存已获准版本的图页、相机、显隐和主通道。
5. 缩窄到手机宽度或切换暗色主题，检查导航、列表、画布、Drawer 与关闭后的焦点。

服务已运行时直接打开。电脑重启后沿用原验收启动方式：

```bash
node /Users/hijacker/.local/share/infrapro/manual-acceptance/scripts/dependencies.mjs
node /Users/hijacker/.local/share/infrapro/manual-acceptance/scripts/start.mjs
```

复跑独立自验：在仓库根目录先执行 `pnpm spatial:demo`，等待样本就绪，再依次执行下列脚本。权限变更脚本与 UI 变更脚本应串行运行，避免彼此改变同一演示项目的授权修订。

```bash
node scripts/spatial-browser-check.mjs
node scripts/spatial-ui-check.mjs
node scripts/spatial-redesign-check.mjs
node scripts/spatial-editors-check.mjs
node scripts/spatial-scope-check.mjs
node --test tests/unit/spatial-metadata.test.mjs
```

## 范围与限制

本轮完成页面重构、真实接口接入和上述自验；没有进行生产部署、大规模模型性能基准或长期并发压测。地图底图继续使用系统配置，示意底图与有效资源坐标保持明确区别。人工验收只有在用户明确确认后才记为 Accepted。
