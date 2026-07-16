import type { Session } from '@mc/protocol';
import { Card } from '@mc/ui';
import { useMemo } from 'react';
import { EventRow } from '../../components/EventRow';
import { useEvents } from '../../store/live';

export function ActivityTab({ session }: { session: Session }) {
  const events = useEvents();
  const sessionEvents = useMemo(
    () => events.filter((e) => e.sessionId === session.id),
    [events, session.id],
  );

  if (sessionEvents.length === 0) {
    return (
      <Card className="p-10 text-center text-sm text-zinc-500">
        No activity recorded for this session yet.
      </Card>
    );
  }

  return (
    <div className="space-y-2">
      {sessionEvents.map((e, i) => (
        <EventRow key={e.id} event={e} index={i} />
      ))}
    </div>
  );
}
