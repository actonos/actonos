import type { InputHTMLAttributes, ReactNode } from 'react';

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
  actionButton?: ReactNode;
}

export function Input({
  label,
  error,
  actionButton,
  className = '',
  id,
  ...props
}: InputProps) {
  return (
    <div className="w-full flex flex-col gap-1.5">
      {label && (
        <label htmlFor={id} className="text-caption uppercase tracking-wider text-slate font-medium">
          {label}
        </label>
      )}
      <div className="relative flex items-center">
        <input
          id={id}
          className={`density-control w-full bg-canvas text-deep-ink placeholder-slate font-sans text-body-sm px-5 rounded-full border border-onyx/25 focus:outline-none focus:ring-2 focus:ring-focus-ring transition-all ${actionButton ? 'pr-32' : ''
            } ${error ? 'border-red-500' : ''} ${className}`}
          {...props}
        />
        {actionButton && (
          <div className="absolute right-1.5">
            {actionButton}
          </div>
        )}
      </div>
      {error && <span className="text-body-sm text-red-600 px-3">{error}</span>}
    </div>
  );
}
