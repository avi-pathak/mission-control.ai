import { useState } from 'react';
import { cn } from './cn';

interface CopyButtonProps {
  value: string;
  className?: string;
  label?: string;
}

/** Button that copies `value` to the clipboard and shows transient feedback. */
export function CopyButton({ value, className, label = 'Copy' }: CopyButtonProps) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
    } catch {
      // Fallback for non-secure contexts.
      const ta = document.createElement('textarea');
      ta.value = value;
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      document.body.removeChild(ta);
    }
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <button
      onClick={copy}
      className={cn(
        'inline-flex h-8 items-center gap-1.5 rounded-lg border border-white/10 px-3 text-xs font-medium transition-colors',
        copied ? 'text-emerald-400' : 'text-zinc-300 hover:bg-white/5',
        className,
      )}
    >
      {copied ? 'Copied!' : label}
    </button>
  );
}
