import type { LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';
import { Card } from './Card';

export interface MetricCardProps {
  label: string;
  value: string;
  detail?: string;
  icon: LucideIcon;
  tone?: 'neutral' | 'success' | 'warning' | 'danger' | 'info';
  progress?: number;
  footer?: ReactNode;
}

const toneClasses = {
  neutral: 'text-deep-ink',
  success: 'text-success',
  warning: 'text-warning',
  danger: 'text-danger',
  info: 'text-info',
};

export function MetricCard({ label, value, detail, icon: Icon, tone = 'neutral', progress, footer }: MetricCardProps) {
  const clamped = Math.max(0, Math.min(100, progress ?? 0));
  return (
    <Card className="border border-onyx/10 p-4">
      <div className="flex items-center justify-between gap-2">
        <span className="text-caption font-semibold uppercase tracking-wide text-slate">{label}</span>
        <Icon className={`h-4 w-4 ${toneClasses[tone]}`} aria-hidden="true" />
      </div>
      <div className={`mt-2 font-serif text-heading font-bold ${toneClasses[tone]}`}>{value}</div>
      {progress !== undefined && (
        <div className="my-2 h-2 overflow-hidden rounded-full bg-deep-ink/10" aria-hidden="true">
          <div className={`h-full rounded-full ${tone === 'danger' ? 'bg-danger' : tone === 'warning' ? 'bg-warning' : 'bg-deep-ink'}`} style={{ width: `${clamped}%` }} />
        </div>
      )}
      {detail && <p className="truncate text-caption text-slate">{detail}</p>}
      {footer}
    </Card>
  );
}
