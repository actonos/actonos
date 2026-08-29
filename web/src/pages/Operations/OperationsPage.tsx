import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Activity, Bot, ChevronDown, ChevronRight, CirclePause,
  CirclePlay, Coins, Cpu, HardDrive, MemoryStick,
  RefreshCw, RotateCcw, Thermometer, XCircle,
} from 'lucide-react';
import { PageContainer } from '@/components/layout/PageContainer';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { useToast } from '@/components/ui/Toast';
import { api } from '@/lib/api';
import type { AgentRun, AutonomousTask, HealthReport, RunEvent } from '@/lib/types';
import { isRunNotFoundError, mergeVisibleRuns } from './operations-runs';
import { SUPERVISOR_COMPONENTS, componentStatus, supervisorTone } from '@/lib/health';
import { useRealtime } from '@/components/providers/RealtimeProvider';
import { PageHeader } from '@/components/ui/PageHeader';
import { IconButton } from '@/components/ui/IconButton';
import { SegmentedControl } from '@/components/ui/SegmentedControl';
import { readHashParams, setHashParam } from '@/lib/url-state';
import { ProactiveAnomaliesCard } from './components/ProactiveAnomaliesCard';

function percent(value: number) {
  return `${Math.max(0, Math.min(100, value)).toFixed(0)}%`;
}

export function OperationsPage() {
  const { t } = useTranslation('operations');
  const { error, success } = useToast();
  const { connection, snapshot } = useRealtime();
  const metrics = snapshot?.metrics || null;
  const runs = snapshot?.runs || [];
  const tokens = snapshot?.tokens || null;
  const [tasks, setTasks] = useState<AutonomousTask[]>([]);
  const [events, setEvents] = useState<RunEvent[]>([]);
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [selectedRun, setSelectedRun] = useState<string>('');
  const [fetchedRun, setFetchedRun] = useState<AgentRun | null>(null);
  const [missingRun, setMissingRun] = useState(false);
  const [hashTick, setHashTick] = useState(0);
  const [view, setView] = useState<'overview' | 'feed' | 'runtime' | 'cost'>(() => {
    const value = readHashParams().get('view');
    return value === 'feed' || value === 'runtime' || value === 'cost' ? value : 'overview';
  });
  const [health, setHealth] = useState<HealthReport | null>(null);
  const [cancellingRun, setCancellingRun] = useState<string | null>(null);
  const refreshTasks = useCallback(async () => {
    const result = await api.listTasks().catch(() => ({ tasks: [], count: 0 }));
    setTasks(result.tasks);
  }, []);

  useEffect(() => {
    let cancelled = false;
    const loadHealth = async () => {
      const report = await api.getHealth().catch(() => null);
      if (!cancelled) setHealth(report);
    };
    void refreshTasks();
    void loadHealth();
    const interval = window.setInterval(loadHealth, 15000);
    return () => {
      cancelled = true;
      window.clearInterval(interval);
    };
  }, [refreshTasks]);

  useEffect(() => {
    const sync = () => {
      const params = readHashParams();
      const viewParam = params.get('view');
      if (viewParam === 'feed' || viewParam === 'runtime' || viewParam === 'cost') {
        setView(viewParam);
      }
      setHashTick((n) => n + 1);
    };
    window.addEventListener('hashchange', sync);
    return () => window.removeEventListener('hashchange', sync);
  }, []);

  useEffect(() => {
    const runId = readHashParams().get('run');
    if (runId) {
      setView('feed');
      if (runs.some((run) => run.id === runId)) {
        setSelectedRun(runId);
        setMissingRun(false);
        setFetchedRun((current) => (current?.id === runId ? current : null));
        return;
      }
      let cancelled = false;
      api.getAgentRun(runId).then((res) => {
        if (cancelled) return;
        setFetchedRun(res.run);
        setSelectedRun(res.run.id);
        setMissingRun(false);
      }).catch((cause) => {
        if (cancelled) return;
        setFetchedRun(null);
        setSelectedRun(runId);
        setMissingRun(isRunNotFoundError(cause));
      });
      return () => { cancelled = true; };
    }
    setMissingRun(false);
    setFetchedRun(null);
    setSelectedRun((current) => current || runs[0]?.id || '');
  }, [runs, hashTick]);

  useEffect(() => {
    if (!selectedRun) {
      setEvents([]);
      return;
    }
    let cancelled = false;
    const load = async () => {
      const result = await api.listRunEvents(selectedRun).catch(() => ({ events: [] }));
      if (!cancelled) setEvents(result.events);
    };
    load();
    const interval = window.setInterval(load, 2000);
    return () => {
      cancelled = true;
      window.clearInterval(interval);
    };
  }, [selectedRun]);

  const handleCancelRun = async (runID: string) => {
    try {
      setCancellingRun(runID);
      await api.cancelAgentRun(runID);
      success(t('process.cancelled'), runID);
    } catch (cause) {
      error(t('process.cancelFailed'), cause instanceof Error ? cause.message : String(cause));
    } finally {
      setCancellingRun(null);
    }
  };

  const handleTaskStatus = async (task: AutonomousTask, status: AutonomousTask['status']) => {
    try {
      await api.updateTask(task.id, { status });
      await refreshTasks();
      success(t('queue.updated'), task.title);
    } catch (cause) {
      error(t('queue.updateFailed'), cause instanceof Error ? cause.message : String(cause));
    }
  };

  const visibleRuns = useMemo(() => mergeVisibleRuns(runs, fetchedRun), [runs, fetchedRun]);
  const selected = useMemo(
    () => visibleRuns.find((run) => run.id === selectedRun),
    [visibleRuns, selectedRun],
  );
  const ramPercent = metrics ? (metrics.memory.used_mb / Math.max(metrics.memory.total_mb, 1)) * 100 : 0;
  const diskPercent = metrics ? (metrics.disk.used_gb / Math.max(metrics.disk.total_gb, 1)) * 100 : 0;

  const gaugeCards = [
    { key: 'cpu', icon: Cpu, value: percent(metrics?.cpu.usage_percent || 0), detail: metrics?.cpu.model || '—', width: metrics?.cpu.usage_percent || 0 },
    { key: 'ram', icon: MemoryStick, value: percent(ramPercent), detail: `${metrics?.memory.used_mb || 0} / ${metrics?.memory.total_mb || 0} MB`, width: ramPercent },
    { key: 'temperature', icon: Thermometer, value: `${(metrics?.cpu.temperature_celsius || 0).toFixed(1)}°C`, detail: `${metrics?.cpu.cores || 0} ${t('hardware.cores')}`, width: Math.min(metrics?.cpu.temperature_celsius || 0, 100) },
    { key: 'disk', icon: HardDrive, value: percent(diskPercent), detail: `${metrics?.disk.used_gb.toFixed(1) || 0} / ${metrics?.disk.total_gb.toFixed(1) || 0} GB`, width: diskPercent },
  ];

  return (
    <PageContainer maxWidth="wide">
      <PageHeader
        eyebrow={t('eyebrow')}
        title={t('title')}
        description={t('subtitle')}
        actions={(
          <Badge variant={connection === 'online' ? 'success' : 'stopped'}>
            <Activity className="w-3 h-3 mr-1" /> {t(`connection.${connection}`)}
          </Badge>
        )}
      />

      <SegmentedControl
        value={view}
        onChange={(next) => {
          setView(next);
          setHashParam('view', next === 'overview' ? undefined : next);
        }}
        label={t('views.label')}
        options={[
          { value: 'overview', label: t('views.overview') },
          { value: 'feed', label: t('views.feed') },
          { value: 'runtime', label: t('views.runtime') },
          { value: 'cost', label: t('views.cost') },
        ]}
        className="mb-6"
      />

      {health && (
        <div className="mb-6 flex flex-wrap items-center gap-2 rounded-[24px] border border-onyx/10 bg-soft-meadow p-3">
          <Badge variant={supervisorTone(health.status) === 'success' ? 'success' : supervisorTone(health.status) === 'warning' ? 'warning' : 'danger'}>
            {t(`health.status.${health.status}`, health.status)}
          </Badge>
          {SUPERVISOR_COMPONENTS.map((key) => {
            const status = componentStatus(health, key);
            const tone = supervisorTone(status);
            return (
              <Badge key={key} variant={tone === 'success' ? 'success' : tone === 'warning' ? 'warning' : tone === 'danger' ? 'danger' : 'neutral'}>
                {t(`health.components.${key}`)}: {t(`health.status.${status}`, status)}
              </Badge>
            );
          })}
        </div>
      )}

      <div className={`${view === 'overview' ? 'grid' : 'hidden'} grid-cols-2 gap-3 xl:grid-cols-4 mb-6`}>
        {gaugeCards.map((gauge) => {
          const Icon = gauge.icon;
          return (
            <Card key={gauge.key} className="p-4 border border-onyx/10">
              <div className="flex items-center justify-between">
                <span className="text-caption uppercase font-semibold text-slate">{t(`hardware.${gauge.key}`)}</span>
                <Icon className="w-4 h-4" />
              </div>
              <div className="font-serif text-heading font-bold mt-2">{gauge.value}</div>
              <div className="h-2 rounded-full bg-deep-ink/10 my-2 overflow-hidden">
                <div className="h-full rounded-full bg-deep-ink transition-all" style={{ width: percent(gauge.width) }} />
              </div>
              <p className="text-caption text-slate truncate">{gauge.detail}</p>
            </Card>
          );
        })}
      </div>

      {/* Proactive Health & Anomalies Component */}
      <div className={`${view === 'overview' ? 'block' : 'hidden'} mb-6`}>
        <ProactiveAnomaliesCard onMissionCreated={refreshTasks} />
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-12 gap-5">
        <Card className={`${view === 'overview' || view === 'feed' ? 'block' : 'hidden'} xl:col-span-12 p-5 border border-onyx/10`}>
          <div className="flex items-start justify-between gap-3 mb-4">
            <div className="min-w-0 flex-1">
              <h2 className="font-serif text-heading-sm font-bold">{t('feed.title')}</h2>
              <p className="text-caption text-slate line-clamp-2 break-words" title={selected?.goal || undefined}>
                {missingRun ? t('process.missingRun') : (selected?.goal || t('feed.empty'))}
              </p>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <select
                value={selectedRun}
                onChange={(event) => {
                  const id = event.target.value;
                  setSelectedRun(id);
                  setHashParam('run', id || undefined);
                }}
                aria-label={t('feed.title')}
                className="rounded-full bg-canvas border border-onyx/10 px-3 py-2 text-caption max-w-[240px]"
              >
                {visibleRuns.map((run) => <option key={run.id} value={run.id}>{run.agent_id} · {run.status}</option>)}
              </select>
              {selected?.status === 'running' && (
                <Button
                  variant="ghost"
                  size="sm"
                  icon={<XCircle className="w-3.5 h-3.5" />}
                  onClick={() => handleCancelRun(selected.id)}
                  disabled={cancellingRun === selected.id}
                >
                  {t('process.cancel')}
                </Button>
              )}
            </div>
          </div>
          <div className="space-y-2 max-h-[420px] overflow-y-auto">
            {events.map((event) => {
              const open = expanded[event.id];
              return (
                <button key={event.id} onClick={() => setExpanded((state) => ({ ...state, [event.id]: !open }))} className="w-full text-left p-3 rounded-[18px] bg-soft-meadow border border-onyx/5">
                  <div className="flex items-center gap-2">
                    {open ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
                    <Badge variant={event.status === 'failed' ? 'stopped' : 'neutral'}>{event.type}</Badge>
                    <span className="font-mono text-caption truncate flex-1">{event.tool_name || event.status}</span>
                    <span className="text-[10px] text-slate">{t('units.milliseconds', { value: event.duration_ms || 0 })}</span>
                  </div>
                  {open && <pre className="mt-3 whitespace-pre-wrap break-words text-[11px] font-mono text-slate">{JSON.stringify(event.data || {}, null, 2)}</pre>}
                </button>
              );
            })}
          </div>
        </Card>

        <Card className={`${view === 'runtime' ? 'block' : 'hidden'} xl:col-span-12 p-5 border border-onyx/10`}>
          <div className="mb-4">
            <h2 className="font-serif text-heading-sm font-bold">{t('process.title')}</h2>
            <p className="text-caption text-slate">{t('process.subtitle')}</p>
          </div>
          <div className="space-y-2">
            {runs.length === 0 ? (
              <p className="p-6 text-center text-caption text-slate">{t('process.empty')}</p>
            ) : runs.map((run) => (
              <div key={run.id} className="flex flex-col gap-2 rounded-[18px] bg-soft-meadow p-3 sm:flex-row sm:items-center">
                <div className="min-w-0 flex-1">
                  <p className="truncate font-mono text-caption font-semibold text-deep-ink">{run.id}</p>
                  <p className="truncate text-caption text-slate">{run.agent_id} · {run.source} · {run.goal}</p>
                </div>
                <Badge variant={run.status === 'running' ? 'info' : run.status === 'failed' || run.status === 'blocked' ? 'danger' : run.status === 'cancelled' ? 'warning' : 'neutral'}>
                  {run.status}
                </Badge>
                {run.status === 'running' && (
                  <Button
                    variant="ghost"
                    size="sm"
                    icon={<XCircle className="w-3.5 h-3.5" />}
                    onClick={() => handleCancelRun(run.id)}
                    disabled={cancellingRun === run.id}
                  >
                    {t('process.cancel')}
                  </Button>
                )}
              </div>
            ))}
          </div>
        </Card>

        <Card className={`${view === 'overview' ? 'block' : 'hidden'} xl:col-span-7 p-5 border border-onyx/10`}>
          <div className="flex items-center justify-between mb-4">
            <div><h2 className="font-serif text-heading-sm font-bold">{t('queue.title')}</h2><p className="text-caption text-slate">{t('queue.subtitle')}</p></div>
            <Button variant="ghost" size="sm" icon={<RefreshCw className="w-3.5 h-3.5" />} onClick={refreshTasks}>{t('refresh')}</Button>
          </div>
          <div className="space-y-2">
            {tasks.slice(0, 8).map((task) => (
              <div key={task.id} className="p-3 rounded-[18px] bg-soft-meadow flex flex-col sm:flex-row sm:items-center gap-3">
                <Bot className="w-4 h-4 shrink-0" />
                <div className="min-w-0 flex-1">
                  <p className="font-semibold truncate">{task.title}</p>
                  <p className="text-[11px] text-slate">{task.status} · {task.progress}%</p>
                  {(task.stalled_cycles ?? 0) >= 3 && (
                    <Badge variant="warning">{t('queue.stalled')}</Badge>
                  )}
                </div>
                <div className="flex gap-1">
                  <IconButton
                    size="sm"
                    label={task.status === 'in_progress' ? t('queue.pause') : t('queue.resume')}
                    icon={task.status === 'in_progress' ? <CirclePause className="w-3.5 h-3.5" /> : <CirclePlay className="w-3.5 h-3.5" />}
                    onClick={() => handleTaskStatus(task, task.status === 'in_progress' ? 'blocked' : 'in_progress')}
                  />
                  <IconButton size="sm" label={t('queue.retry')} icon={<RotateCcw className="w-3.5 h-3.5" />} onClick={() => handleTaskStatus(task, 'pending')} />
                  <IconButton size="sm" tone="danger" label={t('queue.cancel')} icon={<XCircle className="w-3.5 h-3.5" />} onClick={() => handleTaskStatus(task, 'cancelled')} />
                </div>
              </div>
            ))}
          </div>
        </Card>

        <Card className={`${view === 'overview' || view === 'cost' ? 'block' : 'hidden'} xl:col-span-5 p-5 border border-onyx/10`}>
          <div className="flex items-center justify-between gap-2 mb-4">
            <div className="flex items-center gap-2"><Coins className="w-5 h-5" /><h2 className="font-serif text-heading-sm font-bold">{t('cost.title')}</h2></div>
            <button
              type="button"
              className="text-caption font-semibold text-deep-ink underline cursor-pointer"
              onClick={() => { window.location.hash = '/costs'; }}
            >
              {t('cost.openLedger')}
            </button>
          </div>
          <div className="grid grid-cols-2 gap-3 mb-4">
            <div className="p-3 rounded-[18px] bg-soft-meadow"><p className="text-caption text-slate">{t('cost.today')}</p><p className="text-heading-sm font-bold">${(tokens?.today_cost_usd || 0).toFixed(4)}</p><p className="text-[11px] text-slate">{t('units.tokens', { value: (tokens?.today_tokens || 0).toLocaleString() })}</p></div>
            <div className="p-3 rounded-[18px] bg-soft-meadow"><p className="text-caption text-slate">{t('cost.month')}</p><p className="text-heading-sm font-bold">${(tokens?.month_cost_usd || 0).toFixed(4)}</p><p className="text-[11px] text-slate">{t('units.tokens', { value: (tokens?.month_tokens || 0).toLocaleString() })}</p></div>
          </div>
          <div className="space-y-2">{tokens?.by_model.slice(0, 5).map((model) => <div key={model.model} className="flex justify-between text-caption"><span className="truncate">{model.model}</span><span className="font-mono">${model.cost_usd.toFixed(4)}</span></div>)}</div>
        </Card>
      </div>

    </PageContainer>
  );
}
