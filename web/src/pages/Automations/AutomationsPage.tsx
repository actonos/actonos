import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { useToast } from '@/components/ui/Toast';
import {
  Calendar,
  Play,
  Trash2,
  Edit2,
  Plus,
  RefreshCw,
  Sparkles,
  Send,
  X,
} from 'lucide-react';
import { api, type CronJobItem } from '@/lib/api';
import type { AgentManifest } from '@/lib/types';

export function AutomationsPage() {
  const { t } = useTranslation('automations');
  const { success, error } = useToast();
  const [jobs, setJobs] = useState<CronJobItem[]>([]);
  const [agents, setAgents] = useState<AgentManifest[]>([]);
  const [loading, setLoading] = useState(true);
  const [runningJobId, setRunningJobId] = useState<string | null>(null);

  // Modal State
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [editingJob, setEditingJob] = useState<CronJobItem | null>(null);
  const [name, setName] = useState('');
  const [agentID, setAgentID] = useState('agent_system_core');
  const [cronExpr, setCronExpr] = useState('0 8 * * *');
  const [prompt, setPrompt] = useState('');
  const [targetChannel, setTargetChannel] = useState('telegram');
  const [targetRecipient, setTargetRecipient] = useState('');
  const [saving, setSaving] = useState(false);

  // Delete Confirm Modal
  const [deletingJobId, setDeletingJobId] = useState<string | null>(null);

  const loadData = async () => {
    try {
      const [jobsRes, agentsRes] = await Promise.all([
        api.listCronJobs().catch(() => ({ jobs: [], count: 0 })),
        api.listAgents().catch(() => ({ agents: [], count: 0 })),
      ]);
      setJobs(jobsRes.jobs || []);
      setAgents(agentsRes.agents || []);
    } catch (err: any) {
      error('Failed to load cron tasks', err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const openCreateModal = () => {
    setEditingJob(null);
    setName('');
    setAgentID(agents[0]?.agent_id || 'agent_system_core');
    setCronExpr('0 8 * * *');
    setPrompt('');
    setTargetChannel('telegram');
    setTargetRecipient('');
    setIsModalOpen(true);
  };

  const openEditModal = (job: CronJobItem) => {
    setEditingJob(job);
    setName(job.name);
    setAgentID(job.agent_id);
    setCronExpr(job.cron_expr);
    setPrompt(job.prompt);
    setTargetChannel(job.target_channel || job.channel || 'telegram');
    setTargetRecipient(job.target_recipient || job.recipient || '');
    setIsModalOpen(true);
  };

  const handleSave = async () => {
    if (!name.trim() || !cronExpr.trim() || !prompt.trim()) {
      error('Validation Error', 'Please complete all required task fields.');
      return;
    }

    setSaving(true);
    try {
      await api.saveCronJob({
        id: editingJob?.id || `job_${Date.now()}`,
        name: name.trim(),
        agent_id: agentID,
        cron_expr: cronExpr.trim(),
        prompt: prompt.trim(),
        target_channel: targetChannel || 'telegram',
        target_recipient: targetRecipient.trim() || undefined,
        channel: targetChannel || 'telegram',
        recipient: targetRecipient.trim() || undefined,
        enabled: true,
      });

      success(
        editingJob ? 'Cron Task Updated' : 'Cron Task Scheduled',
        `Task "${name}" is registered with schedule ${cronExpr}.`
      );
      setIsModalOpen(false);
      loadData();
    } catch (err: any) {
      error('Failed to schedule cron task', err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleRunNow = async (job: CronJobItem) => {
    setRunningJobId(job.id);
    try {
      const res = await api.triggerCronJob(job.id);
      success('Execution Triggered', res.message || `Running "${job.name}" immediately.`);
    } catch (err: any) {
      error('Failed to trigger execution', err.message);
    } finally {
      setRunningJobId(null);
    }
  };

  const handleDelete = async () => {
    if (!deletingJobId) return;
    try {
      await api.deleteCronJob(deletingJobId);
      success('Task Deleted', 'The scheduled automation has been removed.');
      setDeletingJobId(null);
      loadData();
    } catch (err: any) {
      error('Failed to delete cron task', err.message);
    }
  };

  const cronPresets = [
    { label: t('modal.presetDaily', 'Daily 8 AM'), expr: '0 8 * * *' },
    { label: t('modal.presetHourly', 'Every Hour'), expr: '0 * * * *' },
    { label: t('modal.presetWeekly', 'Weekly (Mon 9 AM)'), expr: '0 9 * * 1' },
    { label: 'Every 30m', expr: '*/30 * * * *' },
  ];

  return (
    <div className="relative min-h-[calc(100vh-64px)]">
      <BlobBackdrop />

      <PageContainer>
        {/* Header */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
          <div className="flex-1">
            <span className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
              {t('eyebrow', 'Proactive Autonomous Scheduling')}
            </span>
            <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight flex items-center gap-3">
              <span>{t('title', 'Cron Automations')}</span>
              <Badge variant="neutral" className="text-caption font-mono">
                {jobs.length} Scheduled
              </Badge>
            </h1>
            <p className="font-sans text-body text-slate mt-1 max-w-2xl">
              {t(
                'subtitle',
                'Schedule recurring AI tasks with automatic ReAct reasoning, sandboxed tool execution, and multi-channel push alerts.'
              )}
            </p>
          </div>

          <div className="flex items-center gap-2.5 shrink-0 self-start sm:self-center">
            <Button
              variant="ghost"
              size="sm"
              icon={<RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />}
              onClick={loadData}
            >
              Refresh
            </Button>
            <Button
              variant="primary"
              size="sm"
              icon={<Plus className="w-3.5 h-3.5" />}
              onClick={openCreateModal}
            >
              {t('actions.newTask', 'Schedule New Task')}
            </Button>
          </div>
        </div>

        {/* Tasks List Table */}
        {loading ? (
          <div className="py-24 text-center text-slate font-sans text-body">Loading automations...</div>
        ) : jobs.length === 0 ? (
          <Card className="p-12 text-center border border-dashed border-onyx/20 bg-canvas/60">
            <div className="w-12 h-12 rounded-full bg-soft-meadow text-deep-ink flex items-center justify-center mx-auto mb-3">
              <Calendar className="w-6 h-6" />
            </div>
            <h3 className="font-serif text-heading-sm text-deep-ink mb-1">No Scheduled Automations</h3>
            <p className="text-body-sm text-slate max-w-md mx-auto mb-6">
              Create recurring tasks like daily briefings, security audits, git health scans, or meeting reminders.
            </p>
            <Button variant="primary" size="sm" icon={<Plus className="w-3.5 h-3.5" />} onClick={openCreateModal}>
              Schedule Your First Task
            </Button>
          </Card>
        ) : (
          <div className="bg-canvas/90 border border-onyx/10 rounded-2xl shadow-xs overflow-hidden">
            <div className="overflow-x-auto">
              <table className="w-full text-left border-collapse">
                <thead>
                  <tr className="border-b border-onyx/10 bg-soft-meadow/50 text-[11px] font-mono uppercase tracking-wider text-slate">
                    <th className="py-3.5 px-4 font-semibold">Schedule</th>
                    <th className="py-3.5 px-4 font-semibold">Task Name</th>
                    <th className="py-3.5 px-4 font-semibold">Agent</th>
                    <th className="py-3.5 px-4 font-semibold">Instructions / Directive</th>
                    <th className="py-3.5 px-4 font-semibold">Push Route</th>
                    <th className="py-3.5 px-4 font-semibold text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-onyx/5 text-body-sm font-sans">
                  {jobs.map((job) => {
                    const matchedAgent = agents.find((a) => a.agent_id === job.agent_id);
                    const isRunning = runningJobId === job.id;
                    const channelName = job.target_channel || job.channel || 'telegram';
                    const recipient = job.target_recipient || job.recipient || '';

                    return (
                      <tr key={job.id} className="hover:bg-soft-meadow/40 transition-colors">
                        {/* Schedule Column */}
                        <td className="py-4 px-4 align-top whitespace-nowrap">
                          <Badge variant="active" className="font-mono text-[11px] px-2 py-0.5">
                            {job.cron_expr}
                          </Badge>
                          {job.next_run && (
                            <div className="text-[11px] text-slate mt-1 font-mono">
                              Next: {new Date(job.next_run).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                            </div>
                          )}
                        </td>

                        {/* Task Name Column */}
                        <td className="py-4 px-4 align-top min-w-[180px]">
                          <div className="font-serif font-semibold text-deep-ink text-body-sm">
                            {job.name}
                          </div>
                          <div className="font-mono text-[11px] text-slate truncate max-w-[200px]">
                            {job.id}
                          </div>
                        </td>

                        {/* Assigned Agent Column */}
                        <td className="py-4 px-4 align-top whitespace-nowrap">
                          <div className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-soft-meadow border border-onyx/10 text-caption text-deep-ink font-medium">
                            <Sparkles className="w-3.5 h-3.5 text-hi-yellow shrink-0" />
                            <span>{matchedAgent ? matchedAgent.name : job.agent_id}</span>
                          </div>
                        </td>

                        {/* Prompt Column */}
                        <td className="py-4 px-4 align-top max-w-sm">
                          <p className="text-caption text-slate line-clamp-2 bg-soft-meadow/40 p-2 rounded-xl border border-onyx/5">
                            "{job.prompt}"
                          </p>
                        </td>

                        {/* Push Route Column */}
                        <td className="py-4 px-4 align-top whitespace-nowrap">
                          <div className="space-y-1">
                            <Badge
                              variant={channelName === 'telegram' ? 'accent' : 'neutral'}
                              className="text-[11px] font-mono capitalize"
                            >
                              {channelName === 'telegram' ? '📱 Telegram Bot' : channelName === 'whatsapp' ? '💬 WhatsApp' : channelName}
                            </Badge>
                            {recipient ? (
                              <div className="text-[11px] font-mono text-slate flex items-center gap-1">
                                <Send className="w-2.5 h-2.5" />
                                <span>{recipient}</span>
                              </div>
                            ) : (
                              <div className="text-[10px] font-mono text-slate/70">Auto-route</div>
                            )}
                          </div>
                        </td>

                        {/* Actions Column */}
                        <td className="py-4 px-4 align-top text-right whitespace-nowrap">
                          <div className="inline-flex items-center gap-1.5">
                            <Button
                              variant="primary"
                              size="sm"
                              icon={<Play className={`w-3.5 h-3.5 ${isRunning ? 'animate-spin' : ''}`} />}
                              onClick={() => handleRunNow(job)}
                              disabled={isRunning}
                              className="px-3"
                            >
                              {isRunning ? 'Running...' : t('actions.runNow', 'Run Now')}
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              icon={<Edit2 className="w-3.5 h-3.5" />}
                              onClick={() => openEditModal(job)}
                              title="Edit task"
                            />
                            <Button
                              variant="danger"
                              size="sm"
                              icon={<Trash2 className="w-3.5 h-3.5" />}
                              onClick={() => setDeletingJobId(job.id)}
                              title="Delete task"
                            />
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </PageContainer>

      {/* Schedule / Edit Modal */}
      {isModalOpen && (
        <div className="fixed inset-0 bg-black/40 backdrop-blur-xs z-50 flex items-center justify-center p-4">
          <Card className="w-full max-w-xl p-6 bg-canvas border border-onyx/20 shadow-xl max-h-[90vh] overflow-y-auto space-y-4">
            <div className="flex items-center justify-between pb-3 border-b border-onyx/10">
              <div className="flex items-center gap-2.5">
                <Calendar className="w-5 h-5 text-deep-ink" />
                <h3 className="font-serif text-heading-sm text-deep-ink font-semibold">
                  {editingJob ? t('modal.editTitle', 'Edit Cron Task') : t('modal.createTitle', 'Schedule New Cron Task')}
                </h3>
              </div>
              <button onClick={() => setIsModalOpen(false)} className="p-1 rounded-full hover:bg-black/5 text-slate">
                <X className="w-5 h-5" />
              </button>
            </div>

            <div className="space-y-4">
              <div>
                <label className="block text-caption font-semibold text-deep-ink mb-1">
                  {t('modal.nameLabel', 'Task Name')}
                </label>
                <Input
                  placeholder={t('modal.namePlaceholder', 'e.g., Daily Morning Briefing & Repo Health')}
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                />
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <label className="block text-caption font-semibold text-deep-ink mb-1">
                    {t('modal.agentLabel', 'Executing Agent')}
                  </label>
                  <select
                    value={agentID}
                    onChange={(e) => setAgentID(e.target.value)}
                    className="w-full bg-soft-meadow text-deep-ink p-2.5 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none"
                  >
                    {agents.map((ag) => (
                      <option key={ag.agent_id} value={ag.agent_id}>
                        {ag.name} ({ag.status})
                      </option>
                    ))}
                  </select>
                </div>

                <div>
                  <label className="block text-caption font-semibold text-deep-ink mb-1">
                    {t('modal.cronExprLabel', 'Cron Schedule Expression')}
                  </label>
                  <Input
                    placeholder="0 8 * * *"
                    value={cronExpr}
                    onChange={(e) => setCronExpr(e.target.value)}
                  />
                </div>
              </div>

              {/* Schedule Presets Chips */}
              <div className="flex flex-wrap items-center gap-1.5">
                {cronPresets.map((preset) => (
                  <button
                    key={preset.expr}
                    type="button"
                    onClick={() => setCronExpr(preset.expr)}
                    className={`px-3 py-1 rounded-full text-caption font-sans transition-all cursor-pointer ${
                      cronExpr === preset.expr
                        ? 'bg-deep-ink text-white font-semibold shadow-xs'
                        : 'bg-soft-meadow text-deep-ink hover:bg-black/5 border border-onyx/5'
                    }`}
                  >
                    {preset.label}
                  </button>
                ))}
              </div>

              <div>
                <label className="block text-caption font-semibold text-deep-ink mb-1">
                  {t('modal.promptLabel', 'Instruction / Prompt')}
                </label>
                <textarea
                  rows={4}
                  placeholder={t(
                    'modal.promptPlaceholder',
                    'Describe the proactive task in detail. The agent will execute tools, analyze data, and summarize findings...'
                  )}
                  value={prompt}
                  onChange={(e) => setPrompt(e.target.value)}
                  className="w-full bg-soft-meadow text-deep-ink p-3 rounded-2xl border border-onyx/10 text-body-sm font-sans focus:outline-none"
                />
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <label className="block text-caption font-semibold text-deep-ink mb-1">
                    {t('modal.channelLabel', 'Push Alert Channel')}
                  </label>
                  <select
                    value={targetChannel}
                    onChange={(e) => setTargetChannel(e.target.value)}
                    className="w-full bg-soft-meadow text-deep-ink p-2.5 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none"
                  >
                    <option value="telegram">{t('modal.channelTelegram', 'Telegram Bot')}</option>
                    <option value="whatsapp">{t('modal.channelWhatsApp', 'WhatsApp Cloud API')}</option>
                    <option value="all">All Paired Channels</option>
                    <option value="none">{t('modal.channelNone', 'None (Internal Execution Only)')}</option>
                  </select>
                </div>

                <div>
                  <label className="block text-caption font-semibold text-deep-ink mb-1">
                    {t('modal.recipientLabel', 'Recipient / Chat ID')}
                  </label>
                  <Input
                    placeholder={t('modal.recipientPlaceholder', 'e.g., 123456789 or leave empty for auto-route')}
                    value={targetRecipient}
                    onChange={(e) => setTargetRecipient(e.target.value)}
                  />
                </div>
              </div>
            </div>

            <div className="flex items-center justify-end gap-2.5 pt-4 border-t border-onyx/10">
              <Button variant="ghost" size="sm" onClick={() => setIsModalOpen(false)}>
                Cancel
              </Button>
              <Button variant="primary" size="sm" onClick={handleSave} disabled={saving}>
                {saving ? 'Scheduling...' : t('modal.save', 'Schedule Task')}
              </Button>
            </div>
          </Card>
        </div>
      )}

      {/* Delete Confirmation Modal */}
      <ConfirmModal
        isOpen={!!deletingJobId}
        onClose={() => setDeletingJobId(null)}
        onConfirm={handleDelete}
        title="Delete Cron Task"
        description="Are you sure you want to permanently remove this automated cron task? Scheduled executions will stop immediately."
        confirmLabel="Delete Task"
        variant="danger"
      />
    </div>
  );
}
