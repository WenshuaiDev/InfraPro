# 审计来源接入契约

适用范围：AM-01–AM-14；实现与自验状态见[本机工程环境与人工验收](../operations/local-environment.md)。事件、生产函数与真实回归入口见[目录](audit-event-catalog.md)。

## 写入与事务拥有者

来源 Module 持有业务事务，在真实副作用出口调用 `auditrecord.AppendTx` 或既有 `access.Audit`／`devicekit.AuditSnapshots` 兼容封装。不得在 HTTP handler 成功响应后补写成功事实，不得为任务中心控制再写一条来源成功记录。

事实身份是 `(source_module, source_key, effect_key)`。同键同规范化内容返回原 event_id，内容冲突使整个事务回滚；operation_id 来自真实命令收据或同一数据库事务。一次接受邀请包含邀请接受与成员加入两个效果，不能用去重丢掉其中一个。parent_operation_id 只接受已存在的明确上游，最长 32 层；最多 32 个有限类型关联，不以同名、相邻时间或客户端请求号推断因果。

普通管理操作使用历史 actor；后台执行分别记录原始 initiator、本代 requested_by 和 executor。自动尝试不变 generation，人工 retry 必须新建 generation。领域终态通过 taskrecord 保存固定来源事实；通知包装只附加 notification_event 关联，不改变原 source_key、指纹或接收人。

事件必须登记于 `auditrecord/catalog.go` 或 `domain_catalog.go`。允许字段为有限白名单；密码、令牌、凭据 URL、正文、任意 storage key 和任意 HTML 不进入公共事实。变更只记录实际发生的 before/after。无法获得历史状态应标记 not_recorded，不能查询当前对象后伪造当时快照。null、空数组和缺失字段不同，超长集合／文本有截断状态；截断摘要保留安全内容的摘要指纹，避免不同尾部被当成同一事实。

writer 严格限制 reason 为 500 字。已有原生领域协议允许更长原因时，兼容入口保留原业务收据协议，将审计原因显式标记截断并保存安全文本摘要；新审计命令不得绕过 500 字校验。

## 命令与失败

原生 request_id／client_request_id 协议不变。新增 X-Command-ID 的入口由 `access.CommandOperations()` 唯一列举，OpenAPI、HTTP 校验及客户端共用它。缺失返回 428；同一键异载荷冲突，tombstone 不允许重执行业务。客户端在网络故障、服务端故障或无法解析的成功响应后保留原键；只在明确完成或确定性拒绝后释放。

提交确认丢失时，用新连接查询原收据。没有证据时结果为 unknown，不能记成确定失败然后换键再改业务。失败事实在原事务回滚后独立、最多 2 秒写入，只有最小动作／错误码和平台安全范围，不复制未经授权的对象名称或半提交差异；数据库本身不可用时只记录有限安全诊断。

## 来源授权与目标解析

Reader 先核实当前账号、精确成员关系、空间、read 权限和恢复协议。业务 Adapter 独立核实其元数据权限和项目授权集合。技术权限只增加有限诊断字段，不增加业务内容。名称来源明确标记 snapshot／current／not_recorded／redacted。

来源 Adapter 必须使用传入事务、保持无网络正文读取、最多 100 个引用／3 秒；权限不足、对象历史不可用和依赖故障不能混成同一个成功空结果。查询在裁剪之后搜索、计数和分页；超过候选预算返回明确错误，不返回“完整”的部分结果。

关联目标必须由服务端核验归属，返回有限 kind／ID／revision／generation／parameters；前端 `targets.ts` 只映射登记路由，不接受任意 URL。设备输出／安装返回已核验的父设备，资源通道返回设施与测点，大屏包返回实例，附件返回固定修订。task 进入固定执行代次，不跳到“最新”。

项目归档允许既有资格的审计读取／导出；禁用租户、旧成员关系和 ReadSubject 不能借审计访问。只拥有审计摘要的人员无法取回被收回的文件名、资源字段、坐标和原始差异。

## 敏感传输

files、monitoring、derived、spatial、audit 的敏感读取在授权响应前建立持久传输 receipt，结束后记录实际 outcome 和可信发送字节数。Range 每次建立独立传输；HEAD／304 不表示正文成功。来源保留自身逐块授权，不能为了公共审计移除原下载门禁。

进程死亡或 Close 持久化失败保留未结束 receipt；20 分钟后有界核对为 interrupted_unknown，sent_bytes 为空。维护不会补造成功或“发送 0 字节”。普通页面浏览、监测数据逐行结果、分片上传进度和每次块授权不新增公共业务事实。

## 任务、通知和受限进程

只有 audit.export 可以拥有 platform scope，其余任务拒绝伪造 platform。任务中心调用来源 Commands，同一原生命令去重、同一业务事实。来源停机显示暂不可核实，恢复后继续解析；不能把无权或停机当成已删除。

审计导出终态生成 `task.result` 通知，source_key 固定为 export:id:generation:n:terminal，只给原所有人。偏好关闭、通知 worker 停机只影响投递，持久化通知事实失败仍必须回滚终态事务。审计读者只见受控投递状态，技术档可见有限数量／错误码，不能看到他人的正文、身份、已读状态和偏好。

受限大屏处理进程由触发器把 job/generation/fence 的安全状态写入域内 handoff。可信 worker 校验原归属后翻译；每条用 savepoint，失败次数持久递增，最多 8 次自动翻译。维护只对确切 handoff 和已观察 attempts 重放，已有包处理结果不变。SQL 角色不给包处理进程公共账号／审计／收件箱权限。

## 查询、导出与恢复

导出先固化当前资格和单次 MVCC 视图，再保存物化行与摘要；同代恢复不重新查询。写 S3 前登记 intent，发布时复验当前授权及 fence；身份、成员关系、业务授权或投影版本变化使旧成果永久失效，恢复权限后必须重新生成。

SQL 角色分别为 writer、api、worker、observer、maintenance、content、restore；具体授权见迁移 43。worker 只取得有限资格／目标元数据和必要协调行锁，不获得文件对象、空间 blob、密码、会话或个人通知正文。新增 Adapter 后必须用实际受限 worker 查询和生成 CSV，超级用户测试不替代此项。

业务历史与审计保留分开。身份 Cleanup 不再删除审计；新策略经 plan/apply 一次性导入实际部署期限。清理默认 plan_only，不存在 force 或任意事件删除。恢复必须冻结真实业务链，独立 pg_restore 与对象复制／校验，随后协调 audit/task/files epoch、旧 fence／lease 和中断事实，不补发旧通知。

## 接入检查

```sh
pnpm audit:contracts
pnpm check
pnpm test:audit
pnpm test:audit:restore
pnpm audit:browser
```

`audit-contract` 使用 Go AST 扫描 access.Audit、identity audit、devicekit Audit/Snapshots、Fact/LegacyInput 和域内 INSERT 日志路径，字面量未登记时检查失败；动态动作仍受 writer 的有限目录约束。真实来源回归必须保留原领域验收，并开启 `INFRAPRO_AUDIT_SOURCE_ASSERTIONS=1` 核验当前裁剪、明确关联及受限 worker CSV；`INFRAPRO_AUDIT_COVERAGE` 可输出只含事件类型／计数的隔离证据。
