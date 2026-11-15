# 工程空间可视化：交付范围与详细方案入口

日期：2026-11-13。总体及详细方案：**Accepted（用户明确确认并授权完整实现）**。功能状态：**Awaiting acceptance（SV-01–SV-10 实现及完整自验通过，待用户人工验收）**。

用户于 2026-11-13 确认方案并要求实现全部 SV-01–SV-10。本文原讨论稿已归并到三份主文档，保留此路径作为方案入口，避免维护重复正文。

## 正式方案位置

| 内容 | 正文 |
| --- | --- |
| 交付目标、范围及领域关系 | [软件需求第18节](../requirements.md#spatial-scope) |
| 图模档案、版本、导入与使用 | [图模管理](../requirements.md#spatial-assets)、[格式与浏览](../requirements.md#spatial-formats) |
| 测点挂载、构件关联及坐标 | [挂载规则](../requirements.md#spatial-placement)、[换版设计](../architecture.md#spatial-version-design) |
| 项目资料版本授权 | [业务规则](../requirements.md#spatial-project-access)、[权限与文件读取设计](../architecture.md#spatial-auth-design) |
| 原始／派生监测及分析成果联动 | [业务衔接](../requirements.md#spatial-monitoring)、[查询接缝](../architecture.md#spatial-monitoring-design) |
| 模块、存储、任务、接口和运行 | [架构第13节](../architecture.md#spatial-design) |
| 技术选择、用途及官方依据 | [技术选型第9节](../technology-selection.md#spatial-technology)、[前期技术研究](spatial-visualization-technical-research.md) |
| Codex 自备资料与后续交付 | [资料及阶段范围](../requirements.md#spatial-delivery)、[样本制作责任](../architecture.md#spatial-samples) |
| 方案及检查状态 | [业务状态](../requirements.md#spatial-status)、[本次文档检查](../architecture.md#spatial-document-check) |

## 本轮整理的重点

- 资料由租户维护，项目使用明确获准版本。资料来源、处理产物、挂载及构件关联按版本固定，发布后修改形成新草稿；新版本不自动开放给项目。
- 同一个测点在图纸、模型及地图中保持同一业务身份；设施和构件另建展示关联，承接各自直接归属的派生通道。
- 图模查看、原件下载、业务对象读取、设备资料和监测／成果权限分别判断。只获得通道的祖先摘要不等于可以看到测点精确位置。
- 派生值使用真实统一查询及递归输入授权；分析成果按原所属空间和固定版本访问，不能因输入共享而共享项目成果。
- **自验和用户操作所需布设图、GLB、IFC、GeoJSON、底图示例、多版本及异常样本全部由 Codex 准备**，已提供原始样本包与装载场景，不把资料制作或购买交给用户。

## 范围与状态说明

覆盖 SV-01–SV-10：空间资料、布设图、三维／BIM、测点挂载、二维 GIS、项目授权、空间工作台、监测联动、分析成果联动与演示交付。三维地理大场景、工程设计、告警及结构仿真不在本轮范围。

总体及详细方案均已确认。本轮实现、原始样本、已装载场景、启动及恢复说明一并交付；功能自验与用户人工验收分开记录。当前状态见[空间验证记录](../verification/spatial-visualization.md)，使用入口见[空间运行手册](../operations/spatial-visualization.md)。本次没有生产部署。
