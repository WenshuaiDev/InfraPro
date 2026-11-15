import { SHA256 } from '../files/sha256';
/** No secure-context-only API: HTTP LAN and localhost have the same handshake. */
export function nonce(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(32)), (v) =>
    v.toString(16).padStart(2, "0"),
  ).join("");
}
export function sha256(input: string | Uint8Array): string { const hash=new SHA256();hash.update(typeof input==='string'?new TextEncoder().encode(input):input);return hash.digest(); }

export function requestUUID(): string {
 const bytes=crypto.getRandomValues(new Uint8Array(16));bytes[6]=(bytes[6]!&15)|64;bytes[8]=(bytes[8]!&63)|128;
 const h=Array.from(bytes,v=>v.toString(16).padStart(2,'0')).join('');return `${h.slice(0,8)}-${h.slice(8,12)}-${h.slice(12,16)}-${h.slice(16,20)}-${h.slice(20)}`;
}
