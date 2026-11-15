import type { ProjectMember } from '@infrapro/api-client';

export const memberOperations = {
  'member.roles': { label: '配置项目角色', description: '选择项目角色，预览权限变化后填写原因并确认。', permission: 'ProjectMemberRoles' },
  'member.suspend': { label: '暂停项目成员', description: '暂停后，该成员暂时无法进入项目，原有角色绑定保留。', permission: 'ProjectMemberSuspend' },
  'member.resume': { label: '恢复项目成员', description: '恢复本次项目成员关系；实际权限仍取决于账号、租户资格及角色状态。', permission: 'ProjectMemberResume' },
  'member.remove': { label: '移除项目成员', description: '结束本次项目关系并撤销当前角色绑定，历史记录保留。', permission: 'ProjectMemberRemove' },
} as const;

export type MemberOperation = keyof typeof memberOperations;
export type MemberMode = 'details' | MemberOperation;

export function availableMemberOperations(member: ProjectMember, viewer: string, allows: (operation: string) => boolean, canReadAuthorization: boolean): MemberOperation[] {
  if (member.is_current_administrator || member.account_id === viewer || member.status === 'ended') return [];
  return (Object.keys(memberOperations) as MemberOperation[]).filter((operation) => {
    if (!allows(memberOperations[operation].permission)) return false;
    if (operation === 'member.roles') return canReadAuthorization;
    if (operation === 'member.suspend') return member.status === 'active';
    if (operation === 'member.resume') return member.status === 'suspended';
    return true;
  });
}

export function projectMemberDate(value: string, full = false) {
  if (!value) return '—';
  return new Date(value).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit', ...(full ? { hour: '2-digit', minute: '2-digit' } as const : {}) });
}
