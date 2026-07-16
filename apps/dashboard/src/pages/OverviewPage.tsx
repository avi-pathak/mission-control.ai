import { useMemo, useState } from 'react';
import { AlertTriangle, CheckCircle2, Clock, Coins, Play, Plus, Server } from 'lucide-react';
import { Button } from '@mc/ui';
import { formatCount } from '@mc/shared';
import { useMachines, useSessions, useLiveStore } from '../store/live';
import { StatCard } from '../components/StatCard';
import { SessionTable } from '../components/SessionTable';
import { NewSessionDialog } from '../components/NewSessionDialog';
import { OverviewCharts } from '../components/OverviewCharts';
import { applyFilters, FilterBar, type Filters } from '../components/FilterBar';

const initialFilters: Filters = { search: '', status: 'all', machine: 'all', repo: 'all' };

export function OverviewPage() {
  const sessions = useSessions();
  const machineList = useMachines();
  const machines = useLiveStore((s) => s.machines);
  const [filters, setFilters] = useState<Filters>(initialFilters);
  const [newOpen, setNewOpen] = useState(false);

  const counts = useMemo(() => {
    // Grand total tokens across UNIQUE sessions (deduped by id). Full detail
    // lives on the dedicated Tokens page.
    const seen = new Set<string>();
    let tokens = 0;
    for (const s of sessions) {
      if (seen.has(s.id)) continue;
      seen.add(s.id);
      if (s.tokens) tokens += s.tokens.total;
    }
    return {
      running: sessions.filter((s) => s.status === 'running').length,
      waiting: sessions.filter((s) => s.status === 'waiting_approval').length,
      finished: sessions.filter((s) => s.status === 'finished').length,
      errors: sessions.filter((s) => s.status === 'error').length,
      tokens,
    };
  }, [sessions]);

  const repos = useMemo(
    () => [...new Set(sessions.map((s) => s.repo).filter(Boolean))].sort(),
    [sessions],
  );

  const filtered = useMemo(() => applyFilters(sessions, filters), [sessions, filters]);

  return (
    <div className="flex h-full flex-col gap-6 overflow-hidden">
      <header className="flex items-start justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Overview</h1>
          <p className="mt-1 text-sm text-zinc-500">
            Live view of every AI coding session across your fleet.
          </p>
        </div>
        <Button variant="primary" size="sm" onClick={() => setNewOpen(true)}>
          <Plus className="h-4 w-4" /> New Session
        </Button>
      </header>

      <NewSessionDialog open={newOpen} onClose={() => setNewOpen(false)} />

      {counts.waiting > 0 && (
        <button
          onClick={() => setFilters({ ...filters, status: 'waiting_approval' })}
          className="flex shrink-0 items-center gap-3 rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-left text-sm text-amber-300 transition-colors hover:bg-amber-500/15"
        >
          <span className="relative flex h-2.5 w-2.5">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-amber-400 opacity-60" />
            <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-amber-400" />
          </span>
          <span className="font-medium">
            {counts.waiting} session{counts.waiting === 1 ? '' : 's'} waiting for your approval
          </span>
          <span className="ml-auto text-amber-400/70">View →</span>
        </button>
      )}

      {/* Fixed top section: stats, charts, filters */}
      <div className="grid shrink-0 grid-cols-2 gap-4 md:grid-cols-3 xl:grid-cols-6">
        <StatCard label="Running" value={counts.running} icon={Play} accent="text-emerald-400" />
        <StatCard label="Waiting" value={counts.waiting} icon={Clock} accent="text-amber-400" />
        <StatCard
          label="Finished"
          value={counts.finished}
          icon={CheckCircle2}
          accent="text-zinc-400"
        />
        <StatCard label="Errors" value={counts.errors} icon={AlertTriangle} accent="text-rose-400" />
        <StatCard label="Machines" value={machineList.length} icon={Server} accent="text-indigo-400" />
        <StatCard label="Tokens" value={formatCount(counts.tokens)} icon={Coins} accent="text-sky-400" />
      </div>

      <div className="shrink-0">
        <OverviewCharts sessions={sessions} />
      </div>

      <div className="shrink-0">
        <FilterBar
          filters={filters}
          onChange={setFilters}
          machines={machineList.map((m) => ({ id: m.id, hostname: m.hostname }))}
          repos={repos}
        />
      </div>

      {/* Table fills remaining height and scrolls internally */}
      <div className="min-h-0 flex-1">
        <SessionTable sessions={filtered} machines={machines} fillHeight />
      </div>
    </div>
  );
}
