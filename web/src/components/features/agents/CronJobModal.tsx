import { useState } from 'react';
import { getErrorMessage } from '@/lib/errors';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Plus } from 'lucide-react';
import { api } from '@/lib/api';
import { useToast } from '@/components/ui/Toast';
import type { AgentManifest } from '@/lib/types';
import { useInstalledChannels } from '@/lib/installed-channels';

interface CronJobModalProps {
  isOpen: boolean;
  onClose: () => void;
  onJobCreated: () => void;
  agents: AgentManifest[];
}

export function CronJobModal({ isOpen, onClose, onJobCreated, agents }: CronJobModalProps) {
  const { t } = useTranslation('agents');
  const { success, error } = useToast();
  const [name, setName] = useState('');
  const [agentId, setAgentId] = useState(agents[0]?.agent_id || 'default');
  const [cronExpr, setCronExpr] = useState('0 8 * * *');
  const [prompt, setPrompt] = useState('');
  const [channel, setChannel] = useState('none');
  const { channels: pluginChannels } = useInstalledChannels();
  const [recipient, setRecipient] = useState('');
  const [saving, setSaving] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim() || !cronExpr.trim() || !prompt.trim()) return;

    setSaving(true);
    try {
      await api.saveCronJob({
        name: name.trim(),
        agent_id: agentId,
        cron_expr: cronExpr.trim(),
        prompt: prompt.trim(),
        target_channel: channel || 'none',
        target_recipient: recipient.trim() || undefined,
        channel: channel || 'none',
        recipient: recipient.trim() || undefined,
        enabled: true,
      });
      success(t('cron.successTitle'), t('cron.successDescription', { name }));
      onJobCreated();
      onClose();
    } catch (err) {
      error(t('cron.failureTitle'), getErrorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={t('cron.title')}>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="text-caption uppercase text-slate font-semibold block mb-1">
            {t('cron.name')}
          </label>
          <Input
            placeholder={t('cron.namePlaceholder')}
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
          />
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="text-caption uppercase text-slate font-semibold block mb-1">
              {t('cron.agent')}
            </label>
            <select
              value={agentId}
              onChange={(e) => setAgentId(e.target.value)}
              className="w-full bg-soft-meadow text-deep-ink text-body-sm p-2.5 rounded-[12px] border border-onyx/10 focus:outline-none focus:ring-2 focus:ring-deep-ink"
            >
              {agents.map((ag) => (
                <option key={ag.agent_id} value={ag.agent_id}>
                  {ag.name} ({ag.agent_id})
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="text-caption uppercase text-slate font-semibold block mb-1">
              {t('cron.expression')}
            </label>
            <Input
              placeholder={t('cron.expressionPlaceholder')}
              value={cronExpr}
              onChange={(e) => setCronExpr(e.target.value)}
              required
            />
          </div>
        </div>

        <div>
          <label className="text-caption uppercase text-slate font-semibold block mb-1">
            {t('cron.prompt')}
          </label>
          <textarea
            rows={3}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder={t('cron.promptPlaceholder')}
            className="w-full bg-canvas text-deep-ink font-sans text-body-sm p-3 rounded-[12px] border border-onyx/20 focus:outline-none focus:ring-2 focus:ring-deep-ink"
            required
          />
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="text-caption uppercase text-slate font-semibold block mb-1">
              {t('cron.channel')}
            </label>
            <select
              value={channel}
              onChange={(e) => setChannel(e.target.value)}
              className="w-full bg-soft-meadow text-deep-ink text-body-sm p-2.5 rounded-[12px] border border-onyx/10 focus:outline-none focus:ring-2 focus:ring-deep-ink"
            >
              <option value="none">{t('cron.channels.none')}</option>
              {pluginChannels.map((item) => (
                <option key={item.id} value={item.id}>{item.label}</option>
              ))}
            </select>
          </div>

          <div>
            <label className="text-caption uppercase text-slate font-semibold block mb-1">
              {t('cron.recipient')}
            </label>
            <Input
              placeholder={t('cron.recipientPlaceholder')}
              value={recipient}
              onChange={(e) => setRecipient(e.target.value)}
            />
          </div>
        </div>

        <div className="flex items-center justify-end gap-2.5 pt-2">
          <Button variant="ghost" size="sm" onClick={onClose}>
            {t('cron.cancel')}
          </Button>
          <Button
            type="submit"
            variant="primary"
            size="sm"
            icon={<Plus className="w-3.5 h-3.5" />}
            disabled={saving}
          >
            {saving ? t('cron.scheduling') : t('cron.save')}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
