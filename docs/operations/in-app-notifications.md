# 站内通知运行与人工验收

2026-11-15 通知界面采用 A 轻量浮层 + C 三栏收件箱；新版界面、自验结果和验收步骤见[通知界面改造验收](../verification/notification-ui-refresh.md)。现有业务规则、账号和 API 保持不变。

实现范围和规则见[主需求](../requirements.md#in-app-notifications)，自验与人工验收状态见[验证记录](../verification/in-app-notifications.md)。人员通知只归属登录账号；管理员没有读取他人通知正文的默认接口。

## 启动和升级

本地已有开发环境运行 `pnpm dev`，会同时启动独立 `notification-worker`。`pnpm dev --no-notifications` 可以停用本次启动中的通知进程；业务仍写入待投递事实。只运行通知进程时，配置受保护的 `INFRAPRO_DATABASE_URL` 或 `INFRAPRO_CONFIG` 后执行：

```bash
go run ./services/api/cmd/notification-worker
pnpm infrapro notifications check
pnpm infrapro notifications check --worker --max-age 1m
pnpm infrapro notifications status
```

worker 仅打开 PostgreSQL 连接。其 metadata Adapter 不启动 Redis、ClickHouse、S3、空间转换器或大屏运行壳。API 的原依赖保持原业务要求；运行通知不代表这些业务依赖已经恢复。默认每批 32 个接收人／投递、租约 30 秒、轮询 1 秒、最多 8 次尝试、指数退避上限 5 分钟。参数见[配置示例](../../deploy/notifications/worker.example.yaml)；不合理参数会阻止启动。

现有库升级顺序：

1. 核对当前版本，完成可恢复的协调数据库备份。迁移 34/35 的空间和大屏内容按其原部署流程准备；不得覆盖已应用迁移或对既有库重复 bootstrap。
2. 暂停旧版本 API 和业务 worker 写入，运行 `pnpm infrapro database migrate`、`pnpm infrapro access sync`、`pnpm infrapro access check`。通知增量是 **00036_notifications.sql**，结构初始为空，保留所有既有业务数据。
3. 部署同一版本的 API、监测／空间／大屏包处理进程与通知进程。大屏包处理专用数据库角色需同步其[最小权限脚本](../../deploy/display/package-worker-role.sql)，仅新增 display 自有交接表权限。
4. 启动 notification-worker，检查协议、心跳、失败量和积压，再启用对应前端。API／worker 检查迁移版本，事实写入检查 notification_protocol=1；版本错误不能降级为静默丢通知。

[Compose](../../deploy/compose/compose.yaml) 已增加独立服务，API 镜像包含通知二进制；[systemd 示例](../../deploy/notifications/notification-worker.service) 从受保护配置文件读取数据库连接。进程被停止后只延迟投递，个人阅读和归档仍可用；恢复时领取待处理事实，不再次执行来源业务。

## 诊断、暂停与重放

`notifications check` 检查数据库通知协议；附加 `--worker` 检查最近心跳，过期或缺失时非零退出。默认容许 1 分钟，可按部署轮询与监控策略调整 `--max-age`。心跳不能代替积压和失败检查。

`notifications status` 输出通知待投递数（含未展开接收集）、失败量、抑制量、最长积压秒数、最近心跳、空接收集数量及原因码。`display_handoffs` 单独报告待转译／失败量、年龄和交接 ID。失败列表最多 50 条，不输出通知正文、姓名、邀请令牌、下载地址或终端凭据。

```bash
pnpm infrapro notifications inspect --event <事件UUID>
pnpm infrapro notifications replay --event <事件UUID> --reason '依赖已修复，重放失败投递'
pnpm infrapro notifications replay --handoff <交接UUID> --reason '修复大屏交接后重新转译'
```

事件重放只重置失败投递，保留来源键、原接收集和已入箱记录，不改变已读／归档，不补发已按偏好或资格抑制的通知。大屏交接失败需先修复实际原因，再按交接 ID 重试；正常记录不需要重置。两种重放均单独审计，不能指定任意接收账号或通知文本。

| 现象／原因码 | 处理 |
| --- | --- |
| 心跳缺失、积压增加 | 检查进程日志、数据库连接和迁移／权限目录版本；恢复 worker 后观察积压下降。 |
| preference_disabled | 用户关闭普通任务或更新类别，属于有原因的完成，不重放。 |
| qualification_lost | 原候选已失去来源资格，属于有原因的完成，不能换成当前所有管理员补发。 |
| no_qualified_recipients | 事实当时没有合资格接收人；检查领域委托／管理关系，不作全平台广播。 |
| delivery_dependency_failed／qualification_unavailable | 检查数据库及 metadata Adapter；自动退避达到上限后人工修复与重放。 |
| invalid_event_template | 核对生产者和消费者发布版本，不把未知事件当成功投递。 |
| translation_failed | 根据交接 ID 核查原 display 事件、包任务及人员归属；失败项不会阻塞其他记录。 |

停止服务管理器中的 notification-worker 即可暂停全部通知投递及通知维护 Adapter；业务原 worker 不受此动作影响。邀请过期业务状态由 tenancy 的幂等维护入口写入，恢复通知进程会继续推进。大屏未转译和失败交接不随诊断 EventRetention 删除，个人通知没有自动删除期限。应监测通知表及其索引大小、最长积压和归档比例；后续若增加自动保留规则需另定业务要求，不直接清空历史或唯一键。

## 备份和恢复

使用原数据库协调备份，必须包含业务事实、notification_protocol、notification_events、notification_recipients、notification_deliveries、notification_inbox_states、account_notifications、notification_preferences、notification_conditions、notification_worker_heartbeats 和 display_notification_handoffs。同时保留 display_events、原任务和邀请等业务表、关系约束和来源唯一键。空间／监测／大屏文件按各自恢复手册处理，通知备份不替代成果文件备份。

恢复时先停 API 与全部来源／通知 worker，恢复到同一个时间点，确认扩展、约束和迁移版本（原库使用 `btree_gist` 等扩展），再按原身份恢复流程失效旧访问、重新认证。先恢复业务依赖，最后启动通知 worker 和前端；过期租约会重新领取，已入箱唯一约束阻止重复投递。阅读状态回到备份时刻，不能保证备份之后的已读状态仍存在。不要删除来源键、接收序号或转译收据后重放。

迁移 36 的 Down 明确拒绝执行。需要回退时先停新写入，保留通知表，使用已验证的协调备份与匹配版本恢复，不通过降低迁移标记伪装旧结构。本轮默认不补发旧审计、运行日志或历史任务；升级后由新代码实际结束的任务正常形成新结果通知，旧待接受邀请仍在个人邀请入口可查询。

## 独立合成人工验收环境

```bash
pnpm notifications:demo
pnpm notifications:demo --status
pnpm notifications:demo --pause
pnpm notifications:demo --resume
```

入口：[通知中心](http://127.0.0.1:15590/notifications)。账号 `e2e_owner` 与 `notification_member`，两者的**本地合成验收密码**为 `Real browser 河流 secret 592`。每次启动创建新的隔离 schema、监测测试命名空间、两租户及项目、一个待接受邀请、真实 CSV 校验成功任务、一个 ZIP 就绪与一个 ZIP 失败的大屏实例；不使用真实项目数据。前台运行时 Ctrl+C 停止，并清理本次隔离数据，重新执行即可重建。端口 15590/18590/15890/15891 若已占用则拒绝启动，不关闭既有服务。

环境的数据库／存储仅使用本机测试依赖（PG 55432、Redis 56379、监测契约测试服务）。脚本需要 Docker、项目依赖和现有 Go 工具链；首次生成真实包样本由脚本完成。`.artifacts/notifications/lab/operator.json` 是权限受限的临时进程配置，含连接信息，不分享该文件或其内容。`acceptance.json` 仅保存合成对象 ID，方便定位。

### 最短验收步骤

1. 用 owner 登录，查看通知中心、铃铛、当前工作空间筛选、接收设置。账号首页与两个租户／项目共用同一个箱；打开列表不自动已读。
2. 打开“任务结果已更新”，应看到 CSV 校验完成；点“打开业务页面”进入该批次，校验没有自动采用。大屏成功／失败消息进入同一大屏实例的原管理页。
3. 进入通知详情后刷新，应保持已读；恢复未读、归档、取消归档，另一个登录同账号的窗口应看到一致结果。“当前筛选全部已读”只影响该筛选及已收到的水位。
4. 在另一个浏览器上下文用 member 登录，从“收到的租户邀请”查看待接受邀请，阅读不加入租户；明确“接受邀请并加入租户”后才能进入。owner 从原成员管理移除该成员，member 的个人回执保留，但工作空间／对象名称与目标入口消失。
5. 执行 `--pause`，在租户监测导入页上传一份 CSV 并启动校验，立即去账号首页。批次最终结果可在原业务页确认，通知暂不增加；`--resume` 后补发一次。闲置页面摘要约 15 秒刷新，面板／中心约 5 秒；重新聚焦／网络恢复立即刷新。
6. 关闭普通任务接收，再完成一次任务，应无新任务通知且业务仍完成；安全／本人资格通知不能关闭。重新开启不补发期间抑制的消息。
7. 同一浏览器退出 owner 后登录 member，不应看到旧列表、计数或目标。手机宽度、键盘 Enter／Escape、200% 字体下入口和正文可用；断开 API 时明确提示暂未更新，不显示伪造的 0。

步骤 5 可使用以下文件，映射到合成设施下的“通知验收位移”通道、UTC、单通道 CSV：

```csv
observed_at,value,quality
2024-02-01T00:00:00Z,16,good
```

完整可重复自验命令与证据见[验证记录](../verification/in-app-notifications.md)。本机功能自验不代表生产部署、容量或用户人工接受。
