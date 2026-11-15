# 项目大屏开发、运行与维护

本轮使用 **HTTP**。控制台、平台可信壳、上传应用资源分别部署到不同 origin；CookieSecure=false。示例域名需要由部署方的内网 DNS 或 hosts 指向代理。这里的步骤不代表已经操作生产环境。

## 构建、迁移与启动

在仓库根目录，沿用锁文件中的 Go 1.27、Node 26、pnpm 12：

```sh
pnpm install --frozen-lockfile
pnpm --filter @infrapro/display-sdk build
pnpm --filter @infrapro/display-runtime build
pnpm --filter @infrapro/display-devtools build
node scripts/go.mjs build
```

配置使用现有 `configs/infrapro.example.yaml`，合并 `deploy/display/http.example.yaml`。API 与运行服务使用相同权威数据库、身份配置和已配置的监测/派生/空间存储。新增 display 配置也可用 `INFRAPRO_DISPLAY_<大写字段名>` 覆盖；代理和 API 的 public_console_url、trusted_origins 必须一致。运行 origin 不包含路径、查询或尾随斜杠。

在目标环境备份后按现有运维流程执行：

```sh
INFRAPRO_CONFIG=/etc/infrapro/infrapro.yaml build/bin/infrapro database migrate
INFRAPRO_CONFIG=/etc/infrapro/infrapro.yaml build/bin/infrapro access sync
INFRAPRO_CONFIG=/etc/infrapro/infrapro.yaml build/bin/display-runtime
```

另一个独立进程：

```sh
INFRAPRO_CONFIG=/etc/infrapro/display-packager.yaml build/bin/display-package-worker
```

迁移 35 新建 display 元数据，并为 monitoring_selections、monitoring_snapshots、monitoring_jobs 增加受外键约束的 display_subject_id。人员和运行主体只能二选一；旧人员记录保持原归属。不清空监测历史。基础空间迁移 34 必须先完成。权限目录同步后既有项目内置管理员具备新增大屏权限，自定义角色按需增加。

本机默认运行端口 5180、资产端口 5181，控制台 5173。已有本机 API/控制台启动方式保持；构建壳后启动上述两个独立 Go 入口即可。完整业务测试还需要既有监测 worker、ClickHouse、私有对象存储及已完成的存储初始化。

部署文件：`deploy/display/nginx.conf` 提供三个 HTTP host，保留控制台 `/console/`；控制台对应使用 `VITE_APP_BASE=/console/ pnpm --filter @infrapro/console build`，复制 dist 到代理 root/console。该文件是完整示例，部署时合并已有监测/空间专用路由，不删除原有模块代理规则。`deploy/docker/display.Dockerfile` 和两个 systemd 示例提供独立进程及资源限额。无需为每个项目或实例启动容器。

API/运行进程不能使用包 worker 的 SQL 登录。`package-worker-role.sql` 给专用 NOLOGIN 角色授予包校验和 display 元数据保留所需权限；部署方为它配置独立登录，不能继承业务角色。worker 可使用 `deploy/display/package-worker.example.yaml`；其 YAML/环境只提供该数据库登录及 `object-policy.example.json` 所示 display/* 私有命名空间凭据，不提供身份、ClickHouse、设备、空间或原始监测存储访问能力。示例中 REPLACE_PRIVATE_BUCKET 需替换；文件均不包含真实密码。

## 项目管理与使用

项目左侧「项目大屏」默认关闭。启用后可以先保留零个实例，也可创建多个实例。每实例具有独立名称、可选人员名单、稳定入口及启停/归档状态。人员名单不替代项目成员和业务权限；管理者仍能管理实例。

1. 「程序」上传构建 ZIP。分片完成后由独立 worker 校验，ready 后才能用于配置。失败可见字段路径；支持重试、取消及无引用包回收。工具断点续传见开发工具 README。
2. 「配置」选择 ready 包，填写 schema 参数，选择固定资源/空间/成果版本或受限动态通道绑定，保存不可变草稿。required 与通道语义分别校验。时间模式为 fixed 或 follow。
3. 「读取授权」选择能力、对象、字段与截止时刻；动态范围是否覆盖未来通道必须显式选择。空间 properties、来源 source、监测 values/quality 各自受字段上限控制。
4. 草稿真实预览使用当前人员与授权交集。不同 schema 的配置升级会列出兼容继承、重置、移除及待补项，由操作者编辑后保存；平台不执行包提供的迁移脚本。
5. 「发布」先核验草稿，再发布。准备摘要固定包/配置/绑定集合/授权修订；并发变化返回 revision_conflict，重新核验后提交。历史回退也形成新的不可变发布，并按当前授权检查。
6. 普通人员打开稳定入口，登录并点击继续。固定墙屏可在「终端」签发一次性配对码，在运行站点 `/terminal` 输入；无需保存人员密码。重置、撤销与实例分配均在管理页完成。

控制台“运行”和“终端”区分目标与实际发布。新包 ready 后才替换旧 iframe；超时或失败显示错误，不伪造成功，也不自动改发布指针。项目关闭/实例关闭/撤权立即停止供数，离线状态最多保留至短租约到期。归档项目保留当前合法读取、停用及收窄，禁止新增/改配置/发布/扩权/新终端。

## 开发接入与协议

独立安装包由 `pnpm display:pack` 生成到 `.artifacts/display/packages/`；SDK 与工具 README 也包含在 tgz 中。最小工程为 `samples/display-sdk/`，没有业务模板。外部工程验收脚本确实在系统临时目录安装本地 tarball，未从 workspace 导入源码。

- SDK：`packages/ts/display-sdk/README.md`，方法、精确数值、固定快照、订阅、取消、文件和生命周期。
- 工具：`packages/ts/display-devtools/README.md`，init/dev/login/validate/pack/upload、配对与 resume。
- 协议：`contracts/display/v1/protocol.json`；生成的方法/事件枚举同时被 Go/TS 使用。管理 API 合并在 OpenAPI，可信壳运行接口见 `contracts/display/v1/runtime.openapi.json`；公开 DTO 声明随 SDK 编译。

能力主版本、通信主版本与 npm 包版本分开维护。组合根安装真实资源、监测、来源、派生/固定成果、空间 Provider；告警保持 unavailable。新 Provider 通过显式 RegisterCapability 加入已版本化的方法、权限、预算和订阅元数据，不通过反射或任意 URL 扩展。

Bridge 仅信任固定 iframe.window + 本代次随机 nonce + MessageChannel；iframe sandbox=allow-scripts。上传应用可使用本地 ESM、Blob Worker、WASM、字体及 WebGL，不能调用任意公网 API或管理接口。所有业务读取在领域授权边界和返回前再次检查；资源引用、选择句柄、文件 ID 都不是通用授权凭据。

## 预算、诊断与故障

默认每租户 100 个运行会话，每会话 16 个订阅、8 个并发查询；同时限制项目、实例、方法和 SSE 连接。默认池 8，包 worker 池 2。单次结果 8 MiB、文件块 4 MiB、查询 15 秒。绑定/动态通道集合最多 500；超限明确返回 budget_exceeded。共享查询含主体、有效范围、配置、授权及源向量；无法证明可跨主体共享时保持隔离。同一主体的等价在途读取独立取消，最后一个引用释放才取消底层读取。

SSE 定期短连接重建；客户端按已应用游标确认，服务器保留最近 16 个事件、5 分钟续接窗口。游标缺口/过期/范围变化发送 reset。原始提交位置、派生 head、资源修订、业务授权和控制 outbox 组成源向量；5 秒有界时钟刷新覆盖跟随窗口及无显式变化通知的情况。权威进度在 PostgreSQL，进程缓存丢失只导致重读；Redis 不可用时无需保留唯一数据或游标。

`/healthz` 仅存活；`/readyz` 检查 display 元数据可读和包存储已配置。监测/派生/空间存储沿用各自运维 check，不能用该 readyz 代替整套存储校验。运行管理显示心跳、最近有效读取及查询状态；心跳不等同观测采样时间。

本机调用运行 backend `/internal/metrics`（Host 必须为配置运行 host）可读进程聚合连接数、查询/事件耗时、失败、共享命中和预算拒绝；代理必须拒绝 `/internal/`，不能公开 backend 端口。计数随进程重启，不含项目 ID/参数/凭据。包 worker 每分钟输出聚合校验耗时/失败指标；运行事件对同一会话同类错误每分钟至多保留一条。包 worker 记录校验失败，管理页保留校验状态/问题与尝试。现有业务读取日志沿用各模块的脱敏统计。

不得打开运行/资产 URL 的 access log：资产路径含短期许可。代理错误日志也不记录原始请求 URL。业务日志/事件不保存 cookie、一次性码、资产许可、源码或观测明细。display 事件和幂等收据按部署 audit_retention 保留（默认 90 天，worker 支持 1–365 天）；正式发布、配置和审计另按其原有规则保存。

| 症状 | 检查与处理 |
| --- | --- |
| 校验排队 | worker 进程、专用 SQL/对象权限、队列预算；租约过期后可接管，最多 5 次尝试。 |
| binding_invalid | 当前对象/指标语义、项目授权时段、绑定数量、读取委托；不能用旧缓存冒充有效。 |
| capability_unavailable | 能力发现、Provider 存储配置、当前人员/委托资格；告警本轮本来不可用。 |
| 运行突然停止 | 人员会话、终端状态、项目/租户启停、委托期限、环境代次和包完整性。 |
| 无法重连 | 三域 HTTP/Host/Origin、SSE 代理缓冲、短租约；重新登录或配对后重建快照。 |
| package_unavailable | verify-storage；恢复相同哈希对象后再验证，不覆盖已封存包。 |
| revision_conflict | 刷新管理信息，重新准备；不要复用旧准备结果覆盖并发修改。 |

## 引用回收、备份与恢复

保留发布、配置及有效运行引用的包不能回收。DELETE 包只标记无引用包为 reclaiming；worker 按对象意图与引用检查清除文件。上传 24 小时过期，孤立对象至少 25 小时后清理；失败/排队任务的上传片最多保留 7 天。事件游标 10 分钟清理，运行权限依然依赖当前租约，不依赖清理是否运行。

```sh
build/bin/infrapro display backup-manifest > /protected/backup/display-files.json
build/bin/infrapro display verify-storage
build/bin/infrapro display reclaim
```

备份清单只是对象清单，不是备份。备份时停止管理写入、运行及包 worker，固定 PostgreSQL 备份切点与私有 display/* 文件；保存清单里的包/path/size/hash/environment。监测、派生、空间同时按各自备份文档取得相容切点。

恢复顺序：停止外部入口和全部写入 → 恢复数据库和相同哈希的私有对象 → 按各模块恢复基础存储 → **启动运行服务之前**执行：

```sh
build/bin/infrapro account revoke-access --reason "备份恢复后撤销历史访问能力"
build/bin/infrapro display restore-invalidate
build/bin/infrapro display verify-storage
```

restore-invalidate 更换环境代次、撤销运行会话/订阅并清除终端续期/配对状态；旧码、许可、游标不能复活。verify-storage 逐文件验证；缺失/不符包标记 missing，不能启动。全部文件恢复且验证通过后才恢复包的 ready。随后启动 worker、运行进程、API/代理；普通人员重新登录，终端重新配对。生产恢复演练和具体项目上线仍需要部署方独立执行与确认。
