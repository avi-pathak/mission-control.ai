/** Minimal ANSI SGR -> HTML span converter for log rendering. */

const FG: Record<number, string> = {
  30: '#3f3f46', 31: '#f87171', 32: '#4ade80', 33: '#fbbf24',
  34: '#60a5fa', 35: '#c084fc', 36: '#22d3ee', 37: '#e4e4e7',
  90: '#71717a', 91: '#fca5a5', 92: '#86efac', 93: '#fde047',
  94: '#93c5fd', 95: '#d8b4fe', 96: '#67e8f9', 97: '#fafafa',
};

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

/** Convert a single line containing ANSI escape codes to safe HTML. */
export function ansiToHtml(line: string): string {
  // eslint-disable-next-line no-control-regex
  const parts = line.split(/\x1b\[([0-9;]*)m/);
  let html = '';
  let open = false;
  let color: string | null = null;
  let bold = false;

  const flushOpen = () => {
    if (open) {
      html += '</span>';
      open = false;
    }
  };

  for (let i = 0; i < parts.length; i++) {
    if (i % 2 === 0) {
      const text = escapeHtml(parts[i] ?? '');
      if (!text) continue;
      if (color || bold) {
        const styles = [color ? `color:${color}` : '', bold ? 'font-weight:600' : '']
          .filter(Boolean)
          .join(';');
        html += `<span style="${styles}">${text}</span>`;
      } else {
        html += text;
      }
    } else {
      const codes = (parts[i] ?? '').split(';').map((c) => parseInt(c || '0', 10));
      for (const code of codes) {
        if (code === 0) {
          color = null;
          bold = false;
        } else if (code === 1) {
          bold = true;
        } else if (FG[code]) {
          color = FG[code] ?? null;
        }
      }
    }
  }
  flushOpen();
  return html;
}
