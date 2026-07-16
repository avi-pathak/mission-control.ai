import { useMemo, useState } from 'react';
import { useMachines, useSessions, useLiveStore } from '../store/live';
import { SessionTable } from '../components/SessionTable';
import { applyFilters, FilterBar, type Filters } from '../components/FilterBar';

const initialFilters: Filters = { search: '', status: 'all', machine: 'all', repo: 'all' };

export function SessionsPage() {
  const sessions = useSessions();
  const machineList = useMachines();
  const machines = useLiveStore((s) => s.machines);
  const [filters, setFilters] = useState<Filters>(initialFilters);

  const repos = useMemo(
    () => [...new Set(sessions.map((s) => s.repo).filter(Boolean))].sort(),
    [sessions],
  );
  const filtered = useMemo(() => applyFilters(sessions, filters), [sessions, filters]);

  return (
    <div className="flex h-full flex-col gap-6 overflow-hidden">
      <header className="shrink-0">
        <h1 className="text-xl font-semibold tracking-tight">Sessions</h1>
        <p className="mt-1 text-sm text-zinc-500">{sessions.length} total sessions.</p>
      </header>
      <div className="shrink-0">
        <FilterBar
          filters={filters}
          onChange={setFilters}
          machines={machineList.map((m) => ({ id: m.id, hostname: m.hostname }))}
          repos={repos}
        />
      </div>
      <div className="min-h-0 flex-1">
        <SessionTable sessions={filtered} machines={machines} fillHeight />
      </div>
    </div>
  );
}
