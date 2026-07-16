import type { Session } from '@mc/protocol';
import { formatBytes, formatRelativeTime } from '@mc/shared';
import { Card } from '@mc/ui';
import { useQuery } from '@tanstack/react-query';
import { Download, FileText } from 'lucide-react';
import { api } from '../../lib/api';

export function FilesTab({ session }: { session: Session }) {
  const files = useQuery({
    queryKey: ['files', session.id],
    queryFn: () => api.files(session.id),
    refetchInterval: 10_000,
  });

  const rows = files.data ?? [];
  if (rows.length === 0) {
    return (
      <Card className="p-10 text-center text-sm text-zinc-500">
        No files published for this session yet.
        <div className="mt-2 font-mono text-xs text-zinc-600">
          mission-control-agent publish &lt;path&gt; --session {session.id}
        </div>
      </Card>
    );
  }

  return (
    <Card className="overflow-hidden">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-white/[0.06] text-left text-xs uppercase tracking-wide text-zinc-500">
            <th className="px-4 py-3 font-medium">Name</th>
            <th className="px-4 py-3 font-medium text-right">Size</th>
            <th className="px-4 py-3 font-medium">Published</th>
            <th className="px-4 py-3 font-medium text-right">Download</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((f) => (
            <tr key={f.id} className="border-b border-white/[0.03]">
              <td className="px-4 py-3">
                <span className="inline-flex items-center gap-2 text-zinc-200">
                  <FileText className="h-3.5 w-3.5 text-zinc-500" />
                  {f.name}
                </span>
              </td>
              <td className="px-4 py-3 text-right tabular-nums text-zinc-400">
                {formatBytes(f.size)}
              </td>
              <td className="px-4 py-3 text-zinc-400">{formatRelativeTime(Date.parse(f.createdAt))}</td>
              <td className="px-4 py-3 text-right">
                <a
                  href={api.fileDownloadUrl(f.id)}
                  className="inline-flex items-center gap-1.5 text-indigo-400 hover:text-indigo-300"
                >
                  <Download className="h-3.5 w-3.5" /> Download
                </a>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </Card>
  );
}
