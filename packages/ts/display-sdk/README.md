# @infrapro/display-sdk 1.0

框架无关的项目大屏读取客户端。只导出编译 ESM JavaScript 和 TypeScript 声明；运行时无 npm 依赖，不导入 React 或 InfraPro workspace。

## 安装与接入

```sh
npm install /path/to/infrapro-display-sdk-1.0.0.tgz
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

SDK 版本 1.0.0、Bridge 主版本 1、每项业务能力主版本 1 分开协商。必需能力缺失阻止发布，可选能力通过发现降级。破坏性版本增加主版本；升级应创建新包、新配置和新发布，保留可回退版本。SDK 不提供任意网络代理、写操作或管理凭据。

每实例最多 500 个绑定目标、每会话默认 8 个并发查询/16 个订阅、结果默认 8 MiB、读取截止 15 秒；部署可进一步收紧。SDK 本地最多 32 个在途调用，参数 256 KiB。复用具名绑定和共享查询，避免逐个通道轮询。

测试入口 `@infrapro/display-sdk/testing` 导出 createFixtureTransport(scenario)，支持 normal/empty/pending/disconnected/revoked/unavailable。夹具只能用于接入测试。

包规范、服务器版本化协议、部署与完整验收记录随 InfraPro 发布材料提供。许可证 MIT。
