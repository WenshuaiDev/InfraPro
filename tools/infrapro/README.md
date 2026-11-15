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

## 派生监测与分析

`pnpm infrapro monitoring derived --help` 提供就绪检查、状态、只读对账／恢复核对，以及通过真实 API 会话执行的暂停、恢复、历史回算和原依据回放。业务命令使用受保护的会话文件及预览收据，不能通过账号 ID 冒充登录。完整步骤和权限边界见[派生监测与分析使用、维护说明](../../docs/operations/derived-monitoring-and-analysis.md)。

## 空间资料维护

`pnpm infrapro spatial verify --tenant <id>` 校验固定版本清单及私有对象大小／SHA-256；`--apply --reason <原因>` 隔离缺失内容或在恢复原字节后恢复可用标记，并写维护审计。仅需数据库与空间对象存储配置；暂时网络错误不改隔离状态。详见[空间运行与恢复](../../docs/operations/spatial-visualization.md)。

## 大屏包维护

`pnpm infrapro display --help` 提供 `backup-manifest`（对象清单）、`verify-storage`（逐文件核验并标记 missing／恢复 ready）、`reclaim`（按引用清理临时对象）和 `restore-invalidate`（更换环境代次并失效旧会话、终端及游标）。这些是部署运维命令，使用受保护的服务配置，不接受人员 ID 冒充登录；完整顺序见[大屏运行与恢复](../../docs/operations/visualization-display.md)。

## 站内通知维护

`pnpm infrapro notifications status` 显示积压、失败原因、最近心跳与大屏交接状态；`check --worker --max-age 1m` 检查协议及心跳。`inspect --event <UUID>` 查看无正文的处理结果，`replay --event <UUID> --reason <原因>` 或 `replay --handoff <UUID> --reason <原因>` 在修复后重放失败记录，保留原接收集和已读状态并单独审计。不提供任意文本／接收人发送入口。完整顺序见[通知运维](../../docs/operations/in-app-notifications.md)。

## 任务索引维护

`pnpm infrapro tasks status` 查询来源回填、交接和 worker 心跳；`backfill --source all` 或 `reconcile --source module:kind` 分批对账；`verify` 检查来源／历史一致性；`replay-handoff --id UUID --reason 原因` 仅重放失败元数据交接。完整备份恢复后执行 `restore-epoch --reason 原因` 废弃旧游标和索引租约，再对账。CLI 不重跑业务任务、不发送结果通知。独立 SQL 角色和升级顺序见[任务运维](../../docs/operations/task-center.md)。
