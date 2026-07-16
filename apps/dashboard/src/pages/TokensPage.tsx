import type { Session } from '@mc/protocol';
import { formatCount } from '@mc/shared';
import { Card, CardContent, CardHeader, CardTitle } from '@mc/ui';
import { ArrowDownToLine, ArrowUpFromLine, Coins, Database, HardDriveDownload } from 'lucide-react';
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
import { useSessions } from '../store/live';

const tooltipStyle = {
  background: '#111114',
  border: '1px solid rgba(255,255,255,0.08)',
  borderRadius: 8,
  fontSize: 12,
  color: '#e4e4e7',
} as const;
const itemStyle = { color: '#e4e4e7' } as const;
const labelStyle = { color: '#a1a1aa' } as const;

interface Totals {
  input: number;
  output: number;
  cacheRead: number;
  cacheCreation: number;
  total: number;
}

const empty: Totals = { input: 0, output: 0, cacheRead: 0, cacheCreation: 0, total: 0 };

function sumTokens(sessions: Session[]): Totals {
  const seen = new Set<string>();
  const t = { ...empty };
  for (const s of sessions) {
    if (seen.has(s.id) || !s.tokens) continue;
    seen.add(s.id);
    t.input += s.tokens.input;
    t.output += s.tokens.output;
    t.cacheRead += s.tokens.cacheRead;
    t.cacheCreation += s.tokens.cacheCreation;
  }
  t.total = t.input + t.output + t.cacheRead + t.cacheCreation;
  return t;
}

export function TokensPage() {
  const sessions = useSessions();

  const all = useMemo(() => sumTokens(sessions), [sessions]);
  const running = useMemo(
    () => sumTokens(sessions.filter((s) => s.status === 'running')),
    [sessions],
  );

  const composition = useMemo(
    () =>
      [
        { name: 'Input', value: all.input, color: '#38bdf8' },
        { name: 'Output', value: all.output, color: '#34d399' },
        { name: 'Cache read', value: all.cacheRead, color: '#818cf8' },
        { name: 'Cache write', value: all.cacheCreation, color: '#fbbf24' },
      ].filter((d) => d.value > 0),
    [all],
  );

  const perSession = useMemo(() => {
    const seen = new Set<string>();
    return sessions
      .filter((s) => {
        if (seen.has(s.id) || !s.tokens) return false;
        seen.add(s.id);
        return true;
      })
      .map((s) => ({
        id: s.id,
        name: s.repo || s.cwd.split('/').pop() || s.id.slice(-6),
        input: s.tokens!.input,
        output: s.tokens!.output,
        cacheRead: s.tokens!.cacheRead,
        total: s.tokens!.total,
        running: s.status === 'running',
      }))
      .sort((a, b) => b.total - a.total);
  }, [sessions]);

  const topByTotal = perSession.slice(0, 8).map((s) => ({ name: s.name, total: s.total }));

  return (
    <div className="h-full space-y-6 overflow-y-auto pr-1">
      <header>
        <h1 className="flex items-center gap-2 text-xl font-semibold tracking-tight">
          <Coins className="h-5 w-5 text-sky-400" /> Token usage
        </h1>
        <p className="mt-1 text-sm text-zinc-500">
          Cumulative Claude token consumption across your fleet.
        </p>
      </header>

      {/* Headline totals */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <BigStat label="Total tokens (all sessions)" value={all.total} icon={Coins} accent="text-sky-400" />
        <BigStat label="Total (running now)" value={running.total} icon={Coins} accent="text-emerald-400" />
        <BigStat label="Input tokens" value={all.input} icon={ArrowUpFromLine} accent="text-sky-400" />
        <BigStat label="Output tokens" value={all.output} icon={ArrowDownToLine} accent="text-emerald-400" />
      </div>

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <BigStat label="Cache read" value={all.cacheRead} icon={Database} accent="text-indigo-400" />
        <BigStat label="Cache write" value={all.cacheCreation} icon={HardDriveDownload} accent="text-amber-400" />
        <BigStat
          label="Running input"
          value={running.input}
          icon={ArrowUpFromLine}
          accent="text-sky-400"
        />
        <BigStat
          label="Running output"
          value={running.output}
          icon={ArrowDownToLine}
          accent="text-emerald-400"
        />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Token composition</CardTitle>
          </CardHeader>
          <CardContent>
            {composition.length === 0 ? (
              <div className="flex h-56 items-center justify-center text-sm text-zinc-600">
                No token data yet
              </div>
            ) : (
              <>
                <div className="h-56">
                  <ResponsiveContainer width="100%" height="100%">
                    <PieChart>
                      <Pie
                        data={composition}
                        dataKey="value"
                        nameKey="name"
                        innerRadius={55}
                        outerRadius={85}
                        paddingAngle={2}
                      >
                        {composition.map((d) => (
                          <Cell key={d.name} fill={d.color} />
                        ))}
                      </Pie>
                      <Tooltip
                        contentStyle={tooltipStyle}
                        itemStyle={itemStyle}
                        labelStyle={labelStyle}
                        formatter={(v: number, _n, p) => [
                          `${formatCount(v)} (${((v / all.total) * 100).toFixed(1)}%)`,
                          String(p?.payload?.name ?? ''),
                        ]}
                      />
                    </PieChart>
                  </ResponsiveContainer>
                </div>
                <div className="mt-2 flex flex-wrap justify-center gap-x-4 gap-y-1 text-xs">
                  {composition.map((d) => (
                    <span key={d.name} className="inline-flex items-center gap-1.5 text-zinc-400">
                      <span className="h-2 w-2 rounded-full" style={{ background: d.color }} />
                      {d.name} · {formatCount(d.value)}
                    </span>
                  ))}
                </div>
              </>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Top sessions by total tokens</CardTitle>
          </CardHeader>
          <CardContent>
            {topByTotal.length === 0 ? (
              <div className="flex h-56 items-center justify-center text-sm text-zinc-600">
                No token data yet
              </div>
            ) : (
              <div className="h-56">
                <ResponsiveContainer width="100%" height="100%">
                  <BarChart data={topByTotal} layout="vertical" margin={{ left: 8, right: 8 }}>
                    <XAxis type="number" hide />
                    <YAxis
                      type="category"
                      dataKey="name"
                      width={100}
                      tick={{ fill: '#a1a1aa', fontSize: 11 }}
                    />
                    <Tooltip
                      contentStyle={tooltipStyle}
                      itemStyle={itemStyle}
                      labelStyle={labelStyle}
                      cursor={{ fill: 'rgba(255,255,255,0.03)' }}
                      formatter={(v: number) => [formatCount(v), 'total tokens']}
                    />
                    <Bar dataKey="total" fill="#818cf8" radius={[0, 4, 4, 0]} />
                  </BarChart>
                </ResponsiveContainer>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Per-session breakdown table */}
      <Card className="overflow-hidden">
        <CardHeader>
          <CardTitle>Per-session breakdown</CardTitle>
        </CardHeader>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-y border-white/[0.06] text-left text-xs uppercase tracking-wide text-zinc-500">
                <th className="px-4 py-3 font-medium">Session</th>
                <th className="px-4 py-3 text-right font-medium">Input</th>
                <th className="px-4 py-3 text-right font-medium">Output</th>
                <th className="px-4 py-3 text-right font-medium">Cache read</th>
                <th className="px-4 py-3 text-right font-medium">Total</th>
              </tr>
            </thead>
            <tbody>
              {perSession.map((s) => (
                <tr key={s.id} className="border-b border-white/[0.03]">
                  <td className="px-4 py-2.5">
                    <span className="inline-flex items-center gap-2 text-zinc-200">
                      {s.running && <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />}
                      {s.name}
                    </span>
                  </td>
                  <td className="px-4 py-2.5 text-right tabular-nums text-sky-400">
                    {formatCount(s.input)}
                  </td>
                  <td className="px-4 py-2.5 text-right tabular-nums text-emerald-400">
                    {formatCount(s.output)}
                  </td>
                  <td className="px-4 py-2.5 text-right tabular-nums text-indigo-400">
                    {formatCount(s.cacheRead)}
                  </td>
                  <td className="px-4 py-2.5 text-right tabular-nums font-medium text-zinc-200">
                    {formatCount(s.total)}
                  </td>
                </tr>
              ))}
              {perSession.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-4 py-10 text-center text-sm text-zinc-500">
                    No sessions with token data yet.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  );
}

function BigStat({
  label,
  value,
  icon: Icon,
  accent,
}: {
  label: string;
  value: number;
  icon: typeof Coins;
  accent: string;
}) {
  return (
    <Card className="p-4">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium uppercase tracking-wide text-zinc-500">{label}</span>
        <Icon className={`h-4 w-4 ${accent}`} />
      </div>
      <div className="mt-2 text-2xl font-semibold tabular-nums text-zinc-100">
        {formatCount(value)}
      </div>
      <div className="mt-0.5 text-xs text-zinc-600">{value.toLocaleString()}</div>
    </Card>
  );
}
