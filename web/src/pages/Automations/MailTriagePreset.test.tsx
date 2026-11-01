import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { I18nextProvider } from 'react-i18next';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '@/lib/i18n';
import { ToastProvider } from '@/components/ui/Toast';
import { MailTriagePreset } from './MailTriagePreset';

const createAgent = vi.fn();
const updateAgent = vi.fn();
const getAgent = vi.fn();
const saveCronJob = vi.fn();

vi.mock('@/lib/api', () => ({
  api: {
    createAgent: (...a: unknown[]) => createAgent(...a),
    updateAgent: (...a: unknown[]) => updateAgent(...a),
    getAgent: (...a: unknown[]) => getAgent(...a),
    saveCronJob: (...a: unknown[]) => saveCronJob(...a),
  },
}));

const wrap = (hasAccount = true) =>
  render(<I18nextProvider i18n={i18n}><ToastProvider><MailTriagePreset hasAccount={hasAccount} /></ToastProvider></I18nextProvider>);

beforeEach(async () => {
  await i18n.changeLanguage('en');
  [createAgent, updateAgent, getAgent, saveCronJob].forEach((m) => m.mockReset());
  getAgent.mockResolvedValue({ model_config: { primary_model: 'anthropic/claude' } });
  createAgent.mockResolvedValue({});
  saveCronJob.mockResolvedValue({ status: 'ok' });
});
afterEach(cleanup);

describe('MailTriagePreset', () => {
  it('is locked until a mail account exists', () => {
    wrap(false);
    expect(screen.getByRole('button', { name: 'Start' })).toBeDisabled();
    expect(screen.getByText(/Add a mail account first/)).toBeInTheDocument();
  });

  it('creates the limited agent first, then the schedule', async () => {
    const user = userEvent.setup();
    wrap();
    await user.click(screen.getByRole('button', { name: 'Start' }));
    await waitFor(() => expect(saveCronJob).toHaveBeenCalledTimes(1));
    expect(createAgent).toHaveBeenCalledTimes(1);
    const agent = createAgent.mock.calls[0][0];
    expect(agent.authorized_tools).not.toContain('native_mail_send');
    expect(agent.model_config).toEqual({ primary_model: 'anthropic/claude' });
    expect(saveCronJob.mock.calls[0][0]).toMatchObject({ agent_id: 'agent_mail_triage', cron_expr: '*/15 * * * *', enabled: true });
    expect(createAgent.mock.invocationCallOrder[0]).toBeLessThan(saveCronJob.mock.invocationCallOrder[0]);
  });

  it('adds the acting tools only when the owner ticks the box', async () => {
    const user = userEvent.setup();
    wrap();
    await user.click(screen.getByLabelText('Let it send and answer invitations under my rules'));
    expect(screen.getByText(/everything else still waits for your approval/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Start' }));
    await waitFor(() => expect(createAgent).toHaveBeenCalled());
    expect(createAgent.mock.calls[0][0].authorized_tools).toContain('native_mail_send');
  });

  it('does not schedule anything when the agent cannot be created or updated', async () => {
    const user = userEvent.setup();
    createAgent.mockRejectedValue(new Error('exists'));
    updateAgent.mockRejectedValue(new Error('forbidden'));
    wrap();
    await user.click(screen.getByRole('button', { name: 'Start' }));
    expect(await screen.findByText('Could not schedule mail triage')).toBeInTheDocument();
    expect(saveCronJob).not.toHaveBeenCalled();
  });

  it('updates the agent when it already exists so the tool list matches the choice', async () => {
    const user = userEvent.setup();
    createAgent.mockRejectedValue(new Error('already exists'));
    updateAgent.mockResolvedValue({});
    wrap();
    await user.click(screen.getByRole('button', { name: 'Start' }));
    await waitFor(() => expect(saveCronJob).toHaveBeenCalled());
    expect(updateAgent).toHaveBeenCalledWith('agent_mail_triage', expect.objectContaining({ authorized_tools: expect.any(Array) }));
  });
});
