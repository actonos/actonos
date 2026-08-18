import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import {
  Bot,
  AlertCircle,
  Radio,
} from 'lucide-react';
import type { AutonomousTask, TaskPriority, TaskStatus, AgentManifest, ChannelAccount } from '@/lib/types';
import { api } from '@/lib/api';

interface TaskModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSave: (taskData: Partial<AutonomousTask>) => Promise<void>;
  task?: AutonomousTask | null;
}

export function TaskModal({ isOpen, onClose, onSave, task }: TaskModalProps) {
  const { t } = useTranslation('missions');
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [priority, setPriority] = useState<TaskPriority>('p2_normal');
  const [status, setStatus] = useState<TaskStatus>('pending');
  const [assignedAgentId, setAssignedAgentId] = useState('auto');
  const [targetChannel, setTargetChannel] = useState('all');
  const [targetAccountId, setTargetAccountId] = useState('all');
  const [progress, setProgress] = useState(0);

  const [availableAgents, setAvailableAgents] = useState<AgentManifest[]>([]);
  const [channelAccounts, setChannelAccounts] = useState<ChannelAccount[]>([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (isOpen) {
      if (task) {
        setTitle(task.title || '');
        setDescription(task.description || '');
        setPriority(task.priority || 'p2_normal');
        setStatus(task.status || 'pending');
        setAssignedAgentId(task.assigned_agent_id || 'auto');
        setTargetChannel(task.target_channel || 'all');
        setTargetAccountId(task.target_account_id || 'all');
        setProgress(task.progress || 0);
      } else {
        setTitle('');
        setDescription('');
        setPriority('p2_normal');
        setStatus('pending');
        setAssignedAgentId('auto');
        setTargetChannel('all');
        setTargetAccountId('all');
        setProgress(0);
      }

      // Load agents and channels
      api.listAgents().then((res) => {
        if (res && res.agents) setAvailableAgents(res.agents);
      }).catch(() => {});

      api.listAllChannelAccounts().then((res) => {
        if (res && res.accounts) setChannelAccounts(res.accounts);
      }).catch(() => {});
    }
  }, [isOpen, task]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim()) return;

    setSaving(true);
    try {
      await onSave({
        id: task?.id,
        title: title.trim(),
        description: description.trim(),
        priority,
        status,
        assigned_agent_id: assignedAgentId,
        target_channel: targetChannel,
        target_account_id: targetAccountId,
        progress: status === 'completed' ? 100 : progress,
      });
      onClose();
    } catch (err) {
      console.error('Failed to save task:', err);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={task ? t('modal.editTitle', 'Edit Mission / Task') : t('modal.createTitle', 'New Mission / Task')}
      maxWidth="max-w-xl"
    >
      <form onSubmit={handleSubmit} className="space-y-4">
        {/* Title */}
        <div>
          <label className="block text-caption font-semibold text-deep-ink mb-1.5">
            {t('modal.taskTitle', 'Mission Title *')}
          </label>
          <Input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder={t('modal.taskPlaceholder')}
            required
            autoFocus
          />
        </div>

        {/* Description / Instructions */}
        <div>
          <label className="block text-caption font-semibold text-deep-ink mb-1.5">
            {t('modal.directive', 'Directive & Criteria')}
          </label>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t('modal.directivePlaceholder')}
            rows={3}
            className="w-full bg-soft-meadow text-deep-ink text-body-sm font-sans p-3 rounded-2xl border border-onyx/10 focus:outline-none focus:border-onyx/30 resize-none"
          />
        </div>

        {/* Priority & Assigned Agent */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div>
            <label className="block text-caption font-semibold text-deep-ink mb-1.5 flex items-center gap-1.5">
              <AlertCircle className="w-3.5 h-3.5 text-slate" />
              <span>{t('modal.priority', 'Priority Level')}</span>
            </label>
            <select
              value={priority}
              onChange={(e) => setPriority(e.target.value as TaskPriority)}
              className="w-full bg-soft-meadow text-deep-ink text-body-sm font-sans p-2.5 rounded-full border border-onyx/10 focus:outline-none"
            >
              <option value="p0_critical">{t('priorities.p0')}</option>
              <option value="p1_high">{t('priorities.p1')}</option>
              <option value="p2_normal">{t('priorities.p2')}</option>
              <option value="p3_low">{t('priorities.p3')}</option>
            </select>
          </div>

          <div>
            <label className="block text-caption font-semibold text-deep-ink mb-1.5 flex items-center gap-1.5">
              <Bot className="w-3.5 h-3.5 text-slate" />
              <span>{t('modal.assignedAgent', 'Assigned Agent')}</span>
            </label>
            <select
              value={assignedAgentId}
              onChange={(e) => setAssignedAgentId(e.target.value)}
              className="w-full bg-soft-meadow text-deep-ink text-body-sm font-sans p-2.5 rounded-full border border-onyx/10 focus:outline-none"
            >
              <option value="auto">{t('modal.autoCoordinator')}</option>
              {availableAgents.map((ag) => (
                <option key={ag.agent_id} value={ag.agent_id}>
                  {ag.name} ({ag.agent_id})
                </option>
              ))}
            </select>
          </div>
        </div>

        {/* Status & Progress (if editing) */}
        {task && (
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 p-3 bg-soft-meadow/50 rounded-2xl border border-onyx/5">
            <div>
              <label className="block text-caption font-semibold text-deep-ink mb-1">
                {t('modal.status', 'Status')}
              </label>
              <select
                value={status}
                onChange={(e) => setStatus(e.target.value as TaskStatus)}
                className="w-full bg-canvas text-deep-ink text-body-sm font-sans p-2 rounded-full border border-onyx/10 focus:outline-none"
              >
                <option value="pending">{t('statuses.pending')}</option>
                <option value="in_progress">{t('statuses.inProgress')}</option>
                <option value="completed">{t('statuses.completed')}</option>
                <option value="blocked">{t('statuses.blocked')}</option>
                <option value="cancelled">{t('statuses.cancelled')}</option>
              </select>
            </div>

            <div>
              <label className="block text-caption font-semibold text-deep-ink mb-1 flex justify-between">
                <span>{t('modal.progress', 'Progress')}</span>
                <span className="font-mono">{progress}%</span>
              </label>
              <input
                type="range"
                min="0"
                max="100"
                step="5"
                value={progress}
                onChange={(e) => setProgress(Number(e.target.value))}
                className="w-full accent-deep-ink cursor-pointer mt-1.5"
              />
            </div>
          </div>
        )}

        {/* Notification Outbound Channel */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div>
            <label className="block text-caption font-semibold text-deep-ink mb-1.5 flex items-center gap-1.5">
              <Radio className="w-3.5 h-3.5 text-slate" />
              <span>{t('modal.channel', 'Push Notification Channel')}</span>
            </label>
            <select
              value={targetChannel}
              onChange={(e) => {
                setTargetChannel(e.target.value);
                setTargetAccountId('all');
              }}
              className="w-full bg-soft-meadow text-deep-ink text-body-sm font-sans p-2.5 rounded-full border border-onyx/10 focus:outline-none"
            >
              <option value="all">{t('channels.allBroadcast')}</option>
              <option value="telegram">{t('channels.telegram')}</option>
              <option value="whatsapp">{t('channels.whatsapp')}</option>
              <option value="discord">{t('channels.discord')}</option>
              <option value="none">{t('channels.noneSilent')}</option>
            </select>
          </div>

          <div>
            <label className="block text-caption font-semibold text-deep-ink mb-1.5">
              {t('modal.account', 'Target Account')}
            </label>
            <select
              value={targetAccountId}
              onChange={(e) => setTargetAccountId(e.target.value)}
              disabled={targetChannel === 'none' || targetChannel === 'all'}
              className="w-full bg-soft-meadow text-deep-ink text-body-sm font-sans p-2.5 rounded-full border border-onyx/10 focus:outline-none disabled:opacity-50"
            >
              <option value="all">{t('modal.allAccounts', { channel: targetChannel })}</option>
              {channelAccounts
                .filter((acc) => acc.channel === targetChannel)
                .map((acc) => (
                  <option key={acc.id} value={acc.id}>
                    {acc.name || acc.label || acc.id}
                  </option>
                ))}
            </select>
          </div>
        </div>

        {/* Action Buttons */}
        <div className="flex items-center justify-end gap-2.5 pt-3 border-t border-onyx/10">
          <Button variant="ghost" size="md" onClick={onClose} type="button" disabled={saving}>
            {t('modal.cancel')}
          </Button>
          <Button variant="primary" size="md" type="submit" disabled={saving || !title.trim()}>
            {saving ? t('modal.saving') : task ? t('modal.update') : t('modal.create')}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
