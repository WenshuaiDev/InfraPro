# 上传限额与代理部署

分片的大小与整文件的大小分别控制：Nginx 放行受支持的单次分片，API 按部署配置决定是否接受一个新文件。整文件限额增加时，不需要把 Nginx 请求体限额同步增加。

## 配置

在部署使用的 YAML 中合并以下字段，单位为字节。环境变量优先于 YAML。

| 文件类型 | YAML 字段 | 环境变量 | 默认整文件上限 | 分片上限 |
|---|---|---|---|---|
| 普通附件 | `files.max_file_bytes` | `INFRAPRO_FILES_MAX_FILE_BYTES` | 104857600（100 MiB） | 8 MiB |
| 空间原件 | `uploads.spatial_max_file_bytes` | `INFRAPRO_UPLOADS_SPATIAL_MAX_FILE_BYTES` | 268435456（256 MiB） | 8 MiB |
| 监测 CSV | `uploads.monitoring_csv_max_bytes` | `INFRAPRO_UPLOADS_MONITORING_CSV_MAX_BYTES` | 34359738368（32 GiB） | 32 MiB |
| 监测 XLSX | `uploads.monitoring_xlsx_max_bytes` | `INFRAPRO_UPLOADS_MONITORING_XLSX_MAX_BYTES` | 536870912（512 MiB） | 32 MiB |
| 大屏 ZIP | `uploads.display_max_package_bytes` | `INFRAPRO_UPLOADS_DISPLAY_MAX_PACKAGE_BYTES` | 134217728（128 MiB） | 4 MiB |

值必须是正整数；0、负数、分数和超出整数互操作范围的值启动失败。上限可以小于一个完整分片，也可以超过旧默认值。普通附件的分片大小仍由 `files.part_bytes` 指定且不得超过 8 MiB；分片总数必须能表示为 PostgreSQL integer。其余分片大小为固定协议值。

API、display-runtime 及有关 worker 使用同一套配置，调整后一起重启。浏览器每次开始专业上传前读取已登录接口 `/api/v1/auth/upload-limits`，普通附件读取空间 capabilities；大屏开发工具从已授权的运行接口读取限额。离线打包不固定服务器的 ZIP 大小配额。

新配置限制新建上传。已建立的会话继续以数据库中记录的文件长度、分片和摘要为准；调小上限不删除历史对象，不使已接收文件失去下载或恢复资格。原有文件格式、权限、SHA-256、空间预算、检查器时间/内存/像素、ZIP 展开大小及压缩比等处理规则仍有效。提高上传上限前应同时核对当前模块的暂存预算和实际转换能力；文件大小配置不代表对同等规模复杂模型或文档的性能保证。

## 数据库及代理升级

1. 本次新增 schema 55，解除普通附件、空间、大屏的旧固定总大小和总分片数约束，保留正数、单片大小、关系、不可变依据和摘要约束。先备份，使用本次发行的 `infrapro database migrate` 执行迁移；首装也包含此迁移。没有对象搬迁。已经接收超过旧上限的文件后，回滚到旧 schema 会明确失败，不能删除文件来强制降级。
2. 安装更新后的 API、文件检查器、相关 worker、display-runtime、前端、大屏开发工具和 Python 空间转换器；确保配置同步。仅替换 Nginx 不能启用整文件限额配置。
3. 按部署方式使用 `deploy/nginx/default.conf`、`deploy/display/nginx.conf` 或 `deploy/display/https.nginx.conf`。API 路由部分由 `scripts/nginx-routes.mjs` 统一生成，修改规则后运行生成器；`pnpm check` 检查模板是否同步。Docker 的基础模板仍通过现有 web 镜像安装。
4. 保留生产 HTTPS 的三个 origin、证书路径和后端地址。执行 `nginx -t`，再按主机服务流程启动/重载 Nginx。首期生产仍部署在 152；本轮自检不会安装生产服务。

上传路由分别设置 8/32/4 MiB 请求体限额、HTTP/1.1 流式传输和读写等待。Go API 的分片读取允许 2 分钟、响应窗口 3 分钟；普通接口继续使用原默认超时。空间封存代理等待 310 秒、监测封存 1810 秒，与后端 5/30 分钟处理窗口配合。Nginx 的读写超时是相邻读写的等待时间，不是整个传输的总耗时。

附件、专业文件下载及大屏事件流关闭响应缓冲与代理缓存。上传与较大 JSON 路由仍有独立上限；登录等默认 16 KiB，附件命令 64 KiB，监测/空间等 256 KiB，工程 272 KiB，分类 8 MiB，解析样本正文 6 MiB，大屏命令 1 MiB。Go handler 继续执行更细的内容规则。代理自己的 413/502/504 转为 JSON，领域错误保留原响应；HTTPS 运行域与资源域的凭据过滤继续有效。

## 自检与人工验收

运行 `pnpm test:uploads`：需要本机 Nginx/OpenSSL、Docker 测试依赖和 `.artifacts/spatial-venv`（缺少虚拟环境时先执行 `pnpm spatial:setup`）。它使用隔离的数据库 schema、对象前缀、临时证书与本机临时端口，执行代理边界检查、配置与数据库迁移测试、配置缩小后的续传检查以及四类真实上传流程。证据写入 `.artifacts/nginx-uploads/`；测试进程结束时清理其数据库和对象。

大文件联合恢复另运行 `INFRAPRO_UPLOAD_DEPLOYMENT=1 pnpm test:files:restore`，验证超过旧附件上限的对象可在较小的新上传配额下恢复。HTTPS 登录与大屏/工具回归运行 `pnpm test:https`。

人工验收：

1. 使用受信任 HTTPS 控制台，分别上传普通附件、空间图件、监测文件和大屏包；至少跨两个分片。暂停或离开后按现有流程继续上传，最终文件摘要应一致，相关后台任务应成功。
2. 在隔离验收环境将某类整文件配额调小并重启相关服务。新上传超过配额时应出现明确提示；已经创建的上传应能按原长度续传，历史文件仍可下载。
3. 调高空间或大屏配额，验证超过原有 32 个分片的上传；超出单片上限的请求仍返回 JSON 413。
4. 验证登录、退出、授权、下载和大屏运行正常。生产目标机的带宽、复杂模型处理、持续并发与磁盘容量需在真实部署后按业务规模验收。
