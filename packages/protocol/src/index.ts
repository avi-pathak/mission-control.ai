import { z } from 'zod';

/** Current protocol version. Mirrors internal/protocol.Version (Go). */
export const PROTOCOL_VERSION = 1;

export const MessageType = {
  // Agent -> Server
  AgentHello: 'agent.hello',
  AgentHeartbeat: 'agent.heartbeat',
  SessionUpsert: 'session.upsert',
  SessionRemoved: 'session.removed',
  LogAppend: 'log.append',
  MetricSample: 'metric.sample',
  CommandAck: 'command.ack',
  CommandResult: 'command.result',
  EventReport: 'event.report',
  // Server -> Agent
  Command: 'command',
  // Server -> Dashboard
  Snapshot: 'snapshot',
  MachineUpsert: 'machine.upsert',
  MachineRemoved: 'machine.removed',
  AgentStatus: 'agent.status',
  EventAppend: 'event.append',
  // Interactive terminal
  TerminalOpen: 'terminal.open',
  TerminalAttach: 'terminal.attach',
  TerminalInput: 'terminal.input',
  TerminalResize: 'terminal.resize',
  TerminalClose: 'terminal.close',
  TerminalOutput: 'terminal.output',
  TerminalOpened: 'terminal.opened',
  TerminalExit: 'terminal.exit',
  // Either
  Error: 'error',
} as const;

export type MessageType = (typeof MessageType)[keyof typeof MessageType];

export const SessionStatus = z.enum([
  'running',
  'waiting_approval',
  'idle',
  'finished',
  'error',
]);
export type SessionStatus = z.infer<typeof SessionStatus>;

export const LogStream = z.enum(['stdout', 'stderr', 'system']);
export type LogStream = z.infer<typeof LogStream>;

export const CommandAction = z.enum(['stop', 'restart']);
export type CommandAction = z.infer<typeof CommandAction>;

export const CommitSchema = z.object({
  hash: z.string(),
  author: z.string(),
  message: z.string(),
  ts: z.number(),
});
export type Commit = z.infer<typeof CommitSchema>;

export const GitInfoSchema = z.object({
  repo: z.string(),
  remoteUrl: z.string(),
  branch: z.string(),
  dirty: z.boolean(),
  modifiedFiles: z.array(z.string()),
  ahead: z.number(),
  behind: z.number(),
  recentCommits: z.array(CommitSchema).optional(),
});
export type GitInfo = z.infer<typeof GitInfoSchema>;

export const TokenUsageSchema = z.object({
  input: z.number(),
  output: z.number(),
  cacheRead: z.number(),
  cacheCreation: z.number(),
  total: z.number(),
});
export type TokenUsage = z.infer<typeof TokenUsageSchema>;

export const SessionSchema = z.object({
  id: z.string(),
  machineId: z.string(),
  provider: z.string(),
  status: SessionStatus,
  repo: z.string(),
  branch: z.string(),
  cwd: z.string(),
  pid: z.number(),
  currentCommand: z.string(),
  claudeVersion: z.string(),
  tmuxSession: z.string().optional(),
  cpuPct: z.number(),
  memBytes: z.number(),
  startedAt: z.number(),
  lastActivityAt: z.number(),
  git: GitInfoSchema.optional(),
  tokens: TokenUsageSchema.optional(),
});
export type Session = z.infer<typeof SessionSchema>;

export const MachineSchema = z.object({
  id: z.string(),
  hostname: z.string(),
  os: z.string(),
  arch: z.string(),
  cpuCores: z.number(),
  totalMem: z.number(),
  agentVersion: z.string(),
  online: z.boolean(),
  cpuPct: z.number(),
  memUsedBytes: z.number(),
  load: z.number(),
  lastSeenAt: z.number(),
});
export type Machine = z.infer<typeof MachineSchema>;

// --- Payloads ---

export const EventSeverity = z.enum(['info', 'success', 'warn', 'error']);
export type EventSeverity = z.infer<typeof EventSeverity>;

export const EventKind = {
  SessionStarted: 'session.started',
  SessionEnded: 'session.ended',
  StatusChanged: 'status.changed',
  BranchChanged: 'branch.changed',
  CommitCreated: 'commit.created',
  CommandChanged: 'command.changed',
  CommandIssued: 'command.issued',
  FilePublished: 'file.published',
  AgentConnected: 'agent.connected',
  AgentOffline: 'agent.disconnected',
} as const;

export const EventSchema = z.object({
  id: z.string(),
  machineId: z.string(),
  sessionId: z.string().optional(),
  kind: z.string(),
  message: z.string(),
  severity: EventSeverity,
  ts: z.number(),
  meta: z.record(z.string()).optional(),
});
export type ActivityEvent = z.infer<typeof EventSchema>;

export const SnapshotSchema = z.object({
  machines: z.array(MachineSchema),
  sessions: z.array(SessionSchema),
  events: z.array(EventSchema).default([]),
});
export type Snapshot = z.infer<typeof SnapshotSchema>;

export const SessionUpsertSchema = z.object({ session: SessionSchema });
export const SessionRemovedSchema = z.object({ sessionId: z.string() });
export const MachineUpsertSchema = z.object({ machine: MachineSchema });
export const MachineRemovedSchema = z.object({ machineId: z.string() });
export const AgentStatusSchema = z.object({
  machineId: z.string(),
  online: z.boolean(),
});

export const LogAppendSchema = z.object({
  sessionId: z.string(),
  seq: z.number(),
  stream: LogStream,
  line: z.string(),
  ts: z.number(),
});
export type LogAppend = z.infer<typeof LogAppendSchema>;

export const MetricSampleSchema = z.object({
  sessionId: z.string(),
  cpuPct: z.number(),
  memBytes: z.number(),
  ts: z.number(),
});
export type MetricSample = z.infer<typeof MetricSampleSchema>;

export const DashboardHelloSchema = z.object({ apiKey: z.string() });

export const ErrorPayloadSchema = z.object({
  code: z.string(),
  message: z.string(),
});

// --- Enrollment REST DTOs (not WebSocket envelopes) ---

export const EnrollTokenSchema = z.object({
  token: z.string(),
  label: z.string(),
  createdAt: z.number(),
  expiresAt: z.number(),
  status: z.enum(['active', 'used', 'expired', 'revoked']),
  command: z.string().optional(),
});
export type EnrollToken = z.infer<typeof EnrollTokenSchema>;

export interface CreateEnrollTokenResponse {
  token: string;
  expiresAt: number;
  command: string;
}

// --- Envelope ---

export const EnvelopeSchema = z.object({
  v: z.number(),
  type: z.string(),
  ts: z.number(),
  id: z.string().optional(),
  payload: z.unknown(),
});
export type Envelope = z.infer<typeof EnvelopeSchema>;

export function encode(type: MessageType, payload: unknown, id?: string): string {
  const env: Envelope = { v: PROTOCOL_VERSION, type, ts: Date.now(), payload };
  if (id) env.id = id;
  return JSON.stringify(env);
}

export function decode(data: string): Envelope {
  return EnvelopeSchema.parse(JSON.parse(data));
}

// --- Interactive terminal payloads ---

export interface TerminalOpen {
  ptyId: string;
  machineId: string;
  provider: string;
  cwd: string;
  initialText?: string;
  cols: number;
  rows: number;
}
export interface TerminalAttach {
  ptyId: string;
  machineId: string;
  sessionId: string;
  cols: number;
  rows: number;
}
export interface TerminalInput {
  ptyId: string;
  data: string; // base64
}
export interface TerminalResize {
  ptyId: string;
  cols: number;
  rows: number;
}
export interface TerminalClose {
  ptyId: string;
}
export interface TerminalOutput {
  ptyId: string;
  data: string; // base64
}
export interface TerminalOpened {
  ptyId: string;
  sessionId: string;
  ok: boolean;
  error?: string;
}
export interface TerminalExit {
  ptyId: string;
  code: number;
}
