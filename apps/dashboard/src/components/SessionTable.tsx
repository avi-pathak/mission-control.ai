import type { Machine, Session } from '@mc/protocol';
import { formatBytes, formatCount, formatDuration, formatPct } from '@mc/shared';
import { Button, Card } from '@mc/ui';
import { useNavigate } from '@tanstack/react-router';
import { motion } from 'framer-motion';
import { GitBranch, RotateCw, Square } from 'lucide-react';
import { api } from '../lib/api';
import { StatusBadge } from './StatusBadge';

interface Props {
  sessions: Session[];
  machines: Record<string, Machine>;
  /** Cap the table height and scroll internally (fixed calc height). */
  scrollInternal?: boolean;
  /** Fill the parent's height (parent must be a sized flex child) and scroll
   *  the table body internally. Used on the Overview page's flex layout. */
  fillHeight?: boolean;
}

export function SessionTable({
  sessions,
  machines,
  scrollInternal = false,
  fillHeight = false,
}: Props) {
  const navigate = useNavigate();

  if (sessions.length === 0) {
    return (
      <Card className="p-10 text-center text-sm text-zinc-500">
        No sessions match the current filters.
      </Card>
    );
  }

  const scrollClass = fillHeight
    ? 'h-full overflow-auto'
    : scrollInternal
      ? 'max-h-[calc(100vh-24rem)] overflow-auto'
      : 'overflow-x-auto';

  return (
    <Card className={fillHeight ? 'flex h-full flex-col overflow-hidden' : 'overflow-hidden'}>
      <div className={scrollClass}>
        <table className="w-full text-sm">
          <thead className="sticky top-0 z-10 bg-[#0e0e13]">
            <tr className="border-b border-white/[0.06] text-left text-xs uppercase tracking-wide text-zinc-500">
              <th className="px-4 py-3 font-medium">Status</th>
              <th className="px-4 py-3 font-medium">Repository</th>
              <th className="px-4 py-3 font-medium">Branch</th>
              <th className="px-4 py-3 font-medium">Machine</th>
              <th className="px-4 py-3 font-medium">Duration</th>
              <th className="px-4 py-3 font-medium text-right">CPU</th>
              <th className="px-4 py-3 font-medium text-right">Memory</th>
              <th className="px-4 py-3 font-medium text-right">Tokens</th>
              <th className="px-4 py-3 font-medium">Current Task</th>
              <th className="px-4 py-3 font-medium text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
          {sessions.map((s, i) => (
            <motion.tr
              key={s.id}
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ delay: Math.min(i * 0.02, 0.3) }}
              onClick={() => navigate({ to: '/sessions/$sessionId', params: { sessionId: s.id } })}
              className="cursor-pointer border-b border-white/[0.03] transition-colors hover:bg-white/[0.02]"
            >
              <td className="px-4 py-3">
                <StatusBadge status={s.status} />
              </td>
              <td className="px-4 py-3 font-medium text-zinc-200">{s.repo || '—'}</td>
              <td className="px-4 py-3">
                <span className="inline-flex items-center gap-1.5 text-zinc-400">
                  <GitBranch className="h-3.5 w-3.5" />
                  {s.branch || '—'}
                </span>
              </td>
              <td className="px-4 py-3 text-zinc-400">
                {machines[s.machineId]?.hostname ?? s.machineId}
              </td>
              <td className="px-4 py-3 tabular-nums text-zinc-400">
                {formatDuration(s.startedAt)}
              </td>
              <td className="px-4 py-3 text-right tabular-nums text-zinc-400">
                {formatPct(s.cpuPct)}
              </td>
              <td className="px-4 py-3 text-right tabular-nums text-zinc-400">
                {formatBytes(s.memBytes)}
              </td>
              <td
                className="px-4 py-3 text-right tabular-nums text-zinc-400"
                title={
                  s.tokens
                    ? `input ${s.tokens.input.toLocaleString()} · output ${s.tokens.output.toLocaleString()} · cache read ${s.tokens.cacheRead.toLocaleString()}`
                    : ''
                }
              >
                {s.tokens ? formatCount(s.tokens.input + s.tokens.output) : '—'}
              </td>
              <td className="px-4 py-3 font-mono text-xs text-zinc-500">
                <div className="max-w-[240px] truncate">{s.currentCommand}</div>
              </td>
              <td className="px-4 py-3">
                <div className="flex justify-end gap-1.5" onClick={(e) => e.stopPropagation()}>
                  <Button
                    size="icon"
                    variant="ghost"
                    title="Restart"
                    onClick={() => api.restart(s.id)}
                  >
                    <RotateCw className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    title="Stop"
                    onClick={() => api.stop(s.id)}
                  >
                    <Square className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </td>
            </motion.tr>
          ))}
          </tbody>
        </table>
      </div>
    </Card>
  );
}
