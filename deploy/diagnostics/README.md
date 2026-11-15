# 独立运行诊断部署

完整步骤、账号初始化、TLS、密钥轮换、升级与状态库恢复见[运维说明](../../docs/operations/independent-runtime-diagnostics.md)。

| 文件 | 用途 |
| --- | --- |
| `Dockerfile` | 独立 UI → embed → Go release → 非 root 最终镜像，不编译业务 server |
| `compose.yaml` | 独立 TLS server；可选无 socket 的协议 observer |
| `compose.docker-observer.yaml` | 单独具备 daemon socket 权限的 observer，必须配置 socket GID |
| `native/` | 原生 loopback 示例；使用 CLI 生成实际 UID／路径更方便 |
| `docker/` | server 容器资源范围与 TLS 示例；所有地址／证书需按实际部署替换 |
| `mixed/` | 原生 server 加独立容器 observer 的拓扑模板；跨主机／容器连接先配置 TLS |
| `systemd/` | 两个独立专用用户的 unit，无业务 Requires/PartOf |

JSON 示例由 `node scripts/diagnostics-templates.mjs` 生成，`--check` 校验幂等。业务组件初始 disabled，空 auth/keyring 不含默认账号。observer 示例的 `observer-bindings.json` 应放到 observer 私有目录并命名为 `bindings.json`；复制 manifest、该节点 key 和公开 CA，不复制 server 的 auth/keyring。本机资源 adapter 的 scope 必须与节点真实边界匹配。

server 默认 UID 10001，协议 observer 10002，daemon observer 10003；目录与文件必须对应实际运行 UID。Compose `DIAGNOSTICS_*` 变量只传路径、端口、UID/GID、镜像，不传密码。server 配置和 TLS 私钥挂载只读，state 独立可写。Docker socket 上的 `:ro` 不是授权防火墙，权限边界来自角色分离和固定请求实现。

提供的 standalone Compose 模板已纳入 `node scripts/diagnostics-deploy-check.mjs`。私有 D-Bus 测试仅证明 systemd 适配契约，真实 Linux unit 部署尚需按运维清单实测。不要将文件存在视为当前主机已经部署。
