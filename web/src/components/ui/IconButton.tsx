import type { ButtonHTMLAttributes, ReactNode } from 'react';

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  label: string;
  icon: ReactNode;
  size?: 'sm' | 'md' | 'lg';
  tone?: 'default' | 'danger' | 'primary';
}

export function IconButton({
  label,
  icon,
  size = 'md',
  tone = 'default',
  className = '',
  ...props
}: IconButtonProps) {
  const sizes = {
    sm: 'h-9 w-9',
    md: 'h-10 w-10',
    lg: 'h-11 w-11',
  };
  const tones = {
    default: 'border-onyx/10 bg-soft-meadow text-deep-ink hover:bg-canvas',
    danger: 'border-status-danger/25 bg-status-danger-soft text-status-danger hover:bg-status-danger/10',
    primary: 'border-deep-ink bg-deep-ink text-canvas hover:bg-deep-ink/90',
  };

  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      className={`inline-flex shrink-0 items-center justify-center rounded-full border transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 ${sizes[size]} ${tones[tone]} ${className}`}
      {...props}
    >
      {icon}
    </button>
  );
}
