import { describe, expect, it } from 'vitest';
import { decode, encode, MessageType, SessionSchema } from './index';

describe('protocol codec', () => {
  it('round-trips an envelope', () => {
    const env = decode(encode(MessageType.SessionRemoved, { sessionId: 'abc' }));
    expect(env.type).toBe('session.removed');
    expect(env.v).toBe(1);
    expect(env.payload).toEqual({ sessionId: 'abc' });
  });

  it('validates a session shape', () => {
    const session = {
      id: 's1',
      machineId: 'm1',
      provider: 'claude-code',
      status: 'running',
      repo: 'mission-control',
      branch: 'main',
      cwd: '/tmp',
      pid: 123,
      currentCommand: 'claude',
      claudeVersion: '1.0.0',
      cpuPct: 12.5,
      memBytes: 1024,
      startedAt: 1,
      lastActivityAt: 2,
    };
    expect(() => SessionSchema.parse(session)).not.toThrow();
  });

  it('rejects an invalid status', () => {
    expect(() =>
      SessionSchema.parse({ status: 'bogus' } as unknown),
    ).toThrow();
  });
});
