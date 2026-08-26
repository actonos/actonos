import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '@/lib/i18n';
import { ToastProvider } from '@/components/ui/Toast';
import type { AgentRun } from '@/lib/types';
import { OperationsPage } from './OperationsPage';

const getAgentRun = vi.fn();
const snapshot = {
  runs: [] as AgentRun[],
  tokens: { today_cost_usd: 0, month_cost_usd: 0, today_tokens: 0, month_tokens: 0, by_model: [] },
};

vi.mock('@/lib/api', () => ({
  api: {
    listTasks: vi.fn().mockResolvedValue({ tasks: [], count: 0 }),
    getHealth: vi.fn().mockResolvedValue(null),
    listRunEvents: vi.fn().mockResolvedValue({ events: [] }),
    getAgentRun: (...args: unknown[]) => getAgentRun(...args),
    cancelAgentRun: vi.fn(),
    updateTask: vi.fn(),
  },
}));

vi.mock('@/components/providers/RealtimeProvider', () => ({
  useRealtime: () => ({
    connection: 'online',
    snapshot,
  }),
}));

function run(partial: Partial<AgentRun> & Pick<AgentRun, 'id' | 'goal'>): AgentRun {
  return {
    trace_id: 'tr',
    agent_id: 'agent_system_core',
    source: 'heartbeat',
    status: 'running',
    iterations: 1,
    prompt_tokens: 1,
    completion_tokens: 1,
    total_tokens: 2,
    started_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...partial,
  };
}

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <ToastProvider>
        <OperationsPage />
      </ToastProvider>
    </I18nextProvider>,
  );
}

describe('OperationsPage deep-linked run', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en');
    snapshot.runs = [run({ id: 'run_recent', goal: 'recent chat turn', source: 'chat', status: 'completed' })];
    getAgentRun.mockReset();
    window.location.hash = '';
  });

  afterEach(() => {
    cleanup();
    window.location.hash = '';
  });

  it('renders a GET /api/runs/{id} hit that is missing from the snapshot', async () => {
    const fetched = run({ id: 'run_old', goal: 'long running heartbeat' });
    getAgentRun.mockResolvedValue({ run: fetched });
    window.location.hash = '#/operations?view=feed&run=run_old';
    renderPage();

    expect(await screen.findByText('long running heartbeat')).toBeInTheDocument();
    await waitFor(() => {
      expect(getAgentRun).toHaveBeenCalledWith('run_old');
    });
    expect(screen.queryByText('Select an execution to inspect its live trace.')).not.toBeInTheDocument();
    expect(screen.queryByText('recent chat turn')).not.toBeInTheDocument();
    expect((screen.getByLabelText('Thought → Action → Observation') as HTMLSelectElement).value).toBe('run_old');
  });

  it('shows the missing-run empty state on 404 instead of substituting snapshot[0]', async () => {
    getAgentRun.mockRejectedValue(new Error('run not found'));
    window.location.hash = '#/operations?view=feed&run=run_gone';
    renderPage();

    expect(await screen.findByText('That run is no longer available.')).toBeInTheDocument();
    expect(screen.queryByText('recent chat turn')).not.toBeInTheDocument();
    expect(getAgentRun).toHaveBeenCalledWith('run_gone');
  });
});
