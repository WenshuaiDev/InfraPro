# 自有进程报告接入

所有 11 个常驻入口的目录源是 `services/diagnostics/settings/catalog.go`，生成到 `contracts/diagnostics/runtime-catalog.json`。新增 `services/api/cmd/*/main.go` 必须登记目录、期望、lane 和接入 `diagnosticreport.FromEnv`；`pnpm diagnostics:coverage` 会拒绝未知入口。

`packages/go/foundation/diagnosticreport` 不依赖 platform 或诊断 server。入口在解析业务配置、连接数据库前建立 reporter，报告 starting；真实初始化完成后 Ready，错误退出前 Failed，原有 defer/errgroup/context/退出码保持不变。没有诊断环境变量时返回 noop。

启用变量：`INFRAPRO_DIAGNOSTICS_INSTANCE_ID`、`INFRAPRO_DIAGNOSTICS_NODE_ID`、`INFRAPRO_DIAGNOSTICS_REPLICA_ID`、`INFRAPRO_DIAGNOSTICS_REPORT_FILE`。report 文件必须绝对路径，私有目录由运行账号拥有。build 通过 `-X github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport.BuildID=<id>` 标识。不要给业务进程 observer key。

业务线程只更新有界内存快照，不同步写盘、不发网络。独立 writer 定时原子落盘，文件 0600，序号递增，拒绝 symlink。写入失败或挂住不改变业务成功／失败；Close 等待最多 100ms。报告不携带 argv、env、数据库 URL、原始错误、业务正文或任务参数。

lane 的 Start/Working/Success/Failure/Idle 跟随真实调用。有限任务使用实际 context deadline；长间隔维护提供真实 next_attempt_at；事件驱动 lane 没有虚假的周期。并发 lane 使用 group/track 记录实际活跃调用，不以 reporter writer 心跳伪造业务循环。依赖状态跟随实际 PG 等调用成功／失败，不能用诊断自己 PING 的结果替代。

monitoring 有 scheduler、foreground、historical、parsing、parsing-maintenance 五条实际 lane；derived 工作由现有 monitoring worker 执行，不新增虚构 derived-worker。spatial 的 converter 预检与实际转换分别报告，Python 是按需子进程。display-runtime 一个 PID 对 runtime/assets 两个端点分别检查。files-worker 的 prepare/cleanup/heartbeat 保持原 errgroup 退出语义，未配置存储保持 opt-in idle；files-checker 只接入父进程，child 不带诊断 key 或 reporter，不向业务 socket 发送伪造解析请求。

ProcessReport schema 是严格输入契约。进程身份用 PID、OS start time、UID、可执行文件共同核对；复制旧 report 不能继承 fresh。sample age 基于原始 emitted_at 和 report_seq，同一来源报告反复读取不产生新 attempt。deadline 超期、错过 next_attempt 加宽限、持续依赖错误分别形成明确原因；只启动 writer 不能掩盖 stalled lane。

parser 是独立常驻服务，报告 catalog 清单验证与 parse 有界子进程执行；解析子进程按需启动，不计入常驻分母，也不继承机器令牌或诊断配置。

接入自验包括 reporter 阻塞／退出边界、真实 owning process 的空闲／工作／失败、业务任务生命周期与原沙箱测试。调用 `node scripts/diagnostics-business-check.mjs` 使用隔离 lab 的真实数据库和独立测试 schema，最终保留脱敏的 ProcessReport 快照。
