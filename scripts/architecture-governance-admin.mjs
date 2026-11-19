import assert from 'node:assert/strict';
import { syntheticTOTP } from '../tests/helpers/totp.ts';
const reason='配对治理权限变更';
const token=v=>new URLSearchParams(new URL(v).hash.slice(1)).get('token');
const t='/tenants/:tenant_id',p=t+'/projects/:project_id',r=t+'/roles/:role_id',pr=p+'/roles/:role_id';
const effect=(name,ok)=>{ assert.ok(ok,name);return {[name]:true}; };
export async function extendedGovernance(c){
  if(c.suite==='governance-security')return security(c);
  if(c.suite==='governance-roles')return scoped(c);
  throw Error('Unknown governance suite');
}
async function security({admin,owner,peer,member,person,client,call,post,get,login,tag}){
  await call('account_profile_write',member,'PATCH','/account/profile',{}, {display_name:'治理用户 '+tag});
  await get('account_profile_after_write',member,'/account/profile',{}, {effect:v=>effect('changed_name',JSON.stringify(v).includes('治理用户 '+tag))});
  const avatar=(await get('avatar_before_remove',member,'/auth/me')).account;
  await call('remove_own_avatar',member,'DELETE','/account/avatar',{},undefined,{query:'?expected_revision='+avatar.avatar_revision});
  const changed='配对密码 '+tag+' !Q7';
  await post('own_password_change',member,'/account/password',{}, {current_password:person.member.password,new_password:changed});
  const fresh=await client();await login('changed_login',fresh,{username:person.member.username,password:changed});
  const note=reason+' '+tag;
  const invite=await post('extra_invite',admin,'/invitations',{}, {note});
  const pendingInvitation=async label=>{
    const page=await get(label,admin,'/invitations',{}, {query:'?q='+encodeURIComponent(note)+'&status=pending'});
    const match=page.items.filter(v=>v.note===note&&v.status==='pending');
    assert.equal(match.length,1);return {id:match[0].id};
  };
  const inv=await post('rotate_invitation',admin,'/invitations/:id/regenerate',await pendingInvitation('original_invite'),undefined);
  await post('revoked_link',admin,'/invitations/:id/revoke',await pendingInvitation('replacement_invite'),undefined);
  assert.notEqual(token(inv.url),token(invite.url));
  const policy=await get('read_current_policy',admin,'/platform/security-policy');
  await call('save_current_policy',admin,'PUT','/platform/security-policy',{}, {require_mfa:policy.require_mfa,expected_version:policy.revision,reason});
  const role=await post('new_neutral_role',admin,'/access/roles',{}, {name:'治理 '+tag,description:reason,reason});
  let current=await call('edit_neutral_role',admin,'PATCH','/access/roles/:id',{id:role.id},{name:'改名 '+tag,description:reason,expected_version:role.version,reason});
  for(const status of ['disabled','active'])current=await call('role_'+status,admin,'PUT','/access/roles/:id/status',{id:role.id},{expected_version:current.version,status,reason});
  await post('archive_neutral_role',admin,'/access/roles/:id/archive',{id:role.id},{expected_version:current.version,reason});
  const account={id:peer.id};
  for(const action of ['disable','enable']){
    const target=await get('account_before_'+action,admin,'/access/accounts/:id',account);
    await post('account_'+action,admin,'/access/accounts/:id/'+action,account,{expected_version:target.management_version,reason});
  }
  let f=await get('factor_state',owner,'/account/mfa');
  let setup=await post('factor_setup_cancel',owner,'/account/mfa/enrollment/start',{}, {mode:'enable',expected_factor_version:f.factor_version});
  await post('cancel_factor_setup',owner,'/account/mfa/enrollment/cancel',{}, {enrollment_id:setup.enrollment_id});
  setup=await post('factor_setup_again',owner,'/account/mfa/enrollment/start',{}, {mode:'enable',expected_factor_version:f.factor_version});
  await post('new_factor',owner,'/account/mfa/enrollment/confirm',{}, {enrollment_id:setup.enrollment_id,new_totp_code:syntheticTOTP(setup.secret)});
  await post('mfa_sensitive_proof',owner,'/auth/reauthenticate',{}, {password:person.owner.password},{status:[204]});
  f=await get('factor_generation',owner,'/account/mfa');
  const replacement=await post('regenerate_recovery_codes',owner,'/account/mfa/recovery-codes/regenerate',{}, {expected_factor_version:f.factor_version,expected_recovery_generation:f.recovery_generation},{effect:v=>effect('ten_replacements',v.recovery_codes.length===10)});
  const challenger=await client(),pending=await login('challenge_to_cancel',challenger,person.owner,{status:[202]});
  await post('cancel_mfa_challenge',challenger,'/auth/mfa/cancel',{}, {challenge_id:pending.challenge_id});
  const target=await get('before_admin_recovery',owner,'/account/mfa');
  const grant=await post('grant_personnel_recovery',admin,'/access/accounts/:id/mfa-reset',{id:owner.id},{expected_factor_version:target.factor_version,reason});
  const recovery=await client(),pendingRecover=await login('recovery_password',recovery,person.owner,{status:[202]});
  const claimed=await post('claim_recovery',recovery,'/auth/mfa/recovery/claim',{}, {challenge_id:pendingRecover.challenge_id,recovery_token:token(grant.recovery_url)},{status:[202]});
  const next=await post('forced_enrollment',recovery,'/auth/mfa/enrollment/start',{}, {challenge_id:claimed.challenge_id});
  await post('confirm_forced_enrollment',recovery,'/auth/mfa/enrollment/confirm',{}, {challenge_id:claimed.challenge_id,new_totp_code:syntheticTOTP(next.secret)},{effect:v=>effect('restored_access',v.status==='authenticated')});
  await post('new_factor_proof',recovery,'/auth/reauthenticate',{}, {password:person.owner.password},{status:[204]});
  const currentFactor=await get('before_mfa_disable',recovery,'/account/mfa');
  await post('disable_own_factor',recovery,'/account/mfa/disable',{}, {expected_factor_version:currentFactor.factor_version});
  await get('mfa_disabled',recovery,'/account/mfa',{}, {effect:v=>effect('factor_disabled',v.state==='disabled')});
  assert.ok(replacement);
}
async function scoped({admin,owner,peer,member,person,call,post,get,key,tag,emergencyDisable}){
  await get('unscoped_tenant_admin_candidate',admin,'/platform/tenant-admin-candidates',{},{
    query:'?q='+encodeURIComponent(person.owner.username),
    effect:page=>{
      const expected={id:owner.id,username:person.owner.username,display_name:'架构 owner',
        replaced_roles:[],has_avatar:false,avatar_revision:''};
      assert.deepEqual(page.items,[expected]);
      return {fields:Object.keys(expected).sort(),matchesRequestedAccount:true,
        hasAvatar:page.items[0].has_avatar,avatarRevision:page.items[0].avatar_revision,
        replacedRoles:page.items[0].replaced_roles};
    },
  });
  const tenantBody={name:'组织治理 '+tag,code:'gov-'+tag,administrator_account_id:owner.id,client_request_id:key('tenant_scope'),reason};
  const tenant=await post('create_governance_tenant',admin,'/platform/tenants',{},tenantBody);
  const tv={tenant_id:tenant.tenant_id},platform='/platform/tenants/:tenant_id';
  await get('eligible_tenant_admin',admin,'/platform/tenant-admin-candidates',{}, {query:'?q='+person.owner.username+'&tenant_id='+tv.tenant_id});
  let current=await get('tenant_before_edit',admin,platform,tv);
  await call('edit_tenant_profile',admin,'PATCH',platform,tv,{name:'已修订 '+tag,reason,expected_version:current.version});
  const own=await get('owner_membership',owner,t+'/context',tv);
  const memberships={owner:own.membership_id};
  for(const [label,user]of [['member',member],['peer',peer]]){
    await post(label+'_resolve',owner,t+'/member-candidates/resolve',tv,{username:person[label].username});
    const invite=await post(label+'_invite_workplace',owner,t+'/member-invitations',tv,{target_account_id:user.id,client_request_id:key(label+'_work'),reason});
    const iv={...tv,invitation_id:invite.invitation.id};
    const prior=await get(label+'_invitation_detail',owner,t+'/member-invitations/:invitation_id',iv);
    const rotated=await post(label+'_rotate_workplace_invite',owner,t+'/member-invitations/:invitation_id/regenerate',iv,{expected_version:invite.invitation.version,client_request_id:key(label+'_rotate'),reason});
    const url=rotated.link;
    await post(label+'_inspect_workplace_invite',user,'/account/tenant-invitations/inspect',{}, {token:token(url)});
    const accepted=await post(label+'_accept_workplace_invite',user,'/account/tenant-invitations/accept',{}, {token:token(url),expected_version:rotated.invitation.version});
    assert.ok(prior);memberships[label]=accepted.membership_id;
  }
  const spare=await post('unredeemed_workplace_invite',owner,t+'/member-invitations',tv,{target_account_id:admin.id??(await get('platform_account',admin,'/auth/me')).account.id,
    client_request_id:key('unused_work'),reason});
  await post('revoke_workplace_invite',owner,t+'/member-invitations/:invitation_id/revoke',{...tv,invitation_id:spare.invitation.id},{expected_version:spare.invitation.version,reason});
  const tr=await post('scoped_role',owner,t+'/roles',tv,{name:'处室 '+tag,description:reason,permission_codes:['tenant.profile.read'],client_request_id:key('scope_role'),reason});
  const rv={...tv,role_id:tr.role.id};
  let role=await get('role_before_edit',owner,r,rv);
  await call('edit_tenant_role',owner,'PATCH',r,rv,{name:'安全处室 '+tag,description:reason,expected_version:role.version,reason});
  role=await get('role_before_copy',owner,r,rv);
  const copied=await post('copy_tenant_role',owner,r+'/copy',rv,{name:'处室备份 '+tag,description:reason,permission_codes:role.permission_codes,source_version:role.version,client_request_id:key('tenant_copy'),reason});
  async function tenantRoleChange(label,rv,suffix,method,body={}){
    const current=await get(label+'_role',owner,r,rv);
    const inputs={...body,expected_version:current.version};
    const prev=await post(label+'_preview',owner,r+suffix+'/preview',rv,inputs);
    return call(label,owner,method,r+suffix,rv,{...inputs,preview_hash:prev.preview_hash,reason});
  }
  await tenantRoleChange('set_tenant_role_permissions',rv,'/permissions','PUT',{permission_codes:[]});
  for(const status of ['disabled','active'])await tenantRoleChange('tenant_role_'+status,rv,'/status','PUT',{status});
  await tenantRoleChange('retire_tenant_role',{...tv,role_id:copied.role.id},'/archive','POST');
  for(const target of ['owner','administrator'])await get('tenant_project_candidate_'+target,owner,t+'/project-candidates',tv,{query:'?q='+person.peer.username});
  const creation=await post('project_in_workplace',owner,t+'/projects',tv,{name:'治理项目 '+tag,code:'G-'+tag,purpose:reason,description:reason,owner_tenant_membership_id:memberships.owner,administrator_tenant_membership_id:memberships.owner,client_request_id:key('create_project'),reason});
  const pv={...tv,project_id:creation.project_id},field={name:'修订项目 '+tag,code:'G-'+tag,purpose:reason,description:reason};
  async function mutation(label,user,template,params,data={},method='POST'){
    const prev=await post(label+'_preview',user,template+'/preview',params,data,{effect:v=>effect('change_allowed',v.can_submit)});
    return call(label,user,method,template,params,{...data,expected_version:prev.expected_version,expected_project_version:prev.expected_project_version,
      preview_hash:prev.preview_hash,client_request_id:key(label),reason});
  }
  await call('amend_project',owner,'PATCH',p+'/profile',pv,{...field,expected_version:creation.project_version,expected_project_version:creation.project_version,client_request_id:key('amend_project'),reason});
  for(const target of ['administrator','member','owner'])await get('project_'+target+'_candidates',owner,p+'/'+target+'-candidates',pv,{query:'?q='+person.peer.username});
  const gov=t+'/project-governance/:project_id';
  await get('project_governance_candidates',owner,gov+'/administrator-candidates',pv,{query:'?q='+person.peer.username});
  let project=(await get('project_manage_context',owner,p+'/context',pv)).project;
  const projectRole=await post('create_project_role_ext',owner,p+'/roles',pv,{name:'项目细分 '+tag,description:reason,permission_codes:['project.profile.read'],expected_project_version:project.version,client_request_id:key('project_role'),reason});
  const prv={...pv,role_id:projectRole.object_id};
  let scope=await get('project_role_detail',owner,pr,prv);
  await call('rename_project_role',owner,'PATCH',pr,prv,{name:'项目修订 '+tag,description:reason,expected_version:scope.version,expected_project_version:projectRole.project_version,client_request_id:key('rename_project_role'),reason});
  scope=await get('scope_before_copy',owner,pr,prv);
  project=(await get('project_before_copy',owner,p+'/context',pv)).project;
  const clone=await post('copy_project_role',owner,pr+'/copy',prv,{name:'备份 '+tag,description:reason,source_version:scope.version,permission_codes:scope.permission_codes,expected_project_version:project.version,client_request_id:key('copy_project_role'),reason});
  await mutation('update_project_permissions',owner,pr+'/permissions',prv,{permission_codes:[]},'PUT');
  for(const status of ['disabled','active'])await mutation('project_role_'+status,owner,pr+'/status',prv,{status});
  await mutation('delete_unused_project_role',owner,pr+'/delete',{...pv,role_id:clone.object_id});
  project=(await get('project_before_member_ext',owner,p+'/context',pv)).project;
  const membership=await post('add_project_member_ext',owner,p+'/members',pv,{expected_project_version:project.version,client_request_id:key('pm_ext'),tenant_membership_id:memberships.member,reason});
  const mv={...pv,membership_id:membership.object_id},memberPath=p+'/members/:membership_id';
  for(const action of ['suspend','resume','remove'])await mutation('project_membership_'+action,owner,memberPath+'/'+action,mv);
  await mutation('project_owner_change',owner,p+'/owner',pv,{tenant_membership_id:memberships.peer});
  await mutation('peer_administrator',owner,p+'/administrator-transfer',pv,{tenant_membership_id:memberships.peer});
  const membershipDetail=await get('tenant_member_before_suspend',owner,t+'/members/:membership_id/authorization',{...tv,membership_id:memberships.peer});
  await post('suspend_admin_membership',owner,t+'/members/:membership_id/suspend',{...tv,membership_id:memberships.peer},{expected_version:membershipDetail.membership_version,reason},{status:[409]});
  await emergencyDisable(person.peer.username);
  await mutation('recover_project_administrator',owner,gov+'/administrator-recovery',pv,{tenant_membership_id:memberships.owner});
  const removable=await get('tenant_member_before_remove',owner,t+'/members/:membership_id/authorization',{...tv,membership_id:memberships.member});
  await post('remove_tenant_member',owner,t+'/members/:membership_id/remove',{...tv,membership_id:memberships.member},{expected_version:removable.membership_version,reason});
}
