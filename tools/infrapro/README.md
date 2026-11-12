# InfraPro 统一运维 CLI

本目录是独立 Go module，构建项目统一运维程序 `infrapro`。CLI 与 `services/api` 分开组织，可以在 API 未启动时运行，不导入 API 的 `internal` 或 HTTP 框架。

```bash
# 从仓库根目录执行，自动读取根 .env
pnpm infrapro --help
pnpm infrapro database --help
pnpm infrapro account --help

# 构建后使用；由进程环境或 INFRAPRO_CONFIG 提供配置
build/bin/infrapro --help

# 从本模块目录独立构建
GOWORK=off go build -o ../../build/bin/infrapro ./cmd/infrapro
```

- `cmd/infrapro`：进程信号、标准输入输出和退出状态。
- `internal/command/root.go`：项目帮助与命令组分发。
- `internal/command/database.go`：数据库迁移，只读取数据库配置。
- `internal/command/account.go`：账号初始化、查找、重置、停用/恢复、审计和恢复后的访问撤销。

新的项目运维能力在此 CLI 中新增命令组或子命令。交互与参数处理放在 command，共享业务规则放在 `packages/go/platform` 中的对应包；禁止复制 API 业务规则或导入 API 的内部实现。不要为各业务模块另建运维可执行程序。

帮助命令不加载配置或连接依赖。密码仍通过终端无回显输入，重置链接只允许受控终端输出。真实维护流程见 [架构设计：运行维护](../../docs/architecture.md#operations)。
## 权限维护

`infrapro access sync` 同步目录和内置角色，`access check` 只读检查，`access recover-admin --username ... --reason ...` 为已有账号添加管理员角色绑定。首次 bootstrap 前必须同步目录；恢复角色不恢复停用状态或历史凭证。详见[架构设计：运行维护](../../docs/architecture.md#migrations)。
