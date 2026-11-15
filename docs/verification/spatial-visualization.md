# 工程空间可视化实施与验证

日期：2026-11-13。状态：**Awaiting acceptance（SV-01–SV-10 已实现，完整自验通过，待用户人工验收）**。用户已确认完整方案并授权实现；本记录不提前确认人工验收。

实现依据：[需求第 18 节](../requirements.md#spatial-scope)、[架构第 13 节](../architecture.md#spatial-design)、[技术选型第 9 节](../technology-selection.md#spatial-technology)。在当前 checkout 和分支实施，保留既有 AGENTS.md 修改；没有创建分支或 worktree。需求确认、功能自验、人工验收及生产部署分别记录。

## 完整交付范围

| 范围 | 已实现行为 | 主要实现位置 |
| --- | --- | --- |
| SV-01 | 资料及固定版本；8 MiB 分片、哈希封存、取消／重试；独立 worker 租约及 fencing；草稿修订、发布／当前／撤回／退役、审计、引用保护。 | `platform/spatial`、迁移 00034、`cmd/spatial-worker`、`asset-manager.tsx`。 |
| SV-02 | 真实 PNG／JPEG／多页 PDF；方向、裁剪和渐进分辨率；独立锚点、标签、引线、矩形／多边形区域。 | Python 转换器、`layout-viewer.tsx`、`draft-editor.tsx`。 |
| SV-03 | 自包含 GLB、IFC2X3／IFC4；节点／实例变换、GlobalId、源树、实例／类型属性；旋转平移缩放、标准视角、高亮、隐藏／隔离／透明、基础剖切。 | IfcOpenShell 真实转换、`model-viewer.tsx`、源构件树。 |
| SV-04 | 同一测点多载体定位；同租户／设施及坐标校验；标签不改锚点；换版候选、人工确认、旧版保留、业务依据变化提示。 | spatial 草稿、版本及资源适配；空间位置入口。 |
| SV-05 | EPSG:4326／3857；六种 GeoJSON 点线面；受控 XYZ、空白／本地底图；范围查询、分页、搜索和聚合；无坐标仍保留目录入口。 | `map-viewer.tsx`、空间对象／图形查询、受控底图配置。 |
| SV-06 | 项目明确固定版本，原件下载独立；资源／设备／数据资格交集；设施关联结束不复活旧授权；归档只读、撤权及受控传输。 | 空间授权、既有授权协调器、文件流和项目授权页。 |
| SV-07 | 资源目录、中央视图、对象面板；共同选择、固定版本深链接、主通道和项目默认视图；悬停、键盘、触屏、取消及错误降级。 | 两种 scope 的工程空间路由、统一 workspace。 |
| SV-08 | 正式 raw／derived latest 与趋势；各 owner 直接所属通道；精确数值、时刻、质量、计算状态、pending、上一有效结果；设备当前及历史来源。 | `runtime/spatial_monitoring.go`、既有 monitoring／derived Provider、`object-panel.tsx`。 |
| SV-09 | 按实际保存的输入反查本空间固定成果；明确后代开关；当前输入资格及受影响状态；带入通道／时段进入历史分析，不自动运行。 | `derivedmonitoring/spatial_artifacts.go`、历史分析页面。 |
| SV-10 | 37 份原创原始文件、生成器和独立预装场景；两租户／两项目、真实业务数据、换版及异常；操作、恢复及本记录。 | `samples/spatial`、`spatial:demo`、空间运行手册、验证脚本。 |

沿用 Go／pgx／Goose／PostgreSQL 实现授权事务及元数据，私有 S3 保存封存文件。独立 Go worker 调用固定 Python／IfcOpenShell／pypdfium2 环境。React／TypeScript／TanStack／shadcn/ui 承担工作台，OpenLayers 用于图面／GIS，Three.js 用于局部模型。ClickHouse 和派生计算仍由现有监测模块拥有。

联调同时修正了监测数据授权列表的一个既有问题：原查询只关联测点，会遗漏设施、构件直属派生通道的授权；现在按通道所属设施读取，三个 owner 类型的授权均能列出、核对及撤销。

## SC-01–SC-13 自验账本

| 场景 | 实际验证及预期结果 | 证据 |
| --- | --- | --- |
| SC-01 | PDF、GLB、IFC 的挂载均引用同一 P-01；工程经纬度为 116.3802／39.9001，正式 A 通道最新值为 20。 | 范围脚本、四种桌面视图。 |
| SC-02 | 多页 PDF 页码可切换，真实转换保留旋转／裁剪；浏览器把标签改为 `[72,-36]` 后锚点不变，取消第二次编辑不增加已保存修订。 | Python 图面检查、交互脚本、`draft-label.png`。 |
| SC-03 | 实际 GLB、IFC2X3／IFC4 转换及网格／实例检查；键盘选中 Main beam 打开关联业务构件和真实 GlobalId／类型属性，构件直属值为 9；俯视、隔离、剖切可操作。 | 真实模型集成、Python 检查、`ifc.png`、`ifc-properties.png`。 |
| SC-04 | V2 的移动、缺失、重编号及 PDF 重排进入候选，不覆盖 V1；候选可逐项处理，业务身份和旧挂载保留。 | 版本迁移单元检查、真实模型／图纸集成、范围脚本。 |
| SC-05 | 项目 A 初始可读 V1、不可读 V2；明确新增 V2 授权后才可读，测试结束恢复原范围。 | 范围脚本、真实版本与项目边界集成。 |
| SC-06 | 项目 B 初始仅通道祖先上下文，精确测点对象返回 404、挂载为空；有资料授权不获得设施直属值。 | 范围脚本、完整对象资格集成。 |
| SC-07 | 仅浏览时原 PDF／IFC 不可下载；明确原件授权后读取真实文件字节，响应为 no-store；后续撤权再次拒绝。 | 范围脚本、真实文件集成及分块下载检查。 |
| SC-08 | 输出级设备投影只含允许上下文，当前与历史分别指向两次来源；不含完整序列号或未授权标识，挂载保持不变。 | 范围脚本、正式来源／来源历史 API。 |
| SC-09 | 测点、设施、构件只展示各自通道；真实原始批次撤销后派生当前输出为空且 pending 区间非空，上一有效结果独立；固定分析成果仍为原版本并标明输入撤销。 | `TestSpatialRealPendingSummaryAndFixedArtifact`，范围脚本。 |
| SC-10 | 撤销项目 A 的递归输入授权，派生摘要及相关成果消失，正式递归 latest 被拒绝；项目 B 和租户仍可读取。只移除原始基准时段时，当前 A 值仍为 20，但基准派生值消失；恢复权限后为 10。八层链与固定基准原始依据撤销通过真实存储回归。 | 11 项范围检查、`spatial-recursive-baseline.log`。 |
| SC-11 | 默认只查 owner 直接输入；明确后代后返回相关成果。两项目共享相同输入和值仍不能读取对方成果；原固定版本链接不被重算覆盖，输入状态来自成果模块。 | 范围脚本、真实 pending／固定成果集成。 |
| SC-12 | 取消上传、缺片、坏文件、人工重试、旧 worker 租约失效、并发草稿冲突、撤回／归档／退役均有真实断言；撤权后旧下载对象不能继续输出；备份恢复和缺失隔离通过。 | 9 项真实空间集成中的生命周期、竞争、下载及恢复检查。 |
| SC-13 | 鼠标悬停显示同一 20 mm、观测时刻、质量及查询状态；键盘源树和触屏对象面板可用；390×844 无横向溢出，本地底图及空白背景可用。 | 7 项浏览器交互，桌面／窄屏截图及真实绘制像素检查。 |

没有使用假派生接口、假 IFC 几何或静态监测值作为交付证据。测试中的阻塞器、对象缺失和网络错误仅用于指定故障注入；基础管线使用实际 PostgreSQL、SeaweedFS S3、ClickHouse、转换器和 HTTP。

## 验证命令与本机证据

本轮使用仓库规定的 Go 1.27、Node 26.10、pnpm 12.18；转换虚拟环境 Python 3.12。正式数据与监测压力测试不是本次测试目标；全部测试采用隔离 schema 和对象前缀。

| 检查 | 结果及证据位置 |
| --- | --- |
| `pnpm check` | **通过**：Go 检查、ESLint、TypeScript、OpenAPI、权限同步；最终日志 `.artifacts/spatial-check-delivery-final.log`。 |
| `pnpm build` | **通过**：Go／console 完整构建；最终日志 `.artifacts/spatial-build-delivery-final.log`。模型和趋势按需加载，构建仍提示部分 chunk 超过 500 kB。 |
| `pnpm test:spatial` | 9 项真实空间集成、7 项 Python 转换检查、5 项客户端取消／迟到响应检查通过；`.artifacts/spatial-integration-final.log`。 |
| `pnpm test:derived:database` | 规则、HTTP、资源、访问回归通过，使用 `-race`；`.artifacts/spatial-derived-regression.log`。 |
| 八层及基准定向回归 | `TestDerivedEightLayerFixedSourcesAndRecursiveRevocation`、`TestDerivedPublishedBaselineKeepsOriginalReferenceAndRevocation` 均通过，真实存储＋`-race`；`.artifacts/spatial-recursive-baseline.log`。 |
| 范围检查 | `node scripts/spatial-scope-check.mjs` 的 11 项检查通过；临时授权全部恢复，结果在 `.artifacts/spatial-scope-check.json`。 |
| 浏览器基线 | `pnpm spatial:browser`；PDF／GLB／IFC／GeoJSON／资源位置地图／窄屏，无页面错误或失败 API，地图另验实际图形像素；`.artifacts/spatial-browser/inspection.json`。 |
| 浏览器交互 | `node scripts/spatial-ui-check.mjs`；标签保存／取消、键盘 IFC、共享视角及隐藏、主通道保存、鼠标悬停、分析预填、触屏；`.artifacts/spatial-browser/interactions.json`。 |
| Linux 转换镜像 | `deploy/docker/spatial.Dockerfile` 构建通过，1 GiB／2 CPU 容器内真实转换 IFC4、IFC2X3、三页 PDF 及毫米／Z 轴 GLB 全部成功；`.artifacts/spatial-image-check.json`。 |
| 样本完整性 | 37 份原始文件大小及 SHA-256 与 manifest 一致；`.artifacts/spatial-samples.zip` 含原件、README、manifest 和 expected。 |
| 恢复 | `pg_dump/pg_restore` 到另一 schema，复制 S3 到独立恢复桶，比较 11 张空间表和实际字节哈希；验证缺失隔离、正确字节修复及网络错误不误隔离。 |
| 交付文件 | `git diff --check` 通过；11 份相关文档的本地链接、空间锚点及代码围栏检查通过。既有 OpenAPI 除追加空间权限枚举外，路径及 schema 语义不变。源码摘要另存 `.artifacts/spatial-delivery-source.json`。 |

本机镜像：`infrapro-spatial:acceptance`，ID `sha256:55095cdecfa09f6b6dd041aa6e6842bd985590eaf42a6c1683c74c83358a5a17`。该镜像已构建及测试，未部署到生产。

## 画面与交互复核

实际查看了图纸、GLB／IFC 全景、IFC 选中及剖切、GIS、鼠标摘要和手机页面截图。图纸标记与标签有清楚的引出关系；桥梁采用可辨识的梁、桥面、墩、基础及柔和明暗，选中金色和普通灰色几何可分辨；没有将浏览器材质当作结构分析结果。源树按真实父子关系排列，提供展开／收起；数据与来源属性在各自区域显示。

地图首次收到有效范围后才定位并发出视野筛选，防止初始零经纬度空视野抢先过滤图层；以后拖动保留当前视野。项目共享视图先载入再允许操作，成功保存后更新缓存，防止旧默认值覆盖刚保存选择。

界面复用现有主题、组件与三栏布局。颜色为位置／选择／退役图例，不推断工程安全；完整规范数值字符串保留，长值减小字号，不以绘图近似数替代。静态模型在无交互时停止重复绘制；切换载体清理 WebGL、Blob URL 和查询资源。

桌面检查为 Chromium 1600×1000，触屏模拟为 390×844；WebGL 使用 SwiftShader。首次资料打开到可读监测摘要约为数秒，具体实测保存在 inspection.json，包含开发服务器及真实查询耗时，不能推广为生产 SLA 或最大模型容量。

## 当前可直接操作的场景

运行入口：`http://127.0.0.1:15473`。实现阶段的隔离演示仍在 `/Users/hijacker/.codex/worktrees/0191/InfraPro/.artifacts/spatial-demo/2026-11-13T03-32-35.980Z/`；账号位于该目录 `login.json`（0600），完整租户／项目链接及 UUID 位于 `spatial-acceptance.json`。不要共享登录文件。停止并重建后入口中的对象 UUID 会变化。

该场景已预装 11 份资料、16 个版本，以及两租户、两项目、桥梁／边坡、测点、设备换源、真实原始／派生通道及固定分析成果。项目 A 可用完整业务范围，项目 B 初始为祖先上下文边界。数据日期为 2024-01-01；查看趋势时请用样本时段，不能把当前日期无数据当作导入失败。

原件及预期见[样本说明](../../samples/spatial/README.md)。启动、重置、普通环境配置、格式边界、备份及恢复命令见[运行手册](../operations/spatial-visualization.md)。停止独立演示进程会清理本次隔离数据；再次 `pnpm spatial:demo` 可重建，不清理用户业务数据库。

## 人工验收路径

1. 打开工程空间，选主梁跨中位移点，通过「空间位置」切换 PDF、GLB、IFC。预期始终为同一测点，A 最新值为 20 mm；图纸可切换三页。
2. 进入租户资料管理，复制 V1 为草稿，移动标签并保存。预期锚点不变；换来源后出现待核对候选，V1 仍可浏览。核对发布预览后再决定是否发布。
3. 打开 IFC 源构件树，用键盘选择 Main beam。预期显示真实 GlobalId、实例／类型属性及业务主梁直属值 9；尝试隔离、透明、剖切和标准视角。
4. 对照项目 A／B 及项目空间授权。预期 V2 不自动开放、B 无精确测点标记、查看与原件下载独立；需要额外操作时按授权页预览明确提交。
5. 查看主通道、趋势、设备来源及分析成果。将趋势时段设为 `2024-01-01T00:00:00Z` 至 `2024-01-01T01:00:00Z`，进入历史分析。预期带入通道／时段，仍需明确保存及运行。
6. 手机尺寸点击对象并查看详情；在 GIS 选择空白／本地底图。预期无在线地图依赖，目录始终可操作，缺坐标对象不画在零点。

人工验收结论：**待用户确认**。

## 明确边界与未验证事项

- 实现及自验完成不代表业务人工验收、提交／推送、生产部署或正式数据库迁移；这些动作尚未执行。主工作区合入另见下节记录。
- 验证使用自制样本和实际转换引擎，不能声明任意供应商导出的 IFC／GLB 均无损。支持格式、扩展、几何和大小边界在运行手册中明确；不支持内容返回失败报告。
- 未测试 Safari、Firefox、实体手机 GPU、生产网络、最大模型并发及长期容量。浏览器模拟与合成文件结果只证明本次功能路径；资源上限不是流畅性承诺。
- IFC 浏览材质采用清晰的几何展示；原材料名称及属性保留为来源信息，不承诺完整复制 BIM 软件的渲染效果。三维地球、自动配准、工程设计、结构计算和告警按已确认范围排除。
- 文件撤权在传输前及分块复核；撤权后停止后续读取，不能收回终端已获得的字节。已授权整个图面／模型的用户可以取得浏览产物，原件独立权限不构成防复制保证。


## 主工作区合入（2026-11-13）

按用户要求，将本次空间可视化的 147 个文件（40 个修改、107 个新增）从 `/Users/hijacker/.codex/worktrees/0191/InfraPro` 合入 `/Users/hijacker/Documents/ChatGPT/InfraPro` 的 `main` 工作区。两个 checkout 的基线均为 `ab8b2feee207b00829ab8765848d11af02c438b7`，本次按文件核对并合入，未创建分支、worktree 或提交，Git 暂存区保持原状。

主工作区原有 ClickHouse 配置、派生权限升级修复、配套测试和 AGENTS.md 保留。本次唯一兼容调整是将已有项目权限测试的总数／普通可分配数从 54／44 更新为 57／47，对应新增的三项空间权限；原有派生权限断言含义保持不变。

合入前文件备份、修改清单及检查日志位于 `.artifacts/spatial-main-merge/2026-11-13T04-17-44Z/`。空间原始样本包与实现阶段证据已复制到主工作区 `.artifacts`，运行中的旧演示及其登录资料仍留在原工作区。普通主工作区服务没有重启，也没有向其业务数据库执行迁移。

合入后自验全部通过，人工验收状态继续为 Awaiting acceptance：

| 主工作区检查 | 结果 | 日志 |
| --- | --- | --- |
| `pnpm install --frozen-lockfile`、`pnpm spatial:setup` | 锁定依赖安装通过。 | `install.log`、`setup.log` |
| `pnpm check` | Go、ESLint、类型、契约及权限同步通过。 | `check.log` |
| `pnpm build` | Go 与 console 构建通过；保留已有按需 chunk 大小提示。 | `build.log` |
| `pnpm test:spatial` | 9 项真实空间集成、7 项转换器、5 项客户端检查通过。 | `spatial-tests.log` |
| 项目模块 `go test -race -count=1` | 完整项目模块通过，包含主工作区既有派生权限升级／冻结保护回归，耗时约 459 秒。 | `projects.log` |
| 文件与文档核对 | 147 个合入文件逐项核对、原有修改保护、37 份样本哈希、11 份文档链接与代码围栏及 `git diff --check` 均通过。 | `manifest.json` |

上述日志均位于本节的合入目录。本次没有改动浏览器功能逻辑，沿用已完成的浏览器证据；运行中的演示没有迁移或重启。

