import type { SessionStatus } from '@mc/protocol';
import { statusMeta } from '@mc/shared';
import { Badge, cn } from '@mc/ui';

export function StatusBadge({ status }: { status: SessionStatus }) {
  const m = statusMeta(status);
  return (
    <Badge className={cn('gap-1.5', m.color)}>
      <span className={cn('h-1.5 w-1.5 rounded-full', m.dot)} />
      {m.label}
    </Badge>
  );
}
