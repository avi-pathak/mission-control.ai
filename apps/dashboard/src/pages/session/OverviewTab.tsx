import type { Session } from '@mc/protocol';
import { formatBytes, formatCount, formatDuration, formatPct, statusMeta } from '@mc/shared';
import { Card, CardContent, CardHeader, CardTitle } from '@mc/ui';
import { useLiveStore } from '../../store/live';

export function OverviewTab({ session }: { session: Session }) {
  const machine = useLiveStore((s) => s.machines[session.machineId]);
  const meta = statusMeta(session.status);

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Live status</CardTitle>
        </CardHeader>
        <CardContent>
          <Row label="Status" value={<span className={meta.color}>{meta.label}</span>} />
          <Row label="Provider" value={session.provider} />
          <Row label="Uptime" value={formatDuration(session.startedAt)} />
          <Row label="CPU" value={formatPct(session.cpuPct)} />
          <Row label="Memory" value={formatBytes(session.memBytes)} />
          <Row label="PID" value={String(session.pid)} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Machine</CardTitle>
        </CardHeader>
        <CardContent>
          <Row label="Hostname" value={machine?.hostname ?? session.machineId} />
          <Row label="OS" value={machine ? `${machine.os}/${machine.arch}` : '—'} />
          <Row label="Cores" value={machine ? String(machine.cpuCores) : '—'} />
          <Row label="Version" value={session.version || '—'} />
          {session.tmuxSession && <Row label="tmux" value={session.tmuxSession} />}
        </CardContent>
      </Card>

      {session.tokens && (
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>Token usage</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="mb-4 flex items-baseline gap-2">
              <span className="text-3xl font-semibold tabular-nums text-zinc-100">
                {formatCount(session.tokens.input + session.tokens.output)}
              </span>
              <span className="text-sm text-zinc-500">input + output tokens</span>
            </div>
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              <TokenStat label="Input" value={session.tokens.input} accent="text-sky-400" />
              <TokenStat label="Output" value={session.tokens.output} accent="text-emerald-400" />
              <TokenStat label="Cache read" value={session.tokens.cacheRead} accent="text-indigo-400" />
              <TokenStat
                label="Cache write"
                value={session.tokens.cacheCreation}
                accent="text-amber-400"
              />
            </div>
          </CardContent>
        </Card>
      )}

      <Card className="lg:col-span-2">
        <CardHeader>
          <CardTitle>Working directory</CardTitle>
        </CardHeader>
        <CardContent>
          <code className="block break-all font-mono text-sm text-zinc-300">{session.cwd}</code>
          <div className="mt-3 text-xs text-zinc-500">Current command</div>
          <code className="mt-1 block break-all font-mono text-xs text-zinc-400">
            {session.currentCommand}
          </code>
        </CardContent>
      </Card>
    </div>
  );
}

function TokenStat({ label, value, accent }: { label: string; value: number; accent: string }) {
  return (
    <div className="rounded-lg border border-white/[0.05] bg-white/[0.02] p-3">
      <div className="text-xs text-zinc-500">{label}</div>
      <div className={`mt-1 text-lg font-medium tabular-nums ${accent}`}>{formatCount(value)}</div>
    </div>
  );
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between border-b border-white/[0.04] py-2.5 text-sm last:border-0">
      <span className="text-zinc-500">{label}</span>
      <span className="font-medium tabular-nums text-zinc-200">{value}</span>
    </div>
  );
}
