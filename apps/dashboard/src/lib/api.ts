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

/** Error carrying the server's HTTP status and error code (from the JSON
 *  {error:{code,message}} envelope) so callers can branch on it. */
export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
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
    let code = '';
    let message = body;
    try {
      const j = JSON.parse(body);
      code = j?.error?.code ?? '';
      message = j?.error?.message ?? body;
    } catch {
      /* non-JSON body */
    }
    throw new ApiError(res.status, code, message);
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
  platformAdmin?: boolean;
}
export interface PendingUser {
  id: string;
  email: string;
  createdAt: number;
}
export interface AdminOrg {
  id: string;
  name: string;
  slug: string;
}
export interface AdminMachine {
  id: string;
  hostname: string;
  orgId: string;
  orgName: string;
  os: string;
  arch: string;
  online: boolean;
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
  register: (email: string, password: string) =>
    req<{ status: string }>('/auth/register', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
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
  changePassword: (currentPassword: string, newPassword: string) =>
    req<void>('/me/password', {
      method: 'POST',
      body: JSON.stringify({ currentPassword, newPassword }),
    }),

  // Web Push (blocked-session notifications)
  pushVapidKey: () => req<{ publicKey: string; enabled: boolean }>('/push/vapid-key'),
  pushSubscribe: (sub: { endpoint: string; keys: { p256dh: string; auth: string } }) =>
    req<void>('/push/subscribe', { method: 'POST', body: JSON.stringify(sub) }),
  pushUnsubscribe: (endpoint: string) =>
    req<void>('/push/unsubscribe', { method: 'POST', body: JSON.stringify({ endpoint }) }),
  pushTest: () => req<{ sent: number }>('/push/test', { method: 'POST' }),

  // Org admin
  orgUsers: () => req<AuthUser[]>('/org/users'),
  setUserRole: (id: string, role: string) =>
    req(`/org/users/${id}`, { method: 'PATCH', body: JSON.stringify({ role }) }),
  removeUser: (id: string) => req(`/org/users/${id}`, { method: 'DELETE' }),
  orgInvites: () => req<OrgInvite[]>('/org/invites'),
  createInvite: (email: string, role: string) =>
    req<{ token: string; link: string; emailed: boolean }>('/org/invites', {
      method: 'POST',
      body: JSON.stringify({ email, role }),
    }),
  revokeInvite: (token: string) => req(`/org/invites/${token}`, { method: 'DELETE' }),

  // Platform admin (superadmin): signup approval queue.
  pendingUsers: () => req<PendingUser[]>('/admin/pending-users'),
  adminOrgs: () => req<AdminOrg[]>('/admin/orgs'),
  approveUser: (id: string, body: { orgId?: string; newOrgName?: string; role: string }) =>
    req<{ status: string; orgId: string; role: string }>(`/admin/users/${id}/approve`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  rejectUser: (id: string) => req(`/admin/users/${id}`, { method: 'DELETE' }),
  adminMachines: () => req<AdminMachine[]>('/admin/machines'),
  reassignMachine: (id: string, orgId: string) =>
    req<{ id: string; orgId: string }>(`/admin/machines/${id}/reassign`, {
      method: 'POST',
      body: JSON.stringify({ orgId }),
    }),

  // Files
  files: (sessionId?: string) =>
    req<FileRow[]>(`/files${sessionId ? `?session=${sessionId}` : ''}`),
  fileDownloadUrl: (id: string) => `${config.apiUrl}/files/${id}?token=${getToken()}`,
};
