import type { LogAppend, Machine, MetricSample, Session } from '@mc/protocol';
import { config, getToken, setToken } from './config';

/** Error thrown on a confirmed 401 (invalid/expired token). Distinct from
 *  network errors so callers only log out on real auth failures. */
export class UnauthorizedError extends Error {
  constructor() {
    super('unauthorized');
    this.name = 'UnauthorizedError';
  }
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const token = getToken();
  const res = await fetch(`${config.apiUrl}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...init?.headers,
    },
  });
  if (res.status === 401) {
    setToken('');
    throw new UnauthorizedError();
  }
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`${res.status}: ${body}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

// REST rows now share the wire shapes (lowercase fields, ts in millis).
export type LogLineRow = LogAppend;
export type MetricRow = MetricSample;

export interface AuthUser {
  id: string;
  email: string;
  role: string;
  orgId: string;
}
export interface AuthResp {
  token: string;
  user: AuthUser;
}
export interface FileRow {
  id: string;
  machineId: string;
  sessionId: string;
  name: string;
  size: number;
  contentType: string;
  createdAt: string;
}
export interface OrgInvite {
  token: string;
  email: string;
  role: string;
  expiresAt: number;
}

export interface CreateEnrollTokenResp {
  token: string;
  serverUrl: string;
  expiresAt: number;
  command: string;
}

export interface EnrollTokenRow {
  token: string;
  label: string;
  createdAt: number;
  expiresAt: number;
  status: 'active' | 'used' | 'expired' | 'revoked';
}

export const api = {
  machines: () => req<Machine[]>('/machines'),
  deleteMachine: (id: string, force = false) =>
    req(`/machines/${id}${force ? '?force=true' : ''}`, { method: 'DELETE' }),
  sessions: () => req<Session[]>('/sessions'),
  session: (id: string) => req<Session>(`/sessions/${id}`),
  stop: (id: string) => req(`/sessions/${id}/stop`, { method: 'POST' }),
  restart: (id: string) => req(`/sessions/${id}/restart`, { method: 'POST' }),
  logs: (id: string, after = 0, limit = 500) =>
    req<{ lines: LogLineRow[]; nextCursor: number }>(
      `/logs/${id}?after=${after}&limit=${limit}`,
    ),
  metrics: (id: string, windowMinutes = 60) =>
    req<MetricRow[]>(`/metrics/${id}?windowMinutes=${windowMinutes}`),

  createEnrollToken: (label: string, ttlMinutes?: number) =>
    req<CreateEnrollTokenResp>('/enroll-tokens', {
      method: 'POST',
      body: JSON.stringify({ label, ttlMinutes }),
    }),
  enrollTokens: () => req<EnrollTokenRow[]>('/enroll-tokens'),
  revokeEnrollToken: (token: string) =>
    req(`/enroll-tokens/${token}`, { method: 'DELETE' }),

  // Auth
  register: (email: string, password: string, orgName?: string) =>
    req<AuthResp>('/auth/register', {
      method: 'POST',
      body: JSON.stringify({ email, password, orgName }),
    }),
  login: (email: string, password: string) =>
    req<AuthResp>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    }),
  acceptInvite: (token: string, password: string) =>
    req<AuthResp>('/auth/accept-invite', {
      method: 'POST',
      body: JSON.stringify({ token, password }),
    }),
  me: () => req<{ user: AuthUser; org: { id: string; name: string; slug: string } }>('/me'),

  // Org admin
  orgUsers: () => req<AuthUser[]>('/org/users'),
  orgInvites: () => req<OrgInvite[]>('/org/invites'),
  createInvite: (email: string, role: string) =>
    req<{ token: string; link: string }>('/org/invites', {
      method: 'POST',
      body: JSON.stringify({ email, role }),
    }),
  revokeInvite: (token: string) => req(`/org/invites/${token}`, { method: 'DELETE' }),

  // Files
  files: (sessionId?: string) =>
    req<FileRow[]>(`/files${sessionId ? `?session=${sessionId}` : ''}`),
  fileDownloadUrl: (id: string) => `${config.apiUrl}/files/${id}?token=${getToken()}`,
};
