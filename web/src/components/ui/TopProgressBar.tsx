import { useEffect, useState } from 'react';

export interface TopProgressBarProps {
  /** When true, animates the progress bar across the screen */
  isLoading?: boolean;
}

export function TopProgressBar({ isLoading = false }: TopProgressBarProps) {
  const [visible, setVisible] = useState(false);
  const [progress, setProgress] = useState(0);

  useEffect(() => {
    let timer: NodeJS.Timeout;
    let stepTimer: NodeJS.Timeout;

    if (isLoading) {
      setVisible(true);
      setProgress(20);

      stepTimer = setInterval(() => {
        setProgress((prev) => {
          if (prev >= 85) return prev;
          return prev + Math.random() * 15;
        });
      }, 150);
    } else if (visible) {
      setProgress(100);
      timer = setTimeout(() => {
        setVisible(false);
        setProgress(0);
      }, 300);
    }

    return () => {
      clearTimeout(timer);
      clearInterval(stepTimer);
    };
  }, [isLoading]);

  if (!visible && progress === 0) return null;

  return (
    <div
      data-testid="top-progress-bar"
      className="fixed top-0 left-0 right-0 z-50 h-[2px] pointer-events-none overflow-hidden bg-transparent"
    >
      <div
        className="h-full bg-gradient-to-r from-hi-yellow via-fuchsia to-moss-green shadow-[0_0_8px_rgba(226,97,229,0.7)] transition-all duration-300 ease-out"
        style={{
          width: `${progress}%`,
          opacity: visible ? 1 : 0,
        }}
      />
    </div>
  );
}
