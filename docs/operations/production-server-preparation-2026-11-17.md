# 生产服务器清理与数据卷准备记录（2026-11-17）

历史记录说明：本文保留最初清理和数据卷准备的执行结果。后续侧边任务已完成四台重启、NTPsec校时、离线运行依赖及兼容性实测，用户已确认主机准备完成；最新结果和实际 `~/env/infrapro` 目录以[离线环境交接](../../.artifacts/offline-env-preparation-20261117/reports/environment-handover.md)及[当前部署方案](production-four-server-deployment-plan-2026-11-17.md)为准。下文“未重启／未修复时钟”等陈述仅描述当时清理任务的范围。

状态：**Awaiting acceptance**。37／151／152 的旧业务清理、151／152 的数据卷准备已执行并通过下列自检；用户人工验收尚未进行。InfraPro 新系统尚未安装，具体分工见[四服务器部署方案](production-four-server-deployment-plan-2026-11-17.md)。

## 执行范围

用户明确要求保留 `10.23.56.38`，清除另外三台的旧业务，随后追加要求处理 151／152 未分配的磁盘空间。实际执行保留操作系统、网络、SSH、虚拟机基础组件与系统日志；删除已盘点的旧业务服务、数据库、业务文件、安装包、备份和对应配置。38 仅作只读检查。

执行前核对实际主机名和 IP；业务卸载先检查软件包删除计划，停止服务与业务定时任务后清理数据。管理员凭据只用于交互认证，没有写入报告、脚本或配置文件。所有本次管理员 SSH 会话已退出。

| 服务器 | 已清除的旧业务 | 清理后的根文件系统 |
|---|---|---|
| 10.23.56.37，jkjcpt-database2 | PostgreSQL 16、MySQL 8、GBase／Gauss、相关数据／日志／SQL 备份、Java 安装目录、GBase 服务及全局环境变量；移除对应专用账号 | 使用约 8.64 GiB，可用约 454.62 GiB，占用 2% |
| 10.23.52.151，gzzxapp | ClickHouse、TDengine 及其组件／链接／服务、Java、旧用户目录中的业务包和备份；移除 clickhouse 账号 | 使用约 8.14 GiB，可用约 84.72 GiB，占用 9%；清理前占用约 97% |
| 10.23.52.152，gzzxdb | 三个 Java 应用、Nginx、Python relay、旧网站／业务文件／定时任务、Java／LibreOffice／Firebird 软件包和相关业务配置 | 使用约 13.05 GiB，可用约 79.80 GiB，占用 15% |

上述空间是核验时点的 `df` 结果，后续系统日志会产生小幅变化。SSH 登录用户及其 `.ssh` 保留；基础 shell 配置和系统组件保留。

## 151／152 已完成的数据卷布局

两台原有 500 GiB 虚盘均已加入 `ubuntu-vg`，但原先只分配 100 GiB 根 LV，余下约 398 GiB 在卷组内闲置。实际从现有空闲空间各新建一个 350 GiB LV，没有修改原根 LV 大小或 UUID。

| 项目 | 10.23.52.151 | 10.23.52.152 |
|---|---|---|
| 根 LV | 100 GiB，挂载 `/` | 100 GiB，挂载 `/` |
| 新数据 LV | `/dev/ubuntu-vg/infrapro-data`，350 GiB | `/dev/ubuntu-vg/infrapro-data`，350 GiB |
| 文件系统／挂载 | ext4，`/data`，`rw,nodev,nosuid` | ext4，`/data`，`rw,nodev,nosuid` |
| 数据卷 UUID | `cb5e644c-14d4-4576-8ed5-9727b559efa0` | `359979ba-7875-4011-b5c7-b589b5ac3db7` |
| 数据卷初始可用空间 | 约 339.92 GiB | 约 339.92 GiB |
| VG 留存扩容空间 | 51,535,413,248 字节，约 48 GiB | 51,535,413,248 字节，约 48 GiB |

数据卷的文件系统元数据与 1% 保留块会占用一部分空间，因此可用容量小于 350 GiB。余下约 48 GiB 是明确保留的扩容余量，可后续分配给根卷或数据卷。新卷当前尚无业务数据；部署时持久数据使用 `/data/infrapro/<component>`，相关 systemd 服务增加 `RequiresMountsFor=/data`，避免数据卷未挂载时误写根目录。

两台 `/etc/fstab` 均已使用数据卷 UUID 配置自动挂载，`data.mount` 已由 systemd 生成并纳入 `local-fs.target`。原始文件留存为 `/etc/fstab.pre-infrapro-data-20261117`。151 的首次新增行被语法检查发现换行转义错误，在首次挂载前修正；最终两台语法及设备解析检查均通过。

## 自检结果与证据

| 检查 | 结果 |
|---|---|
| 三台旧业务软件包／服务项 | 已列举的业务软件包与服务项均无残留 |
| 三台旧业务目录／专用账号 | 清理清单中的路径均不存在，待删除的业务账号均不存在 |
| 三台监听与定时任务 | 仅 SSH 和 loopback DNS 监听；用户业务 cron 已清空 |
| SSH 与软件包状态 | `sshd -t`、`dpkg --audit` 均返回 0；新的 SSH 登录正常 |
| 两台磁盘挂载配置 | `findmnt --verify --verbose` 返回 0，根／boot／数据卷均解析成功；保留的 swap 文件提示属于已有配置提示 |
| 两台读写持久性 | 各写入并 fsync 32 MiB；经 systemd 卸载后重挂，SHA-256 一致；测试文件已删除 |
| 两台开机依赖 | `data.mount` 为 active/mounted，来源 `/etc/fstab`，已进入 `local-fs.target` 依赖 |
| 两台原根卷 | 大小保持 100 GiB，挂载 UUID 保持原值 |
| 38 保留核验 | 原 Nginx／Progma 代理／Python relay 的 8 个已知进程 PID 仍存在；8555–8560、9000、11883／11884 等原监听保留；未执行配置写入或服务重启 |

现场证据保存在本工作副本的 [`.artifacts/production-planning-20261117/`](../../.artifacts/production-planning-20261117/)：`before-root-*`、`after-cleanup-*`、`cleanup-paths-server-*`、`before-data-volume-server-*`、`after-data-volume-server-*`、`fresh-ssh-data-volume-server-*` 及 `after-preservation-server-38.json`。这些是本机核验记录，不是已上传的共享附件。

未执行整机重启、硬盘性能测试或断电测试；开机挂载的依据是已生成的 systemd 启动依赖及实际停挂／重挂结果。没有修复时钟、改变 38 的业务、安装新 InfraPro 服务或运行生产迁移。38 的保留核验没有代替其全部业务端到端测试。

## 人工核验

可通过既有 SSH 登录三台，确认旧业务端口和文件已清理；在 151／152 执行 `df -hT / /data`、`findmnt /data` 和 `systemctl status data.mount`，应看到独立数据卷正常挂载。后续正常维护重启后再次检查 `/data`；38 原站点按既有访问方式核验。人工结果单独记录，不能把本文自检等同于用户接受。
