import { formatDuration } from '@mc/shared';
import { Button } from '@mc/ui';
import { useParams } from '@tanstack/react-router';
import { ArrowLeft, GitBranch, RotateCw, Server, Square } from 'lucide-react';
import { useState } from 'react';
import { Link } from '@tanstack/react-router';
import { api } from '../lib/api';
import { useSessionBackfill } from '../hooks/useSessionBackfill';
import { useLiveStore } from '../store/live';
import { StatusBadge } from '../components/StatusBadge';
import { OverviewTab } from './session/OverviewTab';
import { LogsTab } from './session/LogsTab';
import { TerminalTab } from './session/TerminalTab';
import { MetricsTab } from './session/MetricsTab';
import { GitTab } from './session/GitTab';
import { ActivityTab } from './session/ActivityTab';
import { FilesTab } from './session/FilesTab';

const TABS = ['Overview', 'Activity', 'Logs', 'Terminal', 'Metrics', 'Git', 'Files'] as const;
type Tab = (typeof TABS)[number];

export function SessionPage() {
  const { sessionId } = useParams({ from: '/sessions/$sessionId' });
  const session = useLiveStore((s) => s.sessions[sessionId]);
  const machine = useLiveStore((s) => (session ? s.machines[session.machineId] : undefined));
  const [tab, setTab] = useState<Tab>('Overview');

  // Backfill persisted logs + metrics from REST so history survives refreshes
  // and predates the current WebSocket connection.
  useSessionBackfill(sessionId);

  if (!session) {
    return (
      <div className="space-y-4">
        <Link to="/" className="inline-flex items-center gap-2 text-sm text-zinc-400 hover:text-white">
          <ArrowLeft className="h-4 w-4" /> Back
        </Link>
        <div className="rounded-xl border border-white/[0.06] p-10 text-center text-sm text-zinc-500">
          Session not found or no longer live.
        </div>
      </div>
    );
  }

  return (
    <div className="h-full space-y-6 overflow-y-auto pr-1">
      <Link to="/" className="inline-flex items-center gap-2 text-sm text-zinc-400 hover:text-white">
        <ArrowLeft className="h-4 w-4" /> Overview
      </Link>

      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-xl font-semibold tracking-tight">{session.repo || 'Session'}</h1>
            <StatusBadge status={session.status} />
          </div>
          <div className="mt-2 flex flex-wrap items-center gap-4 text-sm text-zinc-500">
            <span className="inline-flex items-center gap-1.5">
              <GitBranch className="h-3.5 w-3.5" /> {session.branch || '—'}
            </span>
            <span className="inline-flex items-center gap-1.5">
              <Server className="h-3.5 w-3.5" /> {machine?.hostname ?? session.machineId}
            </span>
            <span>Started {formatDuration(session.startedAt)} ago</span>
          </div>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={() => api.restart(session.id)}>
            <RotateCw className="h-3.5 w-3.5" /> Restart
          </Button>
          <Button variant="danger" size="sm" onClick={() => api.stop(session.id)}>
            <Square className="h-3.5 w-3.5" /> Stop
          </Button>
        </div>
      </header>

      <div className="flex gap-1 border-b border-white/[0.06]">
        {TABS.map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`relative px-4 py-2.5 text-sm transition-colors ${
              tab === t ? 'text-white' : 'text-zinc-500 hover:text-zinc-300'
            }`}
          >
            {t}
            {tab === t && (
              <span className="absolute inset-x-2 -bottom-px h-0.5 rounded-full bg-indigo-400" />
            )}
          </button>
        ))}
      </div>

      <div className="animate-fade-in">
        {tab === 'Overview' && <OverviewTab session={session} />}
        {tab === 'Activity' && <ActivityTab session={session} />}
        {tab === 'Logs' && <LogsTab session={session} />}
        {tab === 'Terminal' && <TerminalTab session={session} />}
        {tab === 'Metrics' && <MetricsTab session={session} />}
        {tab === 'Git' && <GitTab session={session} />}
        {tab === 'Files' && <FilesTab session={session} />}
      </div>
    </div>
  );
}
