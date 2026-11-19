# InfraPro

交通基础设施监测管理系统。平台维护账号、公共工程定义和设备产品；租户管理设施、构件、测点、通道与设备，项目按明确授权使用这些资源。

当前包含原始监测导入、查询与统计、派生计算与历史分析、图纸／三维／BIM／GIS、项目大屏 SDK 与独立运行壳、文件附件、任务中心、站内通知、审计追踪和独立运行诊断。

四机生产标签 **`production-2026-11-19`** 对应发行版 **`20261119-b09feb944c39`** 和业务数据库 **schema 59**。HTTP 与六组后台作业运行在兼容模式。生产入口、逐文件摘要及版本对照方法见[11 月 19 日生产版本记录](docs/operations/production-version-2026-11-19.md)，各项验收结果见[架构重构交付记录](docs/verification/system-architecture-refactor-closeout.md)。

## 本机完整环境

已经准备的工程环境、唯一账号的凭据位置和逐项人工操作见[本机工程环境与人工验收](docs/operations/local-environment.md)。在仓库根目录执行：

```bash
pnpm local:start
pnpm local:status
```

管理端为 <http://localhost:25173>，独立诊断为 <http://127.0.0.1:8096>。重启使用 `pnpm local:restart`，停止应用进程使用 `pnpm local:stop`。配置、密钥、持久数据及工程原始资料保存在仓库外，不随代码清理删除。

首次安装全新环境时先安装依赖、构建，再初始化基础设施及账号：

```bash
make install
pnpm build
pnpm local:init
pnpm local:cli account bootstrap
pnpm local:start
```

不要在已有环境重复创建账号。空间转换需按 `tools/spatial-converter/requirements.lock` 安装 Python 依赖，并设置运行配置中的转换器路径。独立诊断的配置与凭据初始化见[诊断运行手册](docs/operations/independent-runtime-diagnostics.md)。

## 开发与检查

Go、Node 和 pnpm 版本分别由 `.go-version`、`.node-version` 和 `package.json` 固定；依赖以 `go.sum` 与 `pnpm-lock.yaml` 为准。

| 命令 | 用途 |
| --- | --- |
| `pnpm setup:local`、`pnpm dev` | 最小 PostgreSQL／Redis 开发环境及热更新 console |
| `pnpm build` | 构建全部前端、API、worker、诊断及 CLI |
| `pnpm check` | Go 格式／vet、ESLint、类型、接口及权限契约 |
| `pnpm test:identity` | 隔离数据库下的 Go 回归及 race 检测 |
| `pnpm test:e2e:preview` | 已构建应用的浏览器回归 |
| `pnpm test:monitoring:contract` | ClickHouse／Keeper／S3 实际存储契约 |
| `pnpm test:derived:contract` | 派生规则、事务及正式计算存储 |
| `pnpm test:spatial`、`pnpm test:display` | 空间转换与大屏运行机制 |
| `pnpm test:tasks`、`pnpm test:files`、`pnpm test:audit` | 公共能力与来源集成 |
| `pnpm test:diagnostics:environment`、`pnpm test:diagnostics` | 隔离诊断环境与故障恢复 |
| `pnpm infrapro --help` | 统一运维 CLI |

测试在隔离 schema、对象前缀和容器中进行。共享存储的故障注入、恢复和领域套件按顺序执行，避免相互干扰。生成的报告、截图、模型转换缓存等位于忽略目录，不属于正式源文件。

正式部署采用仓库中的 Compose／systemd／Nginx 配置；本机预览不代表生产部署。完整运行、升级和恢复约束见[架构设计](docs/architecture.md#operations)和[文档目录](docs/README.md)。
