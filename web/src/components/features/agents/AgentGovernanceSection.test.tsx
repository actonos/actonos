import { render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { describe, expect, it } from 'vitest';
import i18n from '@/lib/i18n';
import { AgentGovernanceSection } from './AgentGovernanceSection';

describe('AgentGovernanceSection', () => {
  it('exposes concurrent-run and hourly token quotas', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <AgentGovernanceSection
          budget={50}
          approvalLevel="Medium"
          allowedPaths="*"
          concurrentRuns={2}
          tokensPerHour={250000}
          onBudgetChange={() => undefined}
          onApprovalLevelChange={() => undefined}
          onAllowedPathsChange={() => undefined}
          onConcurrentRunsChange={() => undefined}
          onTokensPerHourChange={() => undefined}
        />
      </I18nextProvider>
    );
    expect(screen.getByDisplayValue('2')).toBeInTheDocument();
    expect(screen.getByDisplayValue('250000')).toBeInTheDocument();
  });
});
