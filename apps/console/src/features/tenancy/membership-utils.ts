export const memberStatus: Record<string, string> = { active: '有效', suspended: '已暂停', ended: '已结束' };
export const invitationStatus: Record<string, string> = { pending: '待接受', accepted: '已接受', expired: '已过期', revoked: '已撤销', replaced: '已替换', invalidated: '资格已失效' };
export const membershipDate = (value: string) => new Date(value).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false });
export function requestUUID() { const b = crypto.getRandomValues(new Uint8Array(16)); b[6] = (b[6]! & 15) | 64; b[8] = (b[8]! & 63) | 128; const h = Array.from(b, (v) => v.toString(16).padStart(2, '0')).join(''); return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`; }
