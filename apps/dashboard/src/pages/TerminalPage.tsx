import { Link, useParams, useSearch } from '@tanstack/react-router';
import { ArrowLeft } from 'lucide-react';
import { InteractiveTerminal } from './session/InteractiveTerminal';

/** Full-page interactive terminal for an agent-launched PTY session. */
export function TerminalPage() {
  const { ptyId } = useParams({ from: '/terminal/$ptyId' });
  const search = useSearch({ from: '/terminal/$ptyId' });

  return (
    <div className="h-full space-y-4 overflow-y-auto pr-1">
      <Link to="/" className="inline-flex items-center gap-2 text-sm text-zinc-400 hover:text-white">
        <ArrowLeft className="h-4 w-4" /> Overview
      </Link>
      <header>
        <h1 className="text-xl font-semibold tracking-tight">Interactive session</h1>
        <p className="mt-1 text-sm text-zinc-500">
          {search.provider === 'codex' ? 'Codex' : 'Claude'} running in{' '}
          <span className="font-mono">{search.cwd}</span>
        </p>
      </header>
      <InteractiveTerminal
        ptyId={ptyId}
        open={{
          machineId: search.machineId,
          provider: search.provider,
          cwd: search.cwd,
          initialText: search.initialText || undefined,
        }}
      />
    </div>
  );
}
