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
	{"api", "业务 API", "application", "services/api/cmd/api/main.go", []string{"http"}, "continuous", "人员及业务接口进程"},
	{"monitoring-worker", "监测与派生计算", "worker", "services/api/cmd/monitoring-worker/main.go", []string{"scheduler", "foreground", "historical"}, "continuous", "派生计算在此进程内运行"},
	{"notification-worker", "通知投递", "worker", "services/api/cmd/notification-worker/main.go", []string{"delivery"}, "continuous", "实际投递循环，本地报告独立于数据库心跳"},
	{"task-index-worker", "任务索引", "worker", "services/api/cmd/task-index-worker/main.go", []string{"reconcile", "backfill"}, "continuous", "任务索引回填与对账"},
	{"spatial-worker", "空间转换", "worker", "services/api/cmd/spatial-worker/main.go", []string{"converter", "convert"}, "continuous", "Python 子进程按需运行"},
	{"display-package-worker", "大屏包处理", "worker", "services/api/cmd/display-package-worker/main.go", []string{"maintain", "process"}, "continuous", "包校验和维护"},
	{"display-runtime", "大屏运行服务", "application", "services/api/cmd/display-runtime/main.go", []string{"runtime-http", "assets-http"}, "continuous", "一个进程、两个独立端点"},
	{"files-worker", "普通文件处理", "worker", "services/api/cmd/files-worker/main.go", []string{"heartbeat", "cleanup", "prepare"}, "continuous", "文件准备和清理，缺存储时未配置"},
	{"files-checker", "文件检查器", "worker", "services/api/cmd/files-checker/main.go", []string{"socket"}, "continuous", "只观察父进程，不探测解析 socket"},
	{"audit-worker", "审计维护", "worker", "services/api/cmd/audit-worker/main.go", []string{"reconcile", "export"}, "continuous", "沿用独立审计方案，不读取审计正文"},
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
