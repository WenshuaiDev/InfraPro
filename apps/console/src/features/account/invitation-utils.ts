import type { Invitation } from '@infrapro/api-client';
export const invitationStatuses = { pending: '待使用', used: '已使用', expired: '已过期', revoked: '已撤销' };
const formatter = new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false });
export const invitationDate = (value: string) => formatter.format(new Date(value));
export const invitationTitle = (item: Invitation) => item.note || '未填写备注';
