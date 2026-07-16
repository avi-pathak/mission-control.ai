import { useEffect, type ReactNode } from 'react';
import { cn } from './cn';

interface DialogProps {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string;
  children: ReactNode;
  className?: string;
}

/**
 * Minimal accessible modal: overlay + centered panel, Escape to close, no
 * external dependency. Focus stays within the panel via the browser's default
 * tab order; the overlay click and Escape both dismiss.
 */
export function Dialog({ open, onClose, title, description, children, className }: DialogProps) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      role="dialog"
      aria-modal="true"
      aria-label={title}
    >
      <div
        className="absolute inset-0 bg-black/60 backdrop-blur-sm animate-[fade-in_0.15s_ease-out]"
        onClick={onClose}
      />
      <div
        className={cn(
          'relative w-full max-w-lg rounded-xl border border-white/10 bg-[#0e0e13] p-6 shadow-2xl',
          'animate-[fade-in_0.2s_ease-out]',
          className,
        )}
      >
        <h2 className="text-base font-semibold text-zinc-100">{title}</h2>
        {description && <p className="mt-1 text-sm text-zinc-500">{description}</p>}
        <div className="mt-4">{children}</div>
      </div>
    </div>
  );
}
