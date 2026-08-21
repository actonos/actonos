import type { LucideIcon } from 'lucide-react';
import { RotateCcw } from 'lucide-react';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';

export function AgentTextSection({
  icon: Icon, title, description, value, placeholder, resetLabel, statusLabel, onChange, onReset,
}: {
  icon: LucideIcon;
  title: string;
  description: string;
  value: string;
  placeholder: string;
  resetLabel: string;
  statusLabel: string;
  onChange: (value: string) => void;
  onReset: () => void;
}) {
  return (
    <Card className="space-y-4 border border-onyx/15 bg-soft-meadow p-6 shadow-xs">
      <div className="flex flex-col justify-between gap-3 border-b border-onyx/5 pb-3 sm:flex-row sm:items-center">
        <div>
          <h3 className="flex items-center gap-2 font-serif text-heading-sm text-deep-ink">
            <Icon className="h-5 w-5" aria-hidden="true" />{title}
          </h3>
          <p className="text-caption text-slate">{description}</p>
        </div>
        <Button variant="ghost" size="sm" icon={<RotateCcw className="h-3.5 w-3.5" />} onClick={onReset}>
          {resetLabel}
        </Button>
      </div>
      <textarea
        rows={18}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className="w-full rounded-[20px] border border-onyx/20 bg-canvas p-4 font-mono text-body-sm leading-relaxed text-deep-ink shadow-xs focus:border-deep-ink focus:outline-none focus:ring-2 focus:ring-deep-ink/10 transition-all placeholder:text-slate/60"
        placeholder={placeholder}
      />
      <div className="flex items-center justify-between gap-3 text-caption font-mono text-slate">
        <span>{value.length.toLocaleString()}</span>
        <span className="font-semibold text-deep-ink">{statusLabel}</span>
      </div>
    </Card>
  );
}
