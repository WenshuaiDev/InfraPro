import type { Role } from '@infrapro/api-client';
import type { RoleAction } from '../access/role-table';
import type { useProject } from './context';
import type { ProjectRoleMode } from './role-form';

export const projectRoleTitles: Record<ProjectRoleMode, string> = { create: '创建项目角色', copy: '复制为本项目角色', update: '编辑名称与说明', permissions: '配置角色权限', status: '变更角色状态', delete: '删除项目角色' };
export function projectRoleActions(role: Role, ctx: ReturnType<typeof useProject>, open: (mode: ProjectRoleMode, trigger: HTMLButtonElement | null) => void): RoleAction[] {
  const actions: RoleAction[] = [];
  const add = (mode: ProjectRoleMode, label: string, destructive = false) => actions.push({ label, run: trigger => open(mode, trigger), disabled: !ctx.available, destructive });
  if (role.builtin_key !== 'project_admin' && ctx.allows('ProjectRoleCopy')) add('copy', '复制为本项目角色');
  if (!role.builtin_key) {
    if (ctx.allows('ProjectRoleUpdate')) add('update', '编辑名称与说明');
    if (ctx.allows('ProjectRolePermissions')) add('permissions', '配置角色权限');
    if (ctx.allows('ProjectRoleStatus') && (ctx.project.status === 'active' || role.status === 'active')) add('status', role.status === 'active' ? '停用角色' : '启用角色');
    if (ctx.allows('ProjectRoleDelete')) add('delete', '删除项目角色', true);
  }
  return actions;
}
