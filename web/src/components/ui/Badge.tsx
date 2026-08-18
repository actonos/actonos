import type { ReactNode } from 'react';

export interface BadgeProps {
  variant?: 'active' | 'stopped' | 'neutral' | 'accent' | 'success' | 'warning' | 'danger' | 'info';
  children: ReactNode;
  className?: string;
}

export function Badge({
  variant = 'neutral',
  children,
  className = '',
}: BadgeProps) {
  const variantStyles = {
    active: 'bg-status-success-soft text-status-success border border-status-success/25',
    stopped: 'bg-slate-100 text-slate-700 border border-slate-300',
    neutral: 'bg-canvas text-deep-ink border border-onyx',
    accent: 'bg-hi-yellow text-deep-ink font-semibold',
    success: 'bg-status-success-soft text-status-success border border-status-success/25',
    warning: 'bg-status-warning-soft text-status-warning border border-status-warning/25',
    danger: 'bg-status-danger-soft text-status-danger border border-status-danger/25',
    info: 'bg-status-info-soft text-status-info border border-status-info/25',
  };

  return (
    <span
      className={`inline-flex items-center px-3 py-0.5 rounded-full text-caption uppercase tracking-wider font-sans font-medium select-none ${variantStyles[variant]} ${className}`}
    >
      {children}
    </span>
  );
}
