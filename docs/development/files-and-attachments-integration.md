# 文件与附件接入约定

普通附件由 `packages/go/platform/files` 管理。当前只登记 `tenant.profile`、`project.profile`，宿主解析器在 `runtime/files_hosts.go`，不能从调用者传入任意表名、URL、对象键或人员身份。接入一个新宿主必须修改有限注册表、授权解析、生命周期规则和真实事务回归。

## 人员普通附件

- R = 宿主 profile.read + attachments.read；D 另需 attachments.download；M 另需 attachments.manage + profile.update + 宿主可写。R+M 不隐含 D。
- scope、host、attachment、revision、upload、job 都由服务核对；平台治理与 ReadSubject 不进入人员文件入口。归档项目保留获准读取，关闭所有附件修改。
- 客户端先读 capabilities。写请求带 `X-Files-Epoch`、稳定 UUID request_id 和原因；同一意图重试必须保持相同载荷。冲突或新意图要重新取得版本后产生新请求号。
- 分片传输声明实际长度、SHA-256 和恢复 epoch；8 MiB 一片、最后一片精确长度。完整文件 SHA-256 在浏览器 Worker 增量计算。响应不确定时查请求回执/上传收据，不能根据进度条推断已接收。
- complete 返回 202 表示已接受后台任务。文件检查成功、文件就绪、附件绑定、当前下载可用性是独立事实；CAS 竞争进入 needs_action，保留 24 小时后需重新上传。
- 修改名称/说明不改正文。替换与历史还原创建不可变修订；所有正式历史引用保护正文。回收/恢复要求附件控制 revision，保留期按回收时策略确定。

API 契约见 `contracts/openapi/api.yaml` 的 Files 操作和 `packages/ts/api-client/src/files.ts`。`pnpm files:contracts` 从 Go DTO 与已登记路由生成文件部分，再生成 TypeScript schema；生成文件不得手工补字段。列表游标绑定空间、筛选、授权和内容版本，过期后从首页重新查询。

```ts
const caps = await client.fileCapabilities({ tenant: tenantId });
// 创建、传片和 complete 由 ordinaryAdapter + transferFile 负责。
// 普通正文使用实际业务路径，不返回对象地址或签名 URL。
const href = client.fileDownloadURL(
  { tenant: tenantId }, attachment.id, attachment.current_revision.id,
);
```

下载全部经过 `GuardedRead`：先核对完整实际字节，之后才允许响应头；每 64 KiB 在当前身份/授权事务中放行，网络写入 2 秒截止。GET、HEAD、Range 和条件响应共用边界。普通文档正文仍以 attachment 响应；2026-11-15 增加的 `client.fileContent(scope, attachment, revision, signal)` 读取同一路由的 Blob，交给本地 PDF/DOCX/XLSX/CSV/TXT 阅读器。图片预览继续来自隔离检查器重新编码的正文。`preview_state` 和 `/preview` 专指服务器图片产物，不代表浏览器文档阅读能力。正文查询 `gcTime: 0`、不持久缓存；空间边界卸载会 Abort 请求、终止 Worker、销毁 PDF/Word 阅读器并撤销图片 blob URL。账号/授权/空间/附件/指定修订均参与缓存隔离。

## 专业文件的公共界面

`packages/ts/ui/patterns/files` 只包含纯展示组件和有限 `DomainFileRef`/`ManagedFileRef` 类型，不持有 API、权限判断、路由或任意 URL。console 的 DomainFiles 由原模块提供授权后的 descriptor 和原生 action；不可用状态不产生下载动作。descriptor 不是授权令牌，也不能提交到普通附件接口。专业长度未知时不伪造 ZIP 长度。

公共传输控制器在 `apps/console/src/features/files/upload-controller.ts`，四个 Adapter 分别保持原生约束：

| 入口 | 分片 | 上限 | 原生后续流程 |
| --- | --- | --- | --- |
| 普通附件 | 8 MiB、1 起编号 | 100 MiB | files.prepare → 附件绑定 |
| 监测历史 CSV/XLSX | 32 MiB、1 起编号 | CSV 32 GiB / XLSX 512 MiB | 映射 → 校验 → 采用 |
| 空间原件 | 8 MiB、1 起编号 | 256 MiB | 转换 → 草稿 → 版本发布 |
| 大屏 ZIP | 4 MiB、0 起编号 | 128 MiB | 包检查 → 配置/发布 → 隔离运行 |

监测导出、派生结果、分析/比较成果、输入依据的分卷、格式、有效期与原生下载维持原语义。空间来源与草稿文件使用公共展示，项目图模展示授权不授予来源下载。大屏包不增加正文下载入口。空间接口未提供逐片摘要列表时，Adapter 重发原生幂等片；不把缺失列表推断为已收齐。

## 任务、通知与引用

`files.prepare` 使用稳定 source_ref；自动恢复仅推进 attempt/fence，手工 retry/reconfirm 追加 generation，旧输入与报告不变。管理员代取消只改变事件 actor，不改变该执行 requested_by/executor。`taskrecord.Change.EventActor` 带 omitempty，旧快照编码与指纹保持原样。

TaskSource 的目标是有限 `file_upload`、`file_attachment`、`file_host`，固定代次结果由可选 `ResultForExecutionTx` 提供。通知是同事务的 `file_job` + task reference，唯一键包含 job/generation/terminal。只保留原始发起人的精确成员关系，当前权限再裁剪；不广播全部管理员。停止投递或关闭偏好不阻断业务提交。

附件绑定、任务事实、通知生产事实在一个 PostgreSQL 事务中。S3 I/O 在事务外，先登记独立 intent，再写唯一对象，确认实际摘要；失败对象依然可核对。发布动作禁止从旧 fence 或旧 epoch 提交。

`ProtectTx`/`ReleaseTx` 只允许当前真实登记的文件修订或预备输入 owner，并与回收锁同一 file_record 行。本期无外部 consumer 注册；任意模块名/owner 被拒绝。未来增加 consumer 必须实现 owner 存在性、权限、终结与备份恢复语义，不能仅插一条永久保护记录。

数据库锁顺序遵循 protocol → authorization → instance budget（需要预算时）→ account → host → space → file/job。暂存预算的外键账号锁也在 budget 后；worker 在空间锁之前取得账号键锁。禁止在对象 I/O 或内容解码期间持有业务事务。
