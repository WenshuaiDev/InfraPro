import { createHmac } from 'node:crypto';

// Independent test client, never imported by production authentication.
export function syntheticTOTP(secret: string, offset = 0) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
  let bits = ''; for (const char of secret) bits += alphabet.indexOf(char).toString(2).padStart(5, '0');
  const key = Buffer.from(bits.match(/.{8}/g)!.map((byte) => parseInt(byte, 2)));
  const counter = Buffer.alloc(8); counter.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 30_000) + offset));
  const mac = createHmac('sha1', key).update(counter).digest(); const index = mac[19]! & 15;
  return ((mac.readUInt32BE(index) & 0x7fffffff) % 1_000_000).toString().padStart(6, '0');
}
