import { describe, expect, it } from 'vitest';
import {
  formatBytes,
  formatCount,
  formatDuration,
  formatPct,
  formatRelativeTime,
} from './format';

describe('formatBytes', () => {
  it('handles zero and negatives', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(-5)).toBe('0 B');
  });
  it('scales through units', () => {
    expect(formatBytes(512)).toBe('512 B');
    expect(formatBytes(1024)).toBe('1.0 KB');
    expect(formatBytes(1024 * 1024)).toBe('1.0 MB');
    expect(formatBytes(3.5 * 1024 * 1024 * 1024)).toBe('3.5 GB');
  });
});

describe('formatPct', () => {
  it('shows a decimal under 10 and none at/above', () => {
    expect(formatPct(4.25)).toBe('4.3%');
    expect(formatPct(42.9)).toBe('43%');
  });
});

describe('formatCount', () => {
  it('handles zero and negatives', () => {
    expect(formatCount(0)).toBe('0');
    expect(formatCount(-3)).toBe('0');
  });
  it('leaves small numbers alone', () => {
    expect(formatCount(999)).toBe('999');
  });
  it('compacts K/M/B', () => {
    expect(formatCount(1234)).toBe('1.2K');
    expect(formatCount(4_500_000)).toBe('4.5M');
    expect(formatCount(736_000_000)).toBe('736M');
    expect(formatCount(2_000_000_000)).toBe('2.0B');
  });
});

describe('formatDuration', () => {
  const base = 1_000_000_000_000;
  it('formats seconds, minutes, hours, days', () => {
    expect(formatDuration(base, base + 30_000)).toBe('30s');
    expect(formatDuration(base, base + 5 * 60_000)).toBe('5m');
    expect(formatDuration(base, base + 2 * 3_600_000 + 14 * 60_000)).toBe('2h 14m');
    expect(formatDuration(base, base + 26 * 3_600_000)).toBe('1d 2h');
  });
  it('never goes negative', () => {
    expect(formatDuration(base + 10_000, base)).toBe('0s');
  });
});

describe('formatRelativeTime', () => {
  const now = 1_000_000_000_000;
  it('says "just now" within 5s', () => {
    expect(formatRelativeTime(now - 2_000, now)).toBe('just now');
  });
  it('appends " ago" otherwise', () => {
    expect(formatRelativeTime(now - 5 * 60_000, now)).toBe('5m ago');
  });
});
