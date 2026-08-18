import { AlertCircle, LoaderCircle } from 'lucide-react';
import type { ReactNode } from 'react';
import { EmptyState } from './EmptyState';
import { Button } from './Button';

export interface AsyncStateProps {
  loading?: boolean;
  error?: string;
  empty?: boolean;
  emptyTitle: string;
  emptyDescription?: string;
  errorTitle: string;
  retryLabel: string;
  onRetry?: () => void;
  children: ReactNode;
}

export function AsyncState({
  loading = false,
  error,
  empty = false,
  emptyTitle,
  emptyDescription,
  errorTitle,
  retryLabel,
  onRetry,
  children,
}: AsyncStateProps) {
  if (loading) {
    return (
      <div className="flex min-h-32 items-center justify-center text-slate" role="status" aria-live="polite">
        <LoaderCircle className="h-5 w-5 animate-spin" aria-hidden="true" />
      </div>
    );
  }
  if (error) {
    return (
      <EmptyState
        compact
        icon={<AlertCircle className="h-6 w-6 text-danger" />}
        title={errorTitle}
        description={error}
        action={onRetry ? <Button size="sm" variant="ghost" onClick={onRetry}>{retryLabel}</Button> : undefined}
      />
    );
  }
  if (empty) {
    return <EmptyState compact title={emptyTitle} description={emptyDescription} />;
  }
  return <>{children}</>;
}
