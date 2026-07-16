import type { SessionStatus } from '@mc/protocol';

export interface StatusMeta {
  label: string;
  /** Tailwind text/border/bg accent classes. */
  color: string;
  dot: string;
}

export const STATUS_META: Record<SessionStatus, StatusMeta> = {
  running: { label: 'Running', color: 'text-emerald-400', dot: 'bg-emerald-400' },
  waiting_approval: { label: 'Waiting', color: 'text-amber-400', dot: 'bg-amber-400' },
  idle: { label: 'Idle', color: 'text-sky-400', dot: 'bg-sky-400' },
  finished: { label: 'Finished', color: 'text-zinc-400', dot: 'bg-zinc-400' },
  error: { label: 'Error', color: 'text-rose-400', dot: 'bg-rose-400' },
};

export function statusMeta(status: SessionStatus): StatusMeta {
  return STATUS_META[status] ?? STATUS_META.idle;
}
