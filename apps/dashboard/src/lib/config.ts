/** Runtime configuration sourced from Vite env. */
export const config = {
  apiUrl: import.meta.env.VITE_API_URL || '/api/v1',
  wsUrl: import.meta.env.VITE_WS_URL || '/ws',
};

const TOKEN_KEY = 'mc.token';

/** Read/write the JWT from localStorage. */
export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) || '';
}
export function setToken(token: string): void {
  if (token) localStorage.setItem(TOKEN_KEY, token);
  else localStorage.removeItem(TOKEN_KEY);
}

/** Absolute WebSocket URL for the dashboard role, carrying the JWT. */
export function dashboardWsUrl(): string {
  const base = config.wsUrl;
  let url: string;
  if (base.startsWith('ws://') || base.startsWith('wss://')) {
    url = base;
  } else {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    url = `${proto}//${window.location.host}${base}`;
  }
  const u = new URL(url);
  u.searchParams.set('role', 'dashboard');
  const token = getToken();
  if (token) u.searchParams.set('token', token);
  return u.toString();
}
