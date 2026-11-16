import {createContext,useContext,useEffect,useMemo,useRef,useState,type ReactNode} from 'react';
import {useIdentity} from '@infrapro/api-client/react';
import {useQueryClient} from '@tanstack/react-query';
import type {ParsingScope,PermissionCode} from '@infrapro/api-client';
import {Permissions as P} from '@infrapro/api-client';
import {FormNotice} from '@infrapro/ui/patterns/feedback';
type Context={scope:ParsingScope;prefix:readonly unknown[];can:(p:PermissionCode)=>boolean;available:boolean};
const Context=createContext<Context|null>(null);
export function ParsingBoundary({scope,can,available=true,children}:{scope:ParsingScope;can:(p:PermissionCode)=>boolean;available?:boolean;children:ReactNode}){
 const identity=useIdentity(),cache=useQueryClient(),[epoch,setEpoch]=useState(0),stamp=JSON.stringify(scope),revision=useRef(identity.authorization_revision);
 const prefix=useMemo(()=>['parsing',identity.account?.id,stamp,identity.authorization_revision,identity.authorizationEpoch,epoch] as const,[identity.account?.id,stamp,identity.authorization_revision,identity.authorizationEpoch,epoch]);
 useEffect(()=>{const clear=()=>{void cache.cancelQueries({queryKey:['parsing']});cache.removeQueries({queryKey:['parsing']});setEpoch(v=>v+1);};const subs=[identity.client.onAuthorizationChange(value=>{if(!value||value!==revision.current){revision.current=value;clear();}}),identity.client.onTenantRestriction((_id,state)=>{if(state!=='active')clear();}),identity.client.onProjectRestriction(clear),identity.client.onDeviceRestriction(clear)];return()=>{subs.forEach(fn=>fn());void cache.cancelQueries({queryKey:prefix});cache.removeQueries({queryKey:prefix});};},[cache,identity.client,prefix]);
 return <Context value={{scope,prefix,can,available:available&&identity.phase==='authenticated'}}><div key={JSON.stringify(prefix)}>{children}</div></Context>;
}
// eslint-disable-next-line react-refresh/only-export-components
export function useParsing(){const c=useContext(Context),identity=useIdentity();if(!c)throw Error('ParsingBoundary required');const platform=c.scope.kind==='platform',project=c.scope.kind==='project';return {...identity,...c,canExecute:!project&&c.can(platform?P.ParsingTestExecute:P.TenantParsingTestExecute),canRead:!project&&c.can(platform?P.ParsingRunRead:P.TenantParsingRunRead),canRaw:!project&&c.can(platform?P.ParsingSampleRead:P.TenantParsingSampleRead),canMethods:!project&&c.can(platform?P.ParsingMethodRead:P.TenantParsingMethodRead)};}
// eslint-disable-next-line react-refresh/only-export-components
export function useParsingWrite(){const {prefix}=useParsing(),cache=useQueryClient(),[pending,setPending]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState('');const ids=useRef(new Map<string,string>());const meta=(key:string,input:unknown,reason='人工核对解析配置与证据')=>{const fingerprint=key+JSON.stringify(input);let id=ids.current.get(fingerprint);if(!id){id=crypto.randomUUID();ids.current.set(fingerprint,id);}return {request_id:id,reason};};const run=async(fn:()=>Promise<unknown>,message='操作已完成')=>{setPending(true);setError('');setNotice('');try{await fn();setNotice(message);await cache.invalidateQueries({queryKey:prefix});ids.current.clear();return true;}catch(e){setError(e instanceof Error?e.message:'操作失败，请重试。');return false;}finally{setPending(false);}};return {pending,error,notice,meta,run};}
export function WriteFeedback({value}:{value:ReturnType<typeof useParsingWrite>}){return <>{value.error&&<FormNotice error>{value.error}</FormNotice>}{value.notice&&<p role="status" className="text-sm text-muted-foreground">{value.notice}</p>}</>;}
export function ExampleNotice(){return <FormNotice>联调示例，非厂家测量协议。合成样本仅用于验证；业务质量未评定，结果未正式采用。</FormNotice>;}
