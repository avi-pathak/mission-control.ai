import type { Session } from '@mc/protocol';
import { Button, Card } from '@mc/ui';
import { Download, Pause, Play, Search } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { ansiToHtml } from '../../lib/ansi';
import { useLiveStore } from '../../store/live';

export function LogsTab({ session }: { session: Session }) {
  const logs = useLiveStore((s) => s.logs[session.id]) ?? [];
  const [autoScroll, setAutoScroll] = useState(true);
  const [query, setQuery] = useState('');
  const containerRef = useRef<HTMLDivElement>(null);

  const filtered = useMemo(() => {
    if (!query) return logs;
    const q = query.toLowerCase();
    return logs.filter((l) => l.line.toLowerCase().includes(q));
  }, [logs, query]);

  useEffect(() => {
    if (autoScroll && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [filtered.length, autoScroll]);

  const download = () => {
    const text = logs.map((l) => l.line).join('\n');
    const blob = new Blob([text], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${session.repo || 'session'}-${session.id}.log`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <Card className="flex h-[calc(100vh-18rem)] flex-col overflow-hidden">
      <div className="flex items-center gap-2 border-b border-white/[0.06] p-3">
        <div className="relative flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-zinc-500" />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search logs…"
            className="h-9 w-full rounded-lg border border-white/[0.06] bg-white/[0.02] pl-9 pr-3 text-sm outline-none focus:border-indigo-500/50"
          />
        </div>
        <Button variant="outline" size="sm" onClick={() => setAutoScroll((v) => !v)}>
          {autoScroll ? <Pause className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />}
          {autoScroll ? 'Pause' : 'Follow'}
        </Button>
        <Button variant="outline" size="sm" onClick={download}>
          <Download className="h-3.5 w-3.5" />
          Download
        </Button>
      </div>

      <div
        ref={containerRef}
        onScroll={(e) => {
          const el = e.currentTarget;
          const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
          if (!atBottom && autoScroll) setAutoScroll(false);
        }}
        className="flex-1 overflow-y-auto bg-black/30 p-3 font-mono text-xs leading-relaxed"
      >
        {filtered.length === 0 ? (
          <div className="py-10 text-center text-zinc-600">No log lines yet.</div>
        ) : (
          filtered.map((l) => (
            <div
              key={`${l.seq}-${l.ts}`}
              className="whitespace-pre-wrap break-all text-zinc-300"
              // ANSI codes are converted to sanitized HTML (text is escaped).
              dangerouslySetInnerHTML={{ __html: ansiToHtml(l.line) }}
            />
          ))
        )}
      </div>
    </Card>
  );
}
