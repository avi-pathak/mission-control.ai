import { formatBytes, formatPct, formatRelativeTime } from '@mc/shared';
import { Button, Card, Dialog, cn } from '@mc/ui';
import { useMutation } from '@tanstack/react-query';
import { motion } from 'framer-motion';
import { Cpu, MemoryStick, Plus, Server, Trash2 } from 'lucide-react';
import { useState } from 'react';
import type { Machine } from '@mc/protocol';
import { AddMachineDialog } from '../components/AddMachineDialog';
import { api } from '../lib/api';
import { useLiveStore, useMachines } from '../store/live';

export function MachinesPage() {
  const machines = useMachines();
  const [addOpen, setAddOpen] = useState(false);
  const [toRemove, setToRemove] = useState<Machine | null>(null);

  return (
    <div className="h-full space-y-6 overflow-y-auto pr-1">
      <header className="flex items-start justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Machines</h1>
          <p className="mt-1 text-sm text-zinc-500">
            {machines.length} hosts connected to the fleet.
          </p>
        </div>
        <Button variant="primary" size="sm" onClick={() => setAddOpen(true)}>
          <Plus className="h-4 w-4" /> Add Machine
        </Button>
      </header>

      <AddMachineDialog open={addOpen} onClose={() => setAddOpen(false)} />
      <RemoveMachineDialog machine={toRemove} onClose={() => setToRemove(null)} />

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
        {machines.map((m, i) => (
          <motion.div
            key={m.id}
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: Math.min(i * 0.03, 0.3) }}
          >
            <Card className="p-5">
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-2">
                  <Server className="h-4 w-4 text-indigo-400" />
                  <span className="font-medium text-zinc-200">{m.hostname}</span>
                </div>
                <span
                  className={cn(
                    'inline-flex items-center gap-1.5 text-xs',
                    m.online ? 'text-emerald-400' : 'text-zinc-500',
                  )}
                >
                  <span
                    className={cn(
                      'h-1.5 w-1.5 rounded-full',
                      m.online ? 'bg-emerald-400' : 'bg-zinc-600',
                    )}
                  />
                  {m.online ? 'Online' : 'Offline'}
                </span>
              </div>
              <div className="mt-1 text-xs text-zinc-500">
                {m.os}/{m.arch} · {m.cpuCores} cores · agent {m.agentVersion}
              </div>

              <div className="mt-4 grid grid-cols-2 gap-3">
                <Metric icon={Cpu} label="CPU" value={formatPct(m.cpuPct)} />
                <Metric
                  icon={MemoryStick}
                  label="Memory"
                  value={`${formatBytes(m.memUsedBytes)} / ${formatBytes(m.totalMem)}`}
                />
              </div>
              <div className="mt-3 flex items-center justify-between">
                <span className="text-xs text-zinc-600">
                  Last seen {formatRelativeTime(m.lastSeenAt)}
                </span>
                <button
                  onClick={() => setToRemove(m)}
                  className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs text-zinc-500 hover:bg-rose-500/10 hover:text-rose-400"
                  title="Remove machine"
                >
                  <Trash2 className="h-3.5 w-3.5" /> Remove
                </button>
              </div>
            </Card>
          </motion.div>
        ))}
        {machines.length === 0 && (
          <Card className="col-span-full flex flex-col items-center gap-3 p-10 text-center text-sm text-zinc-500">
            <Server className="h-8 w-8 text-zinc-600" />
            No machines connected yet.
            <Button variant="primary" size="sm" onClick={() => setAddOpen(true)}>
              <Plus className="h-4 w-4" /> Add your first machine
            </Button>
          </Card>
        )}
      </div>
    </div>
  );
}

function Metric({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof Cpu;
  label: string;
  value: string;
}) {
  return (
    <div className="rounded-lg border border-white/[0.05] bg-white/[0.02] p-3">
      <div className="flex items-center gap-1.5 text-xs text-zinc-500">
        <Icon className="h-3.5 w-3.5" />
        {label}
      </div>
      <div className="mt-1 text-sm font-medium tabular-nums text-zinc-200">{value}</div>
    </div>
  );
}

function RemoveMachineDialog({
  machine,
  onClose,
}: {
  machine: Machine | null;
  onClose: () => void;
}) {
  const hydrateFleet = useLiveStore((s) => s.hydrateFleet);
  const removeLocal = useLiveStore((s) => s.machines);
  const [error, setError] = useState('');

  const remove = useMutation({
    mutationFn: (force: boolean) => api.deleteMachine(machine!.id, force),
    onSuccess: () => {
      // Drop locally right away (WS machine.removed will also arrive).
      const next = { ...removeLocal };
      delete next[machine!.id];
      hydrateFleet(Object.values(next), []);
      onClose();
    },
    onError: (e: Error) => {
      if (e.message.includes('409')) {
        setError('This machine is online. Remove anyway (its data will be deleted)?');
      } else {
        setError('Could not remove machine.');
      }
    },
  });

  if (!machine) return null;

  return (
    <Dialog
      open={!!machine}
      onClose={onClose}
      title={`Remove ${machine.hostname}?`}
      description="This deletes the machine and all its sessions, logs, metrics, events and files for your workspace. This cannot be undone."
    >
      <div className="space-y-4">
        {machine.online && !error && (
          <p className="text-xs text-amber-400">
            This machine is currently online — its agent should be stopped first.
          </p>
        )}
        {error && <p className="text-xs text-rose-400">{error}</p>}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="danger"
            size="sm"
            disabled={remove.isPending}
            onClick={() => remove.mutate(!!error || machine.online)}
          >
            {error ? 'Remove anyway' : 'Remove'}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
