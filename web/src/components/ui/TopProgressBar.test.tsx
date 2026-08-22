import { render, screen, cleanup } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { TopProgressBar } from './TopProgressBar';

describe('TopProgressBar', () => {
  afterEach(() => {
    cleanup();
  });

  it('renders nothing when not loading', () => {
    const { container } = render(<TopProgressBar isLoading={false} />);
    expect(container.firstChild).toBeNull();
  });

  it('renders progress bar element when loading', async () => {
    render(<TopProgressBar isLoading={true} />);
    expect(screen.getByTestId('top-progress-bar')).toBeInTheDocument();
  });
});
