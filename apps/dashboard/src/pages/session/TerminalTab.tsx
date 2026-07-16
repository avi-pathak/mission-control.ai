import type { Session } from '@mc/protocol';
import { Card } from '@mc/ui';
import { FitAddon } from '@xterm/addon-fit';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import { useEffect, useMemo, useRef } from 'react';
import { useLiveStore } from '../../store/live';
import { InteractiveTerminal } from './InteractiveTerminal';

/** Terminal tab: interactive (drive + approve) when the session runs in tmux,
 *  otherwise a read-only mirror of the log stream. */
export function TerminalTab({ session }: { session: Session }) {
  // A stable ptyId per (session) so remounts reuse the same attach id.
  const ptyId = useMemo(() => `attach-${session.id}`, [session.id]);

  if (session.tmuxSession) {
    return (
      <InteractiveTerminal
        ptyId={ptyId}
        attach={{ machineId: session.machineId, sessionId: session.id }}
      />
    );
  }
  return <ReadOnlyTerminal session={session} />;
}

/** Read-only xterm.js view fed by the session's live log stream. */
function ReadOnlyTerminal({ session }: { session: Session }) {
  const logs = useLiveStore((s) => s.logs[session.id]) ?? [];
  const elRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<Terminal | null>(null);
  const writtenRef = useRef(0);
  const firstSeqRef = useRef<number | null>(null);

  useEffect(() => {
    if (!elRef.current) return;
    const term = new Terminal({
      convertEol: true,
      disableStdin: true,
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
      fontSize: 12,
      theme: { background: '#0a0a0e', foreground: '#d4d4d8' },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(elRef.current);
    fit.fit();
    termRef.current = term;

    const onResize = () => fit.fit();
    window.addEventListener('resize', onResize);
    return () => {
      window.removeEventListener('resize', onResize);
      term.dispose();
      termRef.current = null;
      writtenRef.current = 0;
    };
  }, []);

  // Stream log lines into the terminal. Because backfill can prepend older
  // history (re-sorting the array), we detect a non-append change via the first
  // line's seq and rewrite the buffer; otherwise we append only the new tail.
  useEffect(() => {
    const term = termRef.current;
    if (!term) return;
    const firstSeq = logs[0]?.seq ?? null;
    const prepended = firstSeqRef.current !== null && firstSeq !== firstSeqRef.current;
    if (prepended || logs.length < writtenRef.current) {
      term.clear();
      writtenRef.current = 0;
    }
    firstSeqRef.current = firstSeq;
    for (let i = writtenRef.current; i < logs.length; i++) {
      term.writeln(logs[i]!.line);
    }
    writtenRef.current = logs.length;
  }, [logs]);

  return (
    <Card className="flex h-[calc(100vh-18rem)] flex-col overflow-hidden">
      <div className="border-b border-white/[0.06] px-3 py-2 text-xs text-zinc-500">
        Read-only — to drive &amp; approve this session from the dashboard, run Claude in tmux
        (e.g. <span className="font-mono text-zinc-400">tmux new -s work 'claude'</span>).
      </div>
      <div ref={elRef} className="flex-1 p-2" />
    </Card>
  );
}
