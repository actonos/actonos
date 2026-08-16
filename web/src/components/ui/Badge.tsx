import type { ReactNode } from 'react';

export interface BadgeProps {
  variant?: 'active' | 'stopped' | 'neutral' | 'accent';
  children: ReactNode;
  className?: string;
}

export function Badge({
  variant = 'neutral',
  children,
  className = '',
}: BadgeProps) {
  const variantStyles = {
    active: 'bg-emerald-100 text-emerald-900 border border-emerald-300',
    stopped: 'bg-slate-100 text-slate-700 border border-slate-300',
    neutral: 'bg-canvas text-deep-ink border border-onyx',
    accent: 'bg-hi-yellow text-deep-ink font-semibold',
  };

  return (
    <span
      className={`inline-flex items-center px-3 py-0.5 rounded-full text-caption uppercase tracking-wider font-sans font-medium select-none ${variantStyles[variant]} ${className}`}
    >
      {children}
    </span>
  );
}
