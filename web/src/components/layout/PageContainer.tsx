import type { ReactNode } from 'react';

export interface PageContainerProps {
  children: ReactNode;
  className?: string;
}

export function PageContainer({ children, className = '' }: PageContainerProps) {
  return (
    <main className={`max-w-[1200px] mx-auto px-4 md:px-8 py-8 md:py-12 ${className}`}>
      {children}
    </main>
  );
}
