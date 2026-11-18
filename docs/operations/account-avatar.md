# 账号头像运行与恢复

头像由 API 的账号服务处理，无需新增进程或浏览器依赖。数据库迁移为 **57**，API 和所有依赖平台数据库版本检查的后台进程应使用同一构建。右上角使用共享 UI 包内 shadcn/ui 的 Radix Avatar 与 DropdownMenu。

## 配置与发布

沿用 `INFRAPRO_FILES_OBJECT_ENDPOINT`、`INFRAPRO_FILES_ACCESS_KEY`、`INFRAPRO_FILES_SECRET_KEY`、`INFRAPRO_FILES_BUCKET`。头像对象放在这个桶的独立 `account-avatars/v1/` 前缀，账号服务自动配置现有 S3 适配器。存储未配置或不可用时，更换失败并保留旧头像；读取失败显示文字回退，不阻断账号登录。

升级步骤：构建管理端和 Go 服务，备份 PostgreSQL，停止旧应用进程，执行 `infrapro database migrate` 和 `infrapro access check`，启动同版服务。迁移不改原账号、密码、角色或现有附件；旧账号的头像引用为空、头像版本为 0。迁移不可逆，回退须恢复匹配版本的备份。

代理需允许 `PUT /api/v1/account/avatar` 的 5 MiB 正文；生成规则位于 `scripts/nginx-routes.mjs`，已同步 Docker、原生 HTTP、原生 HTTPS 三份 Nginx 模板。独立维护的代理配置也要同步该路由。浏览器裁剪后通常只传输一张小尺寸 PNG。

## 生命周期

- 原图只在浏览器中使用，不上传保留；服务器对上传像素再次校验并重编码为 256×256 PNG。
- 写入前登记 `account_avatar_objects`，成功后在同一数据库事务中切换 `accounts.avatar_object_id`、递增 `avatar_revision` 并记录审计。
- 未发布的对象一小时后进入回收，替换或移除后的旧对象保留 24 小时后回收。保留期间也不能通过图片接口访问旧版本。
- API 已有 `RunJobs` 每分钟尝试清理，按最多 20 条处理，失败继续保留删除意图以便重试。当前引用不会回收，不扫描并删除未知对象。
- 所有读取实时鉴权，返回 `private, no-store` 和 `nosniff`。前端只用内存 Blob URL，最后一个使用者卸载时撤销。

## 备份与恢复

头像备份需要**同一停写窗口内的 PostgreSQL 备份与文件桶中 `account-avatars/v1/` 的完整对象备份**。普通 `files backup` 只管理普通附件，不能单独用来备份账号头像。

1. 停止所有 API 实例，确保上传和 API 自带清理停止；执行数据库备份，并用部署环境现有 S3 备份工具复制整个头像前缀。记录备份时间、数据库版本与对象校验摘要。对象备份应覆盖该窗口内数据库中的当前引用和待回收记录。
2. 恢复时保持 API 停止，在隔离恢复环境导入匹配的数据库和对象，校验 `account_avatar_objects` 记录的 `bytes`、`sha256` 与实际 PNG。数据库引用与对象必须成套恢复。
3. 使用同版服务启动并执行数据库、权限就绪检查。通过本人、平台账号、租户成员、项目成员入口分别确认头像；缺失或损坏必须先恢复正确对象，不能把图片的文字回退视为恢复成功。
4. 确认恢复环境后再开放访问。若数据库备份与对象不匹配，应重新恢复匹配备份，禁止直接修改引用规避校验。

自动测试覆盖真实 S3 对象删除后恢复、摘要与尺寸核验、失效引用拒绝和回收重试；生产实例的备份恢复演练应在其维护窗口执行。

## 本机验收

入口：`http://localhost:25173/profile`。使用现有账号与密码。

2026-11-17 已在本机完成 v55 → v57 升级，包含当前代码基线的 v56 身份字段迁移；平台强制 MFA 保持关闭，原账号继续使用原密码。按基线要求在本机运行目录创建了私有身份密钥文件，权限为 0600。升级前数据库与原运行配置备份位于 `~/.local/share/infrapro/runtime/backups/20261117-before-account-avatar/`。头像验收没有为现有账号预设图片。

复跑检查：

```bash
node scripts/test-avatars.mjs
node scripts/avatar-browser-check.mjs
```

浏览器检查创建独立数据库 schema、独立 S3 桶和合成账号，结束后清理。截图及结果位于 `.artifacts/account-avatar/browser/`。本机进程管理仍用 `pnpm local:start`、`pnpm local:status` 和 `pnpm local:restart`。
