import { useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Modal } from '@/components/ui/Modal';
import { useToast } from '@/components/ui/Toast';
import type { AgentRun, AutonomousTask, HeartbeatRun, RunEvent } from '@/lib/types';
import {
  Clock,
  Activity,
  Layers,
  Download,
  Copy,
  Check,
} from 'lucide-react';

export interface MissionTimelineViewProps {
  tasks: AutonomousTask[];
  heartbeatRuns: HeartbeatRun[];
  agentRuns: AgentRun[];
  selectedTask?: AutonomousTask | null;
  onSelectTask?: (task: AutonomousTask) => void;
}

export function MissionTimelineView({
  tasks,
  heartbeatRuns,
  agentRuns,
  selectedTask: propSelectedTask,
}: MissionTimelineViewProps) {
  const { t } = useTranslation('missions');
  const { success } = useToast();

  const [activeTaskFilter, setActiveTaskFilter] = useState<string>(
    propSelectedTask?.id || 'all'
  );
  const [selectedEvent, setSelectedEvent] = useState<RunEvent | null>(null);
  const [copiedTrace, setCopiedTrace] = useState(false);

  const currentTask = useMemo(() => {
    if (activeTaskFilter === 'all') return null;
    return tasks.find((t) => t.id === activeTaskFilter) || null;
  }, [tasks, activeTaskFilter]);

  // Merge execution timeline from heartbeat runs and agent runs
  const timelineEvents = useMemo(() => {
    type TimelineItem = {
      id: string;
      timestamp: string;
      title: string;
      category: 'heartbeat' | 'run' | 'task';
      status: string;
      tokens?: number;
      duration_ms?: number;
      agent_id?: string;
      details?: string;
      rawObject?: any;
    };

    const items: TimelineItem[] = [];

    // Heartbeat pulses
    for (const hr of heartbeatRuns) {
      if (activeTaskFilter !== 'all' && hr.agent_id !== currentTask?.assigned_agent_id) {
        continue;
      }
      items.push({
        id: `hb_${hr.id || hr.executed_at}`,
        timestamp: hr.executed_at,
        title: hr.summary || 'Heartbeat Nominal Pulse',
        category: 'heartbeat',
        status: hr.status === 'ok' ? 'Success' : hr.status === 'action_taken' ? 'Action Taken' : hr.status === 'skipped' ? 'Skipped' : 'Failed',
        tokens: hr.tokens_used || 0,
        duration_ms: 0,
        agent_id: hr.agent_id || 'agent_system_core',
        details: hr.summary,
        rawObject: hr,
      });
    }

    // Agent Runs
    for (const r of agentRuns) {
      if (activeTaskFilter !== 'all' && r.goal !== currentTask?.title && !r.goal?.includes(activeTaskFilter)) {
        continue;
      }
      items.push({
        id: `run_${r.id}`,
        timestamp: r.started_at,
        title: r.goal || 'Autonomous Goal Execution',
        category: 'run',
        status: r.status,
        tokens: r.total_tokens,
        agent_id: r.agent_name || r.agent_id,
        details: r.termination_reason || `Iterations: ${r.iterations}`,
        rawObject: r,
      });
    }

    // Sort newest first
    return items.sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime());
  }, [heartbeatRuns, agentRuns, activeTaskFilter, currentTask]);

  const handleExportSummary = () => {
    const lines = [
      `# ActonOS Mission Execution Report`,
      `Generated at: ${new Date().toISOString()}`,
      `Filter: ${currentTask ? currentTask.title : 'All Missions'}`,
      '',
      `## Overview`,
      `- Total Missions: ${tasks.length}`,
      `- Running: ${tasks.filter((t) => t.status === 'in_progress').length}`,
      `- Completed: ${tasks.filter((t) => t.status === 'completed').length}`,
      `- Timeline Events Recorded: ${timelineEvents.length}`,
      '',
      `## Timeline Traces`,
      '| Timestamp | Type | Status | Agent | Tokens | Details |',
      '|:---|:---|:---|:---|:---|:---|',
    ];

    for (const item of timelineEvents.slice(0, 50)) {
      lines.push(
        `| ${new Date(item.timestamp).toLocaleTimeString()} | ${item.category} | ${item.status} | ${item.agent_id || '-'} | ${item.tokens || 0} | ${item.title.replace(/\|/g, '-')} |`
      );
    }

    const blob = new Blob([lines.join('\n')], { type: 'text/markdown' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `mission-report-${new Date().toISOString().slice(0, 10)}.md`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    success('Report Exported', 'Mission timeline report downloaded as Markdown.');
  };

  const handleCopyTraceJSON = (data: any) => {
    navigator.clipboard.writeText(JSON.stringify(data, null, 2));
    setCopiedTrace(true);
    setTimeout(() => setCopiedTrace(false), 2000);
  };

  return (
    <div className="space-y-6">
      {/* Header Toolbar */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-canvas/80 backdrop-blur-md p-4 rounded-2xl border border-onyx/10">
        <div className="flex flex-wrap items-center gap-3">
          <span className="text-caption font-medium text-slate">{t('timeline.filterByMission', 'Focus Mission')}:</span>
          <select
            value={activeTaskFilter}
            onChange={(e) => setActiveTaskFilter(e.target.value)}
            className="text-body-sm font-medium bg-soft-meadow border border-onyx/15 rounded-xl px-3 py-1.5 text-deep-ink focus:outline-none focus:ring-1 focus:ring-deep-ink cursor-pointer"
          >
            <option value="all">{t('timeline.allMissions', 'All Missions & Pulses')}</option>
            {tasks.map((task) => (
              <option key={task.id} value={task.id}>
                {task.title} ({task.status})
              </option>
            ))}
          </select>
        </div>

        <Button
          variant="ghost"
          size="sm"
          onClick={handleExportSummary}
          icon={<Download className="w-3.5 h-3.5" />}
        >
          {t('timeline.exportReport', 'Export Markdown Report')}
        </Button>
      </div>

      {/* DAG Plan Gantt Chart Overlay (If task has plan steps) */}
      {currentTask?.plan && currentTask.plan.steps && (
        <Card className="p-5 border border-onyx/10 bg-canvas/90 space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Layers className="w-4 h-4 text-deep-ink" />
              <h3 className="font-serif text-body font-bold text-deep-ink">
                {t('timeline.dagProgressTitle', 'DAG Step Graph & Execution Flow')}
              </h3>
            </div>
            <Badge variant="neutral" className="text-[11px]">
              {currentTask.plan.steps.filter((s) => s.status === 'completed').length} / {currentTask.plan.steps.length} {t('timeline.stepsCompleted', 'steps completed')}
            </Badge>
          </div>

          {/* Visual Step Progress Bar */}
          <div className="space-y-3 pt-2">
            {currentTask.plan.steps.map((step, idx) => {
              const isCompleted = step.status === 'completed';
              const isInProgress = step.status === 'in_progress';
              const isPaused = step.status === 'pending';
              const isFailed = step.status === 'failed';

              return (
                <div
                  key={step.id || idx}
                  className={`p-3 rounded-xl border transition-all ${
                    isCompleted
                      ? 'bg-soft-meadow/40 border-onyx/10'
                      : isInProgress
                      ? 'bg-blue-500/10 border-blue-500/30'
                      : isPaused
                      ? 'bg-amber-500/10 border-amber-500/30'
                      : isFailed
                      ? 'bg-accent-coral/10 border-accent-coral/30'
                      : 'bg-canvas border-onyx/10 opacity-70'
                  }`}
                >
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex items-center gap-2.5 min-w-0">
                      <div
                        className={`w-6 h-6 rounded-full flex items-center justify-center text-[11px] font-bold shrink-0 ${
                          isCompleted
                            ? 'bg-deep-ink text-white'
                            : isInProgress
                            ? 'bg-blue-600 text-white animate-pulse'
                            : isPaused
                            ? 'bg-amber-600 text-white'
                            : isFailed
                            ? 'bg-accent-coral text-white'
                            : 'bg-onyx/10 text-slate'
                        }`}
                      >
                        {idx + 1}
                      </div>
                      <div className="min-w-0">
                        <span className="text-body-sm font-semibold text-deep-ink truncate block">
                          {step.title || step.description}
                        </span>
                        {step.dependencies && step.dependencies.length > 0 && (
                          <span className="text-[10px] text-slate font-mono">
                            Depends on: {step.dependencies.join(', ')}
                          </span>
                        )}
                      </div>
                    </div>

                    <div className="flex items-center gap-2 shrink-0">
                      <Badge
                        variant={
                          isCompleted
                            ? 'neutral'
                            : isInProgress
                            ? 'accent'
                            : isPaused
                            ? 'stopped'
                            : isFailed
                            ? 'accent'
                            : 'neutral'
                        }
                        className="text-[10px]"
                      >
                        {step.status}
                      </Badge>
                    </div>
                  </div>

                  {step.result && (
                    <div className="mt-2 text-caption text-slate font-mono bg-canvas p-2 rounded-lg border border-onyx/10 line-clamp-2">
                      {step.result}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </Card>
      )}

      {/* Vertical Heartbeat & Execution Timeline */}
      <Card className="p-5 border border-onyx/10 bg-canvas/90 space-y-4">
        <div className="flex items-center justify-between border-b border-onyx/10 pb-3">
          <div className="flex items-center gap-2">
            <Clock className="w-4 h-4 text-deep-ink" />
            <h3 className="font-serif text-body font-bold text-deep-ink">
              {t('timeline.pulseHistory', 'Heartbeat Pulses & Tool Execution Traces')}
            </h3>
          </div>
          <span className="text-caption text-slate font-mono">
            {timelineEvents.length} events
          </span>
        </div>

        {timelineEvents.length === 0 ? (
          <div className="py-12 text-center text-slate space-y-2">
            <Activity className="w-8 h-8 mx-auto text-slate/50" />
            <p className="text-body-sm font-medium">{t('timeline.noEvents', 'No execution events recorded for this selection.')}</p>
          </div>
        ) : (
          <div className="relative pl-6 space-y-6 before:content-[''] before:absolute before:top-3 before:bottom-3 before:left-2.5 before:w-0.5 before:bg-onyx/15">
            {timelineEvents.map((event) => {
              const isSuccess = event.status === 'Success' || event.status === 'completed' || event.status === 'ok';
              const isFailed = event.status === 'Failed' || event.status === 'failed';
              const isPaused = event.status === 'paused' || event.status === 'approval_pending';

              return (
                <div key={event.id} className="relative group">
                  {/* Timeline Dot Indicator */}
                  <div
                    className={`absolute -left-6 top-1.5 w-3.5 h-3.5 rounded-full border-2 border-canvas shadow-xs transition-transform group-hover:scale-125 ${
                      isSuccess
                        ? 'bg-deep-ink'
                        : isFailed
                        ? 'bg-accent-coral'
                        : isPaused
                        ? 'bg-amber-500 animate-ping'
                        : 'bg-blue-500 animate-pulse'
                    }`}
                  />

                  <div
                    onClick={() => setSelectedEvent(event.rawObject)}
                    className="p-3.5 rounded-xl border border-onyx/10 bg-canvas hover:bg-soft-meadow/50 transition-all cursor-pointer shadow-2xs space-y-2"
                  >
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-1">
                      <div className="flex items-center gap-2 min-w-0">
                        <Badge variant="neutral" className="text-[10px] uppercase tracking-wider shrink-0 font-mono">
                          {event.category}
                        </Badge>
                        <span className="text-body-sm font-semibold text-deep-ink truncate">
                          {event.title}
                        </span>
                      </div>

                      <div className="flex items-center gap-2 text-caption text-slate font-mono shrink-0">
                        <span>{new Date(event.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}</span>
                        <Badge
                          variant={isSuccess ? 'neutral' : isFailed ? 'accent' : 'stopped'}
                          className="text-[10px]"
                        >
                          {event.status}
                        </Badge>
                      </div>
                    </div>

                    {event.details && (
                      <p className="text-caption text-slate line-clamp-2 font-mono">
                        {event.details}
                      </p>
                    )}

                    <div className="flex flex-wrap items-center gap-4 text-[11px] text-slate border-t border-onyx/5 pt-2 font-mono">
                      {event.agent_id && (
                        <span>Agent: <strong className="text-deep-ink">{event.agent_id}</strong></span>
                      )}
                      {event.tokens !== undefined && event.tokens > 0 && (
                        <span>Tokens: <strong className="text-deep-ink">{event.tokens.toLocaleString()}</strong></span>
                      )}
                      {event.duration_ms !== undefined && event.duration_ms > 0 && (
                        <span>Latency: <strong className="text-deep-ink">{event.duration_ms} ms</strong></span>
                      )}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </Card>

      {/* Raw Trace JSON Modal */}
      <Modal
        isOpen={Boolean(selectedEvent)}
        onClose={() => setSelectedEvent(null)}
        title={t('timeline.traceDetail', 'Execution Event Trace Inspector')}
        maxWidth="max-w-2xl"
      >
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <span className="text-caption text-slate font-mono">Structured JSON Payload</span>
            <Button
              variant="ghost"
              size="sm"
              icon={copiedTrace ? <Check className="w-3.5 h-3.5 text-green-600" /> : <Copy className="w-3.5 h-3.5" />}
              onClick={() => handleCopyTraceJSON(selectedEvent)}
            >
              {copiedTrace ? 'Copied' : 'Copy JSON'}
            </Button>
          </div>

          <pre className="p-4 rounded-xl bg-onyx/5 border border-onyx/10 text-[12px] font-mono text-deep-ink overflow-x-auto max-h-[60vh]">
            {JSON.stringify(selectedEvent, null, 2)}
          </pre>
        </div>
      </Modal>
    </div>
  );
}
