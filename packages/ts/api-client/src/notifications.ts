import type { components } from './schema';
import type { MonitoringRequestOptions } from './monitoring';
type S<N extends keyof components['schemas']> = components['schemas'][N];
export type NotificationItem = S<'NotificationItem'>;
export type NotificationPage = S<'NotificationPage'>;
export type NotificationSummary = S<'NotificationSummary'>;
export type NotificationFilter = S<'NotificationFilter'>;
export type NotificationPreferences = S<'NotificationPreferences'>;
export type NotificationTarget = S<'NotificationTarget'>;
export type NotificationChange = S<'NotificationChange'>;
type Request = <T>(path: string, method?: string, body?: unknown, notify?: boolean, signal?: AbortSignal, options?: MonitoringRequestOptions) => Promise<T>;
export function notificationMethods(request: Request) {
 const query = (value: object) => new URLSearchParams(Object.entries(value).filter(([, v]) => v !== undefined && v !== '').map(([k, v]) => [k, String(v)])).toString();
 const id = encodeURIComponent;
 return {
  notifications: (filter: NotificationFilter = {}, signal?: AbortSignal) => request<NotificationPage>(`/me/notifications?${query(filter)}`, 'GET', undefined, true, signal),
  notificationSummary: (signal?: AbortSignal) => request<NotificationSummary>('/me/notifications/summary', 'GET', undefined, true, signal),
  notification: (value: string, signal?: AbortSignal) => request<NotificationItem>(`/me/notifications/${id(value)}`, 'GET', undefined, true, signal),
  changeNotification: (value: string, input: NotificationChange, signal?: AbortSignal) => request<NotificationItem>(`/me/notifications/${id(value)}`, 'PATCH', input, true, signal),
  readNotifications: (filter: NotificationFilter, watermark: string) => request<{ changed: number }>('/me/notifications/read-all', 'POST', { filter, watermark }),
  notificationTarget: (value: string, signal?: AbortSignal) => request<NotificationTarget>(`/me/notifications/${id(value)}/target`, 'GET', undefined, true, signal),
  notificationPreferences: (signal?: AbortSignal) => request<NotificationPreferences>('/me/notification-preferences', 'GET', undefined, true, signal),
  saveNotificationPreferences: (input: NotificationPreferences) => request<NotificationPreferences>('/me/notification-preferences', 'PUT', input),
  myTenantInvitations: (filter: { cursor?: string; limit?: number } = {}, signal?: AbortSignal) => request<{ items: S<'TenantInvitationInspection'>[]; next_cursor?: string }>(`/me/tenant-invitations?${query(filter)}`, 'GET', undefined, true, signal),
  myTenantInvitation: (value: string, signal?: AbortSignal) => request<S<'TenantInvitationInspection'>>(`/me/tenant-invitations/${id(value)}`, 'GET', undefined, true, signal),
  acceptMyTenantInvitation: (value: string, version: string) => request<S<'TenantAcceptanceResult'>>(`/me/tenant-invitations/${id(value)}/accept`, 'POST', { expected_version: version }),
 };
}
