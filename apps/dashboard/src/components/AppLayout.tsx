import { Link, Outlet, useRouterState } from '@tanstack/react-router';
import { cn, LogoMark } from '@mc/ui';
import {
  Activity,
  Coins,
  LayoutDashboard,
  Server,
  Settings,
  ShieldCheck,
  TerminalSquare,
} from 'lucide-react';
import { useQuery } from '@tanstack/react-query';
import { useEffect } from 'react';
import { api } from '../lib/api';
import { useAuthStore } from '../store/auth';
import { useLiveStore } from '../store/live';

const NAV = [
  { to: '/', label: 'Overview', icon: LayoutDashboard, exact: true },
  { to: '/sessions', label: 'Sessions', icon: TerminalSquare },
  { to: '/machines', label: 'Machines', icon: Server },
  { to: '/activity', label: 'Activity', icon: Activity },
  { to: '/tokens', label: 'Tokens', icon: Coins },
  { to: '/settings', label: 'Settings', icon: Settings },
];

function ConnectionPill() {
  const status = useLiveStore((s) => s.status);
  const map = {
    open: { label: 'Live', color: 'bg-emerald-400' },
    connecting: { label: 'Connecting', color: 'bg-amber-400' },
    closed: { label: 'Offline', color: 'bg-rose-400' },
  } as const;
  const m = map[status];
  return (
    <div className="flex items-center gap-2 rounded-lg border border-white/[0.06] bg-white/[0.02] px-3 py-1.5 text-xs text-zinc-400">
      <span className={cn('h-1.5 w-1.5 rounded-full', m.color)} />
      {m.label}
    </div>
  );
}

export function AppLayout() {
  const connect = useLiveStore((s) => s.connect);
  const hydrateFleet = useLiveStore((s) => s.hydrateFleet);
  const waitingCount = useLiveStore(
    (s) => Object.values(s.sessions).filter((sess) => sess.status === 'waiting_approval').length,
  );
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const user = useAuthStore((s) => s.user);
  const org = useAuthStore((s) => s.org);

  // Superadmins get an Approvals nav item with a pending-count badge.
  const pending = useQuery({
    queryKey: ['pending-users'],
    queryFn: api.pendingUsers,
    enabled: !!user?.platformAdmin,
    refetchInterval: 30000,
  });
  const pendingCount = pending.data?.length ?? 0;

  useEffect(() => {
    connect();
    // Seed the fleet from REST so machines + sessions appear immediately, even
    // if the WebSocket snapshot is delayed. Live diffs then keep it current.
    Promise.all([api.machines(), api.sessions()])
      .then(([machines, sessions]) => hydrateFleet(machines, sessions))
      .catch(() => {});
  }, [connect, hydrateFleet]);

  // Surface pending approvals in the tab title even when backgrounded.
  useEffect(() => {
    document.title = waitingCount > 0 ? `(${waitingCount}) Mission Control.ai` : 'Mission Control.ai';
  }, [waitingCount]);

  return (
    <div className="flex h-screen overflow-hidden">
      <aside className="flex w-60 shrink-0 flex-col border-r border-white/[0.06] bg-white/[0.015] px-3 py-4">
        <div className="mb-1 flex items-center gap-2.5 px-2">
          <LogoMark className="h-7 w-7 text-white" />
          <span className="text-sm font-semibold tracking-tight">Mission Control.ai</span>
        </div>
        {/* Current workspace — makes the active tenant obvious so users notice a
            wrong-org mismatch (e.g. agents enrolled under a different workspace). */}
        {org?.name && (
          <div className="mb-5 ml-1 flex items-center gap-2 px-2">
            <span className="truncate text-xs text-zinc-500">
              Workspace: <span className="text-zinc-300">{org.name}</span>
            </span>
          </div>
        )}
        <nav className="flex flex-1 flex-col gap-1">
          {NAV.map(({ to, label, icon: Icon, exact }) => {
            const active = exact ? pathname === to : pathname.startsWith(to);
            const showBadge =
              waitingCount > 0 && (to === '/sessions' || to === '/');
            return (
              <Link
                key={to}
                to={to}
                className={cn(
                  'flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors',
                  active
                    ? 'bg-white/[0.06] text-white'
                    : 'text-zinc-400 hover:bg-white/[0.03] hover:text-zinc-200',
                )}
              >
                <Icon className="h-4 w-4" />
                {label}
                {showBadge && (
                  <span className="ml-auto rounded-full bg-amber-500/20 px-1.5 text-xs font-medium text-amber-300">
                    {waitingCount}
                  </span>
                )}
              </Link>
            );
          })}
          {user?.platformAdmin && (
            <Link
              to="/admin"
              className={cn(
                'flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors',
                pathname.startsWith('/admin')
                  ? 'bg-white/[0.06] text-white'
                  : 'text-zinc-400 hover:bg-white/[0.03] hover:text-zinc-200',
              )}
            >
              <ShieldCheck className="h-4 w-4" />
              Approvals
              {pendingCount > 0 && (
                <span className="ml-auto rounded-full bg-indigo-500/20 px-1.5 text-xs font-medium text-indigo-300">
                  {pendingCount}
                </span>
              )}
            </Link>
          )}
        </nav>
        <div className="px-1">
          <ConnectionPill />
        </div>
      </aside>

      <main className="flex-1 overflow-hidden">
        <div className="mx-auto flex h-full max-w-7xl flex-col px-8 py-8">
          <Outlet />
        </div>
      </main>
    </div>
  );
}
