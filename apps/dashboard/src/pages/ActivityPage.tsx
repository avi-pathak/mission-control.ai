import { cn } from '@mc/ui';
import { Activity, Search } from 'lucide-react';
import { useMemo, useState } from 'react';
import { EventRow } from '../components/EventRow';
import { useEvents, useMachines } from '../store/live';

const KINDS = [
  'all',
  'session.started',
  'session.ended',
  'status.changed',
  'branch.changed',
  'commit.created',
  'command.issued',
  'agent.connected',
  'agent.disconnected',
];

const selectClass =
  'h-9 rounded-lg border border-white/[0.06] bg-white/[0.02] px-3 text-sm text-zinc-300 outline-none focus:border-indigo-500/50';

export function ActivityPage() {
  const events = useEvents();
  const machines = useMachines();
  const [search, setSearch] = useState('');
  const [kind, setKind] = useState('all');
  const [machine, setMachine] = useState('all');

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return events.filter((e) => {
      if (kind !== 'all' && e.kind !== kind) return false;
      if (machine !== 'all' && e.machineId !== machine) return false;
      if (q && !e.message.toLowerCase().includes(q)) return false;
      return true;
    });
  }, [events, search, kind, machine]);

  return (
    <div className="h-full space-y-6 overflow-y-auto pr-1">
      <header>
        <h1 className="flex items-center gap-2 text-xl font-semibold tracking-tight">
          <Activity className="h-5 w-5 text-indigo-400" /> Activity
        </h1>
        <p className="mt-1 text-sm text-zinc-500">
          Live feed of everything happening across your fleet.
        </p>
      </header>

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-[200px] flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-zinc-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search activity…"
            className={cn(selectClass, 'w-full pl-9')}
          />
        </div>
        <select className={selectClass} value={kind} onChange={(e) => setKind(e.target.value)}>
          {KINDS.map((k) => (
            <option key={k} value={k}>
              {k === 'all' ? 'All kinds' : k}
            </option>
          ))}
        </select>
        <select
          className={selectClass}
          value={machine}
          onChange={(e) => setMachine(e.target.value)}
        >
          <option value="all">All machines</option>
          {machines.map((m) => (
            <option key={m.id} value={m.id}>
              {m.hostname}
            </option>
          ))}
        </select>
      </div>

      {filtered.length === 0 ? (
        <div className="rounded-xl border border-white/[0.06] p-10 text-center text-sm text-zinc-500">
          No activity yet. Events appear here as sessions start, change, and end.
        </div>
      ) : (
        <div className="space-y-2">
          {filtered.map((e, i) => (
            <EventRow key={e.id} event={e} index={i} />
          ))}
        </div>
      )}
    </div>
  );
}
