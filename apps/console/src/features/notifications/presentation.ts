import type { NotificationItem } from '@infrapro/api-client';
import { categories, currentStates } from './labels';

// Only use currently disclosed fields, never older task/list data or the route.
export function notificationHeading(item: NotificationItem) {
  return !item.redacted && item.object_name ? item.object_name : item.title;
}

export function notificationSource(item: NotificationItem) {
  return item.redacted ? '个人回执' : item.scope_name || categories[item.category as keyof typeof categories] || '账号通知';
}

export function notificationOutcome(item: NotificationItem) {
  const extra: Record<string, string> = { adopted: '已采用', recovered: '已恢复' };
  return currentStates[item.outcome] || extra[item.outcome] || '状态已更新';
}

export function notificationDay(value: string) {
  const date = new Date(value), today = new Date(), yesterday = new Date();
  yesterday.setDate(today.getDate() - 1);
  if (date.toDateString() === today.toDateString()) return '今天';
  if (date.toDateString() === yesterday.toDateString()) return '昨天';
  return date.toLocaleDateString('zh-CN');
}

export function notificationTime(value: string) {
  return new Date(value).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false });
}
