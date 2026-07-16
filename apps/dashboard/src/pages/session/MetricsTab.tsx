import type { Session } from '@mc/protocol';
import { formatBytes, formatPct } from '@mc/shared';
import { Card, CardContent, CardHeader, CardTitle } from '@mc/ui';
import { useMemo } from 'react';
import {
  Area,
  AreaChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { useLiveStore } from '../../store/live';

export function MetricsTab({ session }: { session: Session }) {
  const points = useLiveStore((s) => s.metrics[session.id]) ?? [];

  const data = useMemo(
    () =>
      points.map((p) => ({
        time: new Date(p.ts).toLocaleTimeString(),
        cpu: Number(p.cpuPct.toFixed(1)),
        memMB: Number((p.memBytes / (1024 * 1024)).toFixed(1)),
      })),
    [points],
  );

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>CPU usage — {formatPct(session.cpuPct)}</CardTitle>
        </CardHeader>
        <CardContent>
          <ChartFrame>
            <AreaChart data={data}>
              <defs>
                <linearGradient id="cpu" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="#818cf8" stopOpacity={0.5} />
                  <stop offset="100%" stopColor="#818cf8" stopOpacity={0} />
                </linearGradient>
              </defs>
              <XAxis dataKey="time" hide />
              <YAxis width={36} tick={{ fill: '#71717a', fontSize: 11 }} />
              <Tooltip contentStyle={tooltipStyle} />
              <Area type="monotone" dataKey="cpu" stroke="#818cf8" fill="url(#cpu)" strokeWidth={2} />
            </AreaChart>
          </ChartFrame>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Memory — {formatBytes(session.memBytes)}</CardTitle>
        </CardHeader>
        <CardContent>
          <ChartFrame>
            <AreaChart data={data}>
              <defs>
                <linearGradient id="mem" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="#34d399" stopOpacity={0.5} />
                  <stop offset="100%" stopColor="#34d399" stopOpacity={0} />
                </linearGradient>
              </defs>
              <XAxis dataKey="time" hide />
              <YAxis width={44} tick={{ fill: '#71717a', fontSize: 11 }} />
              <Tooltip contentStyle={tooltipStyle} />
              <Area type="monotone" dataKey="memMB" stroke="#34d399" fill="url(#mem)" strokeWidth={2} />
            </AreaChart>
          </ChartFrame>
        </CardContent>
      </Card>
    </div>
  );
}

const tooltipStyle = {
  background: '#111114',
  border: '1px solid rgba(255,255,255,0.08)',
  borderRadius: 8,
  fontSize: 12,
} as const;

function ChartFrame({ children }: { children: React.ReactElement }) {
  return (
    <div className="h-56 w-full">
      <ResponsiveContainer width="100%" height="100%">
        {children}
      </ResponsiveContainer>
    </div>
  );
}
