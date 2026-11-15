# InfraPro 文档

整理日期：2026-11-12。状态：**Awaiting acceptance（文档自检通过，待用户验收）**。

除正在独立实施的监测数据外，项目文档统一维护在以下三份主文档中，不再按每次迭代分别保存需求、页面改造和运行手册。

| 主文档 | 内容 |
| --- | --- |
| [软件需求规格说明书](requirements.md) | 目标与范围、角色与归属、完整业务规则、页面交互、非功能要求、人工验收及状态。 |
| [技术选型](technology-selection.md) | 当前语言、框架、存储、组件及工具的版本来源、用途、选择理由和使用边界。 |
| [架构设计](architecture.md) | 系统结构、模块职责、数据与事务、接口、前端状态、部署配置、迁移恢复及验证方式。 |

## 平台基础能力深化方向

- [独立运行诊断入口详细开发方案](requirements/independent-runtime-diagnostics-proposal.md)：独立进程与前端、拓扑及状态数据、HTTP／报告协议、独立权限、全部现有模块改造、节点资源和部署适配；60 项验收条件以本机原生、隔离容器及多观察器完成，真实部署证据单列。详细设计待核对，尚未实施本方案代码。
- [平台基础能力深化与下一轮范围建议](requirements/platform-foundation-deepening-proposal.md)：当前优先公共能力，核心监测业务后置；任务中心、文件与附件及审计与操作追踪已实现并自验通过，待人工验收。
- [统一任务中心与现有任务接入详细开发方案](requirements/task-center-development-plan.md)：8 项交付、14 类现有人员任务、数据与接口契约、来源及通知改造、迁移恢复、24 项验收条件和实施顺序。完整实现与自验已完成，待人工验收。
- [任务中心自验记录](verification/task-center.md)、[运行／升级／恢复](operations/task-center.md)、[合成人工验收](operations/task-center-acceptance.md)及[新模块接入指南](development/task-center-integration.md)。
- [文件与附件公共能力迭代范围与约束](requirements/files-and-attachments-proposal.md)：8 项交付及原 20 项验收条件；用户已确认范围，完整实现与自验通过，待人工验收。
- [文件与附件公共能力详细开发方案](requirements/files-and-attachments-development-plan.md)：表结构与约束、上传和附件状态机、HTTP／Go／前端契约、六个权限、现有模块适配、任务与通知联动、运行及恢复；12 项代码改造和 44 项具体验收条件。完整实现与自验通过，软件 Awaiting acceptance。
- [审计与操作追踪详细开发方案](requirements/audit-and-operation-tracing-development-plan.md)：统一事实与关联数据、三空间授权查询、Go／HTTP 契约、14 组来源改造、任务／文件／通知衔接、受控 CSV 导出、保留及恢复；8 项交付、12 项实施任务和 70 项验收条件。全部开发与 70/70 项软件自验通过，Awaiting acceptance（待人工验收）；证据见[逐项自验账本](verification/audit-and-operation-tracing.md)。

- [租户资料 B 方案改造与验收](verification/tenant-profile-ui.md)：租户名片、紧凑附件、原位预览与按权限提供的管理入口；不展示权限清单，含真实后端回归和人工验收步骤。
- [项目资料 A 方案改造与验收](verification/project-profile-ui.md)：项目简报突出业务目的、计划周期和责任分工，管理操作按状态与资格提供，附件在原位查看；含真实后端自验与人工验收步骤。
- [项目角色 A 方案改造与验收](verification/project-roles-redesign.md)：统一角色表格与遮罩 Drawer，权限说明整合到详情和配置，保留真实操作与授权边界；含接口、桌面／手机回归及人工验收步骤。

## 系统级站内通知方案

- [系统级站内通知交付范围与代码重构方案](requirements/in-app-notifications-proposal.md)：按 2026-11-13 用户要求，将原预警迭代收窄为统一人员站内通知；覆盖现有业务接入、任务终态提交、本人租户邀请入口、通知权限与大屏事件衔接。完整接入与自验状态以[验证记录](verification/in-app-notifications.md)为准；[稳定需求](requirements.md#in-app-notifications)、[架构](architecture.md#in-app-notifications)与[选型](technology-selection.md#in-app-notifications)已并入主文档。
- [通知运行与合成人工验收](operations/in-app-notifications.md)：独立 worker、迁移、健康诊断、失败重放、备份恢复及两账号验收环境。

## 项目大屏 SDK 与运行机制

- [HTTP 版完整方案](requirements/visualization-display-proposal.md)：本轮 SDK、开发工具、包托管、配置／授权／发布、人员与可选终端访问、连续供数及隔离运行已完成实现与自验，状态 **Awaiting acceptance**。本轮成果合入主工作区 main；不包含项目成品大屏或业务界面模板。
- [完整交付核对](requirements/visualization-display-implementation.md)
- [开发、管理、HTTP 部署与恢复](operations/visualization-display.md)
- [SDK 使用说明](../packages/ts/display-sdk/README.md)与[开发工具说明](../packages/ts/display-devtools/README.md)
- [自验、合入记录与人工验收步骤](verification/visualization-display.md)

## 监测数据：独立保留

- [监测数据完整需求与总体方案](requirements/monitoring-data-foundation-plan.md)
- [监测数据详细开发方案](requirements/monitoring-data-development-plan.md)
- [监测使用与维护](operations/monitoring-data.md)
- [本机人工验收](operations/monitoring-data-acceptance.md)
- [监测开发与自验证据](verification/monitoring-data.md)
- [派生监测指标与分析成果：已确认需求及验收基线](requirements/derived-monitoring-and-analysis-plan.md)
- [派生监测指标与分析成果：正式详细开发方案](requirements/derived-monitoring-and-analysis-development-plan.md)

两份基础监测方案继续独立维护；用户已确认实施并调整为优先本机功能，[开发与自验记录](verification/monitoring-data.md)分别记录阶段结果和待验证事项。它们引用的[旧项目资源方案地址](requirements/project-resource-management-refactor-plan.md)仅保留跳转说明，正文已并入主文档。派生监测需求基线已获用户确认，正式开发方案细化数据、计算、接口、界面和验证约定；派生功能尚未开始实现，不改变基础监测的本机交付与待验收状态。

## 工程空间可视化

- [交付范围与详细方案入口](requirements/spatial-visualization-proposal.md)：2026-11-13 完整方案已确认并授权实施。
- [交付范围与业务规则](requirements.md#spatial-scope)：图模版本、测点挂载、项目授权和派生监测／成果联动。
- [详细架构设计](architecture.md#spatial-design)、[技术选型](technology-selection.md#spatial-technology)：数据、任务、权限、接口、页面和固定依赖。
- [空间可视化技术研究](requirements/spatial-visualization-technical-research.md)：保留一手技术依据，不另维护功能正文。

- [运行与操作](operations/spatial-visualization.md)、[实施及 SC-01–SC-13 自验记录](verification/spatial-visualization.md)。
- [合成样本及预期数据](../samples/spatial/README.md)：图纸、GLB、IFC2X3／IFC4、GeoJSON、本地底图、多版本及异常文件，配套正式管线装载脚本。

原讨论稿已归入三份主文档，旧路径保留为入口。全部演示资料已由 Codex 自制，无需用户供件、购买资料或配置第三方密钥。功能状态以空间自验记录为准；人工验收仍需用户明确确认。

## 派生监测与分析：独立实施

- [已确认范围与验收方案](requirements/derived-monitoring-and-analysis-plan.md)
- [详细开发方案](requirements/derived-monitoring-and-analysis-development-plan.md)
- [实施、自验与需求完成账本](verification/derived-monitoring-and-analysis.md)
- [派生监测与分析使用、维护说明](operations/derived-monitoring-and-analysis.md)
- [派生监测与分析人工验收手册](acceptance/derived-monitoring-and-analysis.md)

2026-11-13 全部派生监测与分析功能及必要功能自验完成，功能状态为 **Awaiting acceptance**，待用户人工验收。2026-11-13用户明确暂缓性能、压力与长时观察，本轮继续完成全部功能和必要功能自验。验证记录区分软件自验、暂缓未验证项及人工验收；基础监测两份方案保持独立维护。

## 内容归属

| 原文档主题 | 需求归入 | 设计／运行归入 |
| --- | --- | --- |
| 账号方案、账号／开户邀请界面改造、账号操作 | [账号](requirements.md#accounts)、[交互](requirements.md#interaction) | [身份设计](architecture.md#identity)、[运行维护](architecture.md#operations) |
| 角色权限、角色界面、平台审计及维护 | [授权](requirements.md#authorization)、[交互](requirements.md#interaction) | [授权设计](architecture.md#access-tenancy)、[发布恢复](architecture.md#migrations) |
| 租户基础、成员、角色授权、启停、租户页面及各操作手册 | [租户档案](requirements.md#tenants)至[启停](requirements.md#tenant-lifecycle) | [授权与租户](architecture.md#access-tenancy)、[运行维护](architecture.md#operations) |
| 项目管理及操作手册 | [项目](requirements.md#projects) | [项目设计](architecture.md#projects)、[发布恢复](architecture.md#migrations) |
| 工程类型、指标、单位、模板及操作手册 | [工程定义](requirements.md#definitions) | [定义设计](architecture.md#definitions) |
| 设施、构件、测点、通道、工程页面改造及操作手册 | [工程资源](requirements.md#resources)、[交互](requirements.md#interaction) | [资源设计](architecture.md#resources) |
| 项目资源授权重构及操作手册 | [项目资源](requirements.md#project-resources) | [资源授权设计](architecture.md#resources)、[迁移](architecture.md#migrations) |
| 设备产品总体／详细方案及操作手册 | [设备与来源](requirements.md#devices) | [设备设计](architecture.md#devices)、[迁移](architecture.md#migrations) |
| 工作空间导航、居中选择器及操作手册 | [工作空间交互](requirements.md#interaction) | [前端与入口](architecture.md#frontend) |
| 地图／GIS、图模管理、测点挂载及数据联动 | [空间业务范围](requirements.md#spatial-scope) | [空间设计](architecture.md#spatial-design)、[选型](technology-selection.md#spatial-technology)、[运行](operations/spatial-visualization.md) |
| 前端重构、前端开发约定、后端组织、原架构说明 | [通用交互和质量](requirements.md#quality) | [模块](architecture.md#modules)、[前端](architecture.md#frontend)、[技术选型](technology-selection.md) |

重复的迭代背景、逐文件实施清单、相同部署命令和旧阶段权限总数已归并；后续已确认规则覆盖早期阶段描述。人工验收条件统一保存，既有验收状态单独记录，不把历史测试计划写成新的通过结果。

## 仓库中的配套入口

[项目 README](../README.md)保留快速启动；[领域词汇表](../GLOSSARY.md)保留统一术语；[OpenAPI](../contracts/openapi/api.yaml)、[数据库迁移](../packages/go/platform/migrations/README.md)、[CLI](../tools/infrapro/README.md)及[第三方来源](../third-party/README.md)继续作为各自契约、组件说明和许可证依据。它们不重复维护主需求和架构。

<a id="verification"></a>

## 历史文档整理检查与验收

本节保留早期文档整理的检查范围，不代表后续功能开发的验证结果。文档自检已通过；人工验收待用户确认。

| 检查 | 结果 |
| --- | --- |
| 文档归属 | 整理前 39 份文档中的 37 份非监测文档均有明确归属；移除 35 份分散文件，原架构正文重组，项目资源旧路径改为跳转页。最终保留 7 份文档。 |
| 监测文档保护 | 两份文件与整理前逐字节一致，路径未变，SHA-256 一致；其项目资源引用仍可访问。 |
| 链接及锚点 | 主文档、保留文档、根 README、CLI、部署及迁移说明中的本地链接和锚点检查通过；仓库没有遗留指向已移除文档的旧引用。 |
| Markdown 结构 | 代码围栏配对、表格列数及显式锚点唯一性检查通过；`git diff --check` 通过。 |
| 一致性核对 | 合并后采用当前多角色、项目资源／设备授权、唯一入口和隐藏失权空间规则；移除早期权限总数、全新清库步骤及被替代的阶段结论。 |
| 变更范围 | 与整理前工作区快照比较，仅文档及配套 README 改动，未改业务代码、配置、契约或数据库。 |
| 未验证项 | 本次未重跑功能、浏览器、部署、性能或恢复测试；历史功能自验记录不作为本次测试证据，功能人工验收状态保持原有边界。 |

本次不运行应用、不执行数据库迁移或部署，也不将已有功能改为已验收。验收时从本页进入三份主文档，核对业务和技术内容归属；两份监测文档应与整理前一致。

- 文件与附件：[运行与恢复](operations/files-and-attachments.md)、[接入指南](development/files-and-attachments-integration.md)、[人工验收](operations/files-and-attachments-acceptance.md)、[自验矩阵](verification/files-and-attachments.md)。
- 文档在线预览：[范围与设计](requirements/document-preview-proposal.md)、[使用与人工验收](operations/document-preview.md)、[自验记录](verification/document-preview.md)；PDF、DOCX、XLSX、CSV、TXT 共用现有附件权限与历史修订。
## 审计与操作追踪

- [三个层面审计页面 A 方案重构与验收](verification/audit-ui-refactor.md)
- [详细开发方案与 70 项标准](requirements/audit-and-operation-tracing-development-plan.md)
- [逐项自验账本](verification/audit-and-operation-tracing.md)
- [来源接入契约](development/audit-integration.md)与[事件目录](development/audit-event-catalog.md)
- [运维、清理和恢复](operations/audit-and-operation-tracing.md)
- [合成演示与人工验收](operations/audit-and-operation-tracing-acceptance.md)

## 独立运行诊断

- [详细开发方案与 DG-01～DG-60](requirements/independent-runtime-diagnostics-proposal.md)
- [独立启动、账号／密钥、故障恢复与人工验收](operations/independent-runtime-diagnostics.md)
- [自有进程 reporter 接入](development/diagnostic-process-reports.md)
- [逐项自验台账](verification/independent-runtime-diagnostics.md)
- [原生、Compose、systemd 模板](../deploy/diagnostics/README.md)
