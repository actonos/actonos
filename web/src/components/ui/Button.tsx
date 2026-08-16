import type { ButtonHTMLAttributes, ReactNode } from 'react';

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
  size?: 'sm' | 'md' | 'lg';
  icon?: ReactNode;
  children: ReactNode;
}

export function Button({
  variant = 'primary',
  size = 'md',
  icon,
  children,
  className = '',
  disabled,
  ...props
}: ButtonProps) {
  const baseStyles = 'inline-flex items-center justify-center font-sans font-medium transition-all duration-150 rounded-full select-none cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed';

  const sizeStyles = {
    sm: 'px-4 py-1.5 text-body-sm gap-1.5',
    md: 'px-6 py-2.5 text-body gap-2',
    lg: 'px-8 py-3.5 text-subheading gap-2.5',
  };

  const variantStyles = {
    primary: 'bg-hi-yellow text-deep-ink hover:brightness-95 active:brightness-90 font-semibold',
    secondary: 'bg-deep-ink text-white hover:bg-opacity-90 active:bg-opacity-95',
    ghost: 'bg-transparent text-deep-ink border border-deep-ink hover:bg-soft-meadow',
    danger: 'bg-red-600 text-white hover:bg-red-700 active:bg-red-800',
  };

  return (
    <button
      className={`${baseStyles} ${sizeStyles[size]} ${variantStyles[variant]} ${className}`}
      disabled={disabled}
      {...props}
    >
      {icon && <span className="inline-flex shrink-0">{icon}</span>}
      <span>{children}</span>
    </button>
  );
}
