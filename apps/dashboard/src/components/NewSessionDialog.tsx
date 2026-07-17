import { Button, Dialog } from '@mc/ui';
import { useNavigate } from '@tanstack/react-router';
import { useState } from 'react';
import { useMachines } from '../store/live';

interface Props {
  open: boolean;
  onClose: () => void;
}

/** Start a new interactive session (Claude or Codex) on a chosen machine. */
export function NewSessionDialog({ open, onClose }: Props) {
  const machines = useMachines().filter((m) => m.online);
  const navigate = useNavigate();

  const [machineId, setMachineId] = useState('');
  const [providerId, setProviderId] = useState('claude-code');
  const [cwd, setCwd] = useState('');
  const [initialText, setInitialText] = useState('');

  const selectClass =
    'h-9 w-full rounded-lg border border-white/[0.08] bg-white/[0.02] px-3 text-sm text-zinc-200 outline-none focus:border-indigo-500/50';

  const start = () => {
    const target = machineId || machines[0]?.id;
    if (!target) return;
    const ptyId = crypto.randomUUID();
    // Navigate to the live terminal; the page issues terminal.open on mount.
    navigate({
      to: '/terminal/$ptyId',
      params: { ptyId },
      search: { machineId: target, provider: providerId, cwd: cwd || '.', initialText },
    });
    onClose();
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="New interactive session"
      description="Start a Claude session on a machine and drive it live from your browser."
    >
      <div className="space-y-3">
        <div>
          <label className="mb-1 block text-xs text-zinc-500">Machine</label>
          <select
            className={selectClass}
            value={machineId}
            onChange={(e) => setMachineId(e.target.value)}
          >
            {machines.length === 0 && <option value="">No online machines</option>}
            {machines.map((m) => (
              <option key={m.id} value={m.id}>
                {m.hostname}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="mb-1 block text-xs text-zinc-500">Agent</label>
          <select
            className={selectClass}
            value={providerId}
            onChange={(e) => setProviderId(e.target.value)}
          >
            <option value="claude-code">Claude Code</option>
            <option value="codex">OpenAI Codex</option>
          </select>
        </div>
        <div>
          <label className="mb-1 block text-xs text-zinc-500">Working directory (repo path)</label>
          <input
            className={selectClass}
            placeholder="/Users/you/work/my-repo"
            value={cwd}
            onChange={(e) => setCwd(e.target.value)}
          />
        </div>
        <div>
          <label className="mb-1 block text-xs text-zinc-500">Initial prompt (optional)</label>
          <input
            className={selectClass}
            placeholder="e.g. review the auth module"
            value={initialText}
            onChange={(e) => setInitialText(e.target.value)}
          />
        </div>
        <div className="flex justify-end gap-2 pt-1">
          <Button variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" size="sm" disabled={machines.length === 0} onClick={start}>
            Start session
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
