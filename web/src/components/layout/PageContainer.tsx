import type { ReactNode } from 'react';

export interface PageContainerProps {
  children: ReactNode;
  className?: string;
  maxWidth?: 'default' | 'wide' | 'full' | string;
}

export function PageContainer({ children, className = '', maxWidth = 'default' }: PageContainerProps) {
  let widthClass = 'max-w-[1200px]';
  if (maxWidth === 'wide') {
    widthClass = 'max-w-[1440px]';
  } else if (maxWidth === 'full') {
    widthClass = 'max-w-none';
  } else if (maxWidth !== 'default') {
    widthClass = maxWidth;
  }

  if (className.includes('max-w-')) {
    widthClass = '';
  }

  return (
    <main className={`w-full ${widthClass} mx-auto px-4 md:px-8 py-8 md:py-12 ${className}`}>
      {children}
    </main>
  );
}
