import { DisplayError } from './error.js';
import type { CallOptions, Subscription, SubscriptionEvent } from './types.js';
import type { ReadMethod, ReadRequest, ReadResult, ReadData, ReadQueryRequest, ReadDrilldownRequest, ReadDrilldownSelection, ReadConsumer, ReadSubscriptionEvent, QueryConsumer } from './read-types.js';
type Request = <T>(method:string,params?:unknown,options?:CallOptions)=>Promise<T>;
type Subscribe = (kind:string,params:unknown,consumer:(event:SubscriptionEvent<unknown>)=>void|Promise<void>,options?:CallOptions)=>Promise<Subscription>;
const stable=(v:unknown):string=>Array.isArray(v)?`[${v.map(stable).join(',')}]`:v&&typeof v==='object'?`{${Object.entries(v).filter(([,v])=>v!==undefined).sort(([a],[b])=>a.localeCompare(b)).map(([k,v])=>`${JSON.stringify(k)}:${stable(v)}`).join(',')}}`:JSON.stringify(v)??'null';
const pause=(ms:number,signal?:AbortSignal)=>new Promise<void>((resolve,reject)=>{if(signal?.aborted){reject(new DisplayError('aborted'));return;}const done=()=>{signal?.removeEventListener('abort',abort);resolve();};const timer=setTimeout(done,ms);const abort=()=>{clearTimeout(timer);signal?.removeEventListener('abort',abort);reject(new DisplayError('aborted'));};signal?.addEventListener('abort',abort,{once:true});});
export function createReads(request:Request,subscribe:Subscribe,clearURLs:()=>void){
  let epoch=0,closed=false;
  const queries=new Set<string>();
  interface Shared { refs:number;controller:AbortController;result:Promise<ReadResult<unknown>>;query?:string;released:boolean }
  const consumers=new Map<string,Shared>();
  const release=(id:string)=>{queries.delete(id);return request<ReadResult<{query_id:string;released:boolean}>>('queries.release',{query_id:id});};
  async function call<T>(method:string,params:unknown,options?:CallOptions):Promise<ReadResult<T>>{
    if(closed)throw new DisplayError('disposed');
    const current=epoch;
    let result:ReadResult<T>;
    try { result=await request<ReadResult<T>>(method,params,options); }
    catch(error) {
      const code=DisplayError.from(error).code;
      if(['reset_required','session_revoked','query_expired','scope_changed'].includes(code))invalidate(code==='session_revoked');
      if(['query_invalid','method_unknown','unsupported_method','method_not_allowed'].includes(code)){
        const capabilities=await request<{max_view_members?:number}[]>('capabilities.list').catch(()=>null);
        if(Array.isArray(capabilities)&&!capabilities.some(c=>c.max_view_members!==undefined))throw new DisplayError('capability_unavailable','This runtime does not support named display reads.');
      }
      throw error;
    }
    if(closed||current!==epoch||options?.signal?.aborted||options?.generation!==undefined&&result.generation!==options.generation){if(result.query)void release(result.query.id).catch(()=>{});throw new DisplayError(closed?'disposed':options?.signal?.aborted?'aborted':'reset_required');}
    if(result.query)queries.add(result.query.id);
    return result;
  }
  const read=<M extends ReadMethod>(method:M)=>(params:ReadRequest<M>,options?:CallOptions)=>call<ReadData[M]>(method,params,options);
  function invalidate(dispose=false){epoch++;clearURLs();for(const item of consumers.values()){item.released=true;item.controller.abort();}consumers.clear();for(const id of queries)void release(id).catch(()=>{});queries.clear();closed=dispose;}
  async function wait<T>(initial:ReadResult<T>,options:CallOptions={}):Promise<ReadResult<T>>{
    let current=initial;const until=Date.now()+120_000;
    while(current.state==='pending'){
      if(Date.now()>=until)throw new DisplayError('query_timeout');
      await pause(Math.max(100,Math.min(5_000,current.retry_after_ms)),options.signal);
      current=await call<T>('queries.get',{query_id:current.query.id},options);
    }
    return current;
  }
  function consume<M extends ReadMethod>(method:M,params:ReadRequest<M>,options:CallOptions={}):QueryConsumer<ReadData[M]>{
    if(closed||options.signal?.aborted)return {result:Promise.reject(new DisplayError(closed?'disposed':'aborted')),release:()=>{}};
    const key=stable({method,params,generation:options.generation});
    let entry=consumers.get(key);
    if(!entry){
      const controller=new AbortController();
      const result=call<ReadData[M]>(method,params,{generation:options.generation,signal:controller.signal});
      const item:Shared={refs:0,controller,result,released:false};
      item.result=result.then(result=>{item.query=result.query?.id;if(item.released&&item.query)void release(item.query).catch(()=>{});return result;}).catch(e=>{if(consumers.get(key)===item)consumers.delete(key);throw e;});
      entry=item;consumers.set(key,item);
    }
    const current=entry;current.refs++;let ended=false,rejectConsumer:((error:DisplayError)=>void)|undefined;
    const detach=()=>{if(ended)return;ended=true;options.signal?.removeEventListener('abort',detach);rejectConsumer?.(new DisplayError('aborted'));if(--current.refs===0){current.released=true;if(consumers.get(key)===current)consumers.delete(key);current.controller.abort();if(current.query)void release(current.query).catch(()=>{});}};
    const result=new Promise<ReadResult<ReadData[M]>>((resolve,reject)=>{
      rejectConsumer=reject;
      options.signal?.addEventListener('abort',detach,{once:true});
      current.result.then(value=>{rejectConsumer=undefined;if(ended)reject(new DisplayError('aborted'));else resolve(value as ReadResult<ReadData[M]>);},error=>{rejectConsumer=undefined;detach();reject(error);});
    });
    return {result,release:detach};
  }
  const subscribeRead=<M extends 'statistics.resources'|'statistics.structure'|'statistics.devices'|'statistics.deviceCategories'|'statistics.monitoring'>(method:M)=>(params:ReadRequest<M>,consumer:ReadConsumer<ReadData[M]>,options?:CallOptions)=>subscribe(method,params,event=>consumer(event as ReadSubscriptionEvent<ReadData[M]>),options);
  return {
    read,call,invalidate,
    views:{list:(options?:CallOptions)=>read('views.list')({},options),describe:read('views.describe')},
    queries:{
      get:<T=ReadData[ReadMethod]>(params:ReadQueryRequest,options?:CallOptions)=>call<T>('queries.get',params,options),
      wait,consume,
      cancel:(params:{query_id:string},options?:CallOptions)=>call<{query_id:string;released:boolean}>('queries.cancel',params,options),
      release:(params:{query_id:string})=>release(params.query_id),
      drilldown:(params:ReadDrilldownRequest,options?:CallOptions)=>call<ReadDrilldownSelection>('queries.drilldown',params,options),
    },
    devices:{list:read('devices.list'),get:read('devices.get'),outputs:read('devices.outputs'),history:read('devices.history')},
    classification:{categories:(params:ReadRequest<'classification.categories'>={},options?:CallOptions)=>read('classification.categories')(params,options)},
    map:{facilities:read('map.facilities'),features:read('map.features'),basemaps:(options?:CallOptions)=>read('map.basemaps')({},options),readTile:read('map.readTile')},
    statistics:{resources:read('statistics.resources'),structure:read('statistics.structure'),devices:read('statistics.devices'),deviceCategories:read('statistics.deviceCategories'),monitoring:read('statistics.monitoring'),subscribeResources:subscribeRead('statistics.resources'),subscribeStructure:subscribeRead('statistics.structure'),subscribeDevices:subscribeRead('statistics.devices'),subscribeDeviceCategories:subscribeRead('statistics.deviceCategories'),subscribeMonitoring:subscribeRead('statistics.monitoring')},
  };
}
