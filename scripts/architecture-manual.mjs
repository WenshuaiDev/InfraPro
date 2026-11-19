import assert from 'node:assert/strict';
import { randomBytes,randomUUID } from 'node:crypto';
import { existsSync,linkSync,unlinkSync } from 'node:fs';
import { join,resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { client,queryPG,read,runDirectory,write } from './architecture-lab.mjs';
import { navigationInputs } from './architecture-navigation.mjs';

const reason='本地架构验收项目访问样例';
const command=()=>({client_request_id:randomUUID(),reason});
const fragment=link=>new URLSearchParams(new URL(link).hash.slice(1)).get('token');
const id=value=>{assert.match(value,/^[a-f0-9-]{36}$/);return value;};
export async function seedWalkthroughGovernance({dir,side,lab,structure:s,accounts,credentialDirectory=dir}){
  const samples=read(join(dir,'samples.json'));
  const credentials=join(credentialDirectory,'private/manual-project-account.json');
  if(!existsSync(credentials)){
    const prepared=credentials+'.'+randomUUID();
    write(prepared,{username:'arc_field_'+lab.id,password:'现场!2'+randomBytes(24).toString('base64url')});
    try{linkSync(prepared,credentials);}catch(e){if(e.code!=='EEXIST')throw e;}finally{unlinkSync(prepared);}
  }
  const person=read(credentials),name='架构验收项目查阅员';
    const cfg=lab[side],admin=await client(cfg,accounts.admin),guest=await client(cfg);
    let viewer,pendingMember;
    try{
      let account=(await admin.call('/access/accounts?q='+person.username)).items.find(v=>v.username===person.username);
      if(!account){
        const invitation=await admin.call('/invitations',{note:reason});
        await guest.call('/public/invitations/accept',{...person,display_name:name,token:fragment(invitation.url)});
        account=(await admin.call('/access/accounts?q='+person.username)).items.find(v=>v.username===person.username);
      }
      id(account.id);viewer=await client(cfg,person);
      const tenant='/tenants/'+s.tenant,proj=tenant+'/projects/'+s.project;
      const sql=q=>queryPG(lab,side,q);
      let tenantMember=sql("SELECT id FROM tenant_memberships WHERE tenant_id='"+s.tenant+"' AND account_id='"+account.id+"'");
      if(!tenantMember){
        const invitation=await admin.call(tenant+'/member-invitations',{...command(),target_account_id:account.id});
        const accepted=await viewer.call('/account/tenant-invitations/accept',{token:fragment(invitation.link),expected_version:invitation.invitation.version});
        tenantMember=id(accepted.membership_id);
      }
      let member=sql("SELECT id FROM project_memberships WHERE project_id='"+s.project+"' AND tenant_membership_id='"+id(tenantMember)+"'");
      const projectVersion=async()=>(await admin.call(proj+'/context')).project.version;
      if(!member)member=id((await admin.call(proj+'/members',{...command(),expected_project_version:await projectVersion(),tenant_membership_id:tenantMember})).object_id);
      let role=sql("SELECT id FROM roles WHERE owner_project_id='"+s.project+"' AND name='"+name+"'");
      const permissions=['project.profile.read','project.engineering.resource.read','project.engineering.resource.history.read','project.monitoring.data.read'];
      if(!role)role=id((await admin.call(proj+'/roles',{...command(),expected_project_version:await projectVersion(),name,description:reason,permission_codes:permissions})).object_id);
      const auth=await admin.call(proj+'/members/'+member+'/authorization'),path=proj+'/members/'+member+'/roles';
      if(!auth.roles.some(r=>r.id===role)){
        const assignment={role_ids:[...auth.roles.map(v=>v.id),role]};
        const p=await admin.call(path+'/preview',assignment);assert(p.can_submit);
        await admin.call(path,{...command(),...assignment,expected_version:p.expected_version,expected_project_version:p.expected_project_version,role_versions:p.role_versions,preview_hash:p.preview_hash},'PUT');
      }
      const scope=await viewer.call(proj+'/context');
      assert(scope.can_enter&&!scope.authorization.is_current_administrator&&permissions.every(code=>scope.authorization.permissions.includes(code)));
      const accepted=await viewer.call(proj+'/monitoring/selections',{channel_ids:[s.channels.P1.id]});assert.equal(accepted.channel_count,'1');
      const permitted=await viewer.call(proj+'/monitoring/queries/details',{selection_id:accepted.id,all_history:true,limit:20,include_total:true});assert.equal(Number(permitted.total),10);
      const prohibited=await viewer.client.post(viewer.api+'/tenants/'+samples.secondTenant+'/projects/'+samples.secondProject+'/monitoring/selections',{headers:viewer.headers,data:{channel_ids:[s.channels.P1.id]}});
      assert([403,404].includes(prohibited.status()));
      const invitations=tenant+'/member-invitations',memberID=id(samples.actors.member);
      const held=sql("SELECT id FROM tenant_invitations WHERE tenant_id='"+s.tenant+"' AND target_account_id='"+memberID+"' AND status='pending' AND expires_at>clock_timestamp() ORDER BY created_at DESC LIMIT 1");
      const invite=held||(await admin.call(invitations,{...command(),target_account_id:memberID})).invitation.id;
      pendingMember=await client(cfg,accounts.member);
      const received = await pendingMember.call('/me/tenant-invitations/' + invite);
      const issued = await admin.call(invitations + '/' + invite);
      assert.equal(received.invitation_id, invite);
      assert.equal(issued.id, invite);
      assert.equal(received.status, 'pending');
      assert.equal(issued.status, 'pending');
      assert.equal(received.can_accept, true);
      assert.equal(issued.target.id, memberID);
      assert.equal(issued.tenant_id, s.tenant);
      assert.equal(issued.version, received.version);
      assert.equal(issued.expires_at, received.expires_at);
      assert(Date.parse(issued.expires_at) > Date.now());
      const history=await navigationInputs(dir,side);
      const result={credentialFile:'private/manual-project-account.json',console:cfg.origin,diagnostics:'http://127.0.0.1:'+cfg.ports.diagnostics,runtime:cfg.runtime,
        scope:{tenant:s.tenant,project:s.project},limitedProjectViewer:{roles:scope.authorization.roles.map(v=>v.name),permissions,count:10,crossProjectStatus:prohibited.status()},
        sample:{facility:s.nodes.A.id,point:s.nodes.P1.id,channel:s.channels.P1.id,device:s.devices[0].id,monitoringBatch:s.batch},
        pendingInvitation: invite,
        invitationReadback: { status: received.status, canAccept: received.can_accept, expiresAt: received.expires_at, version: received.version },
        ...history};
      write(join(dir,'manual',side+'.json'),result);return result;
    }finally{await pendingMember?.client.dispose();await viewer?.client.dispose();await guest.client.dispose();await admin.client.dispose();}
}
export async function manualEnvironment(dir,selected=['a','self','b']){
  assert(selected.length>0&&new Set(selected).size===selected.length&&selected.every(side=>['a','self','b'].includes(side)));
  const lab=read(join(dir,'private/lab.json')),accounts=read(join(dir,'private/accounts.json')),samples=read(join(dir,'samples.json'));
  const file=join(dir,'manual/environments.json');
  const result=existsSync(file)?read(file):{credentialFile:'private/manual-project-account.json',sides:{}};
  for(const side of selected){
    result.sides[side]=await seedWalkthroughGovernance({dir,side,lab,structure:samples.structure,accounts});
    write(file,result);
  }
  return {...result,passed:selected.every(side=>result.sides[side]?.artifact)};
}
if(process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const result=await manualEnvironment(runDirectory(process.argv[2]));console.log('Interactive environments',result.passed);process.exitCode=result.passed?0:1;
}
