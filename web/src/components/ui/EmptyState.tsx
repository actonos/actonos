import type { ReactNode } from 'react';

export interface EmptyStateProps {
  icon?: ReactNode;
  title: string;
  description?: string;
  action?: ReactNode;
  compact?: boolean;
}

export function EmptyState({ icon, title, description, action, compact = false }: EmptyStateProps) {
  return (
    <div
      className={`flex flex-col items-center justify-center rounded-[24px] border border-dashed border-onyx/15 bg-soft-meadow/50 px-6 text-center ${
        compact ? 'py-8' : 'py-14'
      }`}
    >
      {icon && <div className="mb-3 text-slate">{icon}</div>}
      <h3 className="font-serif text-heading-sm font-semibold text-deep-ink">{title}</h3>
      {description && <p className="mt-1 max-w-lg text-body-sm text-slate">{description}</p>}
      {action && <div className="mt-5">{action}</div>}
    </div>
  );
}
