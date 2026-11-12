# Compose 环境

- `compose.yaml`：正式 HTTP 编排，PostgreSQL/Redis 持久化，Nginx 托管唯一应用 `/console/` 并代理 `/api/`，旧 `/screen` 路径固定跳转。
- `local.yaml`：本地开发依赖，由根目录 `pnpm setup:local` 配置；数据保存在独立卷。
- `test.yaml`：自动化专用数据库与 Redis，使用独立端口；测试按随机 schema 和前缀隔离。
- `http-test.yaml`：完整部署验收专用栈，使用本机普通 LAN HTTP 地址；`pnpm test:http` 自动管理并清理。

`migrate` 使用独立 CLI 镜像完成迁移，随后 `access-sync` 同步目录和内置角色，成功后才启动 API；`ops` 是按需运行的统一运维入口，不常驻。维护使用 `docker compose --env-file deploy/compose/.env -f deploy/compose/compose.yaml run --rm ops <命令组> <子命令>`；不再进入 API 容器执行 CLI。

部署、配置、初始化、账号维护及备份步骤见 [架构设计：运行维护](../../docs/architecture.md#operations)。不要用测试环境的明文固定凭据配置正式环境，也不要对正式环境执行 `down --volumes`。
