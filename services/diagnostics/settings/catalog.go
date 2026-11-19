package settings

// RuntimeCatalog is explicit: new executable entry points fail coverage until
// their lifecycle, evidence and deployment expectation have been reviewed.
type CatalogEntry struct {
	ID                 string   `json:"id"`
	DisplayName        string   `json:"display_name"`
	Kind               string   `json:"kind"`
	EntryPoint         string   `json:"entry_point,omitempty"`
	Lanes              []string `json:"lanes"`
	DefaultExpectation string   `json:"default_expectation"`
	Reason             string   `json:"reason"`
}

var RuntimeCatalog = []CatalogEntry{
	{"replica-proxy", "副本 TCP 转发", "gateway", "deploy/architecture/replica-proxy.py", []string{}, "disabled", "原生或混合发布使用固定监听转发至已登记的多个业务副本"},
	{"identity-api", "身份与空间治理", "application", "services/api/cmd/identity-api/main.go", []string{"http"}, "continuous", "会话验证、身份维护及工作空间治理"},
	{"control-api", "工程与业务控制", "application", "services/api/cmd/control-api/main.go", []string{"http"}, "continuous", "原授权和命令事务"},
	{"monitoring-query-api", "监测查询", "application", "services/api/cmd/monitoring-query-api/main.go", []string{"http"}, "continuous", "有界监测和大屏读取"},
	{"transfer-api", "受控传输", "application", "services/api/cmd/transfer-api/main.go", []string{"http"}, "continuous", "已授权文件的字节流及完成操作"},
	{"compat-api", "兼容业务 API", "application", "services/api/cmd/compat-api/main.go", []string{"http"}, "disabled", "升级后同 schema 回退和分组切流"},
	{"ingest-custody-worker", "原文接管", "worker", "services/api/cmd/ingest-custody-worker/main.go", []string{"pull", "relay-status", "object-maintenance", "confirmation", "source-publication", "association-watermark"}, "continuous", "持久接管、核实对象与交接确认"},
	{"parsing-dispatch-worker", "解析调度", "worker", "services/api/cmd/parsing-dispatch-worker/main.go", []string{"identify", "planning", "parse", "recovery", "parsing", "parsing-maintenance", "events-identify", "events-plan", "events-parse"}, "continuous", "识别、方法计划、人工与自动试解析"},
	{"monitoring-publish-worker", "正式监测发布", "worker", "services/api/cmd/monitoring-publish-worker/main.go", []string{"scheduler", "foreground", "historical"}, "continuous", "导入校验、正式采用、核验与搬迁"},
	{"monitoring-incremental-worker", "监测增量计算", "worker", "services/api/cmd/monitoring-incremental-worker/main.go", []string{"scheduler", "foreground", "historical", "trial-http", "events-derived", "events-summary"}, "continuous", "变更位置消费、派生计算与试算"},
	{"monitoring-history-worker", "历史分析", "worker", "services/api/cmd/monitoring-history-worker/main.go", []string{"historical"}, "continuous", "历史回算、比较、重放和导出"},
	{"monitoring-read-worker", "后台读取", "worker", "services/api/cmd/monitoring-read-worker/main.go", []string{"historical", "display-reads", "events-display-cache", "events-query-cache"}, "continuous", "后台查询与大屏持久读取"},
	{"event-relay", "内部事件转发", "worker", "services/api/cmd/event-relay/main.go", []string{"publish"}, "continuous", "将已提交事实交给 JetStream 并记录确认"},
	{"nats-server", "内部消息存储", "gateway", "", []string{}, "continuous", "JetStream 文件流与副本健康"},
	{"display-assets", "大屏包资产", "application", "services/api/cmd/display-assets/main.go", []string{"assets-http"}, "continuous", "permit、会话与固定版本资产检查"},
	{"parser-runner", "方法执行子进程", "tool", "services/parser/cmd/parser-runner/main.go", []string{}, "on_demand", "由 parser 按版本与预算启动"},
	{"ingest-worker", "设备接入流水", "worker", "services/api/cmd/ingest-worker/main.go", []string{"pull", "relay-status", "confirmation", "identify", "planning", "parse", "source-publication", "association-watermark", "recovery", "object-maintenance"}, "disabled", "机器执行独立于人员授权；接收、识别和测量均有持久队列"},
	{"ingest-relay", "云端转发", "application", "services/ingest-relay/cmd/ingest-relay/main.go", []string{"control", "receivers", "spool-maintenance"}, "disabled", "独立云 PG 持久缓冲；只接受 mTLS 机器身份，无业务库凭证"},
	{"ingest-relay-postgres", "云端缓冲 PostgreSQL", "database", "", []string{}, "disabled", "独立于业务数据库，检查 fsync、synchronous_commit 和容量"},
	{"parser", "独立解析服务", "application", "services/parser/cmd/parser/main.go", []string{"catalog", "parse"}, "disabled", "启用试解析时部署；不读取业务存储，按固定产物启动有界子进程"},
	{"api", "业务 API", "application", "services/api/cmd/api/main.go", []string{"http"}, "continuous", "同源路由网关，兼容拓扑可承接原操作"},
	{"monitoring-worker", "监测、派生计算与试解析", "worker", "services/api/cmd/monitoring-worker/main.go", []string{"scheduler", "foreground", "historical", "parsing", "parsing-maintenance"}, "continuous", "派生计算在此进程内运行"},
	{"notification-worker", "通知投递", "worker", "services/api/cmd/notification-worker/main.go", []string{"delivery", "events-notification"}, "continuous", "实际投递循环，本地报告独立于数据库心跳"},
	{"task-index-worker", "任务索引", "worker", "services/api/cmd/task-index-worker/main.go", []string{"reconcile", "backfill", "events-task-index"}, "continuous", "任务索引回填与对账"},
	{"spatial-worker", "空间转换", "worker", "services/api/cmd/spatial-worker/main.go", []string{"converter", "convert"}, "continuous", "Python 子进程按需运行"},
	{"display-package-worker", "大屏包处理", "worker", "services/api/cmd/display-package-worker/main.go", []string{"maintain", "process"}, "continuous", "包校验和维护"},
	{"display-runtime", "大屏运行服务", "application", "services/api/cmd/display-runtime/main.go", []string{"runtime-http", "assets-http"}, "continuous", "可信壳与 SDK 运行入口，兼容拓扑也承接资产监听"},
	{"files-worker", "普通文件处理", "worker", "services/api/cmd/files-worker/main.go", []string{"heartbeat", "cleanup", "prepare"}, "continuous", "文件准备和清理，缺存储时未配置"},
	{"files-checker", "文件检查器", "worker", "services/api/cmd/files-checker/main.go", []string{"socket"}, "continuous", "只观察父进程，不探测解析 socket"},
	{"audit-worker", "审计维护", "worker", "services/api/cmd/audit-worker/main.go", []string{"reconcile", "export", "events-audit"}, "continuous", "沿用独立审计方案，不读取审计正文"},
	{"web", "业务静态网页", "gateway", "", []string{}, "continuous", "登记实际静态服务，不虚构生产 Node 进程"},
	{"postgres", "PostgreSQL", "database", "", []string{}, "continuous", "只执行 SELECT 1"},
	{"redis", "Redis", "cache", "", []string{}, "continuous", "只执行 PING"},
	{"clickhouse", "ClickHouse", "database", "", []string{}, "continuous", "只执行 SELECT 1"},
	{"keeper", "ClickHouse Keeper", "database", "", []string{}, "disabled", "仅在部署启用并允许 ruok 时检查，不推导 quorum"},
	{"s3", "对象存储", "object_store", "", []string{}, "external", "只执行 HeadBucket；按实际底层进程补充槽位"},
	{"seaweed-master", "SeaweedFS Master", "object_store", "", []string{}, "disabled", "拆分部署时单独登记，合一部署不重复计数"},
	{"seaweed-filer", "SeaweedFS Filer", "object_store", "", []string{}, "disabled", "仅适用已启用的 SeaweedFS 拆分部署"},
	{"seaweed-volume", "SeaweedFS Volume", "object_store", "", []string{}, "disabled", "仅适用已启用的 SeaweedFS 拆分部署"},
	{"seaweed-gateway", "SeaweedFS S3 Gateway", "gateway", "", []string{}, "disabled", "仅适用独立 S3 网关进程"},
	{"python-converter", "Python 转换器", "tool", "", []string{}, "on_demand", "由 spatial-worker 按需启动；空闲时没有子进程正常"},
	{"maintenance-cli", "迁移与维护命令", "tool", "", []string{}, "on_demand", "一次性 CLI 不属于常驻故障分母"},
	{"diagnostics-server", "独立诊断服务", "diagnostics", "", []string{}, "continuous", "不依赖业务初始化"},
	{"diagnostics-observer", "节点观察器", "diagnostics", "", []string{}, "disabled", "需要额外权限或网络视角时独立部署"},
}
