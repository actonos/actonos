import { render, screen, cleanup, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { SplashScreen } from './SplashScreen';
import { ThemeProvider } from '@/components/providers/ThemeProvider';

describe('SplashScreen', () => {
  afterEach(() => {
    cleanup();
  });

  it('renders splash screen with brand title, tagline and stage indicator', () => {
    render(
      <ThemeProvider>
        <SplashScreen isReady={false} />
      </ThemeProvider>
    );

    expect(screen.getByTestId('actonos-splash-screen')).toBeInTheDocument();
    expect(screen.getByText('ActonOS Kernel')).toBeInTheDocument();
    expect(screen.getByText('Autonomous AI Agent Operating System')).toBeInTheDocument();
  });

  it('calls onComplete when isReady becomes true', async () => {
    const handleComplete = vi.fn();
    const { rerender } = render(
      <ThemeProvider>
        <SplashScreen isReady={false} onComplete={handleComplete} />
      </ThemeProvider>
    );

    expect(handleComplete).not.toHaveBeenCalled();

    rerender(
      <ThemeProvider>
        <SplashScreen isReady={true} onComplete={handleComplete} />
      </ThemeProvider>
    );

    await waitFor(
      () => {
        expect(handleComplete).toHaveBeenCalled();
      },
      { timeout: 1500 }
    );
  });
});
