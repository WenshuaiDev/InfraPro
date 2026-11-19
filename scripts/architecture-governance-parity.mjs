// Paired governance journeys use only newly invited people and new workspaces.
// The evidence contains the canonical route and checked effects. Secret-bearing
// responses (invitations, sessions, TOTP, resets) remain in the test process.
import { execFileSync } from 'node:child_process';
import { createServer, request as forward } from 'node:http';
import { setTimeout as delay } from 'node:timers/promises';
import { createHash, randomBytes, randomUUID } from 'node:crypto';
import { isDeepStrictEqual } from 'node:util';
import { readFileSync, unlinkSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { request } from '@playwright/test';
import { read, root, runDirectory, write, processEnv } from './architecture-lab.mjs';
import { extendedGovernance } from './architecture-governance-admin.mjs';
import { pairedCandidate } from './architecture-http-sandbox.mjs';
import { syntheticTOTP } from '../tests/helpers/totp.ts';

const reason = '架构配对隔离验收';
const api = '/api/v1';
const tenant = '/tenants/:tenant_id', project = tenant+'/projects/:project_id';
const platformTenant = '/platform/tenants/:tenant_id';
const fragment = link => new URLSearchParams(new URL(link).hash.slice(1)).get('token');
const ensure = (ok, message) => { if (!ok) throw Error(message); };
const rows = v => Array.isArray(v) ? v : v.items;
const truth = (name, value) => { ensure(value,name); return {[name]:true}; };

// A real loopback proxy forwards the commit and withholds its response until
// the client times out. The client must recover using the same public command.
async function loseCommittedResponse(side, actor, body, command) {
  let resolveStatus, rejectStatus;
  const committed=new Promise((resolve,reject)=>{resolveStatus=resolve;rejectStatus=reject;});
  const proxy=createServer(incoming=>{
    const destination=new URL('/api/v1/platform/tenants',side.api);
    const upstream=forward(destination,{method:'POST',headers:{...incoming.headers,host:destination.host}},response=>{
      response.resume();
      response.once('end',()=>resolveStatus(response.statusCode));
    });
    upstream.once('error',rejectStatus);
    incoming.pipe(upstream);
  });
  await new Promise((resolve,reject)=>proxy.once('error',reject).listen(0,'127.0.0.1',resolve));
  try {
    const result=await actor.http.post('http://127.0.0.1:'+proxy.address().port+'/api/v1/platform/tenants',
      {headers:{...actor.headers,'X-Command-ID':command},data:body,timeout:800,maxRetries:0}).then(()=>false,e=>/timeout/i.test(e.message));
    ensure(result,'Client unexpectedly received the committed response');
    return await Promise.race([committed,delay(5000,0)]);
  } finally {
    await new Promise(resolve=>{ proxy.close(resolve); proxy.closeAllConnections(); });
  }
}

export async function governanceParity(directory, outputDirectory=directory, suite="governance") {
  const lab=read(join(directory,'private/lab.json')),accounts=read(join(directory,'private/accounts.json'));
  const base=read(join(directory,'baseline.json')), ids=new Map(), tag=randomBytes(5).toString('hex');
  const reportFile=join(directory,'http',suite+'.json');
  const contracts=new Set(readFileSync(join(root,'services/api/internal/routes/ownership.tsv'),'utf8').trimEnd().split('\n').map(l=>l.split('\t').slice(0,2).join(' ')));
  ensure(resolve(lab.directory)===resolve(directory)&&lab.container==='infrapro-arch-'+lab.id,'Invalid paired local lab');
  const key = label => { if(!ids.has(label))ids.set(label,randomUUID());return ids.get(label); };
  const person = Object.fromEntries(['owner','peer','member'].map(label=>[label,{username:'arc_'+label+'_'+tag,password:'Arc '+randomBytes(16).toString('hex')+' 河流!'}]));
  const output={started_at:new Date().toISOString(),baseline_commit:base.commit,checks:[],passed:false};
  const ledger = {a:new Map(),b:new Map()};
  const clients=[];
  async function exercise(which) {
    const side=lab[which], checks=ledger[which];
    const cfg=read(side.config);
    const psql=(query)=>execFileSync('docker',['exec','-i',lab.container,'psql','-X','--set','ON_ERROR_STOP=1','-qAt','-U','archtest','-d','arc_'+which],
      {input:query,encoding:'utf8',stdio:['pipe','pipe','pipe']}).trim();
    const validatedUUID=value=>{ensure(/^[a-f0-9]{8}-[a-f0-9]{4}-[0-9a-f]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/u.test(value),'Invalid local fixture identity');return value;};
    ensure(new URL(side.api).hostname==='127.0.0.1','Loopback A/B gateway required');
    async function client(state) {
      const http=await request.newContext({baseURL:side.api,timeout:15000,maxRedirects:0,storageState:state});
      clients.push(http);
      const response=await http.get(api+'/auth/csrf');
      ensure(response.status()===200,'CSRF fixture unavailable');
      const csrf=(await response.json()).token;await response.dispose();
      return {http,headers:{Origin:side.origin,'X-CSRF-Token':csrf}};
    }
    async function raw(c,method,path,body,command) {
      const response=await c.http.fetch(api+path,{method,headers:{...c.headers,'X-Command-ID':command},data:body});
      try { return {status:response.status(),v:response.status()===204?{}:await response.json()}; }
      finally { await response.dispose(); }
    }
    async function call(label,c,method,template,params={},body,options={}) {
      const path=template.replace(/:([a-z_]+)/g,(_,n)=>{ensure(params[n],'Missing parameter '+n);return encodeURIComponent(params[n]);})+(options.query??'');
      ensure(contracts.has(method+' '+api+template),'Undeclared route '+template);
      let received;
      try{received=await raw(c,method,path,body,options.command??key(label))}
      catch(error){
        const reason=/timeout/i.test(error.message)?'timeout':/ECONNRESET/.test(error.message)?'connection reset':'response unavailable';
        throw Error([which,label,method,template,reason].join(' '),{cause:error});
      }
      const {status,v}=received;
      const valid=options.status??[200,201,204];
      const record={method,template:api+template,status,code:status>=400?v.code??'missing_error_code':null,effects:{}};
      checks.set(label,record);
      ensure(valid.includes(status),which+' '+label+' returned '+status+' '+(record.code??'')+' '+Object.keys(v.fields??{}).join(','));
      record.effects=options.effect?options.effect(v):{};
      return v;
    }
    const get=(name,c,t,params,options)=>call(name,c,'GET',t,params,undefined,options);
    const post=(name,c,t,params,body,options)=>call(name,c,'POST',t,params,body,options);
    const login=(name,c,who,options)=>post(name,c,'/auth/login',{},who,options??{effect:v=>truth('authenticated',v.status==='authenticated')});
    const admin=await client();
    await login('operator_login',admin,accounts.admin);
    const users={};
    for(const [label,creds] of Object.entries(person)){
      const guest=await client();
      const invited=await post(label+'_invite',admin,'/invitations',{}, {note:reason},{effect:v=>truth('invite_issued',Boolean(fragment(v.url)))});
      const token=fragment(invited.url);
      await post(label+'_inspect',guest,'/public/invitations/inspect',{}, {token},{effect:v=>truth('unexpired',Date.parse(v.expires_at)>Date.now())});
      await post(label+'_accept',guest,'/public/invitations/accept',{}, {token,...creds,display_name:'架构 '+label},{status:[204]});
      const session=await login(label+'_login',guest,creds);
      users[label]={...guest,id:session.account.id};
    }
    const {owner,peer,member}=users;
    if(suite!=="governance") {
      const emergencyDisable=async(username)=>{
        ensure(username===person.peer.username,'Only the newly created peer may be disabled');
        const file=join(directory,'private',which,'emergency-cli-'+tag+'.json');
        write(file,Object.fromEntries(Object.entries(cfg).filter(([n])=>!n.startsWith('monitoring_')&&!['files','audit','display'].includes(n))));
        const cli=join(which==='a'?base.bin:pairedCandidate(directory).bin,'infrapro');
        try{execFileSync(cli,['account','disable','--username',username,'--reason',reason,'--emergency'],{env:{...processEnv(lab,which),INFRAPRO_CONFIG:file},stdio:'pipe'});}finally{unlinkSync(file);}
      };
      return extendedGovernance({suite,admin,owner,peer,member,person,client,call,post,get,login,key,tag,emergencyDisable});
    }
    const role=await post('platform_role_create',admin,'/access/roles',{}, {name:'ARC-'+tag,description:reason,reason},{effect:v=>truth('platform_role',v.binding_scope==='platform')});
    const assigned=await call('platform_role_permissions',admin,'PUT','/access/roles/:id/permissions',{id:role.id},{expected_version:role.version,permission_codes:['platform.audit.read'],reason},{effect:v=>truth('read_grant',v.permission_codes.includes('platform.audit.read'))});
    const target=await get('platform_member_get',admin,'/access/accounts/:id',{id:member.id});
    await call('platform_member_roles',admin,'PUT','/access/accounts/:id/roles',{id:member.id},{expected_version:target.management_version,role_ids:[...target.roles.map(r=>r.id),assigned.id],reason});
    await get('platform_effective_grant',member,'/auth/me',{}, {effect:v=>truth('effective_read',v.permissions.includes('platform.audit.read'))});
    const tenants=[],projects=[];
    for(const n of [1,2]){
      const create={name:'架构租户 '+tag+' '+n,code:'arc-'+tag+'-'+n,administrator_account_id:owner.id,client_request_id:key('tenant_tx_'+n),reason};
      let tenantID;
      if(n===1) {
        const template='/platform/tenants',cmd=key('tenant_concurrent');
        const replies=await Promise.all([raw(admin,'POST',template,create,cmd),raw(admin,'POST',template,create,cmd)]);
        replies.sort((x,y)=>x.status-y.status);tenantID=replies[0].v.tenant_id;
        for(let i=0;i<replies.length;i++){
          const {status,v}=replies[i]; const label='tenant_concurrent_'+i;
          ensure(status===[200,201][i] && v.tenant_id===tenantID && v.replayed===(i===0),'Concurrent tenant command did not replay once');
          checks.set(label,{method:'POST',template:api+template,status,code:null,effects:{single_fact:true,replayed:v.replayed}});
        }
        await post('tenant_retry_after_lost_response',admin,template,{},create,{command:cmd,status:[200],effect:v=>truth('same_tenant_after_retry',v.tenant_id===tenantID&&v.replayed)});
        await post('tenant_conflicting_payload',admin,template,{}, {...create,name:'架构冲突 '+tag},{command:cmd,status:[409],effect:v=>truth('idempotency_conflict',v.code==='idempotency_conflict')});
        await get('tenant_single_created_object',admin,template,{}, {query:'?q='+create.code,effect:v=>truth('exactly_one_matching_tenant',v.items.filter(t=>t.code===create.code&&t.id===tenantID).length===1)});
      } else {
        const cmd=key('unknown_commit');
        const upstream=await loseCommittedResponse(side,admin,create,cmd);
        ensure(upstream===201,'Time-out fixture did not commit a new tenant');
        checks.set('commit_response_timeout',{method:'POST',template:api+'/platform/tenants',status:upstream,code:null,effects:{client_received_response:false,client_timed_out:true}});
        tenantID=(await post('retry_after_timeout',admin,'/platform/tenants',{},create,{command:cmd,status:[200],effect:v=>truth('existing_command_recovered',v.replayed)})).tenant_id;
      }
      const createdID=validatedUUID(tenantID), adminID=validatedUUID(owner.id);
      const facts=psql(`SELECT (SELECT count(*) FROM account_security_events WHERE event_type='tenant.created' AND object_id='${createdID}' AND outcome='success'),
          (SELECT count(*) FROM notification_events n JOIN notification_recipients r ON r.event_id=n.id WHERE tenant_id='${createdID}' AND n.outcome='administrator_appointed' AND r.account_id='${adminID}'),
          (SELECT count(*) FROM tenants WHERE id='${createdID}' AND creation_request_id='${validatedUUID(create.client_request_id)}')`);
      ensure(facts==='1|1|1','Receipt, success audit or committed notification mismatch: '+facts);
      checks.get(n===1?'tenant_single_created_object':'retry_after_timeout').effects={...checks.get(n===1?'tenant_single_created_object':'retry_after_timeout').effects,creation_receipts:1,success_audits:1,appointment_notifications:1};
      const t={tenant_id:tenantID};tenants.push(t);
      const ownerState=await get('tenant_context_'+n,owner,tenant+'/context',t,{effect:v=>truth('owner_can_enter',v.can_enter&&v.tenant.id===tenantID)});
      if(n===1){
        const profile=tenant+'/profile', before=await get('tenant_profile_initial',owner,profile,t);
        const description='配对租户资料 '+tag;
        await call('tenant_profile_edit',owner,'PATCH',profile,t,{expected_version:before.version,description,reason},{status:[200]});
        const after=await get('tenant_profile_readback',owner,profile,t,{effect:v=>truth('partial_edit_persisted',v.description===description&&v.name===before.name&&v.code===before.code&&v.version!==before.version)});
        await call('tenant_profile_empty_edit',owner,'PATCH',profile,t,{expected_version:after.version,reason},{status:[400]});
      }
      const tenantRole=await post('tenant_role_'+n,owner,tenant+'/roles',t,{client_request_id:key('tenant_role_'+n),name:'治理角色 '+n,description:reason,permission_codes:['tenant.profile.read'],reason},{status:[201],effect:v=>truth('role_created',v.role.binding_scope==='tenant')});
      await get('tenant_role_details_'+n,owner,tenant+'/roles/:role_id',{...t,role_id:tenantRole.role.id},{effect:v=>truth('role_in_own_tenant',v.owner_tenant_id===tenantID)});
      let peerMembership;
      let projectAdministratorMembership=ownerState.membership_id;
      if(n===1) {
        const inv=await post('tenant_member_invite',owner,tenant+'/member-invitations',t,{target_account_id:peer.id,client_request_id:key('tenant_member_invite'),reason},{status:[201]});
        await get('tenant_invite_received',peer,'/me/tenant-invitations/:id',{id:inv.invitation.id},{effect:v=>truth('own_invitation',JSON.stringify(v).includes(inv.invitation.id))});
        const accepted=await post('tenant_member_accept',peer,'/account/tenant-invitations/accept',{}, {token:fragment(inv.link),expected_version:inv.invitation.version},{effect:v=>truth('accepted_in_tenant',v.tenant_id===tenantID)});
        peerMembership=accepted.membership_id;
        const mem={...t,membership_id:peerMembership};
        const suspended=await post('tenant_member_suspend',owner,tenant+'/members/:membership_id/suspend',mem,{expected_version:accepted.membership_version,reason},{effect:v=>truth('member_suspended',v.membership_status==='suspended')});
        await get('tenant_member_denied_while_suspended',peer,tenant+'/context',t,{status:[403,404,409]});
        await post('tenant_member_resume',owner,tenant+'/members/:membership_id/resume',mem,{expected_version:suspended.membership_version,reason},{effect:v=>truth('member_restored',v.membership_status==='active')});
        const auth=await get('tenant_member_authorization',owner,tenant+'/members/:membership_id/authorization',mem);
        const assignment={expected_membership_version:auth.membership_version,role_ids:[...auth.roles.map(r=>r.id),tenantRole.role.id]};
        const path=tenant+'/members/:membership_id/roles';
        const impact=await post('tenant_member_roles_preview',owner,path+'/preview',mem,assignment);
        await call('tenant_member_roles_update',owner,'PUT',path,mem,{...assignment,expected_membership_version:impact.target_version,role_versions:impact.role_versions,preview_hash:impact.preview_hash,reason});
        await get('tenant_member_role_effect',owner,tenant+'/members/:membership_id/authorization',mem,
          {effect:v=>truth('role_contributes_profile_read',v.roles.some(r=>r.id===tenantRole.role.id)&&v.permissions.includes('tenant.profile.read'))});
      } else {
        const inv=await post('second_tenant_member_invite',owner,tenant+'/member-invitations',t,{target_account_id:member.id,client_request_id:key('second_member'),reason},{status:[201]});
        const accepted=await post('second_tenant_invitation_accept',member,'/me/tenant-invitations/:id/accept',{id:inv.invitation.id},{expected_version:inv.invitation.version},{effect:v=>truth('accepted_by_recipient',v.tenant_id===tenantID)});
        projectAdministratorMembership=accepted.membership_id;
      }
      const created=await post('project_create_'+n,owner,tenant+'/projects',t,{name:'架构项目 '+n,code:'arc-'+tag+'-p'+n,purpose:reason,description:reason,
        owner_tenant_membership_id:ownerState.membership_id,administrator_tenant_membership_id:projectAdministratorMembership,client_request_id:key('project_'+n),reason});
      const p={...t,project_id:created.project_id};projects.push(p);
      const manager=n===2?member:owner;
      const pr=await post('project_role_create_'+n,manager,project+'/roles',p,{name:'项目角色 '+n,description:reason,permission_codes:['project.profile.read'],expected_project_version:created.project_version,client_request_id:key('project_role_'+n),reason});
      await get('project_role_details_'+n,manager,project+'/roles/:role_id',{...p,role_id:pr.object_id},{effect:v=>truth('bound_to_project',v.owner_project_id===p.project_id)});
      let projectAdmin=manager;
      if(n===1) {
        const pm=await post('project_member_add',owner,project+'/members',p,{expected_project_version:pr.project_version,client_request_id:key('pm'),tenant_membership_id:peerMembership,reason});
        const mem={...p,membership_id:pm.object_id};
        const auth=await get('project_member_authorization',owner,project+'/members/:membership_id/authorization',mem);
        const assignment={role_ids:[...auth.roles.map(r=>r.id),pr.object_id]},rolesPath=project+'/members/:membership_id/roles';
        const impact=await post('project_member_roles_preview',owner,rolesPath+'/preview',mem,assignment,{effect:v=>truth('assignable',v.can_submit)});
        await call('project_member_roles_update',owner,'PUT',rolesPath,mem,{...assignment,expected_version:impact.expected_version,expected_project_version:impact.expected_project_version,role_versions:impact.role_versions,preview_hash:impact.preview_hash,client_request_id:key('project_grant'),reason});
        await get('project_member_role_effect',peer,project+'/context',p,
          {effect:v=>truth('member_has_profile_read',v.authorization.roles.some(r=>r.id===pr.object_id)&&v.authorization.permissions.includes('project.profile.read'))});
        const input={tenant_membership_id:peerMembership};
        const preview=await post('project_transfer_preview',owner,project+'/administrator-transfer/preview',p,input,{effect:v=>truth('transfer_allowed',v.can_submit)});
        await post('project_transfer',owner,project+'/administrator-transfer',p,{...input,expected_version:preview.expected_version,expected_project_version:preview.expected_project_version,preview_hash:preview.preview_hash,client_request_id:key('pm_transfer'),reason});
        await get('new_project_administrator',peer,project+'/context',p,{effect:v=>truth('new_administrator',v.can_enter&&v.authorization.is_current_administrator&&v.authorization.account_id===peer.id)});
        projectAdmin=peer;
      }
      for(const action of ['archive','restore']) {
        const path=project+'/'+action;
        const preview=await post('project_'+action+'_preview_'+n,projectAdmin,path+'/preview',p,{}, {effect:v=>truth('can_submit',v.can_submit)});
        await post('project_'+action+'_'+n,projectAdmin,path,p,{expected_version:preview.expected_version,expected_project_version:preview.expected_project_version,preview_hash:preview.preview_hash,client_request_id:key('project_'+action+'_'+n),reason});
      }
      await get('project_restored_'+n,projectAdmin,project+'/context',p,{effect:v=>truth('restored_project',v.can_enter&&v.project.status==='active')});
    }
    await get('cross_tenant_project',peer,project+'/context',{...tenants[0],project_id:projects[1].project_id},{status:[403,404]});
    await get('nonmember_space_hidden',peer,tenant+'/context',tenants[1],{status:[403,404]});
    await get('member_workspaces',peer,'/me/workspaces',{}, {effect:v=>{ const body=JSON.stringify(v);return truth('only_granted_workspace_visible',body.includes(tenants[0].tenant_id)&&!body.includes(tenants[1].tenant_id)); }});
    const t=tenants[1],preview=await post('tenant_transfer_preview',admin,platformTenant+'/administrator-transfer/preview',t,{});
    await post('tenant_transfer',admin,platformTenant+'/administrator-transfer',t,{new_administrator_account_id:member.id,expected_version:preview.expected_version,project_preview_hash:preview.project_preview_hash,confirm_project_impact:preview.requires_confirmation,reason});
    await get('actual_tenant_administrator',admin,platformTenant,t,{effect:v=>truth('new_administrator',v.administrator.id===member.id)});
    for(const action of ['disable','enable']){
      const path=platformTenant+'/'+action,pr=await post('tenant_'+action+'_preview',admin,path+'/preview',t,{});
      await post('tenant_'+action,admin,path,t,{expected_version:pr.expected_version,preview_hash:pr.preview_hash,reason,...(action==='disable'?{tenant_notice:reason}:{})},{effect:v=>truth('status_changed',v.status===(action==='enable'?'active':'disabled'))});
    }

    // This member is disposable. The other accounts stay valid for concurrent
    // A/B suites; save old cookie jars in memory to test post-revocation use.
    await post('reauthenticate',member,'/auth/reauthenticate',{}, {password:person.member.password},{status:[204]});
    if(Date.now()%60000>56000) await delay(4100);
    for(let i=0;i<6;i++)await post('refresh_activity_'+i,member,'/auth/activity',{},undefined,{status:[204]});
    await post('session_activity_limit',member,'/auth/activity',{},undefined,{status:[429]});
    const m=await get('mfa_state_before',member,'/account/mfa');
    const start=await post('mfa_enroll',member,'/account/mfa/enrollment/start',{}, {mode:'enable',expected_factor_version:m.factor_version},{effect:v=>truth('enrollment_issued',Boolean(v.secret&&v.enrollment_id))});
    const enrollmentCode = syntheticTOTP(start.secret);
    const nextTOTPWindow = (Math.floor(Date.now() / 30_000) + 1) * 30_000;
    const enrolled=await post('confirm_factor',member,'/account/mfa/enrollment/confirm',{}, {enrollment_id:start.enrollment_id,new_totp_code:enrollmentCode},{effect:v=>truth('ten_recovery_codes',v.recovery_codes?.length===10)});
    const old=await client(await member.http.storageState());
    await post('logout_single',member,'/auth/logout',{},undefined,{status:[204]});
    const protectedRoutes=['/auth/me',tenant+'/projects/:project_id/facilities',tenant+'/projects/:project_id/monitoring/points', '/account/avatar'];
    // Each owner must preserve its established unauthenticated result.
    const byOwner=readFileSync(join(root,'services/api/internal/routes/ownership.tsv'),'utf8').trimEnd().split('\n').map(v=>v.split('\t'));
    const protectedByOwner=['identity-api','control-api','monitoring-query-api','transfer-api'].map(ownerID=>{
      const row=byOwner.find(([method,path,owner])=>owner===ownerID&&method==='GET'&&protectedRoutes.some(s=>api+s===path)) ??
        byOwner.find(([method,path,owner])=>owner===ownerID&&method==='GET'&&path.startsWith(api+'/tenants/:tenant_id/projects/:project_id/')&&!path.includes('*'));
      ensure(row,'No authority sample: '+ownerID);return [ownerID,row[1].slice(api.length)];
    });
    async function verifyRevoked(label,c) {
      for(const [own,tpl] of protectedByOwner){const params={...projects[0]};
        for(const [,p] of tpl.matchAll(/:([a-z_]+)/g))params[p]??=key('missing_'+p);
        await get(label+'_'+own,c,tpl,params,{status:[401]});
      }
    }
    await verifyRevoked('revoked_single',old);
    const challenge=await login('mfa_login',member,person.member,{status:[202],effect:v=>truth('challenge_without_session',v.status==='mfa_required'&&!v.account)});
    await get('pending_denied',member,'/auth/me',{}, {status:[401]});
    await get('mfa_challenge_inspect',member,'/auth/mfa/challenge',{}, {effect:v=>truth('same_challenge',v.challenge_id===challenge.challenge_id)});
    // The enrollment code has already been used. Wait for the next real counter.
    while (Date.now() < nextTOTPWindow) await delay(nextTOTPWindow - Date.now());
    await post('verify_totp',member,'/auth/mfa/verify',{}, {challenge_id:challenge.challenge_id,method:'totp',code:syntheticTOTP(start.secret)},{effect:v=>truth('factor_authenticated',v.status==='authenticated')});
    const other=await client(),next=await login('second_factor_challenge',other,person.member,{status:[202]});
    await post('verify_recovery',other,'/auth/mfa/verify',{}, {challenge_id:next.challenge_id,method:'recovery_code',code:enrolled.recovery_codes[0]},{effect:v=>truth('recovery_authenticated',v.status==='authenticated')});
    await get('remaining_codes',other,'/account/mfa',{}, {effect:v=>truth('one_code_used',v.recovery_codes_remaining===9)});
    const bad=await client(),again=await login('used_code_challenge',bad,person.member,{status:[202]});
    await post('recovery_code_reuse',bad,'/auth/mfa/verify',{}, {challenge_id:again.challenge_id,method:'recovery_code',code:enrolled.recovery_codes[0]},
      {status:[400],effect:v=>truth('used_code_rejected',v.code==='mfa_code_invalid')});
    await get('no_session_after_used_code',bad,'/auth/me',{}, {status:[401]});
    const sessions=await get('active_sessions',member,'/account/sessions',{}, {effect:v=>truth('current_and_second_session',rows(v).some(x=>x.current)&&rows(v).some(x=>!x.current))});
    const second=rows(sessions).find(x=>!x.current);
    await call('revoke_one_device',member,'DELETE','/account/sessions/:id',{id:second.id},undefined,{status:[204]});
    await verifyRevoked('revoked_device',other);
    const prior=await client(await member.http.storageState());
    await post('all_logout',member,'/auth/logout-all',{},undefined,{status:[204]});await verifyRevoked('revoked_all',prior);
    const resetCli=join(which==='a'?base.bin:pairedCandidate(directory).bin,'infrapro');
    // The existing real terminal helper captures the synthetic one-time output
    // into this process only. No URL or factor ever enters the result ledger.
    const accountConfig=join(directory,'private',which,'governance-cli-'+tag+'.json');
    const accountSettings=Object.fromEntries(Object.entries(cfg).filter(([name])=>!name.startsWith('monitoring_')&&!['files','display','audit'].includes(name)));
    write(accountConfig,accountSettings);
    let reset;
    try {
      reset=JSON.parse(execFileSync('python3',['tests/http/cli.py','reset',person.peer.username],{cwd:root,
        env:{...processEnv(lab,which),INFRAPRO_CONFIG:accountConfig,INFRAPRO_ACCEPTANCE_CLI:resetCli},stdio:['ignore','pipe','pipe'],encoding:'utf8'}));
    } finally {unlinkSync(accountConfig);}
    const guest=await client(),resetToken=fragment(reset.url);
    await post('reset_inspect',guest,'/public/password-resets/inspect',{}, {token:resetToken},{effect:v=>truth('reset_valid',Date.parse(v.expires_at)>Date.now())});
    const newPassword='Replacement '+tag+' 河谷! 8A';
    await post('complete_reset',guest,'/public/password-resets/complete',{}, {token:resetToken,password:newPassword},{status:[204]});
    await get('reset_revokes_session',peer,'/auth/me',{}, {status:[401]});
    await login('new_password_valid',guest,{username:person.peer.username,password:newPassword});
    await get('security_history',guest,'/account/security-events',{}, {effect:v=>truth('security_events_recorded',rows(v).length>0)});
    // Age just these two newly authenticated sessions. Drop only their cache
    // and activity entries to force the normal cold authority read.
    const redis=new URL(cfg.redis_url), prefix=cfg.redis_key_prefix;
    ensure(redis.hostname==='127.0.0.1'&&redis.port==='56379'&&prefix==='infrapro:arc-'+lab.id+'-'+which+':identity:v1:','Unexpected synthetic Redis namespace');
    for(const [label,column] of [['idle','last_activity_at'],['absolute','expires_at']]){
      const clock=await client();
      await login('clock_login_'+label,clock,{username:person.peer.username,password:newPassword});
      const state=await clock.http.storageState(),token=state.cookies.find(c=>c.name==='infrapro_session')?.value;
      ensure(token,'Missing login cookie for expiry test');
      const hash=createHash('sha256').update(token).digest('hex');
      const id=validatedUUID(psql(`UPDATE account_sessions SET ${column}=clock_timestamp()-interval '1 year' WHERE token_hash='${hash}' AND account_id='${validatedUUID(peer.id)}' RETURNING id::text`));
      execFileSync('docker',['compose','-f','deploy/compose/test.yaml','exec','-T','-e','REDISCLI_AUTH','redis','redis-cli','DEL',prefix+'session:'+hash,prefix+'activity:'+id],
        {cwd:root,env:{...process.env,REDISCLI_AUTH:decodeURIComponent(redis.password)},stdio:['ignore','pipe','pipe']});
      await verifyRevoked('expired_'+label,clock);
    }
  }
  let failure;
  try {
    const completed=await Promise.allSettled(['a','b'].map(exercise));
    failure=completed.filter(v=>v.status==='rejected').map(v=>{
      if(v.reason?.code||v.reason?.status) return 'Local fixture command or connection failed';
      const line=String(v.reason?.message??'Fixture rejected').split('\n')[0];
      return /^[a-zA-Z0-9_ ：:.|,/-]{1,160}$/.test(line)&&!line.includes('://')?line:'Local fixture assertion failed';
    }).join(' / ');
    for(const label of new Set([...ledger.a.keys(),...ledger.b.keys()])) {
      const a=ledger.a.get(label),b=ledger.b.get(label);
      output.checks.push({scenario:label,method:a?.method??b.method,template:a?.template??b.template,status:[a?.status,b?.status],
        errors:[a?.code,b?.code],effects:a?.effects??{},equal:!!a&&!!b&&isDeepStrictEqual(a,b)});
    }
    output.passed=!failure&&output.checks.every(v=>v.equal);
    if(failure)output.failure=failure.slice(0,300);
  } finally {
    await Promise.allSettled(clients.map(c=>c.dispose()));
    output.finished_at=new Date().toISOString();write(reportFile,output);
  }
  if(!output.passed)throw Error(failure||'A/B governance effects differ: '+output.checks.filter(c=>!c.equal).map(c=>c.scenario).join(','));
  publishGovernance(output,outputDirectory,suite);
  return {checks:output.checks.length,file:reportFile};
}
export function publishGovernance(output,dir,suite="governance") {
  ensure(output.passed&&output.checks.every(c=>c.equal),'A/B governance record incomplete');
  const baseline=output.baseline_commit;
  const routes=new Map(readFileSync(join(root,'services/api/internal/routes/ownership.tsv'),'utf8').trimEnd().split('\n').map(l=>{const [method,template,owner,operationId]=l.split('\t');return [method+' '+template,{owner,operationId}];}));
  const records=output.checks.map(c=>({...routes.get(c.method+' '+c.template),...c,differences:[],assertions:'status, canonical domain error and named business effects'}));
  const idem=/concurrent|retry_after|timeout|conflicting_payload|single_created_object/;
  const security=/^operator_login$|^owner_login$|^peer_login$|^member_login$|reauthenticate|refresh_activity|activity_limit|factor|mfa|code|pending_denied|logout|revoked|sessions|revoke_one_device|reset|password|security_history|expired_|clock_login/;
  const cases=[['ARC-003','身份、因素与撤销后四个 owner',r=>security.test(r.scenario)],['ARC-004','平台及两个租户、项目中的授权治理',r=>!security.test(r.scenario)&&!idem.test(r.scenario)],['ARC-006','命令并发、提交后超时、唯一事实与收据',r=>idem.test(r.scenario)]].map(([arc,name,test])=>({arc,name,passed:true,checks:records.filter(test).map(r=>r.scenario)}));
  write(join(dir,'http','scenarios',suite+'.json'),{scenario:suite,baseline_commit:baseline,inputs:'paired public sessions for new synthetic accounts and workspaces',records,failures:[]});
  write(join(dir,'http','workflows',suite+'.json'),{cases:cases.filter(c=>c.checks.length),baseline_commit:baseline,completed_at:output.finished_at});
}
if(process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const dir=runDirectory(process.argv[2]??process.env.INFRAPRO_ARCHITECTURE_RUN_DIR);
  console.log(await governanceParity(dir,process.argv[3]?resolve(process.argv[3]):dir,process.argv[4]??"governance"));
}
