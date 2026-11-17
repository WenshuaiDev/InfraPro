# 应用侧 HTTPS 适配

状态：Awaiting acceptance（应用 HTTPS 适配范围）。专项与相关回归通过；补充全仓未全绿，详见[验证记录](../verification/https-deployment.md)。生产安装与人工验收尚未执行。

## 目标与范围

支持由 152 的 Nginx 终止 TLS，Go API 与大屏后端继续监听本机 HTTP。人员站点、可信运行壳、资源入口统一使用 HTTPS，并保留本地 HTTP 模式。此次不执行生产安装、签发生产证书、业务迁移或 B3 全模块代理限额改造。

## 实现

- Go 配置接受规范的 HTTP／HTTPS URL，可信来源和大屏入口与人员站点同协议；HTTPS 必须开启 cookie_secure，HTTP 必须关闭。三个入口保持独立 origin，拒绝含凭据、查询、片段、非法端口、非规范 IP 别名和重复入口的配置；人员可信来源不得包含大屏运行／资源入口。
- 沿用 Go net/http 的 Secure／HttpOnly／SameSite Cookie 和 gorilla/csrf。外部协议由受控配置决定，不依赖客户端 X-Forwarded-Proto；Gin 只信任配置的代理。通过真实代理回归登录、写入和退出。
- TypeScript／JavaScript 大屏工具允许 HTTPS console／runtime；本机配对回调仍限 loopback HTTP。Node 使用正常 TLS 校验，内部 CA 通过 NODE_EXTRA_CA_CERTS 提供。
- 新增三个入口的 Nginx HTTPS 模板与应用 YAML；证书由 Nginx 管理，代理覆盖来源信息、保留端口，资源入口剥离 Cookie／Authorization。相同 IP 的不同端口仅是 origin 隔离，不宣称 Cookie 隔离。
- 用隔离 PG schema、Redis namespace、测试对象和临时证书验证。HTTP 模式回归、配置负例、Go race、JS 测试、构建和全仓检查形成交付证据。

## 技术与验收

使用现有 Go／Gin／gorilla/csrf 实现配置和身份边界；Nginx 承担 TLS 和静态前端；Node 大屏工具使用原生 fetch 与 CA 信任；Playwright 验证 React 控制台与运行壳的实际浏览器行为。PostgreSQL／Redis／S3 仅提供隔离测试依赖，无 schema 变更。

验收：可信 HTTPS 入口能够登录、刷新、写操作、邀请及重置、退出；Cookie 属性正确，伪造来源／缺 CSRF 被拒绝；大屏上传、发布、预览、终端配对、SSE／重连及开发工具预览／续传成功；资源后端不接收业务凭据；HTTP 开发流程保持通过。生产证书信任与实际端口验收在正式安装时执行。
