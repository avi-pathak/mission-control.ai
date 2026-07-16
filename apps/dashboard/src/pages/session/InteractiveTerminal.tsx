import { MessageType, type TerminalOpened } from '@mc/protocol';
import { Card } from '@mc/ui';
import { FitAddon } from '@xterm/addon-fit';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import { useEffect, useRef, useState } from 'react';
import { subscribeTerminal, useLiveStore } from '../../store/live';

interface Props {
  /** ptyId to attach to (already opened) OR to open. */
  ptyId: string;
  /** When set, opens a NEW PTY on this machine/cwd. */
  open?: { machineId: string; provider: string; cwd: string; initialText?: string | undefined };
  /** When set, ATTACHES to an existing tmux-backed session (approve/drive). */
  attach?: { machineId: string; sessionId: string };
}

const encoder = new TextEncoder();

function toB64(s: string): string {
  return btoa(String.fromCharCode(...encoder.encode(s)));
}

/** Fully interactive xterm bound to an agent PTY (open) or tmux pane (attach). */
export function InteractiveTerminal({ ptyId, open, attach }: Props) {
  const send = useLiveStore((s) => s.send);
  const elRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<Terminal | null>(null);
  const [status, setStatus] = useState<'connecting' | 'live' | 'exited' | 'error'>('connecting');
  const [errMsg, setErrMsg] = useState('');

  // Quick-input helper (used by Approve/Deny buttons).
  const sendInput = (text: string) =>
    send(MessageType.TerminalInput, { ptyId, data: toB64(text) });

  useEffect(() => {
    if (!elRef.current) return;
    const term = new Terminal({
      cursorBlink: true,
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
      fontSize: 12,
      theme: { background: '#0a0a0e', foreground: '#d4d4d8' },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(elRef.current);
    fit.fit();
    termRef.current = term;

    // Keystrokes → agent.
    const onData = term.onData((data) => {
      send(MessageType.TerminalInput, { ptyId, data: toB64(data) });
    });

    // Keep the tmux window sized to the xterm. Debounced so we only push a
    // resize once the layout settles, and again whenever the panel changes.
    let lastCols = 0;
    let lastRows = 0;
    const pushResize = () => {
      try {
        fit.fit();
      } catch {
        /* fit can throw if the element is detached */
      }
      if (term.cols !== lastCols || term.rows !== lastRows) {
        lastCols = term.cols;
        lastRows = term.rows;
        send(MessageType.TerminalResize, { ptyId, cols: term.cols, rows: term.rows });
      }
    };
    const ro = new ResizeObserver(() => pushResize());
    ro.observe(elRef.current);
    window.addEventListener('resize', pushResize);

    // Subscribe to output/lifecycle for this ptyId.
    const unsub = subscribeTerminal(ptyId, {
      onOutput: (bytes) => term.write(bytes),
      onOpened: (p: TerminalOpened) => {
        if (p.ok) setStatus('live');
        else {
          setStatus('error');
          setErrMsg(p.error || 'failed to open terminal');
        }
      },
      onExit: () => setStatus('exited'),
    });

    // Fit once the DOM has laid out, then open/attach with the real size.
    fit.fit();
    lastCols = term.cols;
    lastRows = term.rows;

    // Open a new PTY, or attach to an existing tmux session.
    if (open) {
      send(MessageType.TerminalOpen, {
        ptyId,
        machineId: open.machineId,
        provider: open.provider,
        cwd: open.cwd,
        initialText: open.initialText,
        cols: term.cols,
        rows: term.rows,
      });
    } else if (attach) {
      send(MessageType.TerminalAttach, {
        ptyId,
        machineId: attach.machineId,
        sessionId: attach.sessionId,
        cols: term.cols,
        rows: term.rows,
      });
    } else {
      setStatus('live');
    }

    return () => {
      onData.dispose();
      ro.disconnect();
      window.removeEventListener('resize', pushResize);
      unsub();
      send(MessageType.TerminalClose, { ptyId });
      term.dispose();
      termRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ptyId]);

  return (
    <Card className="flex h-[calc(100vh-18rem)] flex-col overflow-hidden">
      <div className="flex items-center justify-between border-b border-white/[0.06] px-3 py-2 text-xs">
        <span className="text-zinc-400">
          {attach ? 'Interactive (tmux) — drive this session' : 'Interactive — you are driving this session'}
        </span>
        <div className="flex items-center gap-2">
          {/* Quick approval controls */}
          <button
            onClick={() => sendInput('y\r')}
            className="rounded-md border border-emerald-500/30 px-2 py-0.5 text-emerald-400 hover:bg-emerald-500/10"
            title="Approve (send y + Enter)"
          >
            Approve
          </button>
          <button
            onClick={() => sendInput('n\r')}
            className="rounded-md border border-rose-500/30 px-2 py-0.5 text-rose-400 hover:bg-rose-500/10"
            title="Deny (send n + Enter)"
          >
            Deny
          </button>
          <button
            onClick={() => sendInput('\r')}
            className="rounded-md border border-white/10 px-2 py-0.5 text-zinc-400 hover:bg-white/5"
            title="Send Enter"
          >
            ⏎
          </button>
          <span
            className={
              status === 'live'
                ? 'text-emerald-400'
                : status === 'error'
                  ? 'text-rose-400'
                  : status === 'exited'
                    ? 'text-zinc-500'
                    : 'text-amber-400'
            }
          >
            {status === 'live'
              ? '● live'
              : status === 'error'
                ? `error: ${errMsg}`
                : status === 'exited'
                  ? 'session ended'
                  : 'connecting…'}
          </span>
        </div>
      </div>
      <div ref={elRef} className="flex-1 overflow-hidden bg-[#0a0a0e] p-2" />
    </Card>
  );
}
