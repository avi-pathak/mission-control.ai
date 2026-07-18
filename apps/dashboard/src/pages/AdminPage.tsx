import { Button, Card, CardContent, CardHeader, CardTitle } from '@mc/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Check, X } from 'lucide-react';
import { useState } from 'react';
import { api, type AdminMachine, type PendingUser } from '../lib/api';
import { useAuthStore } from '../store/auth';

/** Platform-admin page: approve pending signups and assign them a namespace. */
export function AdminPage() {
  const user = useAuthStore((s) => s.user);
  const qc = useQueryClient();
  const [approving, setApproving] = useState<PendingUser | null>(null);

  const pending = useQuery({
    queryKey: ['pending-users'],
    queryFn: api.pendingUsers,
    enabled: !!user?.platformAdmin,
    refetchInterval: 15000,
  });
  const reject = useMutation({
    mutationFn: (id: string) => api.rejectUser(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['pending-users'] }),
  });

  if (!user?.platformAdmin) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-zinc-500">
        You don't have access to this page.
      </div>
    );
  }

  const rows = pending.data ?? [];

  return (
    <div className="h-full max-w-3xl space-y-6 overflow-y-auto pr-1">
      <header>
        <h1 className="text-xl font-semibold tracking-tight">Approvals</h1>
        <p className="mt-1 text-sm text-zinc-500">
          Review new signups and assign each to a workspace before they can sign in.
        </p>
      </header>

      <Card>
        <CardHeader>
          <CardTitle>Pending signups {rows.length > 0 && `(${rows.length})`}</CardTitle>
        </CardHeader>
        <CardContent className="divide-y divide-white/[0.05]">
          {rows.length === 0 && (
            <p className="py-6 text-center text-sm text-zinc-500">No pending signups. 🎉</p>
          )}
          {rows.map((u) => (
            <div key={u.id} className="flex items-center justify-between py-3 text-sm">
              <div>
                <div className="text-zinc-200">{u.email}</div>
                <div className="text-xs text-zinc-500">
                  requested {new Date(u.createdAt).toLocaleString()}
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Button variant="primary" size="sm" onClick={() => setApproving(u)}>
                  <Check className="h-3.5 w-3.5" /> Approve
                </Button>
                <button
                  onClick={() => reject.mutate(u.id)}
                  className="rounded-md p-1.5 text-zinc-500 hover:text-rose-400"
                  title="Reject"
                >
                  <X className="h-4 w-4" />
                </button>
              </div>
            </div>
          ))}
        </CardContent>
      </Card>

      {approving && (
        <ApproveDialog
          user={approving}
          onClose={() => setApproving(null)}
          onDone={() => {
            setApproving(null);
            qc.invalidateQueries({ queryKey: ['pending-users'] });
          }}
        />
      )}

      <MachinesSection />
    </div>
  );
}

/** All machines across every workspace, with a reassign control. */
function MachinesSection() {
  const qc = useQueryClient();
  const machines = useQuery({ queryKey: ['admin-machines'], queryFn: api.adminMachines });
  const orgs = useQuery({ queryKey: ['admin-orgs'], queryFn: api.adminOrgs });
  const reassign = useMutation({
    mutationFn: ({ id, orgId }: { id: string; orgId: string }) => api.reassignMachine(id, orgId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin-machines'] }),
  });

  const rows = machines.data ?? [];

  return (
    <Card>
      <CardHeader>
        <CardTitle>Machines (all workspaces)</CardTitle>
      </CardHeader>
      <CardContent className="divide-y divide-white/[0.05]">
        {rows.length === 0 && (
          <p className="py-6 text-center text-sm text-zinc-500">No machines enrolled yet.</p>
        )}
        {rows.map((m: AdminMachine) => (
          <div key={m.id} className="flex items-center justify-between gap-3 py-3 text-sm">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span
                  className={`h-1.5 w-1.5 rounded-full ${m.online ? 'bg-emerald-400' : 'bg-zinc-600'}`}
                />
                <span className="truncate text-zinc-200">{m.hostname}</span>
              </div>
              <div className="ml-3.5 truncate text-xs text-zinc-500">
                {m.os}/{m.arch} · workspace <span className="text-zinc-400">{m.orgName}</span>
              </div>
            </div>
            <select
              className="h-8 shrink-0 rounded-lg border border-white/[0.08] bg-white/[0.02] px-2 text-xs text-zinc-200 outline-none focus:border-indigo-500/50"
              value={m.orgId}
              disabled={reassign.isPending}
              onChange={(e) => {
                const orgId = e.target.value;
                if (orgId && orgId !== m.orgId) reassign.mutate({ id: m.id, orgId });
              }}
            >
              {(orgs.data ?? []).map((o) => (
                <option key={o.id} value={o.id}>
                  {o.name}
                </option>
              ))}
            </select>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

function ApproveDialog({
  user,
  onClose,
  onDone,
}: {
  user: PendingUser;
  onClose: () => void;
  onDone: () => void;
}) {
  const orgs = useQuery({ queryKey: ['admin-orgs'], queryFn: api.adminOrgs });
  const [mode, setMode] = useState<'existing' | 'new'>('existing');
  const [orgId, setOrgId] = useState('');
  const [newOrgName, setNewOrgName] = useState('');
  const [role, setRole] = useState('member');

  const approve = useMutation({
    mutationFn: () =>
      api.approveUser(user.id, {
        role,
        ...(mode === 'existing' ? { orgId } : { newOrgName }),
      }),
    onSuccess: onDone,
  });

  const canSubmit = mode === 'existing' ? !!orgId : newOrgName.trim().length > 0;
  const inputClass =
    'h-9 w-full rounded-lg border border-white/[0.08] bg-white/[0.02] px-3 text-sm text-zinc-200 outline-none focus:border-indigo-500/50';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
      <Card className="w-full max-w-md p-5">
        <h2 className="text-base font-semibold text-zinc-100">Approve {user.email}</h2>
        <p className="mt-1 text-sm text-zinc-500">Assign a workspace and role.</p>

        <div className="mt-4 space-y-3">
          <div className="flex gap-1 rounded-lg border border-white/[0.06] bg-white/[0.02] p-1">
            {(['existing', 'new'] as const).map((m) => (
              <button
                key={m}
                onClick={() => setMode(m)}
                className={`flex-1 rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                  mode === m ? 'bg-white/[0.08] text-white' : 'text-zinc-400 hover:text-zinc-200'
                }`}
              >
                {m === 'existing' ? 'Existing workspace' : 'New workspace'}
              </button>
            ))}
          </div>

          {mode === 'existing' ? (
            <select className={inputClass} value={orgId} onChange={(e) => setOrgId(e.target.value)}>
              <option value="">Select a workspace…</option>
              {(orgs.data ?? []).map((o) => (
                <option key={o.id} value={o.id}>
                  {o.name}
                </option>
              ))}
            </select>
          ) : (
            <input
              className={inputClass}
              placeholder="New workspace name"
              value={newOrgName}
              onChange={(e) => setNewOrgName(e.target.value)}
            />
          )}

          <select className={inputClass} value={role} onChange={(e) => setRole(e.target.value)}>
            <option value="member">Member</option>
            <option value="admin">Admin</option>
          </select>

          {approve.isError && (
            <p className="text-xs text-rose-400">Could not approve. Check the selection.</p>
          )}
        </div>

        <div className="mt-5 flex justify-end gap-2">
          <Button variant="outline" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            size="sm"
            disabled={!canSubmit || approve.isPending}
            onClick={() => approve.mutate()}
          >
            {approve.isPending ? 'Approving…' : 'Approve'}
          </Button>
        </div>
      </Card>
    </div>
  );
}
