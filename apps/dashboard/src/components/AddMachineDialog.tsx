import { Button, CopyButton, Dialog, cn } from '@mc/ui';
import { useMutation } from '@tanstack/react-query';
import { CheckCircle2, Loader2 } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { api, type CreateEnrollTokenResp } from '../lib/api';
import { useMachines } from '../store/live';

interface Props {
  open: boolean;
  onClose: () => void;
}

type Method = 'script' | 'docker' | 'binary';

const DOCKER_IMAGE = 'avipathak/mission-control-agent';

/** Rewrite a localhost/127.0.0.1 server URL to host.docker.internal so the
 *  command actually works from inside a container on Docker Desktop. */
function dockerServerUrl(serverUrl: string): string {
  return serverUrl.replace(/(\/\/)(localhost|127\.0\.0\.1)(:|\/|$)/, '$1host.docker.internal$3');
}

function methodCommand(method: Method, t: CreateEnrollTokenResp): string {
  switch (method) {
    case 'script':
      return t.command;
    case 'docker': {
      const url = dockerServerUrl(t.serverUrl);
      const isLocal = url !== t.serverUrl;
      return [
        '# Linux host: --pid=host + mounts let the agent see the HOST’s Claude',
        '# sessions. On macOS/Windows a container CANNOT see host processes —',
        '# use the Install script or Binary method there instead.',
        ...(isLocal
          ? ['# (server is local → using host.docker.internal so the container can reach it)']
          : []),
        'docker run -d --name mission-control-agent \\',
        '  --pid=host \\',
        '  -v "$HOME/.claude:/host/.claude:ro" \\',
        '  -e MC_CLAUDE_DIR="/host/.claude" \\',
        `  -e MC_SERVER_URL="${url}" \\`,
        `  -e MC_ENROLL_TOKEN="${t.token}" \\`,
        `  ${DOCKER_IMAGE}`,
      ].join('\n');
    }
    case 'binary': {
      const httpBase = t.serverUrl.replace(/^ws/, 'http');
      return [
        '# 1. Download the agent for your platform from this server:',
        `#    macOS (Apple Silicon): ${httpBase}/download/mission-control-agent-darwin-arm64`,
        `#    macOS (Intel):         ${httpBase}/download/mission-control-agent-darwin-amd64`,
        `#    Linux (x86_64):        ${httpBase}/download/mission-control-agent-linux-amd64`,
        `#    Linux (arm64):         ${httpBase}/download/mission-control-agent-linux-arm64`,
        `#    Windows:               ${httpBase}/download/mission-control-agent-windows-amd64.exe`,
        '# 2. chmod +x it, then run with:',
        `MC_SERVER_URL="${t.serverUrl}" \\`,
        `MC_ENROLL_TOKEN="${t.token}" \\`,
        '  ./mission-control-agent',
      ].join('\n');
    }
  }
}

const METHODS: { id: Method; label: string; hint: string }[] = [
  { id: 'script', label: 'Install script', hint: 'One-line curl installer (Linux/macOS)' },
  { id: 'docker', label: 'Docker', hint: 'Run on a Linux host (sees host sessions via --pid=host)' },
  { id: 'binary', label: 'Binary', hint: 'Prebuilt executable + env vars' },
];

export function AddMachineDialog({ open, onClose }: Props) {
  const machines = useMachines();
  const [token, setToken] = useState<CreateEnrollTokenResp | null>(null);
  const [method, setMethod] = useState<Method>('script');
  const baselineIds = useRef<Set<string> | null>(null);
  const [enrolledHost, setEnrolledHost] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: () => api.createEnrollToken('dashboard', 30),
    onSuccess: (t) => setToken(t),
  });

  useEffect(() => {
    if (open && !token && !create.isPending) {
      baselineIds.current = new Set(machines.map((m) => m.id));
      create.mutate();
    }
    if (!open) {
      setToken(null);
      setEnrolledHost(null);
      setMethod('script');
      baselineIds.current = null;
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  useEffect(() => {
    if (!open || !baselineIds.current) return;
    const fresh = machines.find((m) => !baselineIds.current!.has(m.id));
    if (fresh) setEnrolledHost(fresh.hostname);
  }, [machines, open]);

  const cmd = token ? methodCommand(method, token) : '';

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="Add a machine"
      description="Pick how you want to run the agent on the machine you're monitoring. It enrolls with a one-time token and connects automatically."
    >
      {enrolledHost ? (
        <div className="flex flex-col items-center gap-3 py-6 text-center">
          <CheckCircle2 className="h-10 w-10 text-emerald-400" />
          <div className="text-sm text-zinc-200">
            <span className="font-medium">{enrolledHost}</span> connected!
          </div>
          <Button variant="primary" size="sm" onClick={onClose}>
            Done
          </Button>
        </div>
      ) : (
        <div className="space-y-4">
          {/* Method tabs */}
          <div className="flex gap-1 rounded-lg border border-white/[0.06] bg-white/[0.02] p-1">
            {METHODS.map((m) => (
              <button
                key={m.id}
                onClick={() => setMethod(m.id)}
                className={cn(
                  'flex-1 rounded-md px-3 py-1.5 text-xs font-medium transition-colors',
                  method === m.id
                    ? 'bg-white/[0.08] text-white'
                    : 'text-zinc-400 hover:text-zinc-200',
                )}
              >
                {m.label}
              </button>
            ))}
          </div>

          <div className="rounded-lg border border-white/10 bg-black/40 p-3">
            <div className="mb-2 text-xs text-zinc-500">
              {METHODS.find((m) => m.id === method)!.hint} · token expires in 30 min
            </div>
            {create.isPending || !token ? (
              <div className="flex items-center gap-2 py-2 text-sm text-zinc-500">
                <Loader2 className="h-4 w-4 animate-spin" /> Generating…
              </div>
            ) : (
              <>
                <pre className="overflow-x-auto whitespace-pre-wrap break-all font-mono text-xs leading-relaxed text-zinc-300">
                  {cmd}
                </pre>
                <div className="mt-3 flex justify-end">
                  <CopyButton value={cmd} label="Copy" />
                </div>
              </>
            )}
          </div>

          {/* Env var reference */}
          <div className="rounded-lg border border-white/[0.05] bg-white/[0.01] p-3 text-xs">
            <div className="mb-1.5 font-medium text-zinc-400">Required environment</div>
            <dl className="space-y-1 text-zinc-500">
              <div className="flex gap-2">
                <dt className="font-mono text-indigo-300">MC_SERVER_URL</dt>
                <dd>— this server's URL</dd>
              </div>
              <div className="flex gap-2">
                <dt className="font-mono text-indigo-300">MC_ENROLL_TOKEN</dt>
                <dd>— one-time token (shown above)</dd>
              </div>
            </dl>
          </div>

          <div className="flex items-center gap-2 text-xs text-zinc-500">
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
            Waiting for the machine to connect…
          </div>

          {create.isError && (
            <p className="text-xs text-rose-400">
              Failed to generate token. Check that the server is reachable.
            </p>
          )}
        </div>
      )}
    </Dialog>
  );
}
