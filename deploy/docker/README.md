# 容器构建位置

后续镜像构建文件放在此处，并以仓库根目录为构建上下文，包含 go.work、所需 Go 模块、pnpm 工作区包和锁文件。

本轮交付本地可运行骨架，尚未提供或验证容器镜像。Go 可执行文件由 `make build` 生成到 `build/bin/`；静态应用产物分别在 `apps/console/dist/`、`apps/screen/dist/`。
