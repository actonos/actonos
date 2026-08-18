import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import {
  Activity, Bot, Box, CheckCircle2, ChevronDown, ChevronRight, CirclePause,
  CirclePlay, Coins, Cpu, ExternalLink, HardDrive, MemoryStick, Monitor,
  RefreshCw, RotateCcw, SquareTerminal, Thermometer, XCircle,
} from 'lucide-react';
import { PageContainer } from '@/components/layout/PageContainer';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { useToast } from '@/components/ui/Toast';
import { api } from '@/lib/api';
import type { AutonomousTask, RunEvent } from '@/lib/types';
import { useRealtime } from '@/components/providers/RealtimeProvider';
import { PageHeader } from '@/components/ui/PageHeader';
import { EmptyState } from '@/components/ui/EmptyState';
import { IconButton } from '@/components/ui/IconButton';

function percent(value: number) {
  return `${Math.max(0, Math.min(100, value)).toFixed(0)}%`;
}

function eventText(event: RunEvent) {
  const data = event.data ? JSON.stringify(event.data) : '';
  return `[${event.type.toUpperCase()}] ${event.tool_name || event.status}${data ? ` ${data}` : ''}`;
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
  const terminalNode = useRef<HTMLDivElement>(null);
  const terminal = useRef<Terminal | null>(null);

  const refreshTasks = useCallback(async () => {
    const result = await api.listTasks().catch(() => ({ tasks: [], count: 0 }));
    setTasks(result.tasks);
  }, []);

  useEffect(() => {
    refreshTasks();
  }, [refreshTasks]);

  useEffect(() => {
    setSelectedRun((current) => current || runs[0]?.id || '');
  }, [runs]);

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

  useEffect(() => {
    if (!terminalNode.current || terminal.current) return;
    const instance = new Terminal({
      convertEol: true,
      cursorBlink: false,
      disableStdin: true,
      fontFamily: '"Cascadia Code", "SFMono-Regular", Consolas, monospace',
      fontSize: 12,
      theme: { background: '#130e30', foreground: '#f9fbf2', cursor: '#ffe228' },
    });
    const fit = new FitAddon();
    instance.loadAddon(fit);
    instance.open(terminalNode.current);
    fit.fit();
    instance.writeln(t('terminal.ready'));
    terminal.current = instance;
    const resize = () => fit.fit();
    window.addEventListener('resize', resize);
    return () => {
      window.removeEventListener('resize', resize);
      instance.dispose();
      terminal.current = null;
    };
  }, [t]);

  useEffect(() => {
    const instance = terminal.current;
    if (!instance) return;
    instance.clear();
    instance.writeln(`\x1b[33mActonOS observability terminal · ${selectedRun || t('terminal.noRun')}\x1b[0m`);
    events.forEach((event) => instance.writeln(eventText(event)));
  }, [events, selectedRun, t]);

  const handleTaskStatus = async (task: AutonomousTask, status: AutonomousTask['status']) => {
    try {
      await api.updateTask(task.id, { status });
      await refreshTasks();
      success(t('queue.updated'), task.title);
    } catch (cause) {
      error(t('queue.updateFailed'), cause instanceof Error ? cause.message : String(cause));
    }
  };

  const selected = useMemo(() => runs.find((run) => run.id === selectedRun), [runs, selectedRun]);
  const ramPercent = metrics ? (metrics.memory.used_mb / Math.max(metrics.memory.total_mb, 1)) * 100 : 0;
  const diskPercent = metrics ? (metrics.disk.used_gb / Math.max(metrics.disk.total_gb, 1)) * 100 : 0;

  const gaugeCards = [
    { key: 'cpu', icon: Cpu, value: percent(metrics?.cpu.usage_percent || 0), detail: metrics?.cpu.model || '—', width: metrics?.cpu.usage_percent || 0 },
    { key: 'ram', icon: MemoryStick, value: percent(ramPercent), detail: `${metrics?.memory.used_mb || 0} / ${metrics?.memory.total_mb || 0} MB`, width: ramPercent },
    { key: 'temperature', icon: Thermometer, value: `${(metrics?.cpu.temperature_celsius || 0).toFixed(1)}°C`, detail: `${metrics?.cpu.cores || 0} ${t('hardware.cores')}`, width: Math.min(metrics?.cpu.temperature_celsius || 0, 100) },
    { key: 'disk', icon: HardDrive, value: percent(diskPercent), detail: `${metrics?.disk.used_gb.toFixed(1) || 0} / ${metrics?.disk.total_gb.toFixed(1) || 0} GB`, width: diskPercent },
  ];

  return (
    <PageContainer>
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

      <div className="grid grid-cols-2 xl:grid-cols-4 gap-3 mb-6">
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

      <div className="grid grid-cols-1 xl:grid-cols-12 gap-5">
        <Card className="xl:col-span-7 p-5 border border-onyx/10">
          <div className="flex items-center justify-between mb-4">
            <div>
              <h2 className="font-serif text-heading-sm font-bold">{t('feed.title')}</h2>
              <p className="text-caption text-slate">{selected?.goal || t('feed.empty')}</p>
            </div>
            <select value={selectedRun} onChange={(event) => setSelectedRun(event.target.value)} className="rounded-full bg-canvas border border-onyx/10 px-3 py-2 text-caption max-w-[240px]">
              {runs.map((run) => <option key={run.id} value={run.id}>{run.agent_id} · {run.status}</option>)}
            </select>
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

        <Card className="xl:col-span-5 p-5 border border-onyx/10">
          <div className="flex items-center justify-between mb-4">
            <div>
              <h2 className="font-serif text-heading-sm font-bold">{t('containers.title')}</h2>
              <p className="text-caption text-slate">{t('containers.subtitle')}</p>
            </div>
            <Box className="w-5 h-5" />
          </div>
          <div className="space-y-2 max-h-[420px] overflow-y-auto">
            {(metrics?.containers || []).length === 0 ? <EmptyState compact icon={<Box className="h-6 w-6" />} title={t('containers.empty')} /> :
              metrics?.containers.map((container) => (
                <div key={container.id} className="p-3 rounded-[18px] bg-soft-meadow flex items-center gap-3">
                  {container.state === 'running' ? <CheckCircle2 className="w-5 h-5" /> : <XCircle className="w-5 h-5 text-slate" />}
                  <div className="min-w-0 flex-1">
                    <p className="font-semibold truncate">{container.name}</p>
                    <p className="text-[11px] text-slate truncate">{container.image}</p>
                  </div>
                  <div className="text-right font-mono text-[10px] text-slate">
                    <div>{t('units.cpuPercent', { value: container.cpu_percent.toFixed(1) })}</div>
                    <div>{t('units.megabytes', { value: container.memory_usage_mb.toFixed(0) })}</div>
                  </div>
                </div>
              ))}
          </div>
        </Card>

        <Card className="xl:col-span-7 p-5 border border-onyx/10">
          <div className="flex items-center justify-between mb-3">
            <div>
              <h2 className="font-serif text-heading-sm font-bold">{t('canvas.title')}</h2>
              <p className="text-caption text-slate">{t('canvas.subtitle')}</p>
            </div>
            {metrics?.canvas_url && <a href={metrics.canvas_url} target="_blank" rel="noreferrer" className="rounded-full p-2 bg-soft-meadow"><ExternalLink className="w-4 h-4" /></a>}
          </div>
          <div className="aspect-video rounded-[20px] overflow-hidden bg-deep-ink flex items-center justify-center">
            {metrics?.canvas_url ? <iframe title={t('canvas.frameTitle')} src={metrics.canvas_url} className="w-full h-full border-0" sandbox="allow-scripts allow-same-origin" referrerPolicy="no-referrer" /> :
              <div className="text-center text-canvas/70 p-8"><Monitor className="w-10 h-10 mx-auto mb-3" /><p>{t('canvas.waiting')}</p></div>}
          </div>
        </Card>

        <Card className="xl:col-span-5 p-0 border border-onyx/10 overflow-hidden">
          <div className="p-4 flex items-center gap-2 bg-soft-meadow"><SquareTerminal className="w-4 h-4" /><h2 className="font-serif text-heading-sm font-bold">{t('terminal.title')}</h2></div>
          <div ref={terminalNode} className="h-[330px] bg-deep-ink p-2" />
        </Card>

        <Card className="xl:col-span-7 p-5 border border-onyx/10">
          <div className="flex items-center justify-between mb-4">
            <div><h2 className="font-serif text-heading-sm font-bold">{t('queue.title')}</h2><p className="text-caption text-slate">{t('queue.subtitle')}</p></div>
            <Button variant="ghost" size="sm" icon={<RefreshCw className="w-3.5 h-3.5" />} onClick={refreshTasks}>{t('refresh')}</Button>
          </div>
          <div className="space-y-2">
            {tasks.slice(0, 8).map((task) => (
              <div key={task.id} className="p-3 rounded-[18px] bg-soft-meadow flex flex-col sm:flex-row sm:items-center gap-3">
                <Bot className="w-4 h-4 shrink-0" />
                <div className="min-w-0 flex-1"><p className="font-semibold truncate">{task.title}</p><p className="text-[11px] text-slate">{task.status} · {task.progress}%</p></div>
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

        <Card className="xl:col-span-5 p-5 border border-onyx/10">
          <div className="flex items-center gap-2 mb-4"><Coins className="w-5 h-5" /><h2 className="font-serif text-heading-sm font-bold">{t('cost.title')}</h2></div>
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
