import type { NotificationFilter } from '@infrapro/api-client';
const uuid = /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/i;
export function notificationSearch(value: Record<string, unknown>): NotificationFilter {
 const id = (key: string) => typeof value[key] === 'string' && uuid.test(value[key]) ? value[key] : undefined;
 return { status: ['all', 'unread', 'archived'].includes(String(value.status)) ? value.status as NotificationFilter['status'] : undefined, category: ['security', 'organization', 'tasks', 'updates'].includes(String(value.category)) ? value.category as NotificationFilter['category'] : undefined, scope: ['account', 'platform', 'tenant', 'project'].includes(String(value.scope)) ? value.scope as NotificationFilter['scope'] : undefined, tenant_id: id('tenant_id'), project_id: id('tenant_id') ? id('project_id') : undefined, cursor: typeof value.cursor === 'string' && /^\d{1,18}$/.test(value.cursor) ? value.cursor : undefined };
}
