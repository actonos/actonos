import type { HTMLAttributes, ReactNode } from 'react';

export interface CardProps extends Omit<HTMLAttributes<HTMLDivElement>, 'onClick'> {
  children: ReactNode;
  className?: string;
  onClick?: () => void;
  hoverable?: boolean;
}

export function Card({
  children,
  className = '',
  onClick,
  hoverable = false,
  ...props
}: CardProps) {
  return (
    <div
      onClick={onClick}
      {...props}
      className={`bg-soft-meadow rounded-[24px] p-6 md:p-8 transition-all ${
        hoverable ? 'hover:scale-[1.01] cursor-pointer' : ''
      } ${className}`}
    >
      {children}
    </div>
  );
}
