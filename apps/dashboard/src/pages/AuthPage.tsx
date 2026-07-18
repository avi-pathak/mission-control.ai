import { Button, Card, cn, LogoMark } from '@mc/ui';
import { useState } from 'react';
import { ApiError } from '../lib/api';
import { useAuthStore } from '../store/auth';

type Mode = 'login' | 'register';

// Pragmatic email check (mirrors the server-side rule).
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function AuthPage() {
  const [mode, setMode] = useState<Mode>('login');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [pending, setPending] = useState(false); // signup submitted, awaiting approval
  const login = useAuthStore((s) => s.login);
  const register = useAuthStore((s) => s.register);

  // Accept-invite mode is entered via ?token= in the URL.
  const inviteToken = new URLSearchParams(window.location.search).get('token');
  const accepting = window.location.pathname.startsWith('/accept-invite') && inviteToken;
  const acceptInvite = useAuthStore((s) => s.acceptInvite);

  const emailValid = EMAIL_RE.test(email.trim());

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    // Client-side email validation (skip in accept-invite: email comes from the invite).
    if (!accepting && !emailValid) {
      setError('Enter a valid email address (e.g. you@company.com).');
      return;
    }
    setBusy(true);
    try {
      if (accepting) {
        await acceptInvite(inviteToken!, password);
        window.location.href = '/';
      } else if (mode === 'register') {
        await register(email.trim(), password);
        setPending(true); // show "awaiting approval" screen
      } else {
        await login(email.trim(), password);
        window.location.href = '/';
      }
    } catch (err) {
      setError(errorMessage(err, accepting ? 'accept' : mode));
    } finally {
      setBusy(false);
    }
  };

  const inputClass =
    'h-10 w-full rounded-lg border border-white/[0.08] bg-white/[0.02] px-3 text-sm text-zinc-200 outline-none focus:border-indigo-500/50';

  // Post-signup: account created, awaiting admin approval.
  if (pending) {
    return (
      <Shell title="Almost there" subtitle="Your account needs approval.">
        <div className="space-y-3 text-sm text-zinc-400">
          <p>
            Thanks for signing up. An administrator needs to <strong>approve your account</strong>{' '}
            and assign you to a workspace before you can sign in.
          </p>
          <p className="text-zinc-500">You'll be able to log in once you're approved.</p>
        </div>
        <Button
          variant="outline"
          size="md"
          className="mt-5 w-full"
          onClick={() => {
            setPending(false);
            setMode('login');
            setPassword('');
          }}
        >
          Back to sign in
        </Button>
      </Shell>
    );
  }

  return (
    <Shell
      title={accepting ? 'Accept your invite' : mode === 'register' ? 'Create your account' : 'Sign in'}
      subtitle={
        accepting
          ? 'Set a password to join the team.'
          : mode === 'register'
            ? 'Sign up — an admin will approve and assign your workspace.'
            : 'Welcome back.'
      }
    >
      <form onSubmit={submit} className="mt-5 space-y-3">
        {!accepting && (
          <div>
            <input
              type="email"
              required
              placeholder="you@company.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className={cn(
                inputClass,
                email && !emailValid && 'border-rose-500/50 focus:border-rose-500/60',
              )}
            />
            {email && !emailValid && (
              <p className="mt-1 text-xs text-rose-400">Enter a valid email address.</p>
            )}
          </div>
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
          {busy
            ? 'Please wait…'
            : accepting
              ? 'Join workspace'
              : mode === 'register'
                ? 'Create account'
                : 'Sign in'}
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
    </Shell>
  );
}

function Shell({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex flex-col items-center justify-center gap-3">
          <LogoMark className="h-14 w-14 text-white" />
          <span className="text-lg font-semibold tracking-tight">Mission Control.ai</span>
        </div>
        <Card className="p-6">
          <h1 className="text-base font-semibold text-zinc-100">{title}</h1>
          <p className="mt-1 text-sm text-zinc-500">{subtitle}</p>
          {children}
        </Card>
      </div>
    </div>
  );
}

function errorMessage(err: unknown, mode: 'login' | 'register' | 'accept'): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case 'pending_approval':
        return 'Your account is still awaiting admin approval.';
      case 'invalid_credentials':
        return 'Email or password is incorrect.';
      case 'email_taken':
        return 'An account with that email already exists.';
      case 'bad_email':
        return 'Enter a valid email address.';
      case 'bad_request':
        return err.message || 'Password must be 8+ characters.';
      default:
        return err.message || 'Something went wrong.';
    }
  }
  if (mode === 'accept') return 'Invalid or expired invite link.';
  return 'Something went wrong. Please try again.';
}
