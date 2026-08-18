import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { AsyncState } from './AsyncState';

describe('AsyncState', () => {
  it('renders loading status', () => {
    render(
      <AsyncState loading emptyTitle="Empty" errorTitle="Error" retryLabel="Retry">
        <span>content</span>
      </AsyncState>,
    );
    expect(screen.getByRole('status')).toBeInTheDocument();
  });

  it('renders error and retry action', () => {
    render(
      <AsyncState error="Network unavailable" emptyTitle="Empty" errorTitle="Failed" retryLabel="Retry" onRetry={() => undefined}>
        <span>content</span>
      </AsyncState>,
    );
    expect(screen.getByText('Failed')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument();
  });
});
