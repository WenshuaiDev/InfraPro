# 首期生产部署自验记录（2026-11-17）

状态：**Awaiting acceptance**。下表为清理前在四机部署上的历史自验结果；这些合成业务记录已按用户要求从生产删除，不能作为当前业务数据清单。人工验收待用户确认。

发行 `20261117-802a92f81b63`；PG schema 55。证据根目录 `.artifacts/production-deployment-20261117/evidence/`。源快照清单、构建日志、私有测试状态在同一 artifact 根目录，凭据不进入本报告。

| 范围 | 已验证结果 | 证据 |
| --- | --- | --- |
| 首个账号 | `gzzxadmin` 创建、平台管理权限、指定密码真实 HTTPS 登录；Secure/HttpOnly/SameSite 会话 | admin-login.json、browser-smoke.json、production-admin-console.png |
| 组织/工程 | 合成租户、项目、指标/单位/设施定义、设施、点位、直接测量通道创建 | private/acceptance-state.json 中 checks |
| 监测 | CSV 分片上传、映射、校验、发布到单 CH；查询 6 条记录 | 同上 |
| 派生 | A−B 试算、开启自动发布、正式代次查询值 4/5/9 | 同上 |
| 文件 | 8 MiB + 1 byte，首分片重复提交幂等、断点续传、checker/worker 处理；8388609 bytes 下载 SHA-256 匹配 | 同上 |
| 空间 | 实际 Python 转换 PNG，产出两种分辨率，发布 manifest；恢复后读取图像 SHA-256 匹配 | 同上、restore-business.json |
| 解析 | 目录同步、S3 样本、worker 调度独立 parser/子进程，81 byte DDP 样本值 12.34 | 同上 |
| 设备接入 | 产品/设备解析采用、租户确认、入口发布；TLS 1.3 分字节注册及测量、注销 ACK 129/133/130；3 条原文收到并确认，测量成功解析 | ingestion-wire.json、private/acceptance-state.json |
| 大屏 | 实际浏览器 ZIP 上传、包处理、资源绑定、读取授权、核验发布、三 HTTPS 来源、SDK 资源读取、opaque iframe、WASM/Worker/WebGL、刷新重载 | display-browser.json、display-management.png、display-runtime.png |
| 审计 | metadata 最小必要权限补齐；64 行 CSV、55902 bytes 可下载；4 个历史失败合成导出重试第 2 代成功 | private/acceptance-state.json、logs/retry-exports.log |
| 任务/通知 | 真实任务索引、后台投递；当前 12 个结束任务，active=0、attention=0 | private/acceptance-state.json |
| 独立诊断 | 4/4 observer，73/73 检查新鲜完整、39/39 预期常驻副本就绪，无异常组件/资源提示；独立网页登录无 JS 异常 | diagnostics-final.json、diagnostics-browser.json、production-diagnostics.png |
| 运行编排 | 四机 36 个常驻 systemd 服务 enabled/active；受控重启后密码登录、监测、附件、大屏继续可用；失败单元 0；四机 NTP 同步 | host-37/38/151/152-final.json、hosts-summary.json、browser-smoke.json、display-browser.json |
| 维护入口 | 冻结期间严格 CA HTTPS 请求返回 503；恢复后解除维护 | 运行操作日志、host-152-final.json |
| 原 38 业务 | 6 份配置 SHA-256 不变、7 个 HTTP(S) 入口 200、原代理端口保留 | 38-preserved-services-final.json |
| 联合备份 | 2 个完整点存放 38；第二次传输限速 8 MiB/s、停写 23.29 秒；SHA-256 一致，已启用每日 03:00 timer | backup-latest.json、host-152-final.json |
| 备份取回 | 从 38 读取第一份归档；SHA-256 匹配，154 个内部文件逐一核对 | restore-fetch.json |
| 独立业务恢复 | 独立 PG/CH/Keeper/对象桶真实恢复；文件内容核对 checked=3、complete=true；审计 checked=1、invalid=0；恢复写入守卫/旧会话撤销 | restore.json |
| 恢复后业务 | 原管理员密码登录；6 行监测、4/5/9 派生值、8 MiB 附件摘要、图纸正文和大屏发布记录 | restore-business.json |
| relay 恢复 | pg_restore 后 new-epoch；3 条 receipt/ACK/原文字节 hash 全部一致，epoch 已变化 | relay-restore.json、relay-integrity.json |
| 诊断状态备份 | 从第二份归档独立取出 diagnostics 状态并 inspect：73 个最新观察、4 个 observer fence，配置修订正常 | diagnostics-backup.json |

首份完整恢复点为 `checkpoint-20261117T093111Z`，第二份为 `checkpoint-20261117T095125Z`。恢复脚本执行 10.23 秒，仅是该合成数据量下的操作耗时，不包含准备和取回，不作为 RTO 或容量结论。

PG17 恢复时发现 public schema 已存在冲突，采用仓库 `deploy/production/restore-pg-restore.py` 适配器排除唯一重复的 SCHEMA TOC 项后，以单事务和遇错退出成功恢复。应用裸 restore 命令未做源码修改；运维交接已明确该依赖。

最初审计导出的权限失败、浏览器自动化 Node API 客户端的内部 CA 信任配置问题已分别通过角色补齐和浏览器同源请求解决。最终正式控制台、大屏和诊断浏览器自检全部通过。历史失败证据保留在本机 artifact；生产中的合成任务、导出和业务事件已清除。

测试监听已关闭；测试 producer 凭据已撤除；独立恢复实例、数据库、对象桶及目录已清除。生产按用户要求交付空白业务环境，保留首个管理员账号和生产配置。清理及清理后核验记录位于 `.artifacts/production-cleanup-20261117/`。

## 清理后的生产状态（同日）

用户要求正式生产环境不留验收样本，已完成以下清理及复核：

- 按冻结状态清单和逐行值校验删除 195 张表中的 1824 条合成记录；提交前验证全部数据库外键及管理员账号、密码记录和角色绑定不变。未重新 bootstrap。
- 租户、项目、工程定义/资源、厂家/产品/设备、监测导入/派生、附件/空间、大屏、模拟接入来源、验收任务/通知均为 0。遍历 328 张表，原验收租户和项目 UUID 的引用均为 0。
- ClickHouse 6 条原始观测和 3 条派生结果已清除，布局与表结构保留。业务 S3 四桶、历史 S3 均为空；接入缓冲 receipts、ACK、source revision 和 used_bytes 均为 0。
- 两个恢复演练 PostgreSQL 库、六个演练对象桶、恢复 CH/Keeper 单元及两台机器的演练目录已删除。
- 原 `gzzxadmin` 密码再次通过严格 CA HTTPS 登录；工作空间 API 只返回平台入口，tenant_count=0、project_count=0。已刷新用户原 Chrome 窗口，实际看到“平台尚无租户”，任务进行中及通知未读均为 0。
- 四机 36 个常驻服务 enabled/active，无失败单元、无维护标记。独立诊断 4/4 observer、39/39 副本、73/73 新鲜检查，异常组件和资源提示均为 0。
- 两份新备份校验通过，旧合成恢复点已移除；最新归档的 PG dump 已检查租户、项目、设备、任务和通知均无数据。每日备份 timer 已恢复。
- 37/151 已修复登录退出引起 PostgreSQL IPC 丢失的问题，配置和恢复要求已写入生产交接；多次 SSH 会话退出后数据库连接及共享内存正常。

清理证据目录：`.artifacts/production-cleanup-20261117/`。其中 `plan-summary.json` 为删除数量，`evidence/` 保存清理后的 API、数据计数、四机运行、诊断、对象存储及备份快照核验；`private/` 是受限维护材料，不提交。当前备份编号及摘要见生产交接。

未验证真实现场设备和外部网关链路、实际规模吞吐、长期 soak、整机故障/自动切换、PITR/异地灾备。主机准备阶段已经做过主机重启，本轮对新增生产服务做的是 systemd 受控重启与持久化复核，没有再次重启 38 整机。

人工验收入口、证书信任、运维和备份恢复边界见 [生产交接](../operations/production-deployment-2026-11-17.md)。
