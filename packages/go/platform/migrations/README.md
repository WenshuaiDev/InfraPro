# 项目数据库迁移

`00001_identity.sql` 建立账号、密码摘要、会话、邀请、重置凭证、初始化、审计、冷却和缓存失效队列表。

迁移属于独立共享 platform module，由项目统一 CLI `infrapro database migrate` 或本地 `pnpm infrapro database migrate` 执行。Goose Provider 配置 PostgreSQL session locker，多个进程并发只推进一次。API 启动及就绪检查要求迁移版本精确匹配，不自动迁移。目录移动不改变现有迁移版本与 SQL 内容。

账号迁移不允许 Down，防止回退清空初始化与历史；恢复使用完整备份和维护手册中的统一撤销步骤。迁移源只追加后续编号，不修改已部署版本的含义。
`00002_access.sql` 增加授权关系、目录投影、修订锁、账号管理版本及结构化审计。仅建立结构，不回填旧账号角色。空库迁移后运行 `infrapro access sync` 和 `infrapro access check`，再执行首次初始化。

`00003_tenancy.sql` 增量扩展角色绑定范围、租户档案、永久编号归属、成员与角色绑定历史，以及租户审计归属。存量角色保留为 platform 范围，租户初始为空。组合外键与延迟约束检查阻止跨租户绑定及无当前管理员的半成品；不允许 Down。升级流程与隔离恢复证据见 [租户运行手册](../../../../docs/operations/tenant-foundation.md)。

`00004_tenant_membership.sql` 增加普通成员的暂停、恢复、结束与定向邀请，保留关系版本、状态原因和邀请历史。已有管理员关系保持有效，重新加入使用新成员关系。运行与验收见[租户成员手册](../../../../docs/operations/tenant-membership.md)。

`00005_tenant_role_authorization.sql` 增加租户自定义角色、来源追溯、成员多角色和权限可配置标记；迁移前先校验旧租户结构。目录同步维护预置角色，不自动扩充既有自定义角色。运行与验收见[租户授权手册](../../../../docs/operations/tenant-role-authorization.md)。

`00006_tenant_lifecycle.sql` 增加租户启停状态、变更原因及公开说明；既有租户默认启用。状态不能通过 Down 丢弃，防止旧程序重新开放停用租户。运行与验收见[租户启停手册](../../../../docs/operations/tenant-lifecycle.md)。

`00007_projects.sql` 增加项目范围权限、项目档案、成员及角色历史、编号保留、管理员与负责人职责和归档状态。平台或租户身份不自动产生项目成员资格。运行与验收见[项目管理手册](../../../../docs/operations/project-management.md)。

`00008_engineering.sql` 追加工程目录协调锁、单位类别、定义身份/编号保留、草稿、不可变发布版本、独立启停状态、固定依赖及操作收据。仅创建结构，不预置或发布设施类型。发布正文和依赖由数据库历史保护触发器约束，类别被引用后禁止改义；版本必须有状态记录。运行前完成目录同步，已有自定义角色不会自动扩权。v7→v8 及空库初始化测试、实际本机备份恢复证据见[工程定义自验记录](../../../../docs/verification/infrastructure-type-definition.md)。本迁移拒绝破坏性 Down，恢复按操作手册执行。

`00009_engineering_resources.sql` 新增设施、构件、测点、通道、独立变更历史及幂等收据。固定定义版本、同租户同设施组合引用、编号永久保留、父子关系及最大正常数量由约束与触发器保护；最少数量不阻断实际资源。迁移不把定义样例转为实际对象，不修改既有账号及业务数据。升级后同步并检查 90 项权限目录，拒绝破坏性 Down。v8→v9、空库及包含资源的真实备份恢复见[工程资源自验记录](../../../../docs/verification/engineering-resources.md)。
