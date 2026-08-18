import { forwardRef, type HTMLAttributes, type ReactNode } from 'react';

export interface CardProps extends Omit<HTMLAttributes<HTMLDivElement>, 'onClick'> {
  children: ReactNode;
  className?: string;
  onClick?: () => void;
  hoverable?: boolean;
}

export const Card = forwardRef<HTMLDivElement, CardProps>(function Card({
  children,
  className = '',
  onClick,
  hoverable = false,
  ...props
}, ref) {
  return (
    <div
      ref={ref}
      onClick={onClick}
      {...props}
      className={`density-card bg-soft-meadow rounded-[24px] transition-all ${
        hoverable ? 'hover:scale-[1.01] cursor-pointer' : ''
      } ${className}`}
    >
      {children}
    </div>
  );
});
