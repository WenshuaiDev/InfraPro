import { useRef } from 'react';
import { requestUUID } from '../tenancy/membership-utils';
// An uncertain write can be retried explicitly with the same receipt. Changed payloads
// get a fresh ID; no write is automatically replayed after reconnect or CSRF refresh.
export function useRequestMeta() {
 const attempts = useRef(new Map<string, { payload: string; id: string }>());
 return (operation: string, payload: unknown, reason: string) => { const serialized = JSON.stringify([payload, reason]); let attempt = attempts.current.get(operation); if (attempt?.payload !== serialized) { attempt = { payload: serialized, id: requestUUID() }; attempts.current.set(operation, attempt); } return { request_id: attempt.id, reason }; };
}
