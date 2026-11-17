# 自制交通工程空间资料

这些资料是按照工程构造自行设计的仿真数据，CC0-1.0；不是用户真实项目资料、实测成果或可施工设计。四类工程都包含 3 页 A3 矢量 PDF、IFC4、GLB 和 WGS84 GeoJSON。模型采用米 / Y 向上，IFC 导出与转换保留同一几何、实体身份、材料和构件语义。

| 工程 | 主要构造 | 几何实体 | 三角形 | GIS 要素 | 原始测点 |
| --- | --- | ---: | ---: | ---: | ---: |
| 青澜二桥 | 3 × 40 m、18 m 桥面；双幅双室空心箱梁、横隔梁、圆柱墩、盖梁、支座、承台桩基、墩台翼墙、伸缩缝、护栏、泄水管 | 300 | 9,168 | 35 | 8 |
| 北山2号隧道 | 双洞 120 m 展示段；拱形分段衬砌、边墙、仰拱、开放洞门、步道、排水管、照明与风机；右洞局部剖开 | 613 | 8,124 | 63 | 6 |
| 南岭K18+600边坡 | 三级 8 m 坡面与平台、框架梁、锚杆、锚头、截排水沟、急流槽、坡脚挡墙和道路 | 465 | 9,748 | 19 | 9 |
| S218沿江示范段 | 160 m 路段、23 m 路基；分层路面、填方边坡、标线、护栏、边沟、标志和轮廓标 | 207 | 7,028 | 12 | 4 |

总计 1,585 个几何实体、34,068 个三角形、129 个 GIS 要素，原始资源有 25 个业务构件、27 个测点。业务构件可对应多个模型实体，不将每根栏杆都塞进业务树。当前系统另有浏览器验收创建的支座温度测点 `QL-WD-01`，因此当前共 28 个测点。

桥梁基础和隧道顶拱采用展示剖切以暴露内部构造；边坡锚固和路面结构以几何表达构造关系，不含结构验算、施工配筋或实测地质。GIS 线位和经纬度为合成坐标；底图可切换空白背景。PDF 分别为总平面、纵向立面、典型断面，包含尺寸、图号、监测点和统一坐标表。

## 生成、装载与核对

```sh
pnpm spatial:setup
.artifacts/spatial-venv/bin/python tools/spatial-converter/engineering.py
node scripts/spatial-engineering-load.mjs
node scripts/spatial-engineering-verify.mjs
```

生成器使用已锁定的空间转换依赖及 ReportLab。`manifest.json` 记录文件 SHA-256、实体身份、坐标、PDF 归一化点位、投影区域和统计值。默认输出本目录。

装载器仅允许当前本地验收地址 `localhost:25173`，读取既有本地账号文件，不打印凭据；通过正常接口创建独立工程类型、设施、子构件、测点，分片上传、处理、关联、预览并发布。不会清理已有资源。装载记录放在用户运行目录的 `engineering/spatial-engineering-v2.json`；同一来源再次运行不会重复创建，来源文件变化则产生新版本。

核对器是只读接口检查，输出 `.artifacts/spatial-engineering/api-verification.json`。浏览器验收记录与入口见 [设施树工作台说明](../../../docs/development/spatial-tree-workspace.md)。

## 参考

构造关系参考 FHWA [箱梁设计手册](https://www.fhwa.dot.gov/bridge/concrete/hif15016.pdf)、[公路隧道手册](https://www.fhwa.dot.gov/bridge/tunnel/pubs/nhi09010/tunnel_manual.pdf)；IFC 语义参考 [buildingSMART IFC4](https://standards.buildingsmart.org/IFC/RELEASE/IFC4/ADD2_TC1/HTML/)。所有尺寸、测点布置和线位由本次仿真设计确定。
