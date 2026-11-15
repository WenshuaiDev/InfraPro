# 项目数据库迁移

`00001_identity.sql` 建立账号、密码摘要、会话、邀请、重置凭证、初始化、审计、冷却和缓存失效队列表。

迁移属于独立共享 platform module，由项目统一 CLI `infrapro database migrate` 或本地 `pnpm infrapro database migrate` 执行。Goose Provider 配置 PostgreSQL session locker，多个进程并发只推进一次。API 启动及就绪检查要求迁移版本精确匹配，不自动迁移。目录移动不改变现有迁移版本与 SQL 内容。

账号迁移不允许 Down，防止回退清空初始化与历史；恢复使用完整备份和维护手册中的统一撤销步骤。迁移源只追加后续编号，不修改已部署版本的含义。
`00002_access.sql` 增加授权关系、目录投影、修订锁、账号管理版本及结构化审计。仅建立结构，不回填旧账号角色。空库迁移后运行 `infrapro access sync` 和 `infrapro access check`，再执行首次初始化。

`00003_tenancy.sql` 增量扩展角色绑定范围、租户档案、永久编号归属、成员与角色绑定历史，以及租户审计归属。存量角色保留为 platform 范围，租户初始为空。组合外键与延迟约束检查阻止跨租户绑定及无当前管理员的半成品；不允许 Down。升级和恢复流程见 [租户运行手册](../../../../docs/architecture.md#operations)。

`00004_tenant_membership.sql` 增加普通成员的暂停、恢复、结束与定向邀请，保留关系版本、状态原因和邀请历史。已有管理员关系保持有效，重新加入使用新成员关系。运行说明见[租户成员手册](../../../../docs/architecture.md#operations)。

`00005_tenant_role_authorization.sql` 增加租户自定义角色、来源追溯、成员多角色和权限可配置标记；迁移前先校验旧租户结构。目录同步维护预置角色，不自动扩充既有自定义角色。运行说明见[租户授权手册](../../../../docs/architecture.md#operations)。

`00006_tenant_lifecycle.sql` 增加租户启停状态、变更原因及公开说明；既有租户默认启用。状态不能通过 Down 丢弃，防止旧程序重新开放停用租户。运行说明见[租户启停手册](../../../../docs/architecture.md#operations)。

`00007_projects.sql` 增加项目范围权限、项目档案、成员及角色历史、编号保留、管理员与负责人职责和归档状态。平台或租户身份不自动产生项目成员资格。运行说明见[项目管理手册](../../../../docs/architecture.md#migrations)。

`00008_engineering.sql` 追加工程目录协调锁、单位类别、定义身份/编号保留、草稿、不可变发布版本、独立启停状态、固定依赖及操作收据。仅创建结构，不预置或发布设施类型。发布正文和依赖由数据库历史保护触发器约束，类别被引用后禁止改义；版本必须有状态记录。运行前完成目录同步，已有自定义角色不会自动扩权。本迁移拒绝破坏性 Down，恢复按[工程定义手册](../../../../docs/architecture.md#operations)执行。

`00009_engineering_resources.sql` 新增设施、构件、测点、通道、独立变更历史及幂等收据。固定定义版本、同租户同设施组合引用、编号永久保留、父子关系及最大正常数量由约束与触发器保护；最少数量不阻断实际资源。迁移不把定义样例转为实际对象，不修改既有账号及业务数据。升级后同步并检查权限目录，拒绝破坏性 Down。

`00010_project_resource_access.sql` 增量建立项目设施关联、对象授权、新建政策、历史和收据。资源保持租户归属，迁移不复制业务资源、不按项目名称自动授权。升级和恢复见[项目资源手册](../../../../docs/architecture.md#migrations)。

`00011_device_product_resources.sql` 在 v10 项目资源授权基线上增加公共／私有厂家和产品、不可变版本／槽位／依赖、设备／输出代次、安装及纠错引用、即时来源期间、独立项目设备授权、历史与收据。`btree_gist` 安装在 public；安装、输出和来源期间采用数据库排斥约束。旧租户／项目只初始化协调行，can_source_manage 默认 false，不创建业务样例或设备授权。Down 只提示从完整备份恢复；升级顺序见[设备操作手册](../../../../docs/architecture.md#migrations)。

`00012_monitoring_metadata.sql` 增加监测控制元数据、纳秒 `int8range` 数据授权、持久任务、不可变源文件／映射／发布依据及永久通道引用保护。原始逐条观测不进入 PostgreSQL。升级不创建样本、数据授权或批次；系统角色按权限目录同步新增能力，自定义角色不扩权。有数据后禁止 destructive down，使用验证过的恢复或前向修复。阶段证据见[监测自验记录](../../../../docs/verification/monitoring-data.md)。

`00034_spatial.sql` 增加空间资料／固定版本、分片上传与任务、挂载、构件和图形关联、项目固定版本授权、视图配置及审计。正式版本和内容引用受约束；结束项目设施关联会结束对应空间授权。迁移不注入样本，不复制资源或观测。运行与恢复见[空间手册](../../../../docs/operations/spatial-visualization.md)。

`00035_display.sql` 增加默认关闭的大屏设置、0–N 实例、包上传／校验／封存、不可变配置／发布、读取授权、一次性配对、终端、只读运行主体、租约与持久订阅事件。monitoring_selections、monitoring_snapshots、monitoring_jobs 增加受复合外键约束的 display_subject_id，保持人员查询归属并禁止两类主体混用。默认不创建实例或授权，不清理原监测历史；拒绝直接 Down。备份恢复必须更换环境代次，见[大屏运行与恢复](../../../../docs/operations/visualization-display.md)。

`00036_notifications.sql` 增量建立通知协议、事实和原接收集、分批投递、个人接收序号／已读／归档、偏好、条件周期、worker 心跳及大屏持久交接，增加任务的手工通知代次。账号与业务原数据保留，不从旧审计或运行日志补发。拒绝 Down；备份须包含原业务事实、交接收据及个人状态，按[通知运维手册](../../../../docs/operations/in-app-notifications.md)执行协调恢复。

`00037_task_records.sql` 建立统一任务协议、来源身份、当前投影、每代执行／尝试、不可变事件和受控关联，以及事务事实 journal。`00038_task_sources.sql` 增加各来源修订、初始发起人与每代执行身份、白名单快照、旧错误证据、大屏自有交接、回算展示分组、独立索引角色和对账视图。`00039_task_notification_refs.sql` 兼容 v1 事实并加入可选固定代次 TaskRef 的 v2 通知；不重算已有指纹，不补发通知。协调升级、历史纳入和完整恢复按[任务运维](../../../../docs/operations/task-center.md)执行；不支持破坏性 Down 或任意新旧版本混跑。
