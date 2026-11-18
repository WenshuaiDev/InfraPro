# 首期生产部署交接（2026-11-17）

状态：**Awaiting acceptance（部署、自验完成，待用户人工验收）**。

已按确认的四机单实例方案部署全部现有业务进程。发行标识为 `20261117-802a92f81b63`，数据库 schema 为 55。发行源是当时主工作区的冻结快照，包含未提交变更；不是仅按 Git HEAD 打包。本轮未创建分支或 worktree，也未提交、推送。

源清单 SHA-256：`802a92f81b63c5161284ba526458d31b1d405d28b7bdb92548efc6e2bebd4a48`。四机发行物经 SHA256SUMS 核对。安装记录、源清单和验收证据位于 `.artifacts/production-deployment-20261117/`，凭据位于其受限的 `private/`，不得提交。

## 访问与账号

| 用途 | 地址 | 身份 |
| --- | --- | --- |
| 管理控制台 | https://10.23.52.152:18443/console/ | 首个业务账号 `gzzxadmin`，已授予平台管理员；密码按用户指定值设置并实际登录验证 |
| 独立运行诊断 | https://10.23.56.38:18096/ | 独立账号 `gzzxadmin`；独立随机密码保存在本机 `.artifacts/production-deployment-20261117/private/diagnostics-login.json` |
| 大屏运行入口 | https://10.23.52.152:18444 | 从控制台启动已发布大屏，完成身份交接 |
| 大屏资源来源 | https://10.23.52.152:18445 | 受限资源来源，由运行壳使用 |

诊断密码策略要求至少 12 个字符，因此没有复用业务账号密码。业务和诊断均已完成实际浏览器登录，页面没有 JavaScript 异常。

使用内部 CA 和含真实 IP SAN 的服务器证书。客户端首次访问前，应导入并信任本机文件 `.artifacts/production-deployment-20261117/public/infrapro-production-ca.crt`。macOS 可导入“钥匙串访问”，在该证书的“信任”中配置 SSL 信任；Windows 可导入受信任根证书颁发机构。CA 指纹为：

`2E:2F:57:01:C1:BA:15:83:68:75:F3:BF:E5:B6:8F:78:CE:29:B9:8A:3D:89:40:16:FC:58:FE:8D:17:CD:5C:E6`

服务器证书有效期 825 天，CA 有效至 2036-11-14。自动验收使用指定 CA 严格验证 HTTPS；浏览器测试限定证书公钥指纹。没有修改用户操作系统的信任库。

## 实际服务分配

| 主机 | 常驻 systemd 服务 | 数据目录 |
| --- | --- | --- |
| 10.23.56.37 | PostgreSQL、Redis、SeaweedFS master/volume/filer/S3、observer，共 7 个 | `/home/gzzx/env/infrapro/data` |
| 10.23.56.38 | 新增备份 SeaweedFS 四进程、diagnostics、observer，共 6 个；原服务保留 | `/home/gzzx/env/infrapro/data` |
| 10.23.52.151 | 接入专用 PostgreSQL、历史对象 SeaweedFS 四进程、单 Keeper、单 ClickHouse、ingest-relay、observer，共 9 个 | `/home/uadmin/env/infrapro/data` → `/data/infrapro` |
| 10.23.52.152 | API、monitoring/notification/task-index/spatial/display-package/files/audit/ingest workers、display-runtime、files-checker、parser、Nginx、observer，共 14 个 | `/home/uadmin/env/infrapro/data` → `/data/infrapro` |

36 个常驻服务均 enabled/active，另有四台的 scoped firewall oneshot 和 152 的每日备份 timer。诊断目录按逻辑角色登记 39 个常驻副本（其中 S3 逻辑角色共用对应 gateway 进程），因此诊断数字与 OS 进程计数不同；不是数据库有 39 个数据副本。

依赖版本：PostgreSQL 17.12、Redis 7.4.12、SeaweedFS 4.53、Nginx 1.30.6、ClickHouse/Keeper 26.8.38.9 amd64compat。Python 转换使用已准备的 3.12.3 和兼容 CPU 的独立依赖。使用主机准备阶段验证过的程序和 wrapper，没有改成容器部署。

每台安装根为对应用户的 `~/env/infrapro`，包含 `bin/`、`config/`、`data/`、`logs/`、`ops/`、`units/` 和 `releases/`。`current` 指向上述发行物；systemd 当前固定引用该发行目录，单独改动 current 不会切换服务版本。

37 的 PG 55432、Redis TLS 16379、业务 S3 TLS 18333；151 的 CH TLS 9440/HTTPS 9443、relay mTLS 19443；38 的备份 S3 TLS 18334。数据库及内部 S3 监听受专属 nftables 表限制。151 relay PG 为 loopback；Keeper 的 Raft 端口还由防火墙阻断外部访问。152 的业务 HTTP、解析、运行壳内部端口仅监听 loopback，外部经 Nginx HTTPS 进入。

38 新增常驻服务共用 `infrapro-production.slice`，CPU 配额 1 核、内存上限 2 GiB；备份传输限制为 8 MiB/s，接收进程使用较低 CPU/IO 优先级。151/152 数据卷已挂载，四台时间同步正常。

## 备份及恢复

152 的 `infrapro-backup.timer` 已启用，每天北京时间 03:00 起一分钟内执行；Persistent=true，错过的计划在 timer 恢复后补执行。升级前可手工执行同一服务：

```sh
sudo systemctl start infrapro-backup.service
sudo systemctl status infrapro-backup.service infrapro-backup.timer
sudo journalctl -u infrapro-backup.service -n 50 --no-pager
```

程序位于 `/usr/local/libexec/infrapro-production-backup`。仓库中的对应源码为 `deploy/production/production-backup.py`、`backup-node.py` 和 `s3_archive.py`。152 通过限定来源 IP、forced command、禁止转发的专用 SSH 密钥访问备份节点；受限 sudo 只允许停止/启动指定 relay 或 diagnostics 服务。

备份先开启 Nginx 503 维护页、停止 relay 和业务写入进程，再执行文件冻结、审计 hold、监测联合备份和对象复制。覆盖业务 PG、relay PG 与接收确认清单、CH 原生数据、全部业务对象桶、文件协议清单、全部保留发行目录及解析包、受保护配置、各机 units/ops。38 的 diagnostics 状态在自身短暂停机时复制。完成文件封存后恢复业务，再打包并发送到 38。

38 仓库：`/home/gzzx/env/infrapro/data/checkpoints/`；152 工作副本：`/home/uadmin/env/infrapro/data/backups/`。归档和凭据均为受限权限。保留最近两份完整且校验通过的归档，预算 300 GiB，接收时保留至少 20% 文件系统空闲空间；容量不足时拒绝新归档，保留既有恢复点。CH 原生备份的临时 S3 对象在归档传输核验后按精确清单移除，归档内保留可移植的原生备份正文。

当前保留的两份恢复点均在验收数据清理后生成；原含验收数据的两份归档已从 38 仓库和 152 工作副本移除。

| 恢复点（UTC 名称） | 停写窗口 | 总耗时 | 归档字节数 | 核验 |
| --- | --- | --- | --- | --- |
| checkpoint-20261117T102539Z | 21.07 秒 | 78.74 秒 | 350434127 | 发送/接收 SHA-256 一致，完整 tar 读取；四个业务对象桶均为 0 |
| checkpoint-20261117T102755Z | 20.21 秒 | 77.33 秒 | 350456110 | 发送/接收 SHA-256 一致，完整 tar 读取；四个业务对象桶均为 0 |

最新一份 SHA-256：`f47a248a31693b522ee5d1ee01d9daafdce63f523b61b2ccefa100ab6554ee1d`。其 PG dump 内租户、项目、设备、任务和通知均为 0 行。

以下为清理前的历史恢复演练结果；合成数据及演练目标现已清除。

完整恢复演练从 38 重新取回第一份归档，恢复到独立的 `infrapro_restore_20261117`、`infrapro_relay_restore_20261117`、独立 CH/Keeper 和独立对象桶。按监测恢复、文件正文恢复、审计/任务/文件协议对账、内容核验、最后开放写入的顺序执行。旧会话被撤销，relay epoch 已旋转。账号密码、6 条监测数据、派生值 4/5/9、附件 SHA-256、已发布图纸/大屏和全部 3 条 relay 原文及 ACK 核验通过。

恢复命令脚本执行耗时 10.23 秒，不包括下载归档、准备独立目标和业务复核，不将其作为生产 RTO。演练实例、两份独立数据库、独立对象桶及 `data/restore-20261117` 已清除，测试端口已关闭。

当前 PG17 归档包含 `CREATE SCHEMA public`，而监测恢复过程会先为 btree_gist 准备 public。恢复时必须使用已提供的 `deploy/production/restore-pg-restore.py` 适配器（152 已安装到 `ops/restore-pg-tools/pg_restore`），将该目录置于 PATH 前方。它仅在 TOC 中排除重复的 `SCHEMA - public` 项，其余对象仍以 `--exit-on-error --single-transaction` 恢复，不忽略其他错误。现有程序的裸恢复命令尚不直接处理这一 PG17 重复 schema 情形。

恢复到全新机器还需创建部署角色并恢复授权。`config/application-roles.sql` 包含部署账号和授权，须按是否已有角色分开执行；`deploy/production/schema-55-audit-role-completion.sql` 补充当前审计元数据所需权限。不要在已有环境重复执行初始化 SQL 或账号 bootstrap。

计划备份要求短维护窗口，持续时间随数据量变化。备份中断留下 `file_protocol=read_only` 时，保留原备份 ID、目录和日志，补齐同一冻结点的 dump/copy/seal；不要直接修改协议表或删除对象解锁。首期没有自动主备切换，也没有 PITR/异地灾备承诺。

## 日常操作

在 152 上可用 `~/env/infrapro/ops/infrapro-maintenance` 调用配置好的维护 CLI，例如：

```sh
~/env/infrapro/ops/infrapro-maintenance monitoring storage check
~/env/infrapro/ops/infrapro-maintenance files verify --mode content
~/env/infrapro/ops/infrapro-maintenance audit status
```

服务日志通过 `journalctl -u infrapro-具体组件` 查看，Nginx 错误日志位于 `logs/nginx-error.log`。使用平台诊断查看四节点、组件和资源；备份结果同时检查 timer/service 及 152 的 `data/backups/latest.json`。修改配置后先校验，再按依赖顺序重启对应服务。

本轮修正了部署角色的通知投递、审计读取权限，以及 files-checker socket 的受限暂存目录位置。审计权限只补当前元数据读取所需对象，不授予账号密码表或普通文件对象正文权限。验收产生的导出、任务和通知随后已按用户要求清除；历史测试结果只作为本机离线证据保存。

38 的既有配置未修改：6 份原配置摘要与主机准备记录一致，8555–8560 HTTP 和 9000 HTTPS 返回 200，11883/11884 保持监听。本轮只重启 InfraPro 新增服务；没有把旧业务完整流程或旧 Nginx 开机编排计入本轮验收。

## 生产初始状态与人工验收

生产应交付空白业务环境。此前误留的部署验收租户、项目和全部关联业务样本已清除，平台管理员 `gzzxadmin`、原密码、现有会话、系统权限、生产接入节点和运行配置保留。禁止再次以验收为由向生产环境植入示例租户或业务样本。

1. 打开控制台，以 `gzzxadmin` 和用户指定密码登录；可进入平台工作空间。
2. 租户管理显示“平台尚无租户”；工作空间接口仅返回平台入口。
3. 任务中心没有遗留验收任务，通知没有遗留验收消息。公共工程定义、模拟厂家/产品/设备、监测/派生、文件/空间及大屏验收资料均已移除。
4. 独立诊断应显示四台 observer 与所有已部署生产服务健康。
5. 在需要开展正式业务时，使用真实组织资料创建租户、项目和工程资源。

## PostgreSQL 登录退出修复

清理时发现：37、151 的 PostgreSQL 使用 UID 1000 部署用户运行，系统 logind 在该用户最后一个登录会话退出时会清理 IPC，导致数据库报共享内存段不存在。已在这两台机器安装 `/etc/systemd/logind.conf.d/90-infrapro-postgres.conf`，设置 `RemoveIPC=no`，reload logind 并重启 PostgreSQL。参考配置为 `deploy/production/postgresql-logind.conf`；新机恢复时必须同时应用此配置。38 不涉及该修正。

该配置与 PostgreSQL 官方对登录用户运行数据库时的说明一致：[PostgreSQL 17 · RemoveIPC](https://www.postgresql.org/docs/17/kernel-resources.html)。

真实现场设备的公网/专网/网关路径及真实厂商载荷、实际规模负载、长期运行稳定性尚未纳入本轮。用户选择的单实例方式保持不变，机器故障时采用受控重启或备份恢复。软件自验不替代用户人工验收。

详细证据见 [生产部署自验记录](../verification/production-deployment-2026-11-17.md)。
