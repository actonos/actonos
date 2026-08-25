import { render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { describe, expect, it } from 'vitest';
import i18n from '@/lib/i18n';
import { SystemHealthStrip } from './SystemHealthStrip';

describe('SystemHealthStrip', () => {
  it('renders supervisor component statuses from health', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <SystemHealthStrip
          data={null}
          health={{
            status: 'degraded',
            components: {
              llm: 'unhealthy',
              heartbeat: 'healthy',
              embedding: 'degraded',
              disk: 'healthy',
            },
          }}
        />
      </I18nextProvider>
    );
    expect(screen.getByText(/unhealthy/i)).toBeInTheDocument();
    expect(screen.getByText(/LLM/i)).toBeInTheDocument();
    expect(screen.getByText(/Embedding/i)).toBeInTheDocument();
  });
});
