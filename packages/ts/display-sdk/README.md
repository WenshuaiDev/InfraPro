# @infrapro/display-sdk 1.1

框架无关的项目大屏读取客户端。只导出编译 ESM JavaScript 和 TypeScript 声明；运行时无 npm 依赖，不导入 React 或 InfraPro workspace。

## 安装与接入

```sh
npm install /path/to/infrapro-display-sdk-1.1.0.tgz
```

在独立工程构建，并由 InfraPro 可信运行壳加载。直接打开应用 HTML 不会取得业务身份。HTTP 内网地址受支持；核心握手不要求安全上下文。

```ts
import { connect, DisplayError } from '@infrapro/display-sdk';
const sdk = await connect();
const context = await sdk.context.get();
const config = await sdk.config.get();
const site = await sdk.bindings.resolve('site');
const result = await sdk.resources.list({ target: { binding: 'site' } });
render(result.data); // render 是项目自己的渲染函数
await sdk.runtime.ready();
window.addEventListener('pagehide', () => sdk.dispose(), { once: true });
```

`context.simulated` 必须在展示层标识；本地工具的壳也持续显示模拟标记。`preview` 表示使用人员当前资格的草稿预览，正式发布另行核验。

## 读取 API

1.1 新增命名读取视图，和下述旧绑定接口并存。程序在 Manifest 2 的 `read_view_schema` 声明视图名称、领域、用途和是否必需。管理端在 `read_scopes` 授予范围与字段，再用配置的 `read_views` 将程序视图映射到授权范围。调用形态决定返回类型。

```ts
const handle = sdk.queries.consume('statistics.monitoring', {
  targets: [{ view: 'project_raw' }, { view: 'focus_derived' }],
  resource_target: { view: 'project_resources' },
  field_groups: ['statistics.counts', 'resource.metadata'],
  source_kinds: ['direct', 'derived'], group_by: 'facility', all_history: true,
});
let result = await handle.result;
if (result.state === 'pending') result = await sdk.queries.wait(result);
if (result.state !== 'pending') render(result.data);
// 每个组件持有独立句柄；最后一个消费者释放远端查询。
handle.release();
```

新接口包括 `views.list/describe`、`resources.types/query/tree/detail`、`devices.list/get/outputs/history`、`sources.bindings/bindingHistory`、`map.facilities/features/basemaps/readTile`、`spatial.assets/versions/componentLinks/locations/settings`、三类 `statistics.resources/devices/monitoring`。通道、监测和空间旧方法也接受显式 `target.view`。命名调用禁止空 target，只有获准的 view 或服务端生成的 selection 可以选择范围；固定 ID 必须位于所选视图内。

新 `ReadResult<T>` 区分 ready/empty/partial/pending，pending 没有 data。`queries.get({query_id,cursor})` 延续固定结果和分页大小，`queries.cancel/release` 幂等。统计下钻先调用 `queries.drilldown({query_id,group_key,basis,purpose})`，再将返回的 selection 交给监测读取；不能覆盖父查询时间、质量或主体。原始和派生分别计数，total 是完整范围，分组分页不能自行累加替代 total。

可选字段默认省略，`included_field_groups` 给出实际投影；显式请求未授权字段返回 field_not_granted。计数、修订和精确数值保持 string。每个命名视图最多 50,000 个有效成员，显式 IDs 最多 500；目录默认 50/最大 200、地图最大 500、明细最大 1,000、趋势最多 50 通道。超过预算明确失败。后台查询截止 120 秒，结果寿命最多 15 分钟且受主体租约限制。

`statistics.subscribeResources/subscribeDevices/subscribeMonitoring` 接收完整替换结果。事件中的 query、totals、groups 同属一个快照；重复 sequence 和旧 generation 不会重复消费。资源／设备最短刷新 30 秒，监测最短 10 秒；撤权不等待刷新间隔。所有分页、缓存、文件块和订阅交付都会复验当前资格。

以下为继续支持的旧绑定 API；“空 target”和旧 500 目标限制仅适用于这条接口。

- `context.get()`、`config.get()`、`capabilities.list()/get(name)`、`bindings.list()/resolve(name)`。
- `resources.list/get/summary`、`channels.list/get`。
- `monitoring.latest/details/trends/statistics/overview` 和 `monitoring.subscribe(kind, query, consume, options)`；订阅 kind 为 latest、trends、overview、statistics。
- `sources.get/history`；设备来源与观测读取分别鉴权。
- `artifacts.list/get/read`；绑定固定成果版本，递归检查输入资格，不发起计算。
- `spatial.list/manifest/objects/placements/readFile`；绑定固定空间版本。原始 IFC/PDF 不属于此读取接口。
- `alerts.query/summary/subscribe`：当前返回 `capability_unavailable`，无模拟业务告警。
- `navigation.open('resource'|'monitoring'|'display', id)`；用户触发后壳提供受控链接。终端进入人员页面需要人员登录。
- `runtime.on(listener)/ready()/reportError(code)`、`dispose()`。

业务方法返回 `Result<T>`：state、data、read_at、authorization_revision、generation、source_vector。目录、来源、派生状态、覆盖、统计与固定成果完整保留业务含义；没有可读数据与依赖不可用是两种结果。名称、单位、方向、基准只用于展示，不能自行推断工程安全结论。

查询 `target` 可指定命名 binding，或在既有绑定内收窄 IDs；不能传租户、项目、人员、SQL 或任意 URL。空 target 表示同类型绑定的集合，多个时段取交集；不相交时应指定具体绑定。目录支持 cursor/search/limit。资源/空间/成果绑定固定 ID，动态通道范围由管理端受限筛选决定。

观测值、计数、修订、纳秒时间均保持字符串。不要直接用 `Number` 转换精确小数或 64 位整数；图表若需要浮点，应用必须自行决定可接受的精度。时间为 RFC3339Nano；固定成果还保留 `at_ns` 和纳秒区间。来源缺失、派生 pending/受影响和部分覆盖不能渲染成零或正常。

明细第一页返回 selection_id、snapshot.id、filters；后续页把这些原样传回，并使用 next_cursor。快照属于当前运行主体，不能在其他页面、发布或人员中复用。跟随窗口要求配置 mode=follow，查询使用 `follow_seconds`（1 秒至 31 天）；固定历史使用 range/all_history。

## 设备分类与完整工程结构

包含断面、孔位的视图必须在实例授权范围和配置中显式声明 `structure_model: 2`。新增的资源种类为 `monitoring_section` 和 `borehole`；`parent_ref` 使用 `{kind,id}`，测点附属直接通道仍属于测点，派生通道可属于其他工程对象。详情传入 `kind` 可区分不同种类恰有相同 UUID 的合法身份。

新增 `classification.categories`、`resources.hierarchy`、`statistics.structure`、`statistics.deviceCategories`。完整树不要求固定层序，目录与下钻均支持分页。设备名单读取需要 `device.metadata`；分类明细需要 `device.classification`，仅有 `statistics.classification` 不授予名单或设备身份。

```ts
const structure = await sdk.statistics.structure({
  target: { view: 'project_resources' },
  group_by: 'monitoring_section', membership: 'nearest',
  field_groups: ['statistics.counts', 'resource.metadata'],
});
const categories = await sdk.statistics.deviceCategories({
  target: { view: 'project_devices' },
  population: 'authorized', mode: 'direct',
});
```

`nearest` 将测点归到最近的指定种类祖先；`subtree` 包含后代，可能重叠，不可相加得到唯一总数。孔位能力区分 `not_applicable`（明确不支持）、`applicable`（支持，数量可为零）、`undeclared`（旧类型未声明）。未归属保留 `unassigned`，context 祖先不计实体。设备按身份去重；`monitoring_bound` 另需 `channel_target`，安装关系与供数关系分别选择，安装不产生来源绑定。

分类树的当前解释采用固定字典发布版本。查询与下钻保留原分类、原路径和原成员；新查询反映后续调整，交付仍检查当前权限。新统计订阅同样完整替换结果。旧视图不会自动取得新资源种类；遇到不可表达的断面／孔位派生所有者返回 `structure_model_upgrade_required`。升级需明确修改范围和配置，不能仅在客户端改名称。

可运行示例及授权清单见[独立工程](../../../examples/display-engineering-structure/README.md)。模拟模式明确标记，正式验收使用真实发布实例与配对终端。

## 连续供数与取消

```ts
const abort = new AbortController();
const subscription = await sdk.monitoring.subscribe('latest', {
  target: { binding: 'focus_channels' },
}, async event => {
  if (event.kind === 'reset' || event.kind === 'revoked') clearChart();
  if (event.result) await applyChart(event.result);
}, { signal: abort.signal, generation: 'view-1' });
// 切换视口或组件卸载：
subscription.unsubscribe(); // 或 abort.abort()
```

消费者同步返回或 Promise 成功完成后才确认游标；异常不确认，服务器会重放。按订阅代次与序号去重，reset 清除原结果后接收新快照。等价订阅合并，但每个返回句柄独立取消；全部取消后释放远端订阅。处理函数应有界完成。单次方法的第二参数同样接受 signal/generation。

监听 runtime 的 disconnected/reconnecting/resynchronized/revoked/release_changed/paused/resumed/error/disposed。壳在短租约结束或当前资格失效时卸载应用；dispose 会释放请求、端口、订阅及 SDK 创建的 Blob URL。

```ts
const part = await sdk.spatial.readFile({
  target: { binding: 'model' }, file_id: 'manifest-file-id', offset: 0, length: 65536,
}, { signal: abort.signal });
const url = sdk.spatial.createObjectURL(new Blob([part.data.bytes], { type: part.data.media_type }));
// 不再使用时：
sdk.spatial.revokeObjectURL(url);
```

单片最多 4 MiB，返回总长度/偏移/片长/整文件摘要。按 manifest 分块获取，应用自己管理加载与最终完整性校验；不会取得存储密钥或长期下载 URL。

## 错误、限制与兼容

`DisplayError.code` 稳定可分支；issues 包含 path/code/message。常见代码：binding_invalid、capability_unavailable、version_incompatible、query_invalid、budget_exceeded、reset_required、revoked、session_expired、aborted、disposed、dependency_unavailable。收到权限/范围失效时清除对应展示，不能继续把旧值当有效数据。

SDK 版本 1.1.0、Bridge 主版本 1、每项业务能力主版本 1 分开协商。必需能力缺失阻止发布，可选能力通过发现降级。破坏性版本增加主版本；升级应创建新包、新配置和新发布，保留可回退版本。SDK 不提供任意网络代理、写操作或管理凭据。

每实例最多 500 个绑定目标、每会话默认 8 个并发查询/16 个订阅、结果默认 8 MiB、读取截止 15 秒；部署可进一步收紧。SDK 本地最多 32 个在途调用，参数 256 KiB。复用具名绑定和共享查询，避免逐个通道轮询。

测试入口 `@infrapro/display-sdk/testing` 导出 createFixtureTransport(scenario)，支持 normal/empty/pending/partial/disconnected/revoked/unavailable/overlap/scope-changed。夹具只能用于接入测试。

包规范、服务器版本化协议、部署与完整验收记录随 InfraPro 发布材料提供。许可证 MIT。
