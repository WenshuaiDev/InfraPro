# 应用 HTTPS 部署与维护

Nginx 终止 TLS，API 和大屏后端继续使用本机 HTTP。三个外部入口分别为人员站点 `https://10.23.52.152:18443/console/`、运行域 `https://10.23.52.152:18444`、资源域 `https://10.23.52.152:18445`。这些是待安装地址，不表示生产服务已启动。

## 配置与启动

1. 按现有运维流程准备配置、数据库和对象存储。将 [HTTPS 配置片段](../../deploy/display/https.example.yaml) 合入 API／display-runtime 的受限配置；保持数据库、Redis、CSRF 等真实身份通过受限配置或环境注入。此次无需新增数据库迁移。
2. public_console_url 可含 `/console/` 路径，trusted_origins／runtime_origin／asset_origin 只填精确 origin，无路径、尾随斜杠、凭据、查询或片段；使用小写 ASCII／Punycode 主机名、规范 IP 地址，默认 80／443 端口省略；不接受缩写、整数、八进制或十六进制 IP 别名。所有入口协议一致，三个 origin 不同；trusted_origins 不得包含运行或资源入口；HTTPS 必须 cookie_secure=true，HTTP 必须 false。
3. API 监听 `127.0.0.1:18000`；运行／资源后端分别监听 `127.0.0.1:5180`、`127.0.0.1:5181`。trusted_proxies 只包含实际同机代理的 loopback 地址。其他部署拓扑显式调整可信代理和后端网络，禁止对外暴露这些 HTTP 端口。
4. 用 `VITE_APP_BASE=/console/ pnpm --filter @infrapro/console build` 构建人员站点，将 `apps/console/dist/` 内容发布到 `/opt/infrapro/web/console/`。构建 display-runtime，将其静态内容置于 YAML 的 shell_directory。
5. 将 [Nginx HTTPS 模板](../../deploy/display/https.nginx.conf) 纳入 Nginx `http {}`。使用含 `10.23.52.152` IP SAN 的服务器证书；证书链放 `/etc/infrapro/tls/server-chain.pem`、私钥放 `server-key.pem` 并限制读取权限。使用 DNS 时同步更改三组 server_name、Host 检查、证书 SAN 和应用 origin。
6. 访问终端安装签发 CA 的信任链，服务器／终端校时。先执行 `nginx -t`，通过后按服务管理流程启动／重载 Nginx，并以新配置启动 API、display-runtime。证书只由 Nginx 加载，应用不加载 TLS 私钥。

模板包含 B3 的全模块分片、业务请求、合并等待和下载流式策略，与 HTTP 模板使用同一规则生成。配置整文件限额与升级步骤见[上传限额与代理部署](upload-limits-and-proxy.md)。 HTTPS 模板不开放额外 HTTP 入口；本地 HTTP 模式仍使用原模板和 false 配置。

## 登录与代理边界

- 现有人员、CSRF、运行和终端 Cookie 根据配置开启 Secure；保留 HttpOnly、SameSite、作用路径与无 Domain 的现有规则。登录／退出无新增业务步骤，切换入口后需在新地址登录。
- 外部协议及 CSRF 模式由配置决定；X-Forwarded-Proto 不能改变应用安全模式。Nginx 覆写协议与来源 IP，保留含端口的 Host，拒绝不匹配的 Host；内部转发不依赖公网 HTTP。
- 同一 IP 的三个端口有不同 origin，但浏览器 Cookie 不按端口隔离。运行代理只转发 infrapro_display／infrapro_terminal；资源代理清除 Cookie／Authorization 并屏蔽 Set-Cookie。不宣称浏览器到 Nginx 的资源请求必然不带人员 Cookie。
- 运行域保留独立主体／精确 Origin、协议头、短租约与流式 SSE；资源通过许可访问且继续使用 sandbox/CSP。`/internal/` 不公开。代理关闭含凭据 URL 的请求日志；不要开启原始 Cookie、Authorization 或请求 URL 日志。

## 证书更换与开发工具

维护证书有效期和 SAN 清单；到期前先校验证书链、主机名与私钥匹配，安装新文件后 `nginx -t` 并重载，验证三个入口。CA 更换时先分发新信任链；CA 私钥不部署到应用节点。生产证书不使用自检目录中的两天临时证书。

大屏工具的 console／runtime 允许同协议 HTTP 或 HTTPS。内部 CA 使用 `NODE_EXTRA_CA_CERTS=/受限目录/ca-chain.pem` 在 Node 启动前加载，浏览器另外安装 CA 信任链；保持证书验证开启，工具拒绝运行接口重定向，避免把配对请求或读取转到其他地址。上传和真实预览配对回调仍使用本机 loopback HTTP，运行凭据仍只存工具内存。示例见[工具文档](../../packages/ts/display-devtools/README.md)。

## 自检与人工验收

运行 `pnpm test:https`：需要本机 Nginx、OpenSSL、Docker 测试依赖、Playwright Chromium。测试使用独立 PG schema／Redis namespace、合成对象和临时 CA；不会安装生产服务或修改系统信任库。Node／Playwright API 使用 CA 链和 IP SAN 正常校验，Chromium 仅固定本次生成证书的 SPKI；负例单独确认不可信 CA 和错误名称被拒绝。证据写入 `.artifacts/https-20261117/`。

正式安装后的人工验收：

1. 用正常安装 CA 的浏览器打开三个真实入口，证书验证通过且无混合内容；人员登录、资料保存、刷新、邀请、重置及退出正常。
2. 发布代表性大屏，打开运行页和终端配对，核对内容、更新、重连及撤权停供；用已安装 CA 的开发工具完成预览和上传。
3. 核对 Secure／HttpOnly／SameSite、错误 Origin／CSRF 拒绝、资源后端凭据剥离、后端监听与防火墙。使用相同 IP 多端口时特别检查跨端口 Cookie。
4. 更换测试证书并重载，验证三个入口与工具重连；记录实际部署证书期限和运维责任人。

实现自检与生产人工验收分别记录：[验证记录](../verification/https-deployment.md)。
