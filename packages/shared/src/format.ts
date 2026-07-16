/** Human-friendly formatters shared across the dashboard. */

export function formatBytes(bytes: number): string {
  if (!bytes || bytes < 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export function formatPct(pct: number): string {
  return `${pct.toFixed(pct >= 10 ? 0 : 1)}%`;
}

/** Compact count: 1234 → "1.2K", 4_500_000 → "4.5M", 736_000_000 → "736M". */
export function formatCount(n: number): string {
  if (!n || n < 0) return '0';
  if (n < 1000) return String(n);
  const units = [
    { v: 1e9, s: 'B' },
    { v: 1e6, s: 'M' },
    { v: 1e3, s: 'K' },
  ];
  for (const u of units) {
    if (n >= u.v) {
      const val = n / u.v;
      return `${val >= 100 ? Math.round(val) : val.toFixed(1)}${u.s}`;
    }
  }
  return String(n);
}

/** Compact duration since a unix-millis timestamp, e.g. "3m", "2h 14m". */
export function formatDuration(fromMs: number, nowMs = Date.now()): string {
  const s = Math.max(0, Math.floor((nowMs - fromMs) / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  const rem = m % 60;
  if (h < 24) return rem ? `${h}h ${rem}m` : `${h}h`;
  const d = Math.floor(h / 24);
  return `${d}d ${h % 24}h`;
}

export function formatRelativeTime(tsMs: number, nowMs = Date.now()): string {
  const diff = nowMs - tsMs;
  if (diff < 5000) return 'just now';
  return `${formatDuration(tsMs, nowMs)} ago`;
}
