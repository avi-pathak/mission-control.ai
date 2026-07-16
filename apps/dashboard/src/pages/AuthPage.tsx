import { Button, Card, cn, LogoMark } from '@mc/ui';
import { useState } from 'react';
import { useAuthStore } from '../store/auth';

type Mode = 'login' | 'register';

export function AuthPage() {
  const [mode, setMode] = useState<Mode>('login');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [orgName, setOrgName] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const login = useAuthStore((s) => s.login);
  const register = useAuthStore((s) => s.register);

  // Accept-invite mode is entered via ?token= in the URL.
  const inviteToken = new URLSearchParams(window.location.search).get('token');
  const accepting = window.location.pathname.startsWith('/accept-invite') && inviteToken;
  const acceptInvite = useAuthStore((s) => s.acceptInvite);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setBusy(true);
    try {
      if (accepting) await acceptInvite(inviteToken!, password);
      else if (mode === 'register') await register(email, password, orgName || undefined);
      else await login(email, password);
      window.location.href = '/';
    } catch (err) {
      setError(
        accepting || mode === 'login'
          ? 'Invalid credentials or expired link.'
          : (err as Error).message.includes('409')
            ? 'An account with that email already exists.'
            : 'Could not create account. Password must be 8+ characters.',
      );
    } finally {
      setBusy(false);
    }
  };

  const inputClass =
    'h-10 w-full rounded-lg border border-white/[0.08] bg-white/[0.02] px-3 text-sm text-zinc-200 outline-none focus:border-indigo-500/50';

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex flex-col items-center justify-center gap-3">
          <LogoMark className="h-14 w-14 text-white" />
          <span className="text-lg font-semibold tracking-tight">Mission Control.ai</span>
        </div>
        <Card className="p-6">
          <h1 className="text-base font-semibold text-zinc-100">
            {accepting ? 'Accept your invite' : mode === 'register' ? 'Create your workspace' : 'Sign in'}
          </h1>
          <p className="mt-1 text-sm text-zinc-500">
            {accepting
              ? 'Set a password to join the team.'
              : mode === 'register'
                ? 'Start monitoring your agents in minutes.'
                : 'Welcome back.'}
          </p>

          <form onSubmit={submit} className="mt-5 space-y-3">
            {!accepting && (
              <input
                type="email"
                required
                placeholder="you@company.com"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className={inputClass}
              />
            )}
            {!accepting && mode === 'register' && (
              <input
                placeholder="Workspace name (optional)"
                value={orgName}
                onChange={(e) => setOrgName(e.target.value)}
                className={inputClass}
              />
            )}
            <input
              type="password"
              required
              placeholder="Password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className={inputClass}
            />
            {error && <p className="text-xs text-rose-400">{error}</p>}
            <Button type="submit" variant="primary" size="md" className="w-full" disabled={busy}>
              {busy ? 'Please wait…' : accepting ? 'Join workspace' : mode === 'register' ? 'Create account' : 'Sign in'}
            </Button>
          </form>

          {!accepting && (
            <button
              onClick={() => {
                setMode(mode === 'login' ? 'register' : 'login');
                setError('');
              }}
              className={cn('mt-4 w-full text-center text-xs text-zinc-500 hover:text-zinc-300')}
            >
              {mode === 'login'
                ? "Don't have an account? Create one"
                : 'Already have an account? Sign in'}
            </button>
          )}
        </Card>
      </div>
    </div>
  );
}
