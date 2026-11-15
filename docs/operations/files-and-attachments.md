# 文件与附件运行、升级和恢复

状态：Awaiting acceptance（2026-11-14 自验通过）。功能、自验与人工接受分别记录于 [自验矩阵](../verification/files-and-attachments.md) 和 [验收手册](files-and-attachments-acceptance.md)。

## 组成与启动

普通附件使用现有 PostgreSQL、私有 S3、API、`files-worker` 和 `files-checker`。任务与通知元数据仅访问 PostgreSQL，不需要 ClickHouse、S3 或检查器可用。通知由已有 `notification-worker` 投递，任务历史由已有 `task-index-worker` 对账。专业监测、空间和大屏继续使用各自原生存储与 worker。

开发启动 `pnpm dev` 会构建并启动文件 worker 和 checker；`--no-files` 可关闭这两个进程。API 与 worker 必须使用同一普通附件 bucket、暂存目录和 socket。未配置 S3 时，开发脚本以 `--allow-unconfigured` 让文件 worker 空闲等待并明确记录未配置，API 与其他模块继续运行；页面可查询获准元数据，上传显示依赖不可用。独立 `files-worker` 不带此开关，保持严格启动检查。独立合成环境使用 `pnpm files:demo`，无需用户提供业务文件。不要在已有业务环境重新 bootstrap。

配置模板为 `deploy/files/config.example.yaml`，环境变量是 `INFRAPRO_FILES_` 加模板字段大写；凭据使用 `INFRAPRO_FILES_ACCESS_KEY`、`INFRAPRO_FILES_SECRET_KEY`。数据库地址为 `INFRAPRO_DATABASE_URL`；`INFRAPRO_CONFIG` 指向受限 YAML/JSON。worker 通过独立 `LoadFiles` 读取数据库和 files 配置，不借用监测配置。配置默认值、范围由 `config/files.go` 与对应测试维护。

```sh
pnpm infrapro files status
pnpm infrapro files check
pnpm infrapro files worker
pnpm infrapro files checker --directory /run/infrapro-files --socket /run/infrapro-files/checker.sock
```

`check` 实际探测私有对象命名空间、socket 和 worker 心跳；`status` 只输出汇总及心跳，不输出文件名、账号、对象键或凭据。存储密钥不得传到 checker。CLI 的 checker 子命令使用 PATH 中的独立 `files-checker`，或通过 `INFRAPRO_FILES_CHECKER_EXECUTABLE` 指定二进制。

## 部署身份与隔离

迁移创建四个 NOLOGIN 角色：`infrapro_files_worker`、`infrapro_files_observer`、`infrapro_files_maintenance`、`infrapro_files_backup`。为部署进程分别配置独立登录角色并授予对应角色；API 保持现有人员业务身份。观察账号只可读 `file_operations_status` 和迁移版本。worker 可读必要授权列、操作文件状态、写任务与通知生产事实，不能读密码、会话、通知收件箱或通知偏好。备份角色只授予文件冻结和任务事实所需权限；完整 PostgreSQL 快照仍由已有数据库备份身份执行。

```sql
-- 由现有数据库管理员创建独立 LOGIN 身份后执行；密码由秘密管理系统提供。
GRANT infrapro_files_worker TO files_worker_login;
GRANT infrapro_files_observer TO files_observer_login;
GRANT infrapro_files_maintenance TO files_maintenance_login;
GRANT infrapro_files_backup TO files_backup_login;
```

`deploy/files/compose.yaml` 与主 Compose 合并，提供共享目录初始化、worker 与 checker。所有文件进程使用 UID 10001。checker 禁网、根文件系统只读、无 capabilities、无提权、320 MiB 容器内存和 32 PID 限制；子进程额外限制 CPU、输出文件、FD 和 socket 系统调用。systemd 等价模板位于 `deploy/files/systemd/`，API、worker、checker 应共享受限运行目录的同一 OS 身份。多个节点必须具有各自唯一 node_id；同节点的 API 与 worker 使用相同 node_id，以共享暂存和下载计数。

S3 使用专有 bucket 和 `files/v1/*` 命名空间，禁止匿名访问，示例策略见 `deploy/files/s3-policy.example.json`。API/worker 的策略可去掉 DeleteObject；清理进程单独持有删除权。备份导出身份仅需 List/Get，恢复身份需 Put/Get。对象 PUT 是独占创建，正文键永不复用；不能用 bucket 生命周期规则绕过引用保护直接删除正文。

macOS 本地可运行功能链路，但不声称具备 Linux seccomp 或 systemd 隔离。Linux 容器隔离自验使用 `pnpm test:files:sandbox`，实际采用无网络、只读根目录、非 root 和资源限制。

## 迁移与发布顺序

1. 按现有流程备份数据库，停止旧 worker，运行现有迁移命令升级至 40，并执行权限目录同步检查。
2. 创建普通附件专有私有 bucket、独立部署登录身份和暂存目录；配置 API、worker、checker。
3. 先启动 checker，再启动 worker、API；保留现有任务索引和通知投递服务。
4. `files status`、`files check`、`files verify --mode metadata` 通过后执行一个合成上传及下载摘要核对。
5. 新权限仅补本地受保护租户/项目管理员。已有自定义角色、平台管理员和租户项目治理权限不会自动获得附件正文访问权。

迁移不搬动专业文件，不重写其对象键与原生事件指纹。`EventActor` 是可省略的任务事件字段，空值保持旧 JSON 指纹。需要暂停新增上传时设置 `accept_uploads=false` 并重启对应服务；历史读取、已接受后台任务和已建立会话按其资格继续处理。

## 核对与清理

```sh
pnpm infrapro files verify --mode metadata
pnpm infrapro files verify --mode content
pnpm infrapro files verify --mode content --apply --reason '维护窗口完整性核对'
pnpm infrapro files cleanup plan --reason '清理已到期且无保护对象'
pnpm infrapro files cleanup apply --plan-id PLAN_UUID --reason '执行已复核计划'
```

内容核对实际读取完整对象并检查大小、SHA-256，不依赖 HEAD 推断完整性。维护输出中对象键属于受限诊断材料，不能作为用户下载地址或普通监控日志。清理只操作已登记 intent；未知对象只报告。计划 15 分钟有效，执行逐项复查 epoch、当前引用、读取租约、备份 hold、writer fence 和期限。正式历史修订长期保护正文；回收站 30 天后才可进入 purge，墓碑及审计保留。删除响应丢失时保留 delete_pending 和预算，复查实际不存在后才释放。不得手工删行或清空 bucket 来处理积压。

## 冻结备份与独立恢复

普通文件一致备份必须同时包含 PostgreSQL 冻结快照、`manifest.json`、`objects.jsonl` 和对象正文。冻结会关闭新写入、推进 writer fence，保持现有元数据可诊断；历史通知不重新生成。备份包包含受限对象清单，应按业务数据管理。

```sh
pnpm infrapro files backup prepare --directory /secure/backup/run1 --snapshot-ref snapshot-run1 --reason '一致备份'
pnpm infrapro files backup copy --id BACKUP_UUID --directory /secure/backup/run1 --reason '复制已核验正文'
pnpm infrapro files backup seal --id BACKUP_UUID --directory /secure/backup/run1 --reason '封存一致备份'
```

prepare 自动调用本机 `pg_dump` 生成 custom 格式快照；使用现有数据库备份身份，CLI 不把数据库 URL 放进命令行。`--pg-container` 仅允许隔离测试数据库。若生产数据库备份身份与文件操作身份分离，先在 prepare 命令增加 `--external-dump`，由文件维护身份冻结并取得备份编号，再以受控数据库备份身份保存 `database.dump`，继续 copy/seal。封存核验数据库文件及每个对象大小和摘要后恢复在线状态；备份保护仍保留，直到 `backup release --id ... --reason ...` 明确释放。

恢复必须使用独立数据库、独立普通文件 bucket 和空暂存目录。按现有 PostgreSQL 流程恢复 `database.dump`，将配置切换到恢复目标，然后：

```sh
pnpm infrapro files restore objects --directory /secure/backup/run1 --reason '隔离恢复演练'
pnpm infrapro files restore reconcile --reason '核对冻结快照与全部正文'
```

恢复核对推进 restore epoch，中断旧执行、作废旧上传资格、游标、读取租约和 writer fence。缺失/损坏任一必需对象时保留 reconciling，禁止清理且不报告恢复完成；补齐并重做核对才重新在线。历史修订与保护引用保留，原任务保留中断事实，获准操作可开启新代次，不补发历史通知。验证命令为 `pnpm test:files:restore`，使用真正的独立 PostgreSQL 数据库及 S3 bucket，并自动清理其临时资源。
