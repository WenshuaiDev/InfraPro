# 容器构建

`api.Dockerfile` 用仓库固定 Go 工具链构建 HTTP API，运行镜像包含 API、监测 worker 与通知 worker 三个可执行程序。Compose 将通知 worker 作为独立进程启动，只注入 PostgreSQL 配置；它不依赖 Redis、ClickHouse、对象存储或空间转换器。

`infrapro.Dockerfile` 独立构建项目统一运维 CLI，供迁移与维护操作使用，不复制 API 源码。两个 Go 镜像均在 `GOWORK=off` 下构建独立模块，以非 root 用户运行。

`web.Dockerfile` 使用固定 Node/pnpm 和锁文件构建唯一前端 `/console/`，由 Nginx 托管并代理 API。服务器秘密只在运行时注入 API 或运维 CLI，不传入前端构建。

运行方式见 [架构设计：运行维护](../../docs/architecture.md#operations) 和 `deploy/compose/compose.yaml`。

## 统一任务索引进程

API 镜像同时构建 `task-index-worker`。Compose 的 `tasks` profile 使用独立 `TASK_INDEX_DATABASE_URL`，须由 DBA 在 v39 迁移后授予 `infrapro_task_index` 角色及部署 schema USAGE；示例见 `deploy/tasks/index-worker-role.sql`。该账号不能改写源执行队列或读取报告、通知和账号数据。索引只依赖 PostgreSQL，SIGTERM 后按任务配置有界结束。完整升级、回填、故障重放及恢复见[任务运维](../../docs/operations/task-center.md)。
