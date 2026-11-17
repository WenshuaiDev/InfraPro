import { connect, type ReadResult, type ReadMonitoringStatistics } from '../../packages/ts/display-sdk/src/index.js';
declare const client:Awaited<ReturnType<typeof connect>>;
void client.resources.query({target:{view:'resources'},limit:200});
void client.resources.tree({target:{view:'resources'},mode:'children',parent_ref:{kind:'facility',id:'uuid'}});
void client.monitoring.details({target:{selection:'opaque'},limit:1000});
void client.monitoring.details({target:{view:'channels'},all_history:true});
void client.statistics.monitoring({targets:[{view:'raw'},{view:'selected_derived'}],source_kinds:['direct','derived'],group_by:'facility',range:{from:'2026-01-01T00:00:00Z',to:'2026-02-01T00:00:00Z'}});
void client.sources.bindings({target:{view:'channels'},point_ids:['uuid']});
void client.channels.list({target:{binding:'old_binding'}});
void client.monitoring.latest({target:{binding:'old_binding'}});
// @ts-expect-error Empty targets must not silently select an entire project.
void client.resources.query({target:{}});
// @ts-expect-error Named targets cannot combine a view with an opaque selection.
void client.monitoring.details({target:{view:'raw',selection:'opaque'}});
// @ts-expect-error Selection handles retain the parent's immutable time range.
void client.monitoring.details({target:{selection:'opaque'},all_history:true});
// @ts-expect-error Time selectors are mutually exclusive.
void client.statistics.monitoring({targets:[{view:'raw'}],source_kinds:['direct'],group_by:'none',all_history:true,follow_seconds:60});
// @ts-expect-error A source batch chooses either points or channels.
void client.sources.bindings({target:{view:'raw'},point_ids:['uuid'],channel_ids:['uuid']});
// @ts-expect-error A tree path requires a node reference.
void client.resources.tree({target:{view:'resources'},mode:'path'});
// @ts-expect-error Field groups are a closed vocabulary.
void client.devices.list({target:{view:'devices'},field_groups:['all_secrets']});
declare const result:ReadResult<ReadMonitoringStatistics>;
if(result.state==='pending'){
 const id:string=result.query.id;
 // @ts-expect-error Pending has no business data.
 const totals=result.data.totals;
 void id;void totals;
}else{
 const count:string|null|undefined=result.data.totals.raw.active_count;
 // @ts-expect-error Exact quantities cannot be treated as JavaScript numbers.
 const lossy:number=result.data.totals.raw.active_count;
 void count;void lossy;
}
