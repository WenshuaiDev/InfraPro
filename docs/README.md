# InfraPro 文档

正式需求、技术与架构集中维护；模块手册保留操作和接入约束。历史原型、讨论稿、阶段计划和旧验收结果不再保留。

| 文档 | 内容 |
| --- | --- |
| [软件需求规格说明书](requirements.md) | 业务范围、身份与授权、数据规则及可观察行为 |
| [架构设计](architecture.md) | 模块、数据、事务、接口、前端与部署 |
| [技术选型](technology-selection.md) | 语言、框架、存储与固定依赖的用途 |
| [本机工程环境与人工验收](operations/local-environment.md) | 当前入口、账号位置、工程资料、检查范围和人工步骤 |

## 开发规格

- [平台 MFA 开发规格](development/mfa-development-spec.md)：平台强制开关、个人验证器管理、多步登录及会话、恢复与重置、安全存储与审计；明确登录后敏感操作不再要求 TOTP，包含现有系统改动地图、接口与迁移约定及 60 项验收用例。范围已确认，尚未编码或功能验收。
- [解析服务与解析方法配置](development/parsing-service-and-method-binding-spec.md)：两端职责、产品配置、设备继承与特化、固定版本、权限、DDP 样本及验收条件。完整实现已落地，PAR-001—PAR-076 自验通过；[人工验收与证据](verification/parsing-service.md)另行记录。
- [设备数据接入与原始报文管理](development/device-ingestion-and-raw-message-spec.md)：云端统一接收转发、内网主动接管、来源识别、终端关联、自动解析和原文隔离，共 136 项验收条件。[逐项自验与人工接受](verification/device-ingestion-and-raw-messages.md)分开记录。
- [大屏 SDK 监测目录、详情与统计扩展](development/display-sdk-monitoring-read-spec.md)：工程与设备目录、地图与空间资料、三类统计、命名读取范围、后端授权及兼容迁移；不含传感器分类和断面管理。[100 项实施与验收账本](verification/display-sdk-monitoring-read.md)记录当前自验及独立的人工接受状态；[独立验收工程](../examples/display-monitoring-read/README.md)说明安装、配置和运行。
- [设备分类、监测断面与孔位业务模型方案](development/sensor-classification-and-engineering-structure-proposal.md)：平台多层级分类、独立断面／孔位、灵活工程树、设施类型建模与公共／租户模板衔接、旧位置文字迁移及 60 项业务验收场景。完整开发与自验已完成，待人工验收；见[实施规格](development/sensor-classification-and-engineering-structure-implementation.md)和[验收账本](verification/sensor-classification-and-engineering-structure.md)，人工验收尚未开始。

## 运行维护

- [当前生产版本基线](operations/production-version-2026-11-17.md)：Git 标签 `production-2026-11-17` 与实机发行包、运行程序及数据库版本的对应关系。
- [解析服务、固定产物发布与联合恢复](operations/parsing-service.md)
- [设备接入与原始报文：启动、模拟、部署和恢复](operations/device-ingestion-and-raw-messages.md)

- [测点与设备绑定](operations/channel-bindings.md)
- [监测数据](operations/monitoring-data.md)
- [派生监测与历史分析](operations/derived-monitoring-and-analysis.md)
- [工程空间可视化](operations/spatial-visualization.md)
- [项目大屏 SDK 与运行机制](operations/visualization-display.md)
- [文件与附件](operations/files-and-attachments.md)、[文档预览](operations/document-preview.md)
- [任务中心](operations/task-center.md)、[站内通知](operations/in-app-notifications.md)
- [审计与操作追踪](operations/audit-and-operation-tracing.md)
- [独立运行诊断](operations/independent-runtime-diagnostics.md)
- [应用 HTTPS 部署与证书维护](operations/https-deployment.md)：Nginx 终止 TLS、Cookie／来源校验、三入口和大屏工具；[自检与验收](verification/https-deployment.md)。
- [上传限额与代理部署](operations/upload-limits-and-proxy.md)：分片边界、整文件配额、schema 55 升级和统一 Nginx 策略；[自检与验收](verification/upload-deployment.md)。

## 模块接入

- [文件与附件](development/files-and-attachments-integration.md)
- [任务中心](development/task-center-integration.md)
- [审计接入](development/audit-integration.md)、[事件目录](development/audit-event-catalog.md)
- [诊断进程报告](development/diagnostic-process-reports.md)
- [大屏 SDK](../packages/ts/display-sdk/README.md)、[大屏开发工具](../packages/ts/display-devtools/README.md)

接口、权限和结构的精确定义分别以 [OpenAPI](../contracts/openapi/api.yaml)、[权限目录](../packages/go/platform/access/)和[数据库迁移](../packages/go/platform/migrations/)为准。软件自验与用户人工接受分别记录，不能相互替代。
