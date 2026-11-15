# 工程空间合成样本

本目录全部图纸、几何、属性和地理坐标由 InfraPro 样本生成器自制，按 CC0-1.0 提供。它们用于功能演示，不代表真实工程、测绘成果或结构安全判断。原始文件可直接在「工程空间 → 管理资料」重新上传。

## 文件与预期

| 文件 | 可核对内容 |
| --- | --- |
| `bridge-*`、`slope-*` 的 PNG／JPEG | 桥梁及边坡平面、立面、断面；图纸自带 synthetic 标识、参考点和比例示意。 |
| `bridge-layouts-v1.pdf`、`slope-layouts-v1.pdf` | 三页平／立／剖面；归一化位置以统一显示方向的左上角为原点。 |
| `bridge-layouts-v2.pdf` | 变为两页、重排和裁剪，第一页旋转 90°；V1 挂载只能作为候选，须逐项核对。 |
| `bridge-v1.glb`、`slope-v1.glb` | 自包含静态网格、层级、TRS 与实例；能点选真实表面并挂点。 |
| `bridge-v2.glb` | 移动桥面／主梁、删除部分桥墩、同名基础使用新身份；不能按名称自动继承。 |
| `bridge-mm-z-up.glb` | 原局部毫米／Z 向上坐标通过根节点变换转为合法 glTF 的米／Y 向上；查看器不得丢掉节点变换。 |
| `bridge-ifc2x3-v1.ifc`、`bridge-ifc4-v1.ifc` | 实际 IFC 模型：拉伸实体、映射实例、空间层级、稳定 GlobalId、主梁类型属性和材料说明。 |
| `bridge-ifc4-v2.ifc` | 稳定 ID 的元素移动、桥墩缺失、基础换 ID；固定旧版继续存在。 |
| `slope-ifc4-v1.ifc` | IFC4 三角面集；合法无类型对象如实显示无类型信息。 |
| `reference.geojson`、`reference-v2.geojson` | WGS84 的 Point、MultiPoint、LineString、MultiLineString、Polygon、MultiPolygon；V2 移动参考点。 |
| `tiles/12/3372/1553.png` | 自制瓦片原始示例；应用的本地示意底图由同源 API 提供，不依赖在线地图服务。 |
| `invalid/` | 损坏 PDF／GLB、未知必需扩展、外链资源、非法坐标、IFC4X3、网格映射缺失及像素超预算，均应处理失败而非发布部分结果。 |

`manifest.json` 列出每份原始资料的大小与 SHA-256。`expected.json` 列出稳定业务含义及预期值；每次装载产生的真实 UUID、固定版本和项目入口另存会话的 `spatial-acceptance.json`。

## 生成和装载

从仓库根目录执行：

```bash
pnpm spatial:setup
pnpm spatial:samples
pnpm spatial:demo
```

依赖版本与完整哈希锁定在 `tools/spatial-converter/requirements.lock`。IFC 身份由固定种子生成；PDF 等容器可能带创建时间，重新生成后以新清单为准，不承诺跨平台逐字节一致。普通应用启动和数据库迁移不会加载这些样本。

独立演示会创建两租户、两个项目、桥梁／边坡、业务构件、测点、两台换源设备、真实原始及派生数据、固定分析成果和已发布图模。项目 A 有完整业务范围；项目 B 初始只有设施及 A 通道上下文，没有精确测点标记、设备详情或项目 A 成果。停止演示进程清理本次隔离数据，再次运行即恢复初始场景。操作及运行说明见 [空间运行手册](../../docs/operations/spatial-visualization.md)。
