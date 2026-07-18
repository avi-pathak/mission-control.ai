import { describe, expect, it } from 'vitest';
import { ansiToHtml } from './ansi';

describe('ansiToHtml', () => {
  it('passes plain text through unchanged', () => {
    expect(ansiToHtml('hello world')).toBe('hello world');
  });

  it('escapes HTML to prevent injection', () => {
    expect(ansiToHtml('<script>alert(1)</script>')).toBe(
      '&lt;script&gt;alert(1)&lt;/script&gt;',
    );
    expect(ansiToHtml('a & b < c')).toBe('a &amp; b &lt; c');
  });

  it('wraps colored text in a styled span', () => {
    // 31 = red foreground, 0 = reset
    const out = ansiToHtml('\x1b[31merror\x1b[0m');
    expect(out).toBe('<span style="color:#f87171">error</span>');
  });

  it('applies bold', () => {
    const out = ansiToHtml('\x1b[1mbold\x1b[0m');
    expect(out).toBe('<span style="font-weight:600">bold</span>');
  });

  it('combines color + bold', () => {
    const out = ansiToHtml('\x1b[1;34mlink\x1b[0m');
    expect(out).toBe('<span style="color:#60a5fa;font-weight:600">link</span>');
  });

  it('resets styling after code 0', () => {
    const out = ansiToHtml('\x1b[32mon\x1b[0m off');
    expect(out).toBe('<span style="color:#4ade80">on</span> off');
  });

  it('still escapes HTML inside colored spans', () => {
    const out = ansiToHtml('\x1b[31m<b>\x1b[0m');
    expect(out).toBe('<span style="color:#f87171">&lt;b&gt;</span>');
  });
});
