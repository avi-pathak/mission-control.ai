import type { SessionStatus } from '@mc/protocol';
import { cn } from '@mc/ui';
import { Search } from 'lucide-react';

export interface Filters {
  search: string;
  status: SessionStatus | 'all';
  machine: string;
  repo: string;
}

interface Props {
  filters: Filters;
  onChange: (f: Filters) => void;
  machines: { id: string; hostname: string }[];
  repos: string[];
}

const STATUSES: (SessionStatus | 'all')[] = [
  'all',
  'running',
  'waiting_approval',
  'idle',
  'finished',
  'error',
];

const selectClass =
  'h-9 rounded-lg border border-white/[0.06] bg-white/[0.02] px-3 text-sm text-zinc-300 outline-none focus:border-indigo-500/50';

export function FilterBar({ filters, onChange, machines, repos }: Props) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative flex-1 min-w-[200px]">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-zinc-500" />
        <input
          value={filters.search}
          onChange={(e) => onChange({ ...filters, search: e.target.value })}
          placeholder="Search repository, branch, task…"
          className={cn(selectClass, 'w-full pl-9')}
        />
      </div>
      <select
        className={selectClass}
        value={filters.status}
        onChange={(e) => onChange({ ...filters, status: e.target.value as Filters['status'] })}
      >
        {STATUSES.map((s) => (
          <option key={s} value={s}>
            {s === 'all' ? 'All statuses' : s.replace('_', ' ')}
          </option>
        ))}
      </select>
      <select
        className={selectClass}
        value={filters.machine}
        onChange={(e) => onChange({ ...filters, machine: e.target.value })}
      >
        <option value="all">All machines</option>
        {machines.map((m) => (
          <option key={m.id} value={m.id}>
            {m.hostname}
          </option>
        ))}
      </select>
      <select
        className={selectClass}
        value={filters.repo}
        onChange={(e) => onChange({ ...filters, repo: e.target.value })}
      >
        <option value="all">All repos</option>
        {repos.map((r) => (
          <option key={r} value={r}>
            {r}
          </option>
        ))}
      </select>
    </div>
  );
}

export function applyFilters(
  sessions: import('@mc/protocol').Session[],
  filters: Filters,
): import('@mc/protocol').Session[] {
  const q = filters.search.trim().toLowerCase();
  return sessions.filter((s) => {
    if (filters.status !== 'all' && s.status !== filters.status) return false;
    if (filters.machine !== 'all' && s.machineId !== filters.machine) return false;
    if (filters.repo !== 'all' && s.repo !== filters.repo) return false;
    if (q) {
      const hay = `${s.repo} ${s.branch} ${s.currentCommand} ${s.cwd}`.toLowerCase();
      if (!hay.includes(q)) return false;
    }
    return true;
  });
}
