# 解析服务运行、发布与恢复

适用范围：完整单帧样例试解析；不接入 TCP 长连接，不写正式观测。业务 API／monitoring-worker 持有 PG、私有 S3 和机器调用凭证；parser 仅持有固定发行包与机器令牌。独立服务不依赖 platform、PG、Redis、ClickHouse 或 S3。

连续设备接入、长连接、厂家 HTTPS 批次和生产原文由独立 ingest-relay／ingest-worker 负责，启动与恢复见[设备接入运行手册](device-ingestion-and-raw-messages.md)。parser 增加独立接入能力目录和识别接口；原契约 1 的测量目录、人工试解析、固定版本和旧产物继续保留。生产结果也不会自动写入正式观测。

## 本地隔离验收

在本 checkout 的根目录执行 `pnpm install --frozen-lockfile`，然后 `pnpm parsing:lab`。需要本机 Go、Node／pnpm、Docker 和 Playwright Chromium。脚本建立独立测试 schema、私有桶、真实 API、解析服务和 monitoring-worker；页面为 `http://127.0.0.1:15578`。所有厂家、产品、设备和样本均明确标注合成。

账号清单写入 `.artifacts/parsing/browser-lab/accounts.json`（0600），包含平台管理员、两个互不从属的租户管理员和仅项目设备读取账号。对象 ID 与入口上下文在同目录 `acceptance.json`；机器令牌及 operator 配置不得贴入聊天、截图或提交仓库。Ctrl-C 有界停止自有进程，并清理此次独立测试 schema／桶；不会修改主 checkout 的验收环境。再次启动会重新初始化这些隔离样例。

自动验收入口：`pnpm parser:contracts`、`pnpm test:parsing`、`pnpm parsing:browser`。相关回归为 `node scripts/parsing-regression.mjs`；通用检查为 `pnpm check` 和 `pnpm build`。证据目录 `.artifacts/parsing/`；逐项结果见[验收报告](../verification/parsing-service.md)。

## 独立启动与停机

`pnpm build` 生成 `build/bin/parser`、`parser-runner`、`parser-package`、`parsing-coverage`、`parsing-recovery` 及既有 worker。初次本地发行：

```sh
umask 077
mkdir -p .artifacts/parsing/manual/release
openssl rand -hex 48 > .artifacts/parsing/manual/machine-token
build/bin/parser-package build/bin/parser-runner samples/parsing .artifacts/parsing/manual/release
INFRAPRO_PARSER_ADDR=127.0.0.1:8091 INFRAPRO_PARSER_PACKAGE="$PWD/.artifacts/parsing/manual/release" INFRAPRO_PARSER_TOKEN_FILE="$PWD/.artifacts/parsing/manual/machine-token" INFRAPRO_PARSER_RELEASE=local-manual build/bin/parser
```

业务进程在原私有配置中合入 [parsing 配置](../../deploy/parsing/platform.example.yaml)，填入独立桶与同一令牌值的文件引用（不同运行账号各自持有归其所有的 0600 副本），明确设置 `enabled: "true"`。按现有应用运行手册启动 API 和 `INFRAPRO_CONFIG=/private/platform.yaml build/bin/monitoring-worker`。解析 worker 可以独立于 ClickHouse 运行。配置关闭或未提供 URL／令牌时，既有应用仍可启动，目录与历史可读，执行返回明确不可用状态。服务离线没有内嵌备用解析器。

用 SIGTERM／Ctrl-C 停机；parser 停止新请求并在 6 秒内退出，执行子进程最多 5 秒且必须回收；业务执行租约和 fence 使迟到响应失效。worker 下次运行恢复过期租约；人工重试建立新代，重放建立新运行。

## Linux 运行隔离与诊断

生产安装使用[systemd 单元](../../deploy/parsing/infrapro-parser.service)及[服务变量模板](../../deploy/parsing/parser.env.example)，运行账号只读程序和发行包；令牌归该账号所有且 0600。父进程最多并发 4，租户／平台范围最多并发 2，默认队列 20、范围存储预算 10 GiB。输入原字节上限 4 MiB，JSON 请求 6 MiB，响应 16 MiB，记录 10,000、步骤 50,000、嵌套深度 64。

每次执行只继承 TZ=UTC、GOMAXPROCS=1 和 64–256 MiB 可配置 Go 堆预算，不继承凭证。Linux 额外限制地址空间 2 GiB、CPU 6 秒、文件描述符 32、文件写入 0、core 0，并通过 seccomp TSYNC 禁止所有线程创建／连接网络套接字。发行包只读，systemd 额外约束整个服务 cgroup 到 1 GiB／64 tasks。macOS 本地验收有 CPU、描述符、文件及 Go 堆预算，Linux 的地址空间与 seccomp 保证以 Linux 专门测试为证据。

容器可使用 [Dockerfile](../../deploy/parsing/Dockerfile)，上下文必须包含 `CGO_ENABLED=0 GOOS=linux GOARCH=<目标架构>` 构建的 parser 和同架构发行包；以 `--read-only --cap-drop ALL --security-opt no-new-privileges --memory 1g --pids-limit 64` 运行。机器令牌和诊断目录单独挂载，限制监听端口只供业务进程使用。跨主机必须使用并验证 TLS；明文仅允许本机回环或同机容器服务名 parser，私网 IP 也不会自动获得明文豁免；不接受请求自带 URL，禁止重定向。TLS 证书与私钥成对配置；私有 CA 通过操作系统信任库配置，不关闭验证。轮换机器令牌时停止新执行、等待旧请求完成、原子更新两端私有文件并重启 parser；历史版本和证据不变。

诊断目录以 `services/diagnostics/settings/catalog.go` 为源，当前 11 个常驻入口、25 项目录，不把按需 runner 当作常驻进程。parser 上报 catalog／parse，monitoring-worker 新增 parsing／parsing-maintenance。将真实进程报告注册到独立诊断 manifest 后，可在诊断组件详情查看启动、就绪、超时和故障。报告不记录输入、测量值、设备身份、令牌或租户明细；诊断失效不影响业务结果。

## 发行、兼容检查与回滚

1. 修改算法必须新增方法版本，并更新方法清单、独立样本和期望值；不能给同一个版本覆盖新二进制。`samples/parsing/manifest.json` 是手写期望及 SHA-256 索引，`.bin` 与 `.hex` 是原始夹具。
2. 将上一版 `release/` 原样复制到候选发行目录，运行 `parser-package <新runner> samples/parsing <候选release>`。工具保留旧 `(method_id,version)` 对应产物和原始 manifest／outputs；新版本绑定新摘要。HTTP 主程序可更新，旧执行产物仍原样保存。
3. 使用**实际目标数据库**私有配置执行 `INFRAPRO_CONFIG=/private/platform.yaml build/bin/parsing-coverage /opt/infrapro/parser/candidate/release`。检查所有已登记版本（包括待启用和历史版本），以及原始清单、输出契约、只读固定产物的摘要。任何缺失、篡改或同版本冲突都拒绝。不要仅检查当前设备默认。
4. 在受控窗口停止新执行，保留当前包，以同一文件系统的临时 symlink + rename 原子替换 `current`，重启 parser；机器鉴权的 `/readyz` 和 `/v1/catalog` 应成功。平台同步新目录后新版本为待启用，明确启用后才能新采用。产品默认更新与租户设备升级分别进行。
5. 回滚也必须先对回滚包执行同样的 coverage；缺少后来已登记／采用版本的旧包不能切换。兼容回滚只回滚 HTTP 主程序或整体兼容发行，不删除新表、目录、采用或结果证据。

## 联合备份与独立恢复

备份不是单独 pg_dump。必须冻结写入窗口：停止 API 写入、monitoring-worker 及相关任务索引写入，等待在途调用结束；保留原运行服务配置的私有安全副本。用以下命令建立清单及数据库备份，连接凭据由私有 `PGSERVICEFILE` 提供：

```sh
umask 077
mkdir -p /private/backup/parsing
INFRAPRO_CONFIG=/private/platform.yaml build/bin/parsing-recovery manifest /private/backup/parsing/evidence-manifest.json
pg_dump --format=custom --file=/private/backup/parsing/postgres.dump --dbname=service=infrapro
```

使用 S3 管理工具将**整个解析桶**镜像到本次备份目录／备份桶，保留对象键；共享桶时至少完整保留清单列出的 parsing 前缀，并同时保留其他业务自己的备份方案。复制发行包 catalog、manifests、outputs、全部 artifacts、parser 主程序和相关运行版本信息。对备份文件生成 SHA-256 清单，备份存储私有且不可由普通业务用户访问；完成后恢复写入。

恢复到独立 PostgreSQL 数据库和新私有 S3 桶，禁止覆盖现用数据库演练：`pg_restore --exit-on-error --dbname=service=infrapro_restore /private/backup/parsing/postgres.dump`；按原键恢复对象和发行包，并将恢复用私有配置指向新数据库／桶。先停着 API／worker：

```sh
INFRAPRO_CONFIG=/private/restore.yaml build/bin/parsing-coverage /private/restored-release
INFRAPRO_CONFIG=/private/restore.yaml build/bin/parsing-recovery verify /private/restored-release
INFRAPRO_CONFIG=/private/restore.yaml build/bin/parsing-recovery reconcile /private/restored-release
```

verify 对数据库登记的所有可用样本／表示／响应对象核对长度与 SHA-256，并验证全部方法产物。缺对象或缺产物即失败。reconcile 只把恢复时仍 running 的代标为 `restore_interrupted`，保留输入和尝试并使旧 fence 失效；重复执行不再改已结束代。随后启动独立服务和恢复环境，检查采用区间、历史证据，并以旧版本重放固定样本。数据库外键保护引用，清单验证保护对象完整性；两个条件都通过才算可恢复。

自验已在独立数据库／桶中执行真实 pg_dump／pg_restore、逐对象恢复和旧版本重放；它不代表生产环境备份策略、容灾 RTO 或真实硬件联调已验收。
