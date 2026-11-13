# 空间可视化技术研究：地图、三维 / BIM 与布设图

日期：2026-11-12。阶段：**基本方案讨论**。本文是技术研究与工程建议，不代表已经选型、实施或部署，也不改变现有正式方案的状态。

后续整理：用户已认可总体方向，交付范围及详细约定已归入[主需求](../requirements.md#spatial-scope)、[架构设计](../architecture.md#spatial-design)及[规划选型](../technology-selection.md#spatial-technology)。本文件保留研究依据，下面的早期候选说明不覆盖这些正文；功能尚未实施。后续所需图模样本由 Codex 准备，不要求用户提供。

讨论范围：导入和浏览外部已有图件 / 模型，在其中关联已有设施、构件、测点，并通过既有关系查看设备和监测数据；不做模型设计。用户当前没有图件 / 模型资料，基本方案讨论不以提供资料为前提，本文不设置验收流程或性能指标。

文中“官方事实”来自标准组织、工具维护者或厂商的一手资料；“工程建议”是针对本次范围的设计判断。网络文档核查日期均为 2026-11-12。

## 1. 建议基线

**工程建议：采用二维 GIS、设施级三维 / BIM、布设图三种视图，共用已有业务对象的关联；技术起点为 OpenLayers + Three.js + 服务端 IfcOpenShell 转换。**

- OpenLayers 负责二维地理地图和局部布设图，两种视图分别使用地理坐标与图面坐标。
- Three.js 负责设施级三维模型的浏览、点选和测点标记。
- IfcOpenShell / IfcConvert 负责后台读取 IFC，生成用于显示的 GLB，以及独立的构件属性和身份映射。源 IFC 保留为语义依据。
- 地形、长距离地理配准、大范围倾斜摄影或分层加载等完整三维地理场景，留作后续 CesiumJS / 3D Tiles 路线。

这是一套候选基线。选择它的理由是让二维视图、设施三维视图和格式转换各承担明确职责；本文没有做引擎性能对比，也不同时引入全部候选引擎。相关能力依据见下文。

## 2. 概念与格式边界

| 概念 | 官方事实 / 本次用途划分 | 输入与浏览方式建议 |
| --- | --- | --- |
| 地图 / GIS | 本次将地图定位为设施与测点的地理分布视图；GIS 还包含坐标参考、空间数据和图层组织，不能等同于一张背景图片。OpenLayers 官方列明支持瓦片、标记、矢量数据与 OGC 地图服务。[OpenLayers](https://openlayers.org/) | 使用已获授权的地图服务、瓦片或明确坐标参考的空间数据；业务标记独立于底图。 |
| 布设图 | 本次指平面图、剖面图、示意图上的局部位置表达，不要求具备地理坐标。OpenLayers 官方示例使用自定义像素投影显示静态图片。[Static Image](https://openlayers.org/en/latest/examples/static-image.html) | 起步采用 PNG / JPEG 等图面；PDF / CAD 图件经明确的页面选择或外部导出成为图面。浏览和标注不等于 CAD 编辑。 |
| 三维模型 | glTF 是运行时三维资产传输格式，主要承载可渲染的场景、几何和外观；GLB 是其二进制容器形式。[glTF 2.0](https://registry.khronos.org/glTF/specs/2.0/glTF-2.0.html) | 一般三维资产以 GLB 作为候选浏览格式。没有构件语义的数据只能按普通三维模型使用。 |
| BIM / IFC | IFC 不只是几何：其数据模型表达身份、语义、属性及关系，覆盖建筑与土木基础设施。IFC 是 BIM 数据交换标准，BIM 本身不是某一个文件扩展名。[buildingSMART IFC](https://technical.buildingsmart.org/standards/ifc/) | IFC 作为有语义的导入来源；几何转换为浏览资源，构件身份、属性及关系另行保留。 |
| 3D Tiles | OGC 将其定义为组织和流式传输大规模三维地理内容的层次结构；1.1 以 glTF 作为主要瓦片内容格式。它解决空间组织与分层加载，不能替代 IFC 的业务语义。[OGC 3D Tiles](https://www.ogc.org/standards/3DTiles/)、[1.1 规范第 7 章](https://docs.ogc.org/cs/22-025r4/22-025r4.pdf) | 在需要大范围三维地理场景时考虑；一个 GLB 文件与一个分层 tileset 不是同一种交付。 |

**工程建议：不要把所有输入统一称为“BIM”，也不要把“支持 IFC”描述成“浏览器直接支持所有模型格式”。** 文件接入范围、保留的语义和显示能力应分别描述。

## 3. 技术基线与替代路线

### 3.1 二维：优先 OpenLayers

**官方事实：**OpenLayers 支持地图瓦片与矢量图层，且有将图片坐标直接映射为视图坐标的官方示例。因此，同一个二维引擎可以支撑地理地图和独立布设图。[地图能力](https://openlayers.org/)、[图片坐标示例](https://openlayers.org/en/latest/examples/static-image.html)

**工程建议：**两类视图复用标记、选中、定位等交互，但分别声明坐标空间。图片上的一点不能因为使用了地图引擎，就被当作真实经纬度。

MapLibre GL JS 是有效替代项：它面向浏览器矢量地图，当前官方文档也已有 identity projection 下显示平面图片的示例。选择 OpenLayers 是本次职责与使用路径的判断，不能以“MapLibre 不支持平面图”为理由。[MapLibre 简介](https://maplibre.org/maplibre-gl-js/docs/)、[平面图片示例](https://maplibre.org/maplibre-gl-js/docs/examples/display-an-image-in-the-identity-projection/)

### 3.2 设施三维：Three.js + GLB

**官方事实：**Three.js 的 `GLTFLoader` 加载 glTF 2.0；`Raycaster` 提供鼠标点选所需的射线相交能力。[GLTFLoader](https://threejs.org/docs/pages/GLTFLoader.html)、[Raycaster](https://threejs.org/docs/pages/Raycaster.html)

**工程建议：**Three.js 承担模型显示、相机操作、构件高亮与测点位置表达；业务关联和数据查询由 InfraPro 保持。点中某个网格后，先解析该模型版本的构件映射，再找到已有业务对象，不能把前端网格对象直接当作工程构件或测点。

### 3.3 IFC：优先服务端转换，保留浏览器解析备选

**官方事实：**IfcConvert 可以将 IFC 几何转换为 GLB，另有 XML / JSON 属性与分解树输出；IfcOpenShell Python 提供属性集与数量集查询能力。[IfcConvert 用法](https://docs.ifcopenshell.org/ifcconvert/usage)、[属性查询 API](https://docs.ifcopenshell.org/autoapi/ifcopenshell/util/element/index.html#ifcopenshell.util.element.get_psets)

**工程建议：**以 Python 调用 IfcOpenShell，后台生成几何派生物、属性索引和转换映射；前端读取已生成结果。这样可以统一转换配置和结果版本，并将转换过程与日常浏览分开。既有业务服务只需管理资产、版本和关联，不必承担几何解析。

**官方事实：**That Open 的 `web-ifc` 能通过 JavaScript / WebAssembly 读取 IFC 的几何与属性，浏览器直接解析 IFC 在技术上成立。[web-ifc 官方仓库](https://github.com/ThatOpen/engine_web-ifc)

**工程建议：**将 web-ifc / That Open 保留为后续需要直接 IFC 交互时的替代路线，不在当前基线中同时维护两套解析管线；也不从其技术能力推导所有 IFC 文件都适合在终端即时处理。

### 3.4 CesiumJS 与 ion 的关系

**官方事实：**CesiumJS 面向三维地球、地形、影像和 3D Tiles，可使用 ion 或其他在线 / 离线内容来源。官方离线指南明确：默认外部数据依赖均不是必需依赖，可以改用本地数据并关闭在线地理编码等功能。[CesiumJS 官方说明](https://github.com/CesiumGS/cesium)、[Offline Guide](https://github.com/CesiumGS/cesium/blob/main/Documentation/OfflineGuide/README.md)

**工程建议：**“自托管 CesiumJS 与自有数据”不等于“部署 Cesium ion”。ion 是另一个切片、托管和内容服务产品；它也有 Self-Hosted 部署方式，但并非使用 CesiumJS 的前提。[ion 平台说明](https://cesium.com/platform)、[ion Self-Hosted](https://cesium.com/learn/ion/self-hosted/)

设施级模型能满足当前空间浏览时，可先使用 Three.js；需要地形与多个地理配准数据源共同浏览时，再引入 CesiumJS。该判断是范围选择，不是两个引擎的优劣结论。

## 4. 构件身份、语义与模型版本

### 4.1 标准身份不等于运行时对象标识

**官方事实：**`IfcRoot` 为其派生实体提供 `GlobalId`；名称是另一项可选属性。glTF 的数组索引用于文件内部引用，`name` 并不保证唯一，`extras` 是应用自定义数据。[IFC IfcRoot](https://standards.buildingsmart.org/IFC/RELEASE/IFC4/FINAL/HTML/schema/ifckernel/lexical/ifcroot.htm)、[glTF 2.0 第 3.3、5.16 节](https://registry.khronos.org/glTF/specs/2.0/glTF-2.0.html)

**工程建议：**分别保存三种身份，不互相替代：

| 身份 | 建议作用 |
| --- | --- |
| InfraPro 已有设施 / 构件 / 测点 ID | 业务关联的主身份；与设备和通道的既有关系保持一致。 |
| 来源资产及其版本 + IFC `GlobalId` | 标识这一来源模型中的源实体；类型对象和构件实例分别处理，测点挂载不能只定位到共享类型。 |
| 浏览资源版本 + 运行时节点 / 图元定位 | 用于点选命中和高亮；仅在对应的几何派生版本内有效。若有实例化，还需区分具体实例。 |

不使用显示名称、数组顺序、临时运行时 ID 或空间邻近程度作为持久业务主键。一个源构件可以生成多个渲染片段，也可能共享几何；映射要表达具体构件实例，而不只是“这个 mesh 来自哪个类型”。

### 4.2 转成 GLB 不自动等于保留 BIM

**官方事实：**IfcOpenShell 将几何序列化与属性查询分别提供；其文档示例指出，可使用实体 GUID 辅助识别非语义输出中的对象。单纯生成几何文件并不等于完成 BIM 属性与关系交付。[几何处理](https://docs.ifcopenshell.org/ifcopenshell-python/geometry_processing.html)、[属性查询](https://docs.ifcopenshell.org/autoapi/ifcopenshell/util/element/index.html#ifcopenshell.util.element.get_psets)

**工程建议：**同一次转换应形成一个完整版本包：

```text
源 IFC 版本
  ├─ 几何派生物：GLB
  ├─ 语义索引：GlobalId、实体类别、名称、属性/数量、单位、类型与分解关系
  └─ 转换映射：该版本源构件实例 ↔ 该版本可点选节点/图元

已有测点 ID ↔ 关联记录 ↔ 资产版本、源构件和局部位置
```

属性应保留来源与层次，区分实例属性和类型继承属性；不能只摊平成同名键值后丢失含义。保留源 IFC，浏览索引只作为派生结果；暂不展示的属性不等于应从来源中删除。普通 GLB 若没有来源构件身份和属性，只提供一般三维关联，不推测出不存在的 BIM 语义。

技术细节尚有文档边界：IfcOpenShell 的几何处理示例含 `use-element-guids`，但序列化选项页对该选项列举的适用格式未列 GLB。因此，基本方案可以承诺“转换管线负责身份映射”，不能仅凭某个开关就承诺映射完整。具体适配应以实际采用版本的输出约定为准。[几何处理示例](https://docs.ifcopenshell.org/ifcopenshell-python/geometry_processing.html)、[序列化选项](https://docs.ifcopenshell.org/ifcopenshell/serialiser_settings.html)

### 4.3 新版本不能静默改变旧关联

**官方事实：**Autodesk 说明 Revit 可以保存导出时的 IFC GUID，供后续导出继续使用；重建元素等工作流需要维持对应参数。可见跨导出的身份延续还取决于源工具和工作流，而不是文件名相同就天然成立。[Autodesk：Unique Id and IFC GUID Parameter](https://blog.autodesk.io/unique-id-and-ifc-guid/)

**工程建议：**新版本生成新的几何、属性和映射；已有绑定保留其原版本依据。只有明确、无冲突的来源身份映射才可作为迁移依据。名称相同或位置相近可以用于辅助查找，不能自动完成关联迁移。

即使 `GlobalId` 延续，构件几何也可能改变；原来绑定的三角面、局部锚点未必仍有效。无法确定时应保留原关联并标记需重新定位，不能把原测点静默移动到最近表面。基本方案阶段无需设计完整模型差异审查工具。

## 5. 坐标、轴向与地理配准

**官方事实：**glTF 使用右手系、Y 轴向上、长度单位为米。IFC 的 `IfcMapConversion` 描述局部工程坐标到地图坐标的转换；它并不执行从地理坐标到投影平面的地图投影。`IfcProjectedCRS` 还涉及地图单位及水平 / 垂直参考。[glTF 坐标与单位](https://registry.khronos.org/glTF/specs/2.0/glTF-2.0.html#coordinate-system-and-units)、[IfcMapConversion](https://standards.buildingsmart.org/IFC/RELEASE/IFC4/ADD1/HTML/schema/ifcrepresentationresource/lexical/ifcmapconversion.htm)、[IfcProjectedCRS](https://standards.buildingsmart.org/IFC/RELEASE/IFC4_3/HTML/lexical/IfcProjectedCRS.htm)

**工程建议：**坐标必须连同其含义保存，三种视图不要共用一个无类型的 `x/y/z`：

| 视图 | 需要保存的坐标依据 |
| --- | --- |
| 地理地图 | 来源 CRS、经纬度或投影坐标类型、轴顺序、水平单位；涉及高程时另存高程基准与单位。 |
| 设施三维 | 源模型坐标、原点、轴向、单位、模型层级变换，以及转换到浏览坐标的矩阵。 |
| 布设图 | 图件版本、页 / 图面标识、图面范围、原点与方向；可采用归一化二维坐标，但图片换版后不能据此假定位置仍正确。 |

设施模型可以先独立浏览，没有地理配准不妨碍其内部关联测点；缺少配准关系时，不宣称可以自动贴合地图。需要联动时显式维护模型与地理坐标的转换，而不是靠肉眼拖动后覆盖原始工程坐标。

**官方事实：**IfcConvert 提供针对大地图坐标的居中和偏移选项。[IfcConvert 用法](https://docs.ifcopenshell.org/ifcconvert/usage)

**工程建议：**若转换时采用局部原点，必须保存偏移和逆变换。不能因“让模型显示正常”而丢失地理位置；轴向、单位、投影和基准转换也不能靠一次固定旋转统一解决。

**官方事实：**高德文档区分 WGS84、GCJ-02、BD-09，并提供其他坐标转高德坐标的接口。[高德坐标转换](https://developer.amap.com/api/javascript-api-v2/guide/transform/convertfrom)

**工程建议：**国内底图接入记录供应商坐标约定，在显示边界使用其授权转换能力，保留原始测量坐标。通用投影变换不应被当作供应商偏移转换；也不能把显示偏移后的坐标反写为工程测量基准。底图服务与转换接口的使用范围随供应商约定处理，本文不扩展法律结论。

## 6. 原生格式、部署与容量边界

### 6.1 RVT / DWG 等通过明确入口适配

**官方事实：**Revit 提供 IFC 导出及可保存的导出配置；AutoCAD 提供 DWG 图纸到 PDF 的输出功能。[Revit IFC 导出配置](https://help.autodesk.com/cloudhelp/2026/ENU/Revit-DocumentPresent/files/GUID-587E3CB5-CA76-4F06-B8E6-08F0C71945F6.htm)、[AutoCAD PDF 输出](https://help.autodesk.com/cloudhelp/2024/ENU/AutoCAD-DidYouKnow/files/GUID-611C1CF1-DBE5-42E8-AAEB-937AACED4BD9.htm)

**工程建议：**RVT 先由源工具导出 IFC，DWG 布设图先导出图面；若将来接入自动转换服务或 SDK，应单独限定支持版本、外部依赖和输出内容。这是受控适配入口，不是承诺浏览器直接读取任意 RVT / DWG。导出只保留图面时，原 CAD 构件结构不能被视为已保留。

### 6.2 软件组件与地图 / 模型内容分别处理

**官方事实：**OpenLayers 使用 BSD-2-Clause，Three.js 使用 MIT，CesiumJS 使用 Apache-2.0；IfcOpenShell 仓库按子组件列出许可证，IfcConvert / Python 库列有 LGPL 条目及附注。[OpenLayers](https://openlayers.org/)、[Three.js LICENSE](https://github.com/mrdoob/three.js/blob/dev/LICENSE)、[CesiumJS](https://github.com/CesiumGS/cesium)、[IfcOpenShell 组件列表](https://github.com/IfcOpenShell/IfcOpenShell)

**工程建议：**自托管浏览器代码，不代表自动获得底图、影像、地形或模型内容的使用与分发权。组件、转换器分发物和内容来源分别记录；不以开源引擎替代地图源授权。具体依赖版本和部署打包方式留在实施设计中确定。

### 6.3 不从文件扩展名推导容量承诺

**官方事实：**IfcOpenShell 官方区分 IFC schema 的解析支持与几何支持；能解析某一 schema，不等于所有实体都具有同等几何处理覆盖。[IfcOpenShell](https://github.com/IfcOpenShell/IfcOpenShell)

**官方事实：**GLB 仍允许引用外部资源；Three.js 的 glTF 扩展支持具有明确列表，部分压缩资源需要相应解码器。[glTF GLB 结构](https://registry.khronos.org/glTF/specs/2.0/glTF-2.0.html#glb-file-format-specification)、[GLTFLoader](https://threejs.org/docs/pages/GLTFLoader.html)

**工程建议：**可浏览能力取决于几何复杂度、纹理、实例组织、扩展支持和终端资源，不能仅凭“多少 MB”保证体验。自托管时把纹理、解码器与模型引用资源一并纳入资源包。现阶段只保留按设施 / 资产拆分的结构与后续分层加载空间，不给出未经实测的帧率、构件数或文件大小承诺。

## 7. 当前讨论可形成的结论

三种视图可以围绕同一组已有业务对象组织；BIM 导入需要同时处理几何和语义；模型设计不在本轮范围。起点建议为 OpenLayers、Three.js 和后台 IFC 转换，完整三维地理场景作为后续方向。

尚未确定的是具体组件版本、最终输入格式清单、BIM 属性展示深度、地图来源和模型配准方式。这些是后续细化项，不阻止现在讨论基本方案，也不要求用户现在提供图件或模型。
