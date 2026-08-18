import { useState, useEffect } from 'react';
import { getErrorMessage } from '@/lib/errors';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { useToast } from '@/components/ui/Toast';
import {
  HeartPulse,
  CheckSquare,
  Plus,
  RefreshCw,
  Bot,
  Play,
  CheckCircle2,
  Trash2,
  Edit2,
  Sparkles,
  Save,
  MessageSquare,
} from 'lucide-react';
import { api } from '@/lib/api';
import type { AgentRun, ApprovalRequest, AutonomousTask, HeartbeatConfigData, HeartbeatRun, TaskPriority, TaskStatus } from '@/lib/types';
import { TaskModal } from './components/TaskModal';
import { PageHeader } from '@/components/ui/PageHeader';
import { SegmentedControl } from '@/components/ui/SegmentedControl';
import { readHashParams, setHashParam } from '@/lib/url-state';

export interface MissionsPageProps {
  onOpenChat?: (agentID?: string) => void;
}

export function MissionsPage({ onOpenChat }: MissionsPageProps) {
  const { t } = useTranslation('missions');
  const { success, error, info } = useToast();

  const [activeTab, setActiveTab] = useState<'tasks' | 'directives' | 'audit' | 'governance'>(() => {
    const value = readHashParams().get('view');
    return value === 'directives' || value === 'audit' || value === 'governance' ? value : 'tasks';
  });
  const [tasks, setTasks] = useState<AutonomousTask[]>([]);
  const [heartbeatConfig, setHeartbeatConfig] = useState<HeartbeatConfigData>({
    enabled: true,
    interval_minutes: 5,
    directives: '',
    target_channel: 'all',
    target_account_id: 'all',
    auto_delegate: true,
    zero_noise: true,
  });
  const [heartbeatRuns, setHeartbeatRuns] = useState<HeartbeatRun[]>([]);
  const [approvals, setApprovals] = useState<ApprovalRequest[]>([]);
  const [agentRuns, setAgentRuns] = useState<AgentRun[]>([]);
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [priorityFilter, setPriorityFilter] = useState<string>('all');

  const [loading, setLoading] = useState(true);
  const [triggeringPulse, setTriggeringPulse] = useState(false);
  const [savingDirectives, setSavingDirectives] = useState(false);

  // Modal states
  const [isTaskModalOpen, setIsTaskModalOpen] = useState(false);
  const [editingTask, setEditingTask] = useState<AutonomousTask | null>(null);
  const [deletingTaskId, setDeletingTaskId] = useState<string | null>(null);
  const changeTab = (tab: 'tasks' | 'directives' | 'audit' | 'governance') => {
    setActiveTab(tab);
    setHashParam('view', tab === 'tasks' ? undefined : tab);
  };

  const loadData = async () => {
    try {
      setLoading(true);
      const [tasksRes, cfgRes, runsRes, approvalsRes, agentRunsRes] = await Promise.all([
        api.listTasks({
          status: statusFilter !== 'all' ? statusFilter : undefined,
          priority: priorityFilter !== 'all' ? priorityFilter : undefined,
        }).catch(() => ({ tasks: [], count: 0 })),
        api.getHeartbeatConfig().catch(() => null),
        api.listHeartbeatRuns().catch(() => []),
        api.listApprovals('pending').catch(() => ({ approvals: [] })),
        api.listAgentRuns(30).catch(() => ({ runs: [] })),
      ]);

      if (tasksRes && tasksRes.tasks) setTasks(tasksRes.tasks);
      if (cfgRes) setHeartbeatConfig(cfgRes);
      if (runsRes) setHeartbeatRuns(runsRes);
      setApprovals(approvalsRes.approvals);
      setAgentRuns(agentRunsRes.runs);
    } catch (err) {
      error('Failed to load mission data', getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 10000);
    return () => clearInterval(interval);
  }, [statusFilter, priorityFilter]);

  const handleCreateOrUpdateTask = async (taskData: Partial<AutonomousTask>) => {
    try {
      if (taskData.id) {
        await api.updateTask(taskData.id, taskData);
        success(t('toast.taskUpdated', 'Mission Updated'), `Task '${taskData.title}' updated successfully.`);
      } else {
        await api.createTask(taskData);
        success(t('toast.taskCreated', 'Mission Created'), `Task '${taskData.title}' added to active backlog.`);
      }
      loadData();
    } catch (err) {
      error('Failed to save task', getErrorMessage(err));
      throw err;
    }
  };

  const handleDeleteTask = async () => {
    if (!deletingTaskId) return;
    try {
      await api.deleteTask(deletingTaskId);
      success(t('toast.taskDeleted', 'Mission Deleted'), 'Task removed from backlog.');
      setDeletingTaskId(null);
      loadData();
    } catch (err) {
      error('Failed to delete task', getErrorMessage(err));
    }
  };

  const handleQuickStatusChange = async (task: AutonomousTask, newStatus: TaskStatus) => {
    try {
      const updated = {
        ...task,
        status: newStatus,
        progress: newStatus === 'completed' ? 100 : task.progress,
      };
      await api.updateTask(task.id, updated);
      loadData();
    } catch (err) {
      error('Failed to update status', getErrorMessage(err));
    }
  };

  const handleTriggerPulse = async () => {
    setTriggeringPulse(true);
    try {
      info(t('pulse.triggering', 'Heartbeat Triggered'), 'Master agent is evaluating backlog and system state...');
      const run = await api.triggerHeartbeatPulse();
      success(
        t('pulse.triggeredSuccess', 'Pulse Completed'),
        run.status === 'ok' ? 'System Nominal (Zero Noise)' : `Action executed: ${run.summary}`
      );
      loadData();
    } catch (err) {
      error('Pulse execution failed', getErrorMessage(err));
    } finally {
      setTriggeringPulse(false);
    }
  };

  const handleSaveDirectives = async () => {
    setSavingDirectives(true);
    try {
      await api.saveHeartbeatConfig(heartbeatConfig);
      success(t('toast.directivesSaved', 'Directives Saved'), 'Standing HEARTBEAT instructions synchronized.');
    } catch (err) {
      error('Failed to save directives', getErrorMessage(err));
    } finally {
      setSavingDirectives(false);
    }
  };

  const handleApproval = async (approval: ApprovalRequest, approved: boolean) => {
    try {
      if (approved) {
        await api.approveAction(approval.id, t('governance.reviewedReason'));
        success(t('governance.approvedTitle'), approval.tool_name);
      } else {
        await api.rejectAction(approval.id, t('governance.rejectedReason'));
        info(t('governance.rejectedTitle'), approval.tool_name);
      }
      await loadData();
    } catch (err: unknown) {
      error(t('governance.decisionFailed'), err instanceof Error ? getErrorMessage(err) : String(err));
    }
  };

  const activeCount = tasks.filter((t) => t.status === 'pending' || t.status === 'in_progress').length;
  const completedCount = tasks.filter((t) => t.status === 'completed').length;
  const lastRun = heartbeatRuns[0];

  const getPriorityBadge = (p: TaskPriority) => {
    switch (p) {
      case 'p0_critical':
        return <Badge variant="stopped" className="text-[10px] font-mono">{t('priorities.p0Short')}</Badge>;
      case 'p1_high':
        return <Badge variant="accent" className="text-[10px] font-mono">{t('priorities.p1Short')}</Badge>;
      case 'p2_normal':
        return <Badge variant="neutral" className="text-[10px] font-mono">{t('priorities.p2Short')}</Badge>;
      case 'p3_low':
      default:
        return <Badge variant="neutral" className="text-[10px] font-mono opacity-70">{t('priorities.p3Short')}</Badge>;
    }
  };

  const getStatusPill = (s: TaskStatus) => {
    switch (s) {
      case 'completed':
        return <Badge variant="active" className="text-[11px]">{t('statuses.completed')}</Badge>;
      case 'in_progress':
        return <Badge variant="accent" className="text-[11px] animate-pulse">{t('statuses.inProgress')}</Badge>;
      case 'blocked':
        return <Badge variant="stopped" className="text-[11px]">{t('statuses.blocked')}</Badge>;
      case 'cancelled':
        return <Badge variant="neutral" className="text-[11px] line-through">{t('statuses.cancelled')}</Badge>;
      case 'pending':
      default:
        return <Badge variant="neutral" className="text-[11px]">{t('statuses.pending')}</Badge>;
    }
  };

  return (
    <div className="relative min-h-screen bg-soft-meadow/40 pb-16">
      <BlobBackdrop />

      <PageContainer>
        <PageHeader
          eyebrow={t('page.eyebrow')}
          title={t('title')}
          description={t('subtitle')}
          badge={<Badge variant="success" className="font-mono">{t('page.activeBacklog', { count: activeCount })}</Badge>}
          actions={(
            <>
            <Button
              variant="ghost"
              size="sm"
              icon={<RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />}
              onClick={loadData}
            >
              {t('actions.refresh')}
            </Button>
            <Button
              variant="primary"
              size="sm"
              icon={<Play className={`w-3.5 h-3.5 ${triggeringPulse ? 'animate-spin' : ''}`} />}
              onClick={handleTriggerPulse}
              disabled={triggeringPulse}
            >
              {triggeringPulse ? t('actions.pulsing') : t('actions.triggerPulse')}
            </Button>
            </>
          )}
        />

        {/* 3 Metric Cards Strip */}
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-8">
          {/* Active Missions Card */}
          <Card className="p-5 border border-onyx/10 bg-canvas/90 shadow-xs flex items-center justify-between">
            <div>
              <span className="text-caption font-semibold uppercase text-slate block mb-1">
                {t('metrics.activeMissions')}
              </span>
              <div className="text-heading font-serif text-deep-ink">
                {activeCount} <span className="text-body-sm font-sans text-slate font-normal">{t('metrics.total', { count: tasks.length })}</span>
              </div>
              <span className="text-[11px] text-emerald-700 font-mono font-semibold">
                {t('metrics.completed', { count: completedCount })}
              </span>
            </div>
            <div className="w-10 h-10 rounded-full bg-emerald-500/10 text-emerald-600 flex items-center justify-center">
              <CheckSquare className="w-5 h-5" />
            </div>
          </Card>

          {/* Heartbeat Pulse Status Card */}
          <Card className="p-5 border border-onyx/10 bg-canvas/90 shadow-xs flex items-center justify-between">
            <div>
              <span className="text-caption font-semibold uppercase text-slate block mb-1">
                {t('metrics.heartbeat')}
              </span>
              <div className="text-heading font-serif text-deep-ink flex items-center gap-2">
                <span>{t('metrics.interval', { minutes: heartbeatConfig.interval_minutes })}</span>
                <span className="w-2.5 h-2.5 rounded-full bg-emerald-500 animate-pulse" />
              </div>
              <span className="text-[11px] text-slate font-mono truncate block max-w-[200px]">
                {t('metrics.last', { status: lastRun ? lastRun.status : t('metrics.nominal') })}
              </span>
            </div>
            <div className="w-10 h-10 rounded-full bg-rose-500/10 text-rose-600 flex items-center justify-center">
              <HeartPulse className="w-5 h-5" />
            </div>
          </Card>

          {/* Coordinator Master Agent */}
          <Card className="p-5 border border-onyx/10 bg-canvas/90 shadow-xs flex items-center justify-between">
            <div>
              <span className="text-caption font-semibold uppercase text-slate block mb-1">
                {t('metrics.coordinator')}
              </span>
              <div className="text-heading-sm font-serif font-bold text-deep-ink">
                agent_system_core
              </div>
              <span className="text-[11px] text-slate font-mono">
                {heartbeatConfig.auto_delegate ? t('metrics.autoDelegate') : t('metrics.singleAgent')}
              </span>
            </div>
            <div className="w-10 h-10 rounded-full bg-hi-yellow/20 text-deep-ink flex items-center justify-center">
              <Bot className="w-5 h-5" />
            </div>
          </Card>
        </div>

        <SegmentedControl
          value={activeTab}
          onChange={changeTab}
          label={t('tabs.label')}
          className="mb-6 w-fit"
          options={[
            { value: 'tasks', label: t('tabs.tasks', { count: tasks.length }) },
            { value: 'directives', label: t('tabs.directives') },
            { value: 'audit', label: t('tabs.audit', { count: heartbeatRuns.length }) },
            { value: 'governance', label: t('governance.tab', { count: approvals.length }) },
          ]}
        />

        {/* TAB 1: Tasks & Backlog */}
        {activeTab === 'tasks' && (
          <div className="space-y-4">
            {/* Control Bar: Filters & New Task Button */}
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-3 bg-canvas rounded-2xl border border-onyx/10">
              <div className="flex flex-wrap items-center gap-2">
                <select
                  aria-label={t('filters.statusLabel')}
                  value={statusFilter}
                  onChange={(e) => setStatusFilter(e.target.value)}
                  className="bg-soft-meadow text-deep-ink text-[12px] font-sans px-3 py-1.5 rounded-full border border-onyx/10 focus:outline-none"
                >
                  <option value="all">{t('filters.allStatuses')}</option>
                  <option value="pending">{t('statuses.pending')}</option>
                  <option value="in_progress">{t('statuses.inProgress')}</option>
                  <option value="completed">{t('statuses.completed')}</option>
                  <option value="blocked">{t('statuses.blocked')}</option>
                </select>

                <select
                  aria-label={t('filters.priorityLabel')}
                  value={priorityFilter}
                  onChange={(e) => setPriorityFilter(e.target.value)}
                  className="bg-soft-meadow text-deep-ink text-[12px] font-sans px-3 py-1.5 rounded-full border border-onyx/10 focus:outline-none"
                >
                  <option value="all">{t('filters.allPriorities')}</option>
                  <option value="p0_critical">{t('priorities.p0Short')}</option>
                  <option value="p1_high">{t('priorities.p1Short')}</option>
                  <option value="p2_normal">{t('priorities.p2Short')}</option>
                  <option value="p3_low">{t('priorities.p3Short')}</option>
                </select>
              </div>

              <Button
                variant="primary"
                size="sm"
                icon={<Plus className="w-3.5 h-3.5" />}
                onClick={() => {
                  setEditingTask(null);
                  setIsTaskModalOpen(true);
                }}
              >
                {t('actions.newMission')}
              </Button>
            </div>

            {/* Task Cards List */}
            {tasks.length > 0 ? (
              <div className="grid grid-cols-1 gap-3">
                {tasks.map((tItem) => (
                  <Card
                    key={tItem.id}
                    className="p-5 border border-onyx/10 bg-canvas/95 hover:border-onyx/25 transition-all shadow-xs space-y-3"
                  >
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
                      <div className="flex items-center gap-2.5 min-w-0">
                        {getPriorityBadge(tItem.priority)}
                        <h3 className="font-serif text-heading-sm font-semibold text-deep-ink truncate">
                          {tItem.title}
                        </h3>
                        {getStatusPill(tItem.status)}
                      </div>

                      <div className="flex items-center gap-1.5 self-end sm:self-auto shrink-0">
                        {onOpenChat && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => onOpenChat(tItem.assigned_agent_id === 'auto' ? 'agent_system_core' : tItem.assigned_agent_id)}
                            className="text-[11px]"
                            icon={<MessageSquare className="w-3.5 h-3.5" />}
                          >
                            {t('actions.chat')}
                          </Button>
                        )}
                        {tItem.status !== 'completed' && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleQuickStatusChange(tItem, 'completed')}
                            className="text-[11px] text-emerald-700"
                            icon={<CheckCircle2 className="w-3.5 h-3.5 text-emerald-600" />}
                          >
                            {t('actions.complete')}
                          </Button>
                        )}
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => {
                            setEditingTask(tItem);
                            setIsTaskModalOpen(true);
                          }}
                          className="text-[11px]"
                          icon={<Edit2 className="w-3.5 h-3.5" />}
                        >
                          {t('actions.edit')}
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setDeletingTaskId(tItem.id)}
                          className="text-[11px] text-red-600 hover:text-red-700"
                          icon={<Trash2 className="w-3.5 h-3.5" />}
                        >
                          {t('actions.delete')}
                        </Button>
                      </div>
                    </div>

                    {/* Directive / Description */}
                    {tItem.description && (
                      <p className="text-caption font-sans text-slate line-clamp-2">
                        {tItem.description}
                      </p>
                    )}

                    {/* Progress Bar & Latest Log */}
                    <div className="space-y-1.5 pt-1">
                      <div className="flex items-center justify-between text-[11px] font-mono text-slate">
                        <div className="flex items-center gap-2">
                          <span>{t('task.agent')} <strong className="text-deep-ink">{tItem.assigned_agent_id}</strong></span>
                          <span>•</span>
                          <span>{t('task.channel')} <strong className="text-deep-ink">{tItem.target_channel || 'all'}</strong></span>
                        </div>
                        <span className="font-semibold text-deep-ink">{tItem.progress}%</span>
                      </div>
                      <div className="w-full bg-onyx/10 h-1.5 rounded-full overflow-hidden">
                        <div
                          className={`h-full rounded-full transition-all duration-500 ${
                            tItem.status === 'completed'
                              ? 'bg-emerald-600'
                              : tItem.status === 'blocked'
                              ? 'bg-rose-500'
                              : 'bg-deep-ink'
                          }`}
                          style={{ width: `${tItem.progress}%` }}
                        />
                      </div>
                    </div>

                    {/* Latest Execution Log Note */}
                    {tItem.execution_log && (
                      <div className="p-2.5 bg-soft-meadow rounded-xl border border-onyx/5 text-[11px] font-mono text-slate flex items-start gap-2">
                        <Sparkles className="w-3.5 h-3.5 text-deep-ink shrink-0 mt-0.5" />
                        <span className="truncate">{tItem.execution_log}</span>
                      </div>
                    )}
                  </Card>
                ))}
              </div>
            ) : (
              <Card className="p-12 text-center border border-onyx/10 bg-canvas space-y-3">
                <CheckSquare className="w-10 h-10 text-slate/40 mx-auto" />
                <h3 className="font-serif text-heading-sm text-deep-ink">{t('empty.title')}</h3>
                <p className="text-caption text-slate max-w-md mx-auto">
                  {t('empty.description')}
                </p>
                <Button
                  variant="primary"
                  size="sm"
                  onClick={() => {
                    setEditingTask(null);
                    setIsTaskModalOpen(true);
                  }}
                  icon={<Plus className="w-3.5 h-3.5" />}
                >
                  {t('actions.createMission')}
                </Button>
              </Card>
            )}
          </div>
        )}

        {/* TAB 2: Standing Directives (HEARTBEAT.md) */}
        {activeTab === 'directives' && (
          <div className="space-y-6">
            <Card className="p-6 border border-onyx/10 bg-canvas space-y-5 max-w-3xl">
              <div className="flex items-center justify-between border-b border-onyx/10 pb-4">
                <div>
                  <h3 className="font-serif text-heading-sm text-deep-ink font-semibold flex items-center gap-2">
                    <HeartPulse className="w-5 h-5 text-rose-600" />
                    <span>{t('directives.title')}</span>
                  </h3>
                  <p className="text-caption text-slate mt-0.5">
                    {t('directives.description')}
                  </p>
                </div>
                <Button
                  variant="primary"
                  size="sm"
                  icon={<Save className="w-3.5 h-3.5" />}
                  onClick={handleSaveDirectives}
                  disabled={savingDirectives}
                >
                  {savingDirectives ? t('directives.saving') : t('directives.save')}
                </Button>
              </div>

              {/* Directives Markdown Editor */}
              <div className="space-y-2">
                <label className="block text-caption font-semibold text-deep-ink">
                  {t('directives.instructions')}
                </label>
                <textarea
                  rows={8}
                  value={heartbeatConfig.directives}
                  onChange={(e) => setHeartbeatConfig({ ...heartbeatConfig, directives: e.target.value })}
                  placeholder={t('directives.placeholder')}
                  className="w-full bg-soft-meadow text-deep-ink font-mono text-[13px] p-4 rounded-2xl border border-onyx/10 focus:outline-none focus:border-onyx/30 resize-none leading-relaxed"
                />
              </div>

              {/* Settings: Interval & Zero-Noise */}
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-2">
                <div>
                  <label className="block text-caption font-semibold text-deep-ink mb-1.5">
                    {t('directives.interval')}
                  </label>
                  <select
                    value={heartbeatConfig.interval_minutes}
                    onChange={(e) => setHeartbeatConfig({ ...heartbeatConfig, interval_minutes: Number(e.target.value) })}
                    className="w-full bg-soft-meadow text-deep-ink text-body-sm font-sans p-2.5 rounded-full border border-onyx/10 focus:outline-none"
                  >
                    <option value={1}>{t('directives.intervals.one')}</option>
                    <option value={5}>{t('directives.intervals.five')}</option>
                    <option value={15}>{t('directives.intervals.fifteen')}</option>
                    <option value={30}>{t('directives.intervals.thirty')}</option>
                    <option value={60}>{t('directives.intervals.hour')}</option>
                  </select>
                </div>

                <div>
                  <label className="block text-caption font-semibold text-deep-ink mb-1.5">
                    {t('directives.channel')}
                  </label>
                  <select
                    value={heartbeatConfig.target_channel}
                    onChange={(e) => setHeartbeatConfig({ ...heartbeatConfig, target_channel: e.target.value })}
                    className="w-full bg-soft-meadow text-deep-ink text-body-sm font-sans p-2.5 rounded-full border border-onyx/10 focus:outline-none"
                  >
                    <option value="all">{t('channels.all')}</option>
                    <option value="telegram">{t('channels.telegram')}</option>
                    <option value="whatsapp">{t('channels.whatsapp')}</option>
                    <option value="discord">{t('channels.discord')}</option>
                    <option value="none">{t('channels.none')}</option>
                  </select>
                </div>
              </div>
            </Card>
          </div>
        )}

        {/* TAB 3: Pulse Audit Ledger */}
        {activeTab === 'audit' && (
          <div className="space-y-4">
            <Card className="p-6 border border-onyx/10 bg-canvas space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="font-serif text-heading-sm text-deep-ink font-semibold">
                    {t('audit.title')}
                  </h3>
                  <p className="text-caption text-slate mt-0.5">
                    {t('audit.description')}
                  </p>
                </div>
              </div>

              <div className="border border-onyx/10 rounded-2xl overflow-hidden">
                <table className="w-full text-left border-collapse text-body-sm font-sans">
                  <thead>
                    <tr className="border-b border-onyx/10 bg-soft-meadow text-[11px] font-semibold uppercase text-slate">
                      <th className="py-2.5 px-3">{t('audit.columns.time')}</th>
                      <th className="py-2.5 px-3">{t('audit.columns.agent')}</th>
                      <th className="py-2.5 px-3">{t('audit.columns.status')}</th>
                      <th className="py-2.5 px-3">{t('audit.columns.tokens')}</th>
                      <th className="py-2.5 px-3">{t('audit.columns.summary')}</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-onyx/5 font-mono text-[12px]">
                    {heartbeatRuns.length > 0 ? (
                      heartbeatRuns.map((r) => (
                        <tr key={r.id} className="hover:bg-soft-meadow/30 transition-colors">
                          <td className="py-2.5 px-3 text-slate whitespace-nowrap">
                            {new Date(r.executed_at).toLocaleTimeString()}
                          </td>
                          <td className="py-2.5 px-3 font-semibold text-deep-ink">
                            {r.agent_id}
                          </td>
                          <td className="py-2.5 px-3">
                            <span className={`inline-flex items-center px-2 py-0.5 rounded text-[10px] font-semibold ${
                              r.status === 'ok'
                                ? 'bg-emerald-500/10 text-emerald-700'
                                : r.status === 'action_taken'
                                ? 'bg-hi-yellow/20 text-deep-ink'
                                : 'bg-red-500/10 text-red-700'
                            }`}>
                              {r.status === 'ok' ? t('audit.zeroNoise') : r.status}
                            </span>
                          </td>
                          <td className="py-2.5 px-3 text-slate">
                            {r.tokens_used.toLocaleString()}
                          </td>
                          <td className="py-2.5 px-3 text-slate truncate max-w-md font-sans">
                            {r.summary}
                          </td>
                        </tr>
                      ))
                    ) : (
                      <tr>
                        <td colSpan={5} className="py-8 text-center text-caption text-slate font-sans">
                          {t('audit.empty')}
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            </Card>
          </div>
        )}

        {activeTab === 'governance' && (
          <div className="grid grid-cols-1 xl:grid-cols-2 gap-5">
            <Card className="p-6 border border-onyx/10 bg-canvas space-y-4">
              <h3 className="font-serif text-heading-sm text-deep-ink">{t('governance.approvalsTitle')}</h3>
              <p className="text-caption text-slate">{t('governance.approvalsDescription')}</p>
              {approvals.length === 0 ? (
                <p className="p-6 text-center text-caption text-slate">{t('governance.noApprovals')}</p>
              ) : approvals.map((approval) => (
                <div key={approval.id} className="p-4 rounded-[24px] border border-onyx/10 bg-soft-meadow space-y-3">
                  <div className="flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <p className="font-mono text-body-sm font-semibold text-deep-ink truncate">{approval.tool_name}</p>
                      <p className="text-caption text-slate">{approval.agent_id} · {approval.risk_level}</p>
                    </div>
                    <Badge variant="stopped">{t('governance.pending')}</Badge>
                  </div>
                  <pre className="max-h-32 overflow-auto rounded-2xl bg-canvas p-3 text-[11px] text-slate">
                    {JSON.stringify(approval.input, null, 2)}
                  </pre>
                  <div className="flex justify-end gap-2">
                    <Button variant="ghost" size="sm" onClick={() => handleApproval(approval, false)}>
                      {t('governance.reject')}
                    </Button>
                    <Button variant="primary" size="sm" onClick={() => handleApproval(approval, true)}>
                      {t('governance.approve')}
                    </Button>
                  </div>
                </div>
              ))}
            </Card>
            <Card className="p-6 border border-onyx/10 bg-canvas space-y-4">
              <h3 className="font-serif text-heading-sm text-deep-ink">{t('governance.runsTitle')}</h3>
              <p className="text-caption text-slate">{t('governance.runsDescription')}</p>
              {agentRuns.map((run) => (
                <div key={run.id} className="p-3 rounded-2xl border border-onyx/10 bg-soft-meadow">
                  <div className="flex justify-between gap-3">
                    <span className="font-mono text-[11px] text-deep-ink truncate">{run.id}</span>
                    <Badge variant={run.status === 'completed' ? 'active' : run.status === 'failed' ? 'stopped' : 'neutral'}>
                      {run.status}
                    </Badge>
                  </div>
                  <p className="mt-1 text-caption text-slate line-clamp-2">{run.goal}</p>
                  <p className="mt-1 font-mono text-[10px] text-slate">
                    {t('governance.runMetrics', {
                      iterations: run.iterations,
                      tokens: run.total_tokens,
                      reason: run.termination_reason || '-',
                    })}
                  </p>
                </div>
              ))}
            </Card>
          </div>
        )}

        {/* Task Modal */}
        <TaskModal
          isOpen={isTaskModalOpen}
          onClose={() => {
            setIsTaskModalOpen(false);
            setEditingTask(null);
          }}
          onSave={handleCreateOrUpdateTask}
          task={editingTask}
        />

        {/* Delete Confirmation Modal */}
        <ConfirmModal
          isOpen={deletingTaskId !== null}
          onClose={() => setDeletingTaskId(null)}
          onConfirm={handleDeleteTask}
          title={t('delete.title')}
          description={t('delete.description')}
          confirmLabel={t('delete.confirm')}
          variant="danger"
        />
      </PageContainer>
    </div>
  );
}
