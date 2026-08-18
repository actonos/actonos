import type { ReactNode } from 'react';

export interface PageHeaderProps {
  eyebrow?: string;
  title: string;
  description?: string;
  badge?: ReactNode;
  actions?: ReactNode;
}

export function PageHeader({ eyebrow, title, description, badge, actions }: PageHeaderProps) {
  return (
    <header className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
      <div className="min-w-0">
        {eyebrow && (
          <p className="mb-1 text-caption font-semibold uppercase tracking-wider text-slate">
            {eyebrow}
          </p>
        )}
        <div className="flex flex-wrap items-center gap-2.5">
          <h1 className="font-serif text-heading text-deep-ink sm:text-heading-lg">{title}</h1>
          {badge}
        </div>
        {description && (
          <p className="mt-1 max-w-3xl text-body-sm text-slate sm:text-body">{description}</p>
        )}
      </div>
      {actions && (
        <div className="flex shrink-0 flex-wrap items-center gap-2 self-start">{actions}</div>
      )}
    </header>
  );
}
