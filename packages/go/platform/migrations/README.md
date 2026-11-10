# 项目数据库迁移

`00001_identity.sql` 建立账号、密码摘要、会话、邀请、重置凭证、初始化、审计、冷却和缓存失效队列表。

迁移属于独立共享 platform module，由项目统一 CLI `infrapro database migrate` 或本地 `pnpm infrapro database migrate` 执行。Goose Provider 配置 PostgreSQL session locker，多个进程并发只推进一次。API 启动及就绪检查要求迁移版本精确匹配，不自动迁移。目录移动不改变现有迁移版本与 SQL 内容。

账号迁移不允许 Down，防止回退清空初始化与历史；恢复使用完整备份和维护手册中的统一撤销步骤。迁移源只追加后续编号，不修改已部署版本的含义。
`00002_access.sql` 增加授权关系、目录投影、修订锁、账号管理版本及结构化审计。仅建立结构，不回填旧账号角色。空库迁移后运行 `infrapro access sync` 和 `infrapro access check`，再执行首次初始化。

`00003_tenancy.sql` 增量扩展角色绑定范围、租户档案、永久编号归属、成员与角色绑定历史，以及租户审计归属。存量角色保留为 platform 范围，租户初始为空。组合外键与延迟约束检查阻止跨租户绑定及无当前管理员的半成品；不允许 Down。升级流程与隔离恢复证据见 [租户运行手册](../../../../docs/operations/tenant-foundation.md)。
