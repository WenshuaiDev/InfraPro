# InfraPro 文档

整理日期：2026-11-12。状态：**Awaiting acceptance（文档自检通过，待用户验收）**。

除尚未开发的监测数据外，项目文档统一维护在以下三份主文档中，不再按每次迭代分别保存需求、页面改造和运行手册。

| 主文档 | 内容 |
| --- | --- |
| [软件需求规格说明书](requirements.md) | 目标与范围、角色与归属、完整业务规则、页面交互、非功能要求、人工验收及状态。 |
| [技术选型](technology-selection.md) | 当前语言、框架、存储、组件及工具的版本来源、用途、选择理由和使用边界。 |
| [架构设计](architecture.md) | 系统结构、模块职责、数据与事务、接口、前端状态、部署配置、迁移恢复及验证方式。 |

## 监测数据：独立保留

- [监测数据完整需求与总体方案](requirements/monitoring-data-foundation-plan.md)
- [监测数据详细开发方案](requirements/monitoring-data-development-plan.md)

两份文档的路径和内容均保持不变，仍属尚未开发的独立范围。它们引用的[旧项目资源方案地址](requirements/project-resource-management-refactor-plan.md)仅保留跳转说明，正文已并入主文档。

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
| 前端重构、前端开发约定、后端组织、原架构说明 | [通用交互和质量](requirements.md#quality) | [模块](architecture.md#modules)、[前端](architecture.md#frontend)、[技术选型](technology-selection.md) |

重复的迭代背景、逐文件实施清单、相同部署命令和旧阶段权限总数已归并；后续已确认规则覆盖早期阶段描述。人工验收条件统一保存，既有验收状态单独记录，不把历史测试计划写成新的通过结果。

## 仓库中的配套入口

[项目 README](../README.md)保留快速启动；[领域词汇表](../GLOSSARY.md)保留统一术语；[OpenAPI](../contracts/openapi/api.yaml)、[数据库迁移](../packages/go/platform/migrations/README.md)、[CLI](../tools/infrapro/README.md)及[第三方来源](../third-party/README.md)继续作为各自契约、组件说明和许可证依据。它们不重复维护主需求和架构。

<a id="verification"></a>

## 本次整理检查与验收

文档自检已通过；人工验收待用户确认。

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
