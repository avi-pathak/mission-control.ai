import type { Session } from '@mc/protocol';
import { formatCount } from '@mc/shared';
import { Card, CardContent, CardHeader, CardTitle } from '@mc/ui';
import { useMemo } from 'react';
import {
  Bar,
  BarChart,
  Cell,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

const STATUS_COLORS: Record<string, string> = {
  running: '#34d399',
  waiting_approval: '#fbbf24',
  idle: '#38bdf8',
  finished: '#a1a1aa',
  error: '#fb7185',
};

const tooltipStyle = {
  background: '#111114',
  border: '1px solid rgba(255,255,255,0.08)',
  borderRadius: 8,
  fontSize: 12,
  color: '#e4e4e7',
} as const;

const tooltipItemStyle = { color: '#e4e4e7' } as const;
const tooltipLabelStyle = { color: '#a1a1aa' } as const;

export function OverviewCharts({ sessions }: { sessions: Session[] }) {
  const byStatus = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const s of sessions) counts[s.status] = (counts[s.status] ?? 0) + 1;
    return Object.entries(counts).map(([name, value]) => ({ name, value }));
  }, [sessions]);

  const topRepos = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const s of sessions) {
      const repo = s.repo || '(no repo)';
      counts[repo] = (counts[repo] ?? 0) + 1;
    }
    return Object.entries(counts)
      .map(([name, count]) => ({ name, count }))
      .sort((a, b) => b.count - a.count)
      .slice(0, 6);
  }, [sessions]);

  const tokensByRepo = useMemo(() => {
    // input+output only (cache-read is excluded — it dwarfs real usage), deduped
    // by session id so two processes in the same repo aren't double-counted.
    const totals: Record<string, number> = {};
    const seen = new Set<string>();
    for (const s of sessions) {
      if (!s.tokens || seen.has(s.id)) continue;
      seen.add(s.id);
      const repo = s.repo || '(no repo)';
      totals[repo] = (totals[repo] ?? 0) + s.tokens.input + s.tokens.output;
    }
    return Object.entries(totals)
      .map(([name, tokens]) => ({ name, tokens }))
      .sort((a, b) => b.tokens - a.tokens)
      .slice(0, 6);
  }, [sessions]);

  if (sessions.length === 0) return null;

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
      <Card>
        <CardHeader>
          <CardTitle>Sessions by status</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="h-48">
            <ResponsiveContainer width="100%" height="100%">
              <PieChart>
                <Pie
                  data={byStatus}
                  dataKey="value"
                  nameKey="name"
                  innerRadius={45}
                  outerRadius={70}
                  paddingAngle={2}
                >
                  {byStatus.map((d) => (
                    <Cell key={d.name} fill={STATUS_COLORS[d.name] ?? '#6366f1'} />
                  ))}
                </Pie>
                <Tooltip
                  contentStyle={tooltipStyle}
                  itemStyle={tooltipItemStyle}
                  labelStyle={tooltipLabelStyle}
                  formatter={(v: number, _n, p) => [
                    `${v} session${v === 1 ? '' : 's'}`,
                    String(p?.payload?.name ?? '').replace('_', ' '),
                  ]}
                />
              </PieChart>
            </ResponsiveContainer>
          </div>
          <div className="mt-2 flex flex-wrap justify-center gap-x-3 gap-y-1 text-xs">
            {byStatus.map((d) => (
              <span key={d.name} className="inline-flex items-center gap-1.5 text-zinc-400">
                <span
                  className="h-2 w-2 rounded-full"
                  style={{ background: STATUS_COLORS[d.name] ?? '#6366f1' }}
                />
                {d.name.replace('_', ' ')} ({d.value})
              </span>
            ))}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Sessions by repository</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="h-48">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={topRepos} layout="vertical" margin={{ left: 8, right: 8 }}>
                <XAxis type="number" hide />
                <YAxis
                  type="category"
                  dataKey="name"
                  width={90}
                  tick={{ fill: '#a1a1aa', fontSize: 11 }}
                />
                <Tooltip
                  contentStyle={tooltipStyle}
                  itemStyle={tooltipItemStyle}
                  labelStyle={tooltipLabelStyle}
                  cursor={{ fill: 'rgba(255,255,255,0.03)' }}
                  formatter={(v: number) => [`${v} session${v === 1 ? '' : 's'}`, 'sessions']}
                />
                <Bar dataKey="count" fill="#818cf8" radius={[0, 4, 4, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>I/O tokens by repository</CardTitle>
        </CardHeader>
        <CardContent>
          {tokensByRepo.length === 0 ? (
            <div className="flex h-48 items-center justify-center text-sm text-zinc-600">
              No token data yet
            </div>
          ) : (
            <div className="h-48">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={tokensByRepo} layout="vertical" margin={{ left: 8, right: 8 }}>
                  <XAxis type="number" hide tickFormatter={formatCount} />
                  <YAxis
                    type="category"
                    dataKey="name"
                    width={90}
                    tick={{ fill: '#a1a1aa', fontSize: 11 }}
                  />
                  <Tooltip
                    contentStyle={tooltipStyle}
                    itemStyle={tooltipItemStyle}
                    labelStyle={tooltipLabelStyle}
                    cursor={{ fill: 'rgba(255,255,255,0.03)' }}
                    formatter={(v: number) => [formatCount(v), 'tokens']}
                  />
                  <Bar dataKey="tokens" fill="#34d399" radius={[0, 4, 4, 0]} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
