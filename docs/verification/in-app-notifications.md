# 站内通知实施、自验与人工验收

2026-11-15 的 A + C 通知界面改造单独记录在[通知界面改造验收](notification-ui-refresh.md)，本文件保留原通知业务能力的验证证据。

状态：**Awaiting acceptance**。完整实现及必要功能自验已通过；人工验收尚未进行。范围为 [NT-01–NT-08 和 AC-01–AC-16](../requirements/in-app-notifications-proposal.md)，不改变其他业务的人工验收状态。实现使用当前 main checkout，没有创建分支或工作树，没有提交、推送或部署生产。

## 交付核对

| 范围 | 实现与验证入口 |
| --- | --- |
| NT-01 事实目录 | `notificationevent` 版本化白名单、来源键指纹、原事务 AppendTx；迁移 36、协议版本 1。事务失败／同键冲突测试。 |
| NT-02 可靠投递 | 原接收集及成员身份快照、分批展开、短租约/fence、幂等入箱、持久退避与重放；真实 PG/race、迟提交及过期租约测试。 |
| NT-03 个人中心 | 账号级 Provider、铃铛、分页／筛选、详情、单条状态、接收水位全部已读；HTTP、客户端及真实浏览器。 |
| NT-04 接收与偏好 | 安全／本人资格／直接邀请必收；任务与业务更新可关闭；抑制有原因且不补历史。 |
| NT-05 全业务接入 | 身份、组织与角色、项目资源／设备／数据／空间授权、监测、派生分析、空间和大屏。下面按来源列出实际终态出口。 |
| NT-06 安全跳转 | 当前业务 metadata Adapter、结构化白名单目标、原模块鉴权；邀请按本人和 ID 接受；不保存邀请或下载能力。 |
| NT-07 隔离与故障 | 个人脱敏／范围隐藏统一用于列表、计数、详情、筛选、已读和目标；ReadSubject 拒绝；CLI 失败、心跳和重放。 |
| NT-08 运行材料 | PG-only 独立 worker、开发／Compose／systemd、迁移与备份恢复、两账号两租户／项目可停启演示。 |

### 真实来源提交点

- 身份：修改／重置密码、账号启停、撤销其他／全部会话、开户邀请使用，均在原 repository/access 事务记录，不以通用审计读取代替通知来源。普通登录／退出不通知。
- 组织：租户邀请创建／替换／撤销／到期／接受、成员加入／暂停／恢复／移除、平台／租户／项目角色及有效权限变化、管理员交接、租户启停、项目归档／恢复。个人 ID 邀请与原分享令牌入口共用业务规则，旧已接受邀请历史快照继续保留；通知展示另外复验当前资格。
- 授权：resources、devices、monitoring、spatial 在一次原授权命令中生成项目管理摘要，按发生时接收集固定，不按对象逐条广播。
- 监测与派生：`FinalizeTaskTx`／`TaskLease.CompleteTx` 收口正式成功；`RecordTaskOutcomeTx` 覆盖取消、过期租约和最终失败。实际采用、原始导出、成果导出、派生发布、试算、分析、比较、回放成功事务同时写通知。无人员内部任务不入箱；手工重试增加通知代次。CSV 校验有错误时为需要处理，不误报“可采用”；校验成功与正式采用不同。
- 自动派生：按租户、通道、配置／控制／委托代次和故障类别持久归并；重复失败增加计数，旧代次、暂停、取消不产生恢复。相关实际 control 回归验证失败／恢复归并。
- 空间：真实上传、转换封存成功／失败／取消及发布／撤回、授权接入。撤回／退役面向项目的消息落到仍可解释当前授权的原入口，不暴露已失权版本。
- 大屏：管理命令、上传 ready/failed、第五次租约到期回收、发布／回退／撤回、实例状态／读取授权、终端撤销、已发布包 missing／恢复写域内交接。受限包处理角色只访问 display 表。桥接核验原事件、范围及上传人归属后，事实和转译收据原子提交。运行查询、心跳、SDK ack 不转成人员结果通知。

## 检查命令和证据

以下检查已通过。最终浏览器实测闲置／展开摘要间隔为 15,303 / 5,310 ms，真实断网重连、隐藏暂停、聚焦刷新、故障恢复和同一浏览器账号切换通过；截图另外在数据加载及动画稳定后复核。

所有数据使用隔离 schema、合成账号和文件；PG、Redis、ClickHouse、对象存储为本机测试实例。测试脚本负责独立命名和清理，不向真实项目生成通知。证据日志与浏览器图像保存在 `.artifacts/notifications*`，不作为永久源码提交。

| 检查 | 证据及验证内容 |
| --- | --- |
| `pnpm check` | `.artifacts/notifications-check.log`；gofmt、go vet、ESLint、TS、OpenAPI 与权限目录一致性。 |
| `pnpm build` | `.artifacts/notifications-build.log`；全部 Go module/CLI/worker 与 console/display-runtime 生产构建。 |
| `pnpm test:notifications` | `.artifacts/notifications-database.log`；真实 PG/race 事务、重复／并发 worker、资格、序号水位、偏好、租约、毒事件、重放和 HTTP。 |
| `pnpm test:notifications:client` | `.artifacts/notifications-client.log`；身份切换迟到响应与 Abort、CSRF/no-store、水位、故障不变成 0。 |
| `pnpm test:notifications:restore` | `.artifacts/notifications-restore.log`；从迁移 35 升级、旧账号不变／不补历史、pg_dump 恢复到独立测试库，逐表状态哈希一致，恢复后待投递只入箱一次。 |
| `pnpm test:notifications:domains` | `.artifacts/notifications-domain-regression.log`；真实监测／派生／分析／空间终态断言，大屏受限身份、超限失败、迟提交及毒交接重放。 |
| `pnpm test:display` | `.artifacts/notifications-display-regression.log`；SDK/配置/包测试、真实 PG/race 大屏生命周期、权限、缺包修复、保留、只读主体及 HTTP 运行边界。 |
| `pnpm notifications:browser` | `.artifacts/notifications-browser.log`、`.artifacts/notifications/browser/result.json` 和同目录截图；真实独立 worker、真实业务 API、浏览器交互、故障和响应式。 |
| `pnpm display:browser` | `.artifacts/notifications-display-browser.log`、`.artifacts/display/browser/result.json`；受影响的大屏上传／发布／HTTP 三 origin、SDK 导航、终端、撤权、重连和移动管理链路。 |
| 既有身份／授权／组织回归 | `.artifacts/notifications-identity-regression.log`、`notifications-identity-recheck.log`、`notifications-tenancy-recheck.log`；access、projects、derivedmonitoring、monitoring 原回归，修复后重跑邀请历史保留用例。 |
| 最终详情与界面核对 | `.artifacts/notifications-detail-check.log`、`notifications-visual-check.log`、`notifications-final-ui-build.log`、`notifications-final-ui-lint.log`；详情直接切换独立已读、延迟状态响应不会覆盖新详情；成功／失败摘要、加载稳定后的桌面／手机／200% 字体与可滚动面板。 |
| 取消原因与真实健康诊断 | `.artifacts/notifications-cancellation-recheck.log`、`notifications-cli-health.json`、`notifications-cli-status.json`；监测／空间实际取消有原因码，运行中独立 worker 心跳、无积压和无失败。 |
| 自动派生故障周期 | `.artifacts/notifications-derived-condition.log`；实际持久控制与执行代次、重复故障、暂停、新委托、旧代次恢复拒绝。 |

首次联合检查暴露并已修正的问题：原已接受租户邀请历史快照被通知展示规则误伤；通知中心刷新重试只刷新列表而未同时恢复铃铛；隐藏／恢复后网络重连未稳定触发立即刷新，已在账号 Provider 增加随账号清理的统一 online 刷新；新路由 Hook 命名的 lint 错误。身份原 activity 时间断言曾在高并发回归中一次失败，原用例独立重跑通过，未修改该业务逻辑。大屏浏览器前两次运行分别在新页面身份／项目上下文等待时超时，最终完整链路重跑通过，未因此改变原运行语义。浏览器字体检查等待字号动画稳定后再测移动布局，避免把过渡帧当成最终页面。详情直接切换补充按通知 ID 独立已读及状态响应原查询归属，延迟响应回归通过。

### 验收项与证据对应

| 编号 | 本次自验 |
| --- | --- |
| AC-01 | 两账号两租户／项目浏览器；HTTP 跨账号列表、详情、计数、目标和状态 404；同账号跨工作空间统一入口。 |
| AC-02 | AppendTx 业务回滚／来源冲突；真实创建租户在协议不匹配时整体回滚；监测提交失败与恢复。 |
| AC-03 | 真实并发 worker、崩溃租约 fence；浏览器独立进程停启，任务完成后仅补一次。 |
| AC-04 | 通知迟提交和 Batch=1；大屏较小事件 ID 晚提交、重复桥接、未转译记录保留及独立失败重放。 |
| AC-05 | 个人 CAS、重复同状态、归档／恢复、接收水位及迟到旧事件；浏览器详情和筛选全部已读。 |
| AC-06 | 关闭任务类别仍完成业务；抑制持久、不因重开补发；必收配置不暴露可关闭项。 |
| AC-07 | 实际创建／读取／接受／撤权；跨账号 404；幂等到期维护无需先打开邀请；原历史快照回归。 |
| AC-08 | 既有角色、成员、生命周期、项目归档回归；通知成员身份快照和当前资格裁剪；撤权后来源名称／筛选消失。 |
| AC-09 | 真实 CSV、采用、Parquet 导出、提交故障／过期 worker／无效整批；所有终态按实际代次断言。 |
| AC-10 | 正式派生、试算、分析多输出、比较、依据回放、成果导出及自动故障周期；原固定结果事务自验。 |
| AC-11 | 真实 GLB/IFC/PDF 等管线、封存／失败／取消、项目边界和原目标；转换不等于发布。 |
| AC-12 | 真实包处理、受限角色、第五次租约到期、发布／回退／撤回、缺包／恢复；域内交接和个人事件一致。 |
| AC-13 | ReadSubject 在通知服务入口被拒绝，运行 HTTP 仍隔离，SDK 无私人通知 capability，原订阅确认回归。 |
| AC-14 | 原任务／文件当前资格和失效目标、通知脱敏；客户端取消迟到响应，浏览器同页退出切换账号。 |
| AC-15 | 桌面、390px 手机、200% 字体、44px 铃铛、Enter/Escape/焦点返回；15s/5s、隐藏暂停、聚焦／网络恢复、刷新 503。 |
| AC-16 | 35→36 升级、独立实际备份恢复及阅读状态哈希；失败事件与交接重放，已送达不改状态。 |

界面证据：[桌面](../../.artifacts/notifications/browser/desktop.png)、[手机](../../.artifacts/notifications/browser/mobile.png)、[手机通知面板](../../.artifacts/notifications/browser/mobile-panel.png)、[200% 字体](../../.artifacts/notifications/browser/font-200.png)、[刷新错误](../../.artifacts/notifications/browser/refresh-error.png)。

## 人工验收与实际边界

按[运行手册的合成环境步骤](../operations/in-app-notifications.md#独立合成人工验收环境)验收；用户明确确认后才将状态改为 Accepted。演示数据、账号、启动和停止方法均已提供，不要求用户先准备图模或实际项目资料。

本次未执行生产部署／生产数据库迁移、Safari 或物理手机验证、目标规模的性能压力与长期观察。构建中的已有模型／图表大包警告单独保留，不作为吞吐容量证明。浏览器约定的 15 秒／5 秒是刷新行为验证，不承诺生产实时推送延迟。现有业务的存储恢复、文件下载和各自人工验收仍以原模块记录为准。
