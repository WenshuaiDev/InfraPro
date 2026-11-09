# 容器构建

`api.Dockerfile` 用仓库固定 Go 工具链构建 HTTP API，运行镜像只包含 API 可执行程序。

`infrapro.Dockerfile` 独立构建项目统一运维 CLI，供迁移与维护操作使用，不复制 API 源码。两个 Go 镜像均在 `GOWORK=off` 下构建独立模块，以非 root 用户运行。

`web.Dockerfile` 使用固定 Node/pnpm 和锁文件构建唯一前端 `/console/`，由 Nginx 托管并代理 API。服务器秘密只在运行时注入 API 或运维 CLI，不传入前端构建。

运行方式见 [账号运行手册](../../docs/operations/account-management.md) 和 `deploy/compose/compose.yaml`。
