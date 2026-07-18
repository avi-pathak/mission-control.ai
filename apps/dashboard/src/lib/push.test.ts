import { describe, expect, it } from 'vitest';
import { urlBase64ToUint8Array } from './push';

describe('urlBase64ToUint8Array', () => {
  it('decodes a base64url VAPID key to the right bytes', () => {
    // "hello" in base64url is "aGVsbG8" (no padding).
    const out = urlBase64ToUint8Array('aGVsbG8');
    expect(Array.from(out)).toEqual([104, 101, 108, 108, 111]);
  });

  it('handles url-safe chars (- and _)', () => {
    // Bytes [255, 224] → standard base64 "/+A=" → url-safe "_-A".
    const out = urlBase64ToUint8Array('_-A');
    expect(Array.from(out)).toEqual([255, 224]);
  });

  it('returns an ArrayBuffer-backed Uint8Array', () => {
    const out = urlBase64ToUint8Array('aGVsbG8');
    expect(out.buffer).toBeInstanceOf(ArrayBuffer);
    expect(out.byteLength).toBe(5);
  });
});
