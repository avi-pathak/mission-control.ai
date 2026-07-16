import { Button, Card, CardContent, CardHeader, CardTitle } from '@mc/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { LogOut, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { api } from '../lib/api';
import { useAuthStore } from '../store/auth';

export function SettingsPage() {
  const user = useAuthStore((s) => s.user);
  const org = useAuthStore((s) => s.org);
  const logout = useAuthStore((s) => s.logout);
  const qc = useQueryClient();

  const isAdmin = user?.role === 'owner' || user?.role === 'admin';
  const [email, setEmail] = useState('');
  const [role, setRole] = useState('member');
  const [inviteLink, setInviteLink] = useState('');

  const users = useQuery({ queryKey: ['org-users'], queryFn: api.orgUsers });
  const invites = useQuery({ queryKey: ['org-invites'], queryFn: api.orgInvites, enabled: isAdmin });

  const createInvite = useMutation({
    mutationFn: () => api.createInvite(email, role),
    onSuccess: (res) => {
      setInviteLink(res.link);
      setEmail('');
      qc.invalidateQueries({ queryKey: ['org-invites'] });
    },
  });
  const revokeInvite = useMutation({
    mutationFn: (token: string) => api.revokeInvite(token),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['org-invites'] }),
  });

  const inputClass =
    'h-9 rounded-lg border border-white/[0.08] bg-white/[0.02] px-3 text-sm text-zinc-200 outline-none focus:border-indigo-500/50';

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

      <Card>
        <CardHeader>
          <CardTitle>Members</CardTitle>
        </CardHeader>
        <CardContent className="divide-y divide-white/[0.05]">
          {(users.data ?? []).map((u) => (
            <div key={u.id} className="flex items-center justify-between py-2.5 text-sm">
              <span className="text-zinc-200">{u.email}</span>
              <span className="text-xs text-zinc-500">{u.role}</span>
            </div>
          ))}
        </CardContent>
      </Card>

      {isAdmin && (
        <Card>
          <CardHeader>
            <CardTitle>Invite a teammate</CardTitle>
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

            {inviteLink && (
              <div className="mt-3 rounded-lg border border-white/10 bg-black/30 p-3">
                <div className="text-xs text-zinc-500">Share this invite link:</div>
                <code className="mt-1 block break-all font-mono text-xs text-indigo-300">
                  {inviteLink}
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
      )}
    </div>
  );
}
