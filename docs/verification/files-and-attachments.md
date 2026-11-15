# 文件与附件公共能力自验记录

实施依据：[详细开发方案](../requirements/files-and-attachments-development-plan.md)。软件状态：**Awaiting acceptance**（2026-11-14 自验通过）；人工接受尚未进行。工作在原 checkout/main 完成，未创建分支或 worktree。已有监测、空间、大屏、任务及通知的未提交工作保留。

## 验证环境与范围

使用合成账号、两租户及每租户两项目；隔离 PostgreSQL schema、专用私有 S3 bucket、真实 checker 子进程、文件处理、任务索引和通知投递。专业回归使用真实 ClickHouse/S3、空间 Python 转换器及三 origin 大屏运行环境。演练没有访问正式业务数据。

普通格式检查覆盖 PNG/JPEG/PDF/TXT/CSV/DOCX/XLSX/PPTX。HTTP 路径执行真实分片、封存、完整性检查、下载、Range/HEAD/条件请求；原生下载保存到磁盘后对 SHA-256。受控时钟测试 24 小时、30 天和租约边界，不改变操作系统时间。Linux checker 在无网络、非 root、只读根文件系统容器中实际验证 seccomp 与资源限制。

证据目录 `.artifacts/files/` 为本机生成材料；测试代码与本文进入仓库，账号和含连接信息的测试配置不进入仓库。主机 macOS 的功能运行与 Linux 隔离验证分别记录；未进行生产部署、跨机房恢复、容量/压力或长时测试，这些不属于本轮必要功能自验。

## 范围与实现

| 范围 | 实现入口 | 自验类别 |
| --- | --- | --- |
| FA-01 / FD-01 | `files`、迁移 00040：不可变正文、对象 intent、固定修订及状态 | 真实 PG/S3、字节边界、恢复 |
| FA-02 / FD-01/04 | 分片收据、共享预算、完整校验、fence、epoch、严格 HTTP | 并发、丢响应、崩溃、HTTP |
| FA-03 / FD-01 | 附件元数据、替换、还原、回收、恢复、保护引用 | CAS、历史、清理、浏览器 |
| FA-04 / FD-07 | 纯 UI pattern、摘要 Worker、三页签、两资料页 | 键盘、响应式、真实预览、账号切换 |
| FA-05 / FD-08 | 四个上传 Adapter、DomainFileRef、全部现有导出与原件入口 | 专业真实服务、浏览器及 24 项客户端检查 |
| FA-06 / FD-02/03/04 | 六权限、Host Adapter、人员读取、每块授权、审计 | 权限矩阵、成员快照、归档、慢连接 |
| FA-07 / FD-05/06/10 | files.prepare、固定代结果、file_job 通知、独立维护 CLI | 事实回滚、索引、通知投递、最小权限身份 |
| FA-08 / FD-09/11/12 | OpenAPI/客户端、配置/发行、演示、冻结 PGDMP 与对象恢复 | 目录升级、契约检查、独立数据库与 bucket 恢复 |

所有 FD-01–FD-12 均在上表落到实际代码、页面或运行材料。完整接口和四个适配器示例见 [接入指南](../development/files-and-attachments-integration.md)，部署和恢复见 [运行手册](../operations/files-and-attachments.md)，账号及手工预期见 [验收手册](../operations/files-and-attachments-acceptance.md)。

## DA 验收证据索引

共同前置为上述隔离环境、合成数据和当前权限身份；各行记录实际操作及对应预期，实际结果均已核对为通过。涉及时间和并发边界的前置由测试夹具显式建立，证据见下方日志和测试源码。

下表引用的测试位于 `packages/go/platform/resources/files_*_test.go`；内容检查位于 `internal/contentcheck/check_test.go`，检查器监督与 Linux 隔离位于 `files/*_test.go`，HTTP 位于 `services/api/internal/transport/http/files_test.go`。浏览器脚本均调用真实服务，局部 Adapter 测试单独标识。

| 条件 | FC 对应 | 实现、操作、预期与证据 | 实际结果 |
| --- | --- | --- | --- |
| DA-01 | FC-01 | `files-browser-check` 在租户与项目资料页上传并刷新，以相同附件 ID 返回文件页；第二租户为空。 | 通过 |
| DA-02 | FC-01/20 | `Boundary` 明确权限状态，FileList 独立 loading/error/empty；真实空状态及 profile-only 账号切换；HTTP/领域依赖错误保持 503。 | 通过 |
| DA-03 | FC-02 | 浏览器重新选 8 MiB+1 文件仅 PUT 第 2 片；控制器独立摘要校验拒绝同名异文及歧义收据。 | 通过 |
| DA-04 | FC-02 | `ByteBoundariesReceiptsAndSharedBudgets`、`ConcurrentPartsMaintainBudgetLockOrder`、`LateWriterFenceAndProducerRollback`。 | 通过 |
| DA-05 | FC-02/16 | create/complete/reconfirm/attachment 命令保存 payload 指纹与原结果；浏览器中断已提交 complete 响应，复用同 request_id；领域检查异文冲突。 | 通过 |
| DA-06 | FC-03 | 0/1/8 MiB/8 MiB+1/100 MiB/100 MiB+1 实际输入、声明长度偏差、摘要及片边界检查；100 MiB 正文真正读写。 | 通过 |
| DA-07 | FC-03 | 严格 JSON 拒绝重复/未知字段；路径/CRLF/格式/摘要验证；伪装正文不能绑定。 | 通过 |
| DA-08 | FC-03/10 | Office 重复项、越界、加密、符号链接、宏、条目/展开预算；独立子进程挂起、崩溃与结果身份错误。 | 通过 |
| DA-09 | FC-03/10 | PNG/JPEG 重新编码得到新摘要，1600 边长/2 MiB 上限；超像素、解码失败分别标识；浏览器实际受控预览。 | 通过 |
| DA-10 | FC-04 | Abort 停止 hash Worker/未发送片；complete 后独立处理；`BindingUsesOriginalMembershipAfterRejoinAndIgnoresLogout` 验证注销不取消已接受委托。 | 通过 |
| DA-11 | FC-04/09 | 每次提交核验账号、原成员 ID 与宿主；最后检查后退出再加入不能绑定；项目归档拒绝写入。 | 通过 |
| DA-12 | FC-04/05 | `ReplacementConflictReconfirmHistoricalTargetAndReads`：并发预期修订冲突、needs_action、重确认新代。 | 通过 |
| DA-13 | FC-05 | 浏览器替换再还原生成修订 3；领域保存旧正文、原因与操作者，数据库禁止改写历史。 | 通过 |
| DA-14 | FC-05 | 浏览器改名与说明；正文文件名/大小/hash 固定；blobstore 禁止复用键覆盖。 | 通过 |
| DA-15 | FC-06 | 回收关闭正文入口、恢复原历史；命令幂等；过期恢复被拒绝。 | 通过 |
| DA-16 | FC-06/15 | 清理与恢复均持空间/附件/文件锁，apply 再查期限与引用；backup hold、晚到 consumer fact 保留正文和墓碑。 | 通过 |
| DA-17 | FC-07 | `PermissionsCurrentReadBoundaryAndArchivedHost` 覆盖 profile-only、R、R+M 无 D、R+D；多账号演示可直接复验。 | 通过 |
| DA-18 | FC-07/18 | 租户治理不能隐含读项目附件；六权限目录/受保护管理员升级，现存自定义角色保持原权限。 | 通过 |
| DA-19 | FC-08 | 同空间复合外键与每路由授权；跨空间附件/历史/任务拒绝；篡改游标/过滤和 restore epoch 拒绝。 | 通过 |
| DA-20 | FC-08/09 | 人员 HTTP 身份边界拒绝 ReadSubject；原 display subject/resource 契约和实际隔离运行回归。 | 通过 |
| DA-21 | FC-09 | GuardedRead 每 64 KiB 同事务复核；撤权、结束会话、回收后不再输出；真实 TCP 停读，租约约 2.6 秒关闭。 | 通过 |
| DA-22 | FC-10 | 真实 HTTP GET/HEAD、单 Range、溢出/多 Range、ETag 条件、无权 304/416、attachment/nosniff/no-store 检查。 | 通过 |
| DA-23 | FC-10/20 | 缓存包含账号/成员/空间/授权版本，离开 Abort/remove；同浏览器退出再登录 profile-only 账号不残留旧附件；键盘和 375px/200% 缩放。 | 通过 |
| DA-24 | FC-11 | 真实监测 UI 完成 CSV 上传→映射→校验→采用→导出，公共控制器保留 32 MiB 分片和 CSV/XLSX 原限制。 | 通过 |
| DA-25 | FC-11/13 | `files-spatial-browser-check`：真实 PNG 上传、Python 转换、发布、原件摘要；原空间项目边界集成测试。 | 通过 |
| DA-26 | FC-11/13 | 大屏 UI ZIP→校验→配置/授权→发布；HTTP 三 origin、opaque iframe、SDK 导航、终端、离线租约与撤权；未增加包正文下载接口。 | 通过 |
| DA-27 | FC-12 | 监测/派生/分析/比较/输入依据导出 DomainFiles；实际 CSV/Parquet 下载、清单、卷次和期限保留。 | 通过 |
| DA-28 | FC-12/13 | 描述不发授权，动作调用原模块；过期不显示动作，原服务检查失效/撤权/缺失；普通附件严格输入不接受 DomainFileRef。 | 通过 |
| DA-29 | FC-14 | 真实 file_jobs/job_runs 投影 queued/running/failed/cancelled/needs_action/succeeded；稳定 task ID；来源错误明确拒绝。 | 通过 |
| DA-30 | FC-14 | 自动恢复同代不同 attempt，人工 retry/reconfirm 新代；原请求人不变，管理员 EventActor 正确；旧指纹兼容测试。 | 通过 |
| DA-31 | FC-14 | 固定代唯一通知键、个人偏好关闭、管理员代取消不广播；浏览器暂停真实投递 worker，业务成功后恢复投递。 | 通过 |
| DA-32 | FC-09/14 | 旧成功目标在替换后仍指旧修订；回收/撤权裁剪；任务→文件与通知→固定任务浏览器往返。 | 通过 |
| DA-33 | FC-14/16 | 任务终态或通知生产触发器故障使附件提交回滚，intent 留存；恢复后只绑定一次。 | 通过 |
| DA-34 | FC-15 | 历史、read lease、consumer fact 和备份 hold 独立保护；迟到保护在 apply 再查；未登记 consumer 的 ProtectTx 被拒绝。 | 通过 |
| DA-35 | FC-16 | 延迟旧 Put、失败终态提交、删除响应丢失、实际 SIGKILL 进程后同代接管；无重复附件或事件。 | 通过 |
| DA-36 | FC-17 | CleanupPlan 仅登记对象、有限批次，apply 复核版本/epoch/引用；未知对象只诊断；恢复未完成禁止清理。 | 通过 |
| DA-37 | FC-03/16/20 | 多 Service 并发分片与数据库共享预算；会话、队列、临时字节及下载上限；不确定删除不提前释放，核对后释放。 | 通过 |
| DA-38 | FC-17/20 | 检查器无凭据环境、超时/崩溃监督；Linux 实测禁 socket、CPU/FD/文件限制、非 root/只读；observer 只能读安全汇总。 | 通过 |
| DA-39 | FC-18 | 旧数据库迁移到 40，保留 190 张既有表及业务源；权限精确计数和自定义角色回归，任务/通知旧指纹与旧目标兼容。 | 通过 |
| DA-40 | FC-19 | 实际 PGDMP、独立数据库和 S3 bucket、5 个已核验对象；故意缺对象保持 reconciling，补齐后恢复。 | 通过 |
| DA-41 | FC-19 | 恢复换 epoch、旧上传/游标/read lease/fence 失效，原 queued 代记 restore_interrupted，不补发历史通知。 | 通过 |
| DA-42 | FC-20 | 请求收据、上传/job、附件/修订、审计和 TaskRef 相互关联；页面区分传输、后台检查、绑定和受控下载。 | 通过 |
| DA-43 | FC-01/20 | accept_uploads=false 不取消既有会话或历史读取；ComposeTasks/ComposeNotifications 无存储依赖；worker/checker 独立健康与安全错误。 | 通过 |
| DA-44 | FC-20 | API/生成类型/权限、配置模板、Docker/Compose/systemd、CLI、样本、迁移/恢复/人工验收材料齐备。 | 通过 |

外部 consumer 首版没有真实调用者，因此没有登记虚构业务模块。清理的 consumer 分支用测试内持久引用验证；公开 ProtectTx 同时验证不允许任意模块建立保护。普通附件不会把专业引用转换成可永久保存的正文。

## 命令与证据

| 命令/检查 | 证据 |
| --- | --- |
| `pnpm test:files` | `domain.log`：14 个领域场景通过；`crash-recovery.log`：新增实际 SIGKILL 恢复通过；`domain-run.log`：内容检查/配置/核心单元通过 |
| `pnpm test:files:http` | `http.log`：真实 HTTP/慢下载通过 |
| `pnpm test:files:restore` | `restore.log`：独立 PGDMP + S3 恢复通过 |
| `pnpm test:files:client` | `client.log`；相关 24 项客户端检查见 `affected-clients.log` |
| 真实 CLI `files status` | `cli-status.log`：在线、worker/checker 心跳、允许汇总正常；没有文件名/正文键/凭据 |
| worker 开发启动退化与独立启动 | `worker-startup.log`：无存储配置时开发 worker 保持空闲并能正常退出；独立启动仍严格校验；最后定点 vet/lint 通过 |
| `pnpm test:files:sandbox` | `linux-sandbox.log`；进程监督见 `checker-process.log` |
| `pnpm files:browser` | `browser/result.json`：12 条真实旅程通过；桌面/手机/200%/预览截图已目视检查，原生下载 SHA-256 一致 |
| `node scripts/files-spatial-browser-check.mjs` | `spatial-browser/result.json`、发布截图和原件文件，通过 |
| `pnpm display:browser` | `.artifacts/display/browser/result.json`：15 条检查通过；管理/运行/离线截图 |
| 监测/派生真实 Playwright | `professional-e2e.log` 首轮 5 条通过，`professional-e2e-rerun.log` 派生绑定/输入依据导出通过，`professional-e2e-final.log` 余下分析、局部授权与监测完整导出 3 条通过；合计 9 条相关旅程全部完成 |
| `pnpm test:tasks` 的来源集成及后续检查 | 初轮 31 个来源场景中 30 个通过，任务汇总失败经修复后定点通过（`indexer-and-producers.log`）；后续任务/通知/恢复全部通过（`task-notification-final.log`）；旧权限计数断言修正后通过（`catalog-regression.log`） |
| `pnpm check`（Go fmt/vet、lint、全部 typecheck、契约及权限生成校验） | `final-check.log`：全部通过 |
| `node scripts/go.mjs check`、Compose 合并解析 | `go-check.log`、`deployment-config.log`：通过；最终 Go 检查也包含在 `final-check.log` |

## 本轮发现并修正

- 分片暂存登记与人员授权锁顺序不一致导致并发死锁：统一 protocol→authorization→budget→account→host/space 顺序，增加真实并发回归。
- worker 最小数据库角色缺少必要授权列及行锁权限：按列补齐并在真实受限身份下处理、观察及备份。
- 任务索引轮询漏掉 files 来源：加入常规 Tick，验证精确任务汇总恢复。
- complete 响应丢失后页面不能重放、已停新增上传时不能续传：保留请求身份与原分片政策，恢复同一会话。
- 附件改名入口动作枚举不匹配、改名完成前替换可能取旧修订：修正动作映射并等待元数据刷新。
- 统一导出按钮丢失原文件名可访问标识：保留文件名作为下载链接标识。
- 大屏 HTTP 环境不支持 crypto.randomUUID：复用 getRandomValues 生成标准 UUID，保留非安全上下文上传能力。
- 默认开发启动在未配置 S3 时会被文件 worker 连带中止：仅开发启动传入 `--allow-unconfigured`，不建立虚假健康心跳、不领取任务，独立 worker 保持严格校验。

最终人工接受仅由用户确认，不由自动测试代替。浏览器空间/账号导航等待最终业务内容后再断言；响应式截图已排除加载态。专业回归中的可选旧派生 CLI 场景未启用，它不涉及本次文件适配；文件 CLI 状态入口已实际运行，维护/恢复服务使用真实数据库和对象存储验证。

中途失败日志保留用于追溯：生产者失败注入夹具已修正且领域全量复验通过；任务来源遗漏已修复并复验；旧权限目录计数已更新并复验。并发启动大量测试数据库曾触发 PostgreSQL 共享内存不足，减少并行后实际 SIGKILL 场景通过。已有的本机服务未停止或重建。

## 2026-11-15：方案 A 正式页面改造

本节仅记录文件工作区与资料页附件入口的本轮 UI 改造。上述 2026-11-14 后端、恢复、专业模块检查是历史验证记录，并非本次重复执行的结果。

- 列表采用表格，附件、添加和上传记录使用共用 `ResponsivePanel` / `Drawer` 与遮罩；共用面板新增可选固定底部操作区。
- 初轮概览、编辑、替换、版本记录、还原、回收/恢复和技术信息分开显示；其后按用户要求，预览与版本记录已合并为一个阅读界面，版本选择直接位于预览上方，追加验证见[文档预览自验](document-preview.md)。文件摘要、内部编号、任务与审计仍保留在技术信息中。服务端名称/日期筛选、游标分页与租户/项目权限边界继续使用原协议。
- 类型检查、Console 生产构建、定向 ESLint 通过；6 项文件客户端测试通过；当前人工验收环境完成 5 项只读浏览器检查，无业务数据修改及浏览器运行时异常。
- 隔离真实服务浏览器最终复验 21 条旅程全部通过，无浏览器运行时异常。覆盖真实上传、下载摘要、缺片续传与回执丢失、编辑/替换/历史还原/回收恢复、CAS 冲突与重新确认绑定、取消、任务/通知、筛选/跨页历史、键盘/响应式以及四种权限组合。机器结果在 `.artifacts/files/browser/result.json`，日志在 `.artifacts/files/ui-redesign/browser.log`。隔离服务已关闭，原有人工验收环境保留运行。

本轮修正了菜单打开 Drawer 及抽屉内部切换表单时的焦点交接，Esc 优先关闭菜单，续传成功刷新时保持提交回执，以及已提交会话刷新后仍可重放丢失回执。元数据表单保留打开时的 CAS，历史直达链接能够继续读取游标定位旧版本。

UI 截图位于 `.artifacts/files/ui-redesign/` 和 `.artifacts/files/browser/`。375px 与深色模式已目视检查；200% 自动检查使用 720×500 CSS 视口模拟 1440×1000 窗口缩放后的布局空间，真实浏览器菜单缩放由人工验收复核。图片沿用安全预览，其他附件格式沿用下载，没有增加格式能力。

本轮状态为 Awaiting acceptance。实施和验收入口见 [正式改造记录](../development/files-workspace-redesign.md) 与 [人工验收手册](../operations/files-and-attachments-acceptance.md)。用户确认前不记为 Accepted。
