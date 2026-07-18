import { Button, Card, CardContent, CardHeader, CardTitle } from '@mc/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { LogOut, Trash2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { api, ApiError } from '../lib/api';
import {
  disablePush,
  enablePush,
  isSubscribed,
  pushSupport,
  type PushSupport,
} from '../lib/push';
import { useAuthStore } from '../store/auth';

const inputClass =
  'h-9 rounded-lg border border-white/[0.08] bg-white/[0.02] px-3 text-sm text-zinc-200 outline-none focus:border-indigo-500/50';

export function SettingsPage() {
  const user = useAuthStore((s) => s.user);
  const org = useAuthStore((s) => s.org);
  const logout = useAuthStore((s) => s.logout);

  const isAdmin = user?.role === 'admin';

  return (
    <div className="h-full max-w-3xl space-y-6 overflow-y-auto pr-1">
      <header className="flex items-start justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Settings</h1>
          <p className="mt-1 text-sm text-zinc-500">
            {org?.name} · signed in as {user?.email} ({user?.role})
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={logout}>
          <LogOut className="h-3.5 w-3.5" /> Sign out
        </Button>
      </header>

      <ChangePasswordCard />
      <NotificationsCard />
      <MembersCard isAdmin={isAdmin} selfId={user?.id ?? ''} />
      {isAdmin && <InviteCard />}
    </div>
  );
}

function NotificationsCard() {
  const [support, setSupport] = useState<PushSupport>('default');
  const [subscribed, setSubscribed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  // Only meaningful when the server has push configured.
  const vapid = useQuery({ queryKey: ['push-vapid'], queryFn: api.pushVapidKey });

  useEffect(() => {
    setSupport(pushSupport());
    isSubscribed().then(setSubscribed).catch(() => {});
  }, []);

  const serverEnabled = vapid.data?.enabled ?? false;

  const toggle = async (on: boolean) => {
    setMsg(null);
    setBusy(true);
    try {
      if (on) {
        const ok = await enablePush();
        setSubscribed(ok);
        setSupport(pushSupport());
        setMsg(
          ok
            ? { ok: true, text: 'Notifications enabled on this device.' }
            : { ok: false, text: 'Permission was not granted.' },
        );
      } else {
        await disablePush();
        setSubscribed(false);
        setMsg({ ok: true, text: 'Notifications disabled on this device.' });
      }
    } catch (e) {
      setMsg({ ok: false, text: e instanceof Error ? e.message : 'Something went wrong.' });
    } finally {
      setBusy(false);
    }
  };

  const test = useMutation({
    mutationFn: () => api.pushTest(),
    onSuccess: (r) =>
      setMsg({ ok: true, text: `Test notification sent to ${r.sent} device(s).` }),
    onError: (e) =>
      setMsg({
        ok: false,
        text: e instanceof ApiError ? e.message : 'Could not send test notification.',
      }),
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle>Notifications</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="mb-3 text-sm text-zinc-400">
          Get a push notification when one of your sessions is blocked (waiting for approval)
          for longer than the configured threshold — even when this tab is closed.
        </p>

        {support === 'unsupported' ? (
          <p className="text-xs text-amber-400">
            This browser doesn't support push notifications.
          </p>
        ) : !serverEnabled ? (
          <p className="text-xs text-amber-400">
            Push isn't configured on this server. Set VAPID keys (see docs) to enable it.
          </p>
        ) : support === 'denied' ? (
          <p className="text-xs text-amber-400">
            Notifications are blocked in your browser settings. Allow them for this site, then
            reload.
          </p>
        ) : (
          <div className="flex flex-wrap items-center gap-2">
            {subscribed ? (
              <Button variant="outline" size="sm" disabled={busy} onClick={() => toggle(false)}>
                {busy ? 'Working…' : 'Disable notifications'}
              </Button>
            ) : (
              <Button variant="primary" size="sm" disabled={busy} onClick={() => toggle(true)}>
                {busy ? 'Working…' : 'Enable notifications'}
              </Button>
            )}
            <Button
              variant="outline"
              size="sm"
              disabled={!subscribed || test.isPending}
              onClick={() => test.mutate()}
            >
              {test.isPending ? 'Sending…' : 'Send test'}
            </Button>
          </div>
        )}

        {msg && (
          <p className={`mt-3 text-xs ${msg.ok ? 'text-emerald-400' : 'text-rose-400'}`}>
            {msg.text}
          </p>
        )}
      </CardContent>
    </Card>
  );
}

function ChangePasswordCard() {
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  const change = useMutation({
    mutationFn: () => api.changePassword(current, next),
    onSuccess: () => {
      setMsg({ ok: true, text: 'Password updated.' });
      setCurrent('');
      setNext('');
      setConfirm('');
    },
    onError: (e) => {
      const text =
        e instanceof ApiError && e.code === 'wrong_password'
          ? 'Current password is incorrect.'
          : e instanceof ApiError
            ? e.message
            : 'Could not update password.';
      setMsg({ ok: false, text });
    },
  });

  const canSubmit = current && next.length >= 8 && next === confirm;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Change password</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="grid max-w-sm gap-2">
          <input
            type="password"
            placeholder="Current password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            className={inputClass}
          />
          <input
            type="password"
            placeholder="New password (8+ chars)"
            value={next}
            onChange={(e) => setNext(e.target.value)}
            className={inputClass}
          />
          <input
            type="password"
            placeholder="Confirm new password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            className={inputClass}
          />
          {next && confirm && next !== confirm && (
            <p className="text-xs text-rose-400">Passwords don't match.</p>
          )}
          {msg && (
            <p className={`text-xs ${msg.ok ? 'text-emerald-400' : 'text-rose-400'}`}>{msg.text}</p>
          )}
          <div>
            <Button
              variant="primary"
              size="sm"
              disabled={!canSubmit || change.isPending}
              onClick={() => change.mutate()}
            >
              {change.isPending ? 'Updating…' : 'Update password'}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function MembersCard({ isAdmin, selfId }: { isAdmin: boolean; selfId: string }) {
  const qc = useQueryClient();
  const users = useQuery({ queryKey: ['org-users'], queryFn: api.orgUsers });

  const setRole = useMutation({
    mutationFn: ({ id, role }: { id: string; role: string }) => api.setUserRole(id, role),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['org-users'] }),
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.removeUser(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['org-users'] }),
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle>Members</CardTitle>
      </CardHeader>
      <CardContent className="divide-y divide-white/[0.05]">
        {(users.data ?? []).map((u) => {
          const isSelf = u.id === selfId;
          return (
            <div key={u.id} className="flex items-center justify-between gap-2 py-2.5 text-sm">
              <span className="truncate text-zinc-200">
                {u.email} {isSelf && <span className="text-xs text-zinc-500">(you)</span>}
              </span>
              <div className="flex items-center gap-2">
                {isAdmin && !isSelf ? (
                  <>
                    <select
                      className="h-8 rounded-lg border border-white/[0.08] bg-white/[0.02] px-2 text-xs text-zinc-200 outline-none focus:border-indigo-500/50"
                      value={u.role}
                      disabled={setRole.isPending}
                      onChange={(e) => setRole.mutate({ id: u.id, role: e.target.value })}
                    >
                      <option value="member">Member</option>
                      <option value="admin">Admin</option>
                    </select>
                    <button
                      onClick={() => remove.mutate(u.id)}
                      className="text-zinc-500 hover:text-rose-400"
                      title="Remove member"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </>
                ) : (
                  <span className="text-xs text-zinc-500">{u.role}</span>
                )}
              </div>
            </div>
          );
        })}
      </CardContent>
    </Card>
  );
}

function InviteCard() {
  const qc = useQueryClient();
  const [email, setEmail] = useState('');
  const [role, setRole] = useState('member');
  const [result, setResult] = useState<{ link: string; emailed: boolean } | null>(null);

  const invites = useQuery({ queryKey: ['org-invites'], queryFn: api.orgInvites });
  const createInvite = useMutation({
    mutationFn: () => api.createInvite(email, role),
    onSuccess: (res) => {
      setResult({ link: res.link, emailed: res.emailed });
      setEmail('');
      qc.invalidateQueries({ queryKey: ['org-invites'] });
    },
  });
  const revokeInvite = useMutation({
    mutationFn: (token: string) => api.revokeInvite(token),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['org-invites'] }),
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle>Invite a member</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap items-center gap-2">
          <input
            type="email"
            placeholder="teammate@company.com"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className={`${inputClass} flex-1`}
          />
          <select className={inputClass} value={role} onChange={(e) => setRole(e.target.value)}>
            <option value="member">Member</option>
            <option value="admin">Admin</option>
          </select>
          <Button
            variant="primary"
            size="sm"
            disabled={!email || createInvite.isPending}
            onClick={() => createInvite.mutate()}
          >
            Invite
          </Button>
        </div>

        {result && (
          <div className="mt-3 rounded-lg border border-white/10 bg-black/30 p-3">
            <div className="text-xs text-zinc-500">
              {result.emailed
                ? 'Invite emailed. You can also share this link:'
                : 'Share this invite link:'}
            </div>
            <code className="mt-1 block break-all font-mono text-xs text-indigo-300">
              {result.link}
            </code>
          </div>
        )}

        <div className="mt-4 divide-y divide-white/[0.05]">
          {(invites.data ?? []).map((inv) => (
            <div key={inv.token} className="flex items-center justify-between py-2 text-sm">
              <span className="text-zinc-300">
                {inv.email} <span className="text-xs text-zinc-500">· {inv.role}</span>
              </span>
              <button
                onClick={() => revokeInvite.mutate(inv.token)}
                className="text-zinc-500 hover:text-rose-400"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}
