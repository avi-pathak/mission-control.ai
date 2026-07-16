import {
  type ActivityEvent,
  type Envelope,
  type LogAppend,
  type Machine,
  type MetricSample,
  MessageType,
  type Session,
  type Snapshot,
  type TerminalExit,
  type TerminalOpened,
  type TerminalOutput,
} from '@mc/protocol';
import { ReconnectingWS, type WSStatus } from '@mc/shared';
import { create } from 'zustand';
import { useShallow } from 'zustand/react/shallow';
import { dashboardWsUrl } from '../lib/config';

// --- Interactive terminal pub/sub ---
// Terminal output is high-frequency raw bytes, so it bypasses the React store
// and is delivered directly to subscribers (the TerminalTab) keyed by ptyId.
type TermHandlers = {
  onOutput?: (bytes: Uint8Array) => void;
  onOpened?: (p: TerminalOpened) => void;
  onExit?: (p: TerminalExit) => void;
};
const termSubs = new Map<string, TermHandlers>();

export function subscribeTerminal(ptyId: string, handlers: TermHandlers): () => void {
  termSubs.set(ptyId, handlers);
  return () => termSubs.delete(ptyId);
}

function b64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

const MAX_LOGS_PER_SESSION = 5000;
const MAX_METRICS_PER_SESSION = 600;
const MAX_EVENTS = 500;

export interface LogEntry {
  seq: number;
  stream: string;
  line: string;
  ts: number;
}

export interface MetricPoint {
  ts: number;
  cpuPct: number;
  memBytes: number;
}

interface LiveState {
  status: WSStatus;
  machines: Record<string, Machine>;
  sessions: Record<string, Session>;
  logs: Record<string, LogEntry[]>;
  metrics: Record<string, MetricPoint[]>;
  events: ActivityEvent[];
  connect: () => void;
  disconnect: () => void;
  hydrateLogs: (sessionId: string, entries: LogEntry[]) => void;
  hydrateMetrics: (sessionId: string, points: MetricPoint[]) => void;
  hydrateFleet: (machines: Machine[], sessions: Session[]) => void;
  send: (type: MessageType, payload: unknown) => void;
}

/** Merge two ordered log arrays, deduped by seq, capped, sorted by seq. */
function mergeLogs(a: LogEntry[], b: LogEntry[]): LogEntry[] {
  const bySeq = new Map<number, LogEntry>();
  for (const e of a) bySeq.set(e.seq, e);
  for (const e of b) bySeq.set(e.seq, e);
  const merged = [...bySeq.values()].sort((x, y) => x.seq - y.seq);
  if (merged.length > MAX_LOGS_PER_SESSION) {
    merged.splice(0, merged.length - MAX_LOGS_PER_SESSION);
  }
  return merged;
}

/** Merge metric points, deduped by ts, capped, sorted. */
function mergeMetrics(a: MetricPoint[], b: MetricPoint[]): MetricPoint[] {
  const byTs = new Map<number, MetricPoint>();
  for (const p of a) byTs.set(p.ts, p);
  for (const p of b) byTs.set(p.ts, p);
  const merged = [...byTs.values()].sort((x, y) => x.ts - y.ts);
  if (merged.length > MAX_METRICS_PER_SESSION) {
    merged.splice(0, merged.length - MAX_METRICS_PER_SESSION);
  }
  return merged;
}

let socket: ReconnectingWS | null = null;

export const useLiveStore = create<LiveState>((set, get) => ({
  status: 'connecting',
  machines: {},
  sessions: {},
  logs: {},
  metrics: {},
  events: [],

  connect: () => {
    if (socket) return;
    socket = new ReconnectingWS({
      url: dashboardWsUrl, // function → re-reads the token on every (re)connect
      onStatus: (status) => set({ status }),
      onMessage: (env) => applyMessage(set, get, env),
    });
    socket.connect();
  },

  disconnect: () => {
    socket?.close();
    socket = null;
    // Clear cached tenant data so a different user never sees stale state.
    set({ machines: {}, sessions: {}, logs: {}, metrics: {}, events: [] });
  },

  hydrateLogs: (sessionId, entries) =>
    set((s) => ({
      logs: { ...s.logs, [sessionId]: mergeLogs(s.logs[sessionId] ?? [], entries) },
    })),

  hydrateFleet: (machines, sessions) =>
    set((s) => {
      const m = { ...s.machines };
      for (const mc of machines) m[mc.id] = mc;
      const ss = { ...s.sessions };
      for (const sess of sessions) ss[sess.id] = sess;
      return { machines: m, sessions: ss };
    }),

  send: (type, payload) => socket?.send(type, payload),

  hydrateMetrics: (sessionId, points) =>
    set((s) => ({
      metrics: { ...s.metrics, [sessionId]: mergeMetrics(s.metrics[sessionId] ?? [], points) },
    })),
}));

function applyMessage(
  set: (fn: (s: LiveState) => Partial<LiveState>) => void,
  _get: () => LiveState,
  env: Envelope,
) {
  switch (env.type) {
    case MessageType.Snapshot: {
      const snap = env.payload as Snapshot;
      set(() => ({
        machines: Object.fromEntries(snap.machines.map((m) => [m.id, m])),
        sessions: Object.fromEntries(snap.sessions.map((s) => [s.id, s])),
        events: (snap.events ?? []).slice(0, MAX_EVENTS),
      }));
      break;
    }
    case MessageType.EventAppend: {
      const { event } = env.payload as { event: ActivityEvent };
      set((s) => {
        if (s.events.some((e) => e.id === event.id)) return {};
        const next = [event, ...s.events];
        if (next.length > MAX_EVENTS) next.length = MAX_EVENTS;
        return { events: next };
      });
      break;
    }
    case MessageType.MachineUpsert: {
      const { machine } = env.payload as { machine: Machine };
      set((s) => ({ machines: { ...s.machines, [machine.id]: machine } }));
      break;
    }
    case MessageType.MachineRemoved: {
      const { machineId } = env.payload as { machineId: string };
      set((s) => {
        const machines = { ...s.machines };
        delete machines[machineId];
        // Also drop any sessions on that machine.
        const sessions = Object.fromEntries(
          Object.entries(s.sessions).filter(([, sess]) => sess.machineId !== machineId),
        );
        return { machines, sessions };
      });
      break;
    }
    case MessageType.AgentStatus: {
      const { machineId, online } = env.payload as { machineId: string; online: boolean };
      set((s) => {
        const m = s.machines[machineId];
        if (!m) return {};
        return { machines: { ...s.machines, [machineId]: { ...m, online } } };
      });
      break;
    }
    case MessageType.SessionUpsert: {
      const { session } = env.payload as { session: Session };
      set((s) => ({ sessions: { ...s.sessions, [session.id]: session } }));
      break;
    }
    case MessageType.SessionRemoved: {
      const { sessionId } = env.payload as { sessionId: string };
      set((s) => {
        const sessions = { ...s.sessions };
        delete sessions[sessionId];
        return { sessions };
      });
      break;
    }
    case MessageType.LogAppend: {
      const l = env.payload as LogAppend;
      set((s) => ({
        logs: {
          ...s.logs,
          [l.sessionId]: mergeLogs(s.logs[l.sessionId] ?? [], [
            { seq: l.seq, stream: l.stream, line: l.line, ts: l.ts },
          ]),
        },
      }));
      break;
    }
    case MessageType.MetricSample: {
      const m = env.payload as MetricSample;
      set((s) => ({
        metrics: {
          ...s.metrics,
          [m.sessionId]: mergeMetrics(s.metrics[m.sessionId] ?? [], [
            { ts: m.ts, cpuPct: m.cpuPct, memBytes: m.memBytes },
          ]),
        },
      }));
      break;
    }
    case MessageType.TerminalOutput: {
      const p = env.payload as TerminalOutput;
      termSubs.get(p.ptyId)?.onOutput?.(b64ToBytes(p.data));
      break;
    }
    case MessageType.TerminalOpened: {
      const p = env.payload as TerminalOpened;
      termSubs.get(p.ptyId)?.onOpened?.(p);
      break;
    }
    case MessageType.TerminalExit: {
      const p = env.payload as TerminalExit;
      termSubs.get(p.ptyId)?.onExit?.(p);
      break;
    }
    default:
      break;
  }
}

// Selectors
//
// These derive arrays from the normalized record maps. Because `Object.values`
// returns a new array reference on every call, they MUST be consumed with
// shallow equality — Zustand v5 compares snapshots with `Object.is`, so an
// unwrapped selector would re-render on every store read and loop infinitely.
export const useSessions = () => useLiveStore(useShallow((s) => Object.values(s.sessions)));
export const useMachines = () => useLiveStore(useShallow((s) => Object.values(s.machines)));
export const useEvents = () => useLiveStore((s) => s.events);
