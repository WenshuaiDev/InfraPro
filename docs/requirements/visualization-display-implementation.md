# 大屏机制实施与验收清单

2026-11-13，Awaiting acceptance。用户已授权按 visualization-display-proposal.md 完整开发；该方案是本轮范围基线。人工验收与自验分别记录。

当前目标（2026-11-13 用户调整）：按修订后的方案，以 HTTP 完整交付大屏 SDK、运行机制及必要基础能力，完成自验后交付验收。HTTPS／TLS、证书配置及其自验不属于本轮要求。

## 设计与步骤

沿用 Go / PostgreSQL / 私有 BlobStore / ClickHouse 领域服务及 TypeScript / React 控制台；SDK 与工具编译成不依赖 workspace 的 npm 包。控制台、可信壳和上传应用使用三个独立 origin，HTTP 查询、SSE 连续供数、MessageChannel 通信。新增独立运行进程和受限包 worker，部署各自资源预算。

内部实施顺序：
1. 固定 manifest、SDK / Provider 契约；大屏元数据、权限和只读 ReadSubject 接缝。
2. 分片上传、独立校验 worker、不可变包；配置与绑定、读取授权、发布准备、幂等发布和回退。
3. 人员跨站启动、短期会话、固定终端配对及轮换；业务 Provider、订阅和恢复。
4. 外部 SDK、开发工具、可信壳、独立样例和管理界面。
5. HTTP / 代理、备份与恢复、故障诊断；契约、数据库、浏览器及仓库外安装自验。

## 完整范围核对

| 范围 | 实现和自验目标 | 状态 |
| --- | --- | --- |
| DM-01 | 默认关闭、0–N 实例、启停归档、权限与历史 | Awaiting acceptance |
| DM-02 | JS / TS 编译包、公开导出、版本协商、离线安装 | Awaiting acceptance |
| DM-03 | 目录、监测、来源、派生、固定成果、空间真实接入 | Awaiting acceptance |
| DM-04 | 快照、订阅、取消、应用确认、重连、reset、卸载 | Awaiting acceptance |
| DM-05 | init / dev / login / validate / pack / upload，模拟与真实预览 | Awaiting acceptance |
| DM-06 | 分片幂等、取消和恢复、ZIP 安全校验、封存、受控资产 | Awaiting acceptance |
| DM-07 | 受限 schema 表单、固定和动态绑定、语义、修订和影响 | Awaiting acceptance |
| DM-08 | 草稿预览、发布准备、CAS 与幂等、切换、回退、撤回 | Awaiting acceptance |
| DM-09 | 人员与终端资格、最小范围、当前复验、撤权清理 | Awaiting acceptance |
| DM-10 | 独立运行进程、壳与隔离、HTTP、预算和查询共享 | Awaiting acceptance |
| DM-11 | Provider 注册与版本、告警 unavailable、扩展接缝 | Awaiting acceptance |
| DM-12 | 状态、脱敏事件、终端、引用清理、备份恢复和文档 | Awaiting acceptance |

## 人工可观察的验收场景

- 项目默认没有大屏；启用并创建多个实例后可各自上传独立构建包，非法包有可定位错误且不能执行。
- 由包 schema 填写参数和绑定；草稿不影响正式运行。缺必需绑定、授权或能力不能发布；相同请求重试不重复发布，并发修订返回冲突。
- 仓库外安装 SDK 和工具，最小 TypeScript 工程可以构建、校验、打包、上传、预览及发布；模拟数据持续标识。
- 普通登录打开稳定入口，无需登记终端；读取结果保留精确数值、时间、数据版本及派生状态。越权对象、跨项目引用和写操作不可调用。
- 订阅有首屏结果、更新和取消；处理失败不确认游标，断线或游标缺口后有界重同步；撤权立即销毁运行内容和资源。
- 发布切换显示目标与实际版本；新应用 ready 后完成切换；回退仍按当前授权验证。
- 配对终端不保存人员密码；重置、撤销、过期、项目停用和恢复代次变化会中止它，归档项目只保留合法读取和收窄操作。
- 管理页能看包处理、配置、发布、会话及终端状态；运维能核对包引用、缺失文件及备份恢复，并让旧会话失效。

自验记录见 [完整验收记录](../verification/visualization-display.md)，DM-01～DM-12 已全部实现并完成自验。当前为 Awaiting acceptance；未经用户确认不标 Accepted。

- HTTP 验收补充：明确配置的非 localhost HTTP 地址能完成登录、一次性启动、Bridge 握手、SSE、包资源加载与终端配对；CookieSecure=false，不依赖安全上下文专属浏览器 API。
