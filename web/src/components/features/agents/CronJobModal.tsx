import { useState } from 'react';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Plus } from 'lucide-react';
import { api } from '@/lib/api';
import { useToast } from '@/components/ui/Toast';
import type { AgentManifest } from '@/lib/types';

interface CronJobModalProps {
  isOpen: boolean;
  onClose: () => void;
  onJobCreated: () => void;
  agents: AgentManifest[];
}

export function CronJobModal({ isOpen, onClose, onJobCreated, agents }: CronJobModalProps) {
  const { success, error } = useToast();
  const [name, setName] = useState('');
  const [agentId, setAgentId] = useState(agents[0]?.agent_id || 'default');
  const [cronExpr, setCronExpr] = useState('0 8 * * *');
  const [prompt, setPrompt] = useState('');
  const [channel, setChannel] = useState('telegram');
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
        target_channel: channel || 'telegram',
        target_recipient: recipient.trim() || undefined,
        channel: channel || 'telegram',
        recipient: recipient.trim() || undefined,
        enabled: true,
      });
      success('Proactive Cron Task Scheduled', `Task "${name}" is registered and active.`);
      onJobCreated();
      onClose();
    } catch (err: any) {
      error('Failed to schedule cron task', err.message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Schedule Autonomous Proactive Task (Cron)">
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="text-caption uppercase text-slate font-semibold block mb-1">
            Task Name
          </label>
          <Input
            placeholder="e.g. Daily Morning Briefing"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
          />
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="text-caption uppercase text-slate font-semibold block mb-1">
              Executing Agent
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
              Cron Expression (5-Field)
            </label>
            <Input
              placeholder="e.g. 0 8 * * * or */30 * * * *"
              value={cronExpr}
              onChange={(e) => setCronExpr(e.target.value)}
              required
            />
          </div>
        </div>

        <div>
          <label className="text-caption uppercase text-slate font-semibold block mb-1">
            Instruction / Prompt to Execute
          </label>
          <textarea
            rows={3}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="e.g. Generate a concise weather forecast, summarize pending GitHub PRs, and list high-priority items."
            className="w-full bg-canvas text-deep-ink font-sans text-body-sm p-3 rounded-[12px] border border-onyx/20 focus:outline-none focus:ring-2 focus:ring-deep-ink"
            required
          />
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="text-caption uppercase text-slate font-semibold block mb-1">
              Push Notification Channel
            </label>
            <select
              value={channel}
              onChange={(e) => setChannel(e.target.value)}
              className="w-full bg-soft-meadow text-deep-ink text-body-sm p-2.5 rounded-[12px] border border-onyx/10 focus:outline-none focus:ring-2 focus:ring-deep-ink"
            >
              <option value="telegram">Telegram Bot</option>
              <option value="whatsapp">WhatsApp Cloud API</option>
              <option value="discord">Discord Gateway</option>
              <option value="webhook">Inbound/Outbound Webhook</option>
            </select>
          </div>

          <div>
            <label className="text-caption uppercase text-slate font-semibold block mb-1">
              Recipient Chat ID / Phone
            </label>
            <Input
              placeholder="e.g. 123456789 or +84912345678"
              value={recipient}
              onChange={(e) => setRecipient(e.target.value)}
            />
          </div>
        </div>

        <div className="flex items-center justify-end gap-2.5 pt-2">
          <Button variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button
            type="submit"
            variant="primary"
            size="sm"
            icon={<Plus className="w-3.5 h-3.5" />}
            disabled={saving}
          >
            {saving ? 'Scheduling...' : 'Save & Schedule Task'}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
