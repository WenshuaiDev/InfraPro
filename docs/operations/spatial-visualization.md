# 工程空间运行与操作

适用版本：迁移 00034，空间转换契约 `infrapro.spatial/1`。实现与自验见 [本机工程环境与人工验收](local-environment.md)。正常运行沿用既有账号、租户、项目、工程资源和监测模块。

## 隔离回归环境

前置条件为仓库规定的 Go、Node、pnpm、Python 3.12／uv、Docker 和 Playwright Chromium；不需要用户制作图纸、购买底图或准备 IFC。

```bash
pnpm install --frozen-lockfile
pnpm spatial:setup
pnpm spatial:samples
pnpm test:spatial:environment
```

测试控制台固定为 `http://127.0.0.1:15473`，专用 API 为 `127.0.0.1:18480`。脚本检测端口占用，不终止其他服务；创建隔离数据库 schema 和测试对象前缀，通过正式 API 建立业务资源、上传、处理、发布和授权。原有业务数据库不迁移、不清空。启动约需数分钟，完成后打印固定版本入口及登录资料文件路径。

会话目录为 `.artifacts/spatial-tests/<UTC时间>/`：`login.json` 为本次随机登录资料（本地权限 0600）；`spatial-acceptance.json` 记录对象、文件哈希、固定版本及两项目入口；`session.json` 的 `state: "ready"` 表示装载完成。不要分享登录文件。Ctrl+C 停止该会话后清理其隔离数据，重新运行即恢复初始场景。若装载失败，服务保留供诊断，`session.json` 标记 `seed_failed`；停止后再启动可重建。

原始文件位于 [samples/spatial](../../samples/spatial/README.md)，既能复现已有场景，也能从「管理资料」自行重新上传。所有图件均为合成示例。

## 常用操作

1. 在租户「工程空间」选择资源或资料。地图只显示有有效地理坐标的对象；缺少坐标仍可在目录中选择。选中测点后用「空间位置」切换图纸和模型，身份保持一致。
2. 新建空间资料，填写类型、设施、分类及来源。创建草稿后选择原件，上传并校验，然后启动处理。刷新后可继续原上传，须重新选择相同文件；更换文件先取消旧上传。处理报告失败时核对来源，支持人工重试或取消。
3. 在「编辑挂载、关联与默认视图」选择已有对象。图纸支持点击锚点、单独拖标签、绘制区域；模型支持点击表面或输入米制坐标，源树可用键盘选择。应用到草稿后点「保存草稿」；取消不改已保存内容。发布前查看预览。发布版本不可编辑，修正须复制新草稿。
4. 原件相同的复制草稿复用文件；更换来源生成待核对候选。缺失、移动、同名换 ID 和 PDF 方向变化都需人工确认。旧版锚点不随工程位置修正移动，会显示依据变化。
5. 在租户项目治理的空间授权页选择明确版本，分别设置查看和原件下载并预览提交。租户发布 V2 不自动改项目 V1。业务标记、数据、设备和成果分别取当前授权交集；图面／背景几何属于整个获准资料内容。
6. 项目可保存本版本默认图页、相机、隐藏元素、底图和主通道。个人旋转／缩放是临时查看；保存才修改项目视图。归档后仍能读取获准版本，不能扩权或保存共享视图。撤回版本会结束对应授权；结束设施关联后重新关联不会复活旧授权。
7. 右侧显示直接属于对象的通道。设施和构件不隐式汇总下级。值保留服务端精度，派生值展示控制状态、计算状态、新鲜度、推进时间及上次有效结果。设备当前来源与观测历史依据分开。
8. 分析成果按实际保存输入反查；设施下级须明确勾选。成果链接固定版本且限定原项目。进入「新建历史分析」带入通道与所选时段，核对规则和量纲后显式运行。

## 接入普通开发环境

空间元数据使用 PostgreSQL，原件和浏览文件使用既有私有 S3 兼容服务中独立 `spatial/` 前缀。使用常规迁移命令应用 00034；既有自定义角色不自动扩权，按目录明确授予空间权限。对既有环境升级前先备份。

API 与独立 worker 使用同一数据库、对象存储和转换器配置：

```bash
export INFRAPRO_SPATIAL_PYTHON="$PWD/.artifacts/spatial-venv/bin/python"
export INFRAPRO_SPATIAL_CONVERTER_SCRIPT="$PWD/tools/spatial-converter/convert.py"
export INFRAPRO_SPATIAL_TEMP_DIRECTORY="/tmp/infrapro-spatial"
pnpm dev:spatial
```

`dev:spatial` 启动 API、console 和 spatial-worker；监测导入／派生任务还需按监测模块的常规方式启动 monitoring-worker。对象存储复用 `INFRAPRO_MONITORING_OBJECT_ENDPOINT`、`INFRAPRO_MONITORING_OBJECT_ACCESS_KEY`、`INFRAPRO_MONITORING_OBJECT_SECRET_KEY`、`INFRAPRO_MONITORING_BUCKET`。配置字段的实际映射以 `packages/go/platform/config` 为准，不将访问密钥放入前端或版本正文。

也可构建 `deploy/docker/spatial.Dockerfile`，镜像内同时提供 `api` 与 `spatial-worker` 两个命令，分别作为进程运行。镜像采用 Python 3.12／Debian trixie，满足实际 IFC wheel 的 glibc 要求；使用非 root 用户并在构建中运行依赖检查。部署时仍需配置持久数据库、私有对象存储、可写临时目录及监测服务；构建通过不等于生产部署完成。

可选 `INFRAPRO_SPATIAL_BASEMAPS` 为最多 8 个受控 XYZ 配置的 JSON 数组，字段为 `id,name,url,attribution,min_zoom,max_zoom`。URL 必须为 HTTPS 或同源路径，包含 `{z}/{x}/{y}`，缩放在 0–22；不接受 URL 用户凭据。默认空白与同源本地示意底图始终可用。在线源失败时界面提供切回本地的路径。

## 格式与运行边界

| 项目 | 当前边界 |
| --- | --- |
| 原件 | PNG、JPEG、PDF、自包含静态 GLB、IFC2X3／IFC4、WGS84 GeoJSON；单件 256 MiB。 |
| 上传 | 8 MiB 分片、总大小由 `uploads.spatial_max_file_bytes` 配置（默认 256 MiB）、24 小时窗口；每片及整件 SHA-256，封存后不可替换。 |
| 图像／PDF | 单图最多 32 百万像素，文档累计最多 256 百万像素，保存方向／裁剪及多级分辨率。 |
| 模型 | 10 万元素／实例、展开后 500 万三角形；支持完整节点变换、IFC 实体／映射／三角面几何与来源索引。 |
| GLB 扩展 | 支持 `KHR_materials_unlit`、`KHR_texture_transform`、`KHR_mesh_quantization`、`EXT_mesh_gpu_instancing`；外部资源、未知扩展、动画、蒙皮、形变等明确拒绝。 |
| 转换 | 每进程一次任务；租约 30 秒、每 5 秒续期；自动抢占最多 5 次，人工重试重新计数并留审计；CPU／时长、地址空间、输出体积和文件数均有限制。 |
| 查询 | 元素、挂载和地图范围分页，每页最多 100；摘要每对象最多 10 通道、每批最多 100，优先配置主通道；超出可进入正式监测工作台。 |
| 坐标 | GIS 显示转换支持 EPSG:4326／3857；局部模型统一米／Y 向上，原变换写入清单；未知 CRS、桩号与高程不伪造地理定位。 |

这些是拒绝和资源保护边界，不能当作所有设备均流畅处理最大文件的性能承诺。实际工程文件应依照其导出内容验证。

## 备份、恢复和文件完整性

一致备份应包含 PostgreSQL 的资料、版本、文件清单、挂载、授权、任务、审计和引用，以及同一备份窗口内 S3 `spatial/` 前缀的全部封存对象。保留被历史／审计引用的来源和产物；不能只备份当前版本或只备份原件。停止写入／worker 后取得一致恢复点，先恢复数据库和对象存储到隔离目标，再校验。

```bash
pnpm infrapro spatial verify --tenant <tenant-uuid>
pnpm infrapro spatial verify --tenant <tenant-uuid> --apply --reason '恢复后核对固定版本和封存文件'
```

命令读取固定清单及实际对象字节，核对大小和 SHA-256。`--apply` 在再次核对版本修订后隔离缺失／不一致版本，并记录维护审计；补回正确原字节后再次校验可恢复可用标记。普通网络故障报告服务错误，不将内容误判为损坏。不从备份自动恢复已结束的项目授权，也不重新转换冒充原固定产物。

恢复测试使用 `pg_dump/pg_restore` 和独立 S3 恢复桶，比较 11 张空间表及文件哈希，并验证缺失隔离、正确字节修复和暂时网络故障。生产保留策略、容量和真实环境恢复时间需按部署条件设定。

## 复核命令

```bash
pnpm check
pnpm build
pnpm test:spatial
pnpm spatial:browser
node scripts/spatial-ui-check.mjs
node scripts/spatial-scope-check.mjs
```

后三项需已完成的独立 `test:spatial:environment` 会话；可用 `SPATIAL_SESSION` 指定会话目录。范围检查会在合成项目中临时修改授权并恢复，交互检查会创建并删除未发布草稿；不应指向业务环境。证据输出在 `.artifacts/spatial-*`。
