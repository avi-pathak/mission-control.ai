import { api } from './api';

/** Base64url (VAPID public key) → Uint8Array for PushManager.subscribe. */
export function urlBase64ToUint8Array(base64: string): Uint8Array {
  const padding = '='.repeat((4 - (base64.length % 4)) % 4);
  const b64 = (base64 + padding).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(b64);
  // Back the array with a plain ArrayBuffer so it satisfies BufferSource.
  const buffer = new ArrayBuffer(raw.length);
  const arr = new Uint8Array(buffer);
  for (let i = 0; i < raw.length; i++) arr[i] = raw.charCodeAt(i);
  return arr;
}

export type PushSupport = 'unsupported' | 'denied' | 'default' | 'granted';

/** Whether the browser supports service workers + Push. */
export function pushSupported(): boolean {
  return 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;
}

/** Current push state: unsupported, or the Notification permission value. */
export function pushSupport(): PushSupport {
  if (!pushSupported()) return 'unsupported';
  return Notification.permission as PushSupport;
}

/** Is there an active push subscription in this browser? */
export async function isSubscribed(): Promise<boolean> {
  if (!pushSupported()) return false;
  const reg = await navigator.serviceWorker.ready;
  const sub = await reg.pushManager.getSubscription();
  return !!sub;
}

/** Request permission, subscribe, and register with the server. Returns true on success. */
export async function enablePush(): Promise<boolean> {
  if (!pushSupported()) throw new Error('Push notifications are not supported in this browser.');

  const permission = await Notification.requestPermission();
  if (permission !== 'granted') return false;

  const { publicKey, enabled } = await api.pushVapidKey();
  if (!enabled || !publicKey) {
    throw new Error('Push notifications are not configured on this server.');
  }

  const reg = await navigator.serviceWorker.ready;
  let sub = await reg.pushManager.getSubscription();
  if (!sub) {
    sub = await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToUint8Array(publicKey) as BufferSource,
    });
  }

  const json = sub.toJSON() as { endpoint?: string; keys?: { p256dh?: string; auth?: string } };
  await api.pushSubscribe({
    endpoint: json.endpoint ?? '',
    keys: { p256dh: json.keys?.p256dh ?? '', auth: json.keys?.auth ?? '' },
  });
  return true;
}

/** Unsubscribe locally and on the server. */
export async function disablePush(): Promise<void> {
  if (!pushSupported()) return;
  const reg = await navigator.serviceWorker.ready;
  const sub = await reg.pushManager.getSubscription();
  if (sub) {
    const endpoint = sub.endpoint;
    await sub.unsubscribe().catch(() => {});
    await api.pushUnsubscribe(endpoint).catch(() => {});
  }
}
