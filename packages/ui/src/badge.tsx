import type { HTMLAttributes } from 'react';
import { cn } from './cn';

export function Badge({ className, ...props }: HTMLAttributes<HTMLSpanElement>) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs font-medium',
        'border border-white/[0.06] bg-white/[0.03]',
        className,
      )}
      {...props}
    />
  );
}

/** Pulsing status dot + label. */
export function StatusDot({ className }: { className?: string }) {
  return (
    <span className="relative flex h-2 w-2">
      <span className={cn('absolute inline-flex h-full w-full animate-ping rounded-full opacity-60', className)} />
      <span className={cn('relative inline-flex h-2 w-2 rounded-full', className)} />
    </span>
  );
}
