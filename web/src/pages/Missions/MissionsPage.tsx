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
  MessageSquare,
  Eye,
} from 'lucide-react';
import { Modal } from '@/components/ui/Modal';
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

  const [activeTab, setActiveTab] = useState<'tasks' | 'audit' | 'governance'>(() => {
    const value = readHashParams().get('view');
    return value === 'audit' || value === 'governance' ? value : 'tasks';
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

  // Modal states
  const [isTaskModalOpen, setIsTaskModalOpen] = useState(false);
  const [editingTask, setEditingTask] = useState<AutonomousTask | null>(null);
  const [deletingTaskId, setDeletingTaskId] = useState<string | null>(null);
  const [selectedRunDetail, setSelectedRunDetail] = useState<AgentRun | null>(null);

  const changeTab = (tab: 'tasks' | 'audit' | 'governance') => {
    setActiveTab(tab);
    setHashParam('view', tab === 'tasks' ? undefined : tab);
  };

  const loadData = async (isBackground = false) => {
    try {
      if (!isBackground) {
        setLoading(true);
      }
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
      if (!isBackground) {
        error('Failed to load mission data', getErrorMessage(err));
      }
    } finally {
      if (!isBackground) {
        setLoading(false);
      }
    }
  };

  useEffect(() => {
    loadData(false);
    const interval = setInterval(() => loadData(true), 10000);
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
      await api.triggerHeartbeatPulse();
      success(
        t('pulse.triggeredSuccess', 'Pulse Initiated'),
        t('pulse.triggeredDescription', 'Autonomous pulse cycle launched in background.')
      );
      loadData();
    } catch (err) {
      error('Pulse execution failed', getErrorMessage(err));
    } finally {
      setTriggeringPulse(false);
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

      <PageContainer maxWidth="wide">
        <PageHeader
          eyebrow={t('page.eyebrow')}
          title={t('title')}
          description={t('subtitle')}
          actions={(
            <>
              <Button
                variant="ghost"
                size="sm"
                icon={<RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />}
                onClick={() => loadData(false)}
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
                            {t('actions.detail', 'Detail')}
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
                          className={`h-full rounded-full transition-all duration-500 ${tItem.status === 'completed'
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

        {/* TAB 2: Pulse Audit Ledger */}
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
                            <span className={`inline-flex items-center px-2 py-0.5 rounded text-[10px] font-semibold ${r.status === 'ok'
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
                <div key={run.id} className="p-3 rounded-2xl border border-onyx/10 bg-soft-meadow space-y-2">
                  <div className="flex items-center justify-between gap-3">
                    <span className="font-mono text-[11px] text-deep-ink font-semibold truncate">{run.id}</span>
                    <div className="flex items-center gap-2 shrink-0">
                      <Badge variant={run.status === 'completed' ? 'active' : run.status === 'failed' ? 'stopped' : 'neutral'}>
                        {run.status}
                      </Badge>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setSelectedRunDetail(run)}
                        className="text-[11px] px-2 py-0.5 h-6.5"
                        icon={<Eye className="w-3.5 h-3.5" />}
                      >
                        {t('governance.viewDetail', 'Detail')}
                      </Button>
                    </div>
                  </div>
                  <p className="text-caption text-slate line-clamp-2">{run.goal}</p>
                  <p className="font-mono text-[10px] text-slate/80">
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

        {/* Run Detail Modal */}
        <Modal
          isOpen={selectedRunDetail !== null}
          onClose={() => setSelectedRunDetail(null)}
          title={t('governance.runDetailTitle', 'Durable Agent Run Details')}
          maxWidth="max-w-2xl"
        >
          {selectedRunDetail && (
            <div className="space-y-4 font-sans text-body-sm text-deep-ink">
              {/* Header stats bar */}
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-2.5 p-3 rounded-2xl bg-soft-meadow border border-onyx/10">
                <div>
                  <span className="text-[10px] uppercase font-mono text-slate block">{t('governance.status')}</span>
                  <Badge variant={selectedRunDetail.status === 'completed' ? 'active' : selectedRunDetail.status === 'failed' ? 'stopped' : 'neutral'}>
                    {selectedRunDetail.status}
                  </Badge>
                </div>
                <div>
                  <span className="text-[10px] uppercase font-mono text-slate block">{t('governance.iterations')}</span>
                  <span className="font-mono font-bold text-deep-ink">{selectedRunDetail.iterations}</span>
                </div>
                <div>
                  <span className="text-[10px] uppercase font-mono text-slate block">{t('governance.tokens')}</span>
                  <span className="font-mono font-bold text-deep-ink">{selectedRunDetail.total_tokens.toLocaleString()}</span>
                </div>
                <div>
                  <span className="text-[10px] uppercase font-mono text-slate block">{t('governance.source')}</span>
                  <span className="font-mono font-bold text-deep-ink uppercase text-[11px]">{selectedRunDetail.source || 'heartbeat'}</span>
                </div>
              </div>

              {/* IDs and Agent */}
              <div className="space-y-2 p-3.5 rounded-2xl bg-canvas border border-onyx/10">
                <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-1 text-caption">
                  <span className="text-slate font-medium">{t('governance.runId')}:</span>
                  <span className="font-mono text-deep-ink select-all">{selectedRunDetail.id}</span>
                </div>
                <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-1 text-caption">
                  <span className="text-slate font-medium">{t('governance.traceId')}:</span>
                  <span className="font-mono text-deep-ink select-all">{selectedRunDetail.trace_id}</span>
                </div>
                <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-1 text-caption">
                  <span className="text-slate font-medium">{t('governance.agentId')}:</span>
                  <span className="font-mono text-deep-ink font-semibold">{selectedRunDetail.agent_id}</span>
                </div>
              </div>

              {/* Goal */}
              <div className="space-y-1.5">
                <label className="text-caption font-semibold text-slate uppercase tracking-wide block">
                  {t('governance.goal')}
                </label>
                <div className="p-3.5 rounded-2xl bg-soft-meadow border border-onyx/10 text-body-sm font-sans whitespace-pre-wrap max-h-48 overflow-y-auto leading-relaxed">
                  {selectedRunDetail.goal}
                </div>
              </div>

              {/* Termination Reason */}
              {selectedRunDetail.termination_reason && (
                <div className="space-y-1.5">
                  <label className="text-caption font-semibold text-slate uppercase tracking-wide block">
                    {t('governance.terminationReason')}
                  </label>
                  <div className="p-3 rounded-2xl bg-hi-yellow/15 border border-onyx/10 text-caption font-mono text-deep-ink">
                    {selectedRunDetail.termination_reason}
                  </div>
                </div>
              )}

              {/* Detailed Token Usage */}
              <div className="space-y-1.5">
                <label className="text-caption font-semibold text-slate uppercase tracking-wide block">
                  {t('governance.tokens')}
                </label>
                <div className="grid grid-cols-3 gap-2 p-3 rounded-2xl bg-soft-meadow border border-onyx/10 text-caption font-mono text-center">
                  <div>
                    <span className="text-slate block text-[10px]">{t('governance.promptTokens')}</span>
                    <strong className="text-deep-ink">{selectedRunDetail.prompt_tokens?.toLocaleString() || 0}</strong>
                  </div>
                  <div>
                    <span className="text-slate block text-[10px]">{t('governance.completionTokens')}</span>
                    <strong className="text-deep-ink">{selectedRunDetail.completion_tokens?.toLocaleString() || 0}</strong>
                  </div>
                  <div>
                    <span className="text-slate block text-[10px]">{t('governance.totalTokens')}</span>
                    <strong className="text-deep-ink">{selectedRunDetail.total_tokens?.toLocaleString() || 0}</strong>
                  </div>
                </div>
              </div>

              {/* Timestamps */}
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-[11px] font-mono text-slate/90 pt-1">
                <div>{t('governance.startedAt')}: {new Date(selectedRunDetail.started_at).toLocaleString()}</div>
                <div>{t('governance.updatedAt')}: {new Date(selectedRunDetail.updated_at).toLocaleString()}</div>
              </div>

              {/* Close Action */}
              <div className="flex justify-end pt-2">
                <Button variant="secondary" size="sm" onClick={() => setSelectedRunDetail(null)}>
                  {t('governance.close')}
                </Button>
              </div>
            </div>
          )}
        </Modal>

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
