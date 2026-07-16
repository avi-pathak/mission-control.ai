import { useQuery } from '@tanstack/react-query';
import { useEffect } from 'react';
import { api } from '../lib/api';
import { useLiveStore } from '../store/live';

/**
 * Hydrates the live store with persisted logs + metrics for a session from the
 * REST API on mount. This backfills history that predates the current
 * WebSocket connection (or was lost on page refresh), then live WS messages
 * continue to append. Merges are deduped by seq/ts so nothing is duplicated.
 */
export function useSessionBackfill(sessionId: string) {
  const hydrateLogs = useLiveStore((s) => s.hydrateLogs);
  const hydrateMetrics = useLiveStore((s) => s.hydrateMetrics);

  const logsQuery = useQuery({
    queryKey: ['logs', sessionId],
    queryFn: () => api.logs(sessionId, 0, 2000),
    staleTime: 10_000,
  });

  const metricsQuery = useQuery({
    queryKey: ['metrics', sessionId],
    queryFn: () => api.metrics(sessionId, 120),
    staleTime: 10_000,
  });

  useEffect(() => {
    if (logsQuery.data?.lines?.length) {
      hydrateLogs(
        sessionId,
        logsQuery.data.lines.map((l) => ({
          seq: l.seq,
          stream: l.stream,
          line: l.line,
          ts: l.ts,
        })),
      );
    }
  }, [logsQuery.data, sessionId, hydrateLogs]);

  useEffect(() => {
    if (metricsQuery.data?.length) {
      hydrateMetrics(
        sessionId,
        metricsQuery.data.map((m) => ({ ts: m.ts, cpuPct: m.cpuPct, memBytes: m.memBytes })),
      );
    }
  }, [metricsQuery.data, sessionId, hydrateMetrics]);

  return { loading: logsQuery.isLoading || metricsQuery.isLoading };
}
