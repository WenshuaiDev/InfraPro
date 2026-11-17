# 上传部署适配自检

日期：2026-11-17。状态：**Awaiting acceptance**；实现和自检已完成，人工验收尚未执行。

## 交付行为

- B3：基础 HTTP、大屏 HTTP、生产 HTTPS 三份 Nginx 模板使用统一生成的 API 路由策略。普通附件／空间分片上限 8 MiB，监测 32 MiB，大屏与工具 4 MiB。保留普通接口的小请求体边界。
- 五类整文件配额通过 YAML 或环境变量配置；默认值保持附件 100 MiB、空间 256 MiB、CSV 32 GiB、XLSX 512 MiB、大屏 128 MiB。API、浏览器、开发工具与 worker 对齐，非法配置拒绝启动。
- schema 55 解除旧的整文件大小和总分片数限制；保留单片、范围、摘要和关系约束。降低新上传配额不影响已经接收的上传、历史下载或附件恢复。
- 上传禁用请求缓冲；下载和事件流禁用响应缓冲。分片读取允许 2 分钟，响应窗口 3 分钟；空间／监测封存代理等待分别为 310／1810 秒。代理 413 和网关错误返回 JSON，应用自身错误保持原样。

## 自检与证据

所有数据均为隔离环境中的合成数据。没有连接生产主机，也没有对现有业务库执行迁移。

| 检查 | 结果与范围 | 证据 |
|---|---|---|
| 三种真实 Nginx 模板 | 126 项检查通过；上游为字节计数服务。包含各分片精确上限与超出 1 字节、JSON 路由边界、上传首字节流式到达、下载摘要、事件流、错误转译和 HTTPS 凭据隔离 | `.artifacts/nginx-uploads/proxy-result.json`、`proxy.log` |
| 四类真实上传链路 | 经基础 Nginx 和真实 API／PG／S3／worker，附件 101 MiB、CSV 32 MiB + 1 字节、空间 GeoJSON 257 MiB、大屏 ZIP 141559708 字节均成功；验证重发分片及续传。附件／空间通过后台处理及原件下载摘要，大屏包验证成功；CSV 完成封存及原件下载 | `.artifacts/nginx-uploads/journeys/result.json`、`journeys/process-0.log` |
| 分片超时 | 附件首片在首字节后等待 16 秒再发送余下数据，成功越过原 API 10／15 秒超时；其余普通接口保留原默认超时 | `scripts/upload-journeys.mjs`、上述真实链路证据 |
| 配置、续传和编号边界 | 默认配置、YAML／环境变量覆盖、非法值及独立 worker 加载；调低配额后保留原会话与创建回执；空间第 33 片、大屏第 32 片（从 0 开始）、CSV 第 1025 片及 XLSX 第 17 片可按已接收大小续传，越界分片拒绝 | `.artifacts/nginx-uploads/suite.log`、`limits-final.log` |
| 数据库升级 | 空库和并发迁移通过；54→55 保留完成附件、未完成上传、对象、分片、任务及版本记录；已接受较大上传时回滚失败且事务不改数据 | `.artifacts/nginx-uploads/upgrade.log`、`suite.log` |
| 附件联合恢复 | 101 MiB 附件在新上传配额 100 MiB 的恢复环境中仍可恢复及下载。真实 PGDMP 导入独立库，17 个对象进入独立 S3 桶；缺对象阻止完成，旧 epoch／cursor／lease 拒绝 | `.artifacts/nginx-uploads/restore.log` |
| HTTPS 回归 | 登录、写操作、邀请、密码重置、退出；大屏 UI 上传／发布、终端、预览、工具配对／续传、离线失效、移动布局；CA／SAN 验证及三入口凭据隔离通过 | `.artifacts/https-20261117/result.json`、`.artifacts/nginx-uploads/https.log` |
| 客户端与模块回归 | 28 项客户端测试、附件真实 HTTP 与下载租约测试、3 项真实监测 XLSX／CSV／续传测试、7 项 Python 转换器测试通过 | `.artifacts/nginx-uploads/client.log`、`files-http.log`、`module-regression.log` |
| 静态与契约检查 | `pnpm check` 通过：Go 格式／vet、ESLint、TypeScript、OpenAPI／模块契约、权限目录和 Nginx 生成同步 | `.artifacts/nginx-uploads/check.log` |

复现主流程：`pnpm test:uploads`；大附件恢复：`INFRAPRO_UPLOAD_DEPLOYMENT=1 pnpm test:files:restore`；HTTPS：`pnpm test:https`。依赖、配置字段与升级顺序见[部署手册](../operations/upload-limits-and-proxy.md)。HTTPS 回归使用独立后端端口，避免与同时运行的 HTTP 浏览器自检串用服务。

CSV 超过 32 GiB、XLSX 超过 512 MiB 的用例验证了上传声明与末片边界，没有传输整份该规模文件。257 MiB GeoJSON 采用有效小图件加空白，135 MiB ZIP 采用多个有效大小的静态文件；这些用例证明解除旧硬上限，不能替代真实复杂模型、超大表格、并发负载及目标机磁盘容量验收。在线文档预览、ZIP 解包、像素、内存和执行时间预算仍独立生效。

## 人工验收

1. 按手册完成备份、schema 55 迁移、程序与前端更新、配置同步、相关服务重启，以及 `nginx -t` 后重载。
2. 分别上传跨分片的普通附件、空间原件、监测源文件和大屏包；继续未完成上传，检查后台状态及可下载原件的摘要。
3. 调小一类整文件配额：新建超限上传应被拒绝，已有会话仍能续传，历史文件仍可下载。调高空间／大屏配额后可超过原有 32 片边界。
4. 发送超出单片上限的请求应得到 JSON 413；正常登录、下载、大屏预览与运行不受影响。

人工接受：待用户确认。生产安装、生产数据迁移和目标环境容量验收：未执行。
