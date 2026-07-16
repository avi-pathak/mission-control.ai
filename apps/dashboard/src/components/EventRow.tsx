import type { ActivityEvent } from '@mc/protocol';
import { formatRelativeTime } from '@mc/shared';
import { cn } from '@mc/ui';
import { motion } from 'framer-motion';
import {
  AlertTriangle,
  GitBranch,
  GitCommit,
  Play,
  Power,
  PowerOff,
  Square,
  Terminal,
  type LucideIcon,
} from 'lucide-react';

interface KindMeta {
  icon: LucideIcon;
  color: string;
}

const KIND_META: Record<string, KindMeta> = {
  'session.started': { icon: Play, color: 'text-emerald-400' },
  'session.ended': { icon: Square, color: 'text-zinc-400' },
  'status.changed': { icon: AlertTriangle, color: 'text-amber-400' },
  'branch.changed': { icon: GitBranch, color: 'text-sky-400' },
  'commit.created': { icon: GitCommit, color: 'text-emerald-400' },
  'command.changed': { icon: Terminal, color: 'text-zinc-400' },
  'command.issued': { icon: Terminal, color: 'text-indigo-400' },
  'agent.connected': { icon: Power, color: 'text-emerald-400' },
  'agent.disconnected': { icon: PowerOff, color: 'text-rose-400' },
};

const SEVERITY_ACCENT: Record<string, string> = {
  info: 'border-white/[0.06]',
  success: 'border-emerald-500/20',
  warn: 'border-amber-500/20',
  error: 'border-rose-500/25',
};

function kindMeta(kind: string): KindMeta {
  return KIND_META[kind] ?? { icon: Terminal, color: 'text-zinc-400' };
}

export function EventRow({ event, index = 0 }: { event: ActivityEvent; index?: number }) {
  const { icon: Icon, color } = kindMeta(event.kind);
  return (
    <motion.div
      initial={{ opacity: 0, x: -6 }}
      animate={{ opacity: 1, x: 0 }}
      transition={{ delay: Math.min(index * 0.015, 0.25) }}
      className={cn(
        'flex items-start gap-3 rounded-lg border bg-white/[0.02] px-4 py-3',
        SEVERITY_ACCENT[event.severity] ?? SEVERITY_ACCENT.info,
      )}
    >
      <Icon className={cn('mt-0.5 h-4 w-4 shrink-0', color)} />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm text-zinc-200">{event.message}</div>
        <div className="mt-0.5 text-xs text-zinc-500">
          <span className="font-mono">{event.kind}</span>
          {event.meta?.branch ? <span> · {event.meta.branch}</span> : null}
        </div>
      </div>
      <div className="shrink-0 text-xs tabular-nums text-zinc-600">
        {formatRelativeTime(event.ts)}
      </div>
    </motion.div>
  );
}
