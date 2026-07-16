import type { Session } from '@mc/protocol';
import { formatRelativeTime } from '@mc/shared';
import { Card, CardContent, CardHeader, CardTitle } from '@mc/ui';
import { GitBranch, GitCommit, FileDiff } from 'lucide-react';

export function GitTab({ session }: { session: Session }) {
  const git = session.git;
  if (!git) {
    return (
      <Card className="p-10 text-center text-sm text-zinc-500">
        This session is not running inside a git repository.
      </Card>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Repository</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <div className="flex items-center gap-2 text-zinc-200">
            <GitBranch className="h-4 w-4 text-indigo-400" />
            <span className="font-medium">{git.branch}</span>
            {git.dirty && (
              <span className="rounded bg-amber-500/15 px-1.5 py-0.5 text-xs text-amber-400">
                dirty
              </span>
            )}
          </div>
          <div className="text-xs text-zinc-500">{git.remoteUrl || 'no remote'}</div>
          <div className="text-xs text-zinc-500">
            ↑ {git.ahead} ahead · ↓ {git.behind} behind
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>
            <span className="inline-flex items-center gap-2">
              <FileDiff className="h-4 w-4 text-amber-400" />
              Modified files ({git.modifiedFiles.length})
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {git.modifiedFiles.length === 0 ? (
            <div className="text-sm text-zinc-500">Working tree clean.</div>
          ) : (
            <ul className="max-h-48 space-y-1 overflow-y-auto font-mono text-xs text-zinc-400">
              {git.modifiedFiles.map((f) => (
                <li key={f} className="truncate">
                  {f}
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <Card className="lg:col-span-2">
        <CardHeader>
          <CardTitle>Recent commits</CardTitle>
        </CardHeader>
        <CardContent>
          <ul className="space-y-3">
            {(git.recentCommits ?? []).map((c) => (
              <li key={c.hash} className="flex items-start gap-3">
                <GitCommit className="mt-0.5 h-4 w-4 shrink-0 text-zinc-600" />
                <div className="min-w-0">
                  <div className="truncate text-sm text-zinc-200">{c.message}</div>
                  <div className="text-xs text-zinc-500">
                    <span className="font-mono">{c.hash.slice(0, 7)}</span> · {c.author} ·{' '}
                    {formatRelativeTime(c.ts)}
                  </div>
                </div>
              </li>
            ))}
            {(git.recentCommits ?? []).length === 0 && (
              <li className="text-sm text-zinc-500">No commit history available.</li>
            )}
          </ul>
        </CardContent>
      </Card>
    </div>
  );
}
