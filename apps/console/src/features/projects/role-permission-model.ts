import { Permissions as P, type Permission } from '@infrapro/api-client';

export const projectPermissionSections = [
  { name: '项目资料', prefixes: ['project.profile.', 'project.attachments.'] },
  { name: '成员与角色', prefixes: ['project.member.', 'project.role.', 'project.permission.', 'project.authorization.'] },
  { name: '工程资源', prefixes: ['project.engineering.'] },
  { name: '设备与来源', prefixes: ['project.device.', P.ProjectMonitoringSourceManage] },
  { name: '监测数据', prefixes: ['project.monitoring.'] },
  { name: '派生与分析', prefixes: ['project.derived.'] },
  { name: '工程空间', prefixes: ['project.spatial.'] },
  { name: '项目大屏', prefixes: ['project.display.'] },
  { name: '审计记录', prefixes: ['project.audit.'] },
  { name: '项目治理', prefixes: ['project.'] },
];
export function projectPermissionGroup(permission: Permission) {
  return projectPermissionSections.find(section => section.prefixes.some(prefix => permission.code.startsWith(prefix)))?.name ?? permission.group ?? '其他权限';
}
