import { useState, useEffect } from 'react';
import { getErrorMessage } from '@/lib/errors';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { useToast } from '@/components/ui/Toast';
import {
  Activity,
  Cpu,
  HardDrive,
  Clock,
  Bot,
  Wrench,
  Calendar,
  Shield,
  Layers,
  MessageSquare,
  Plus,
  ArrowUpRight,
  RefreshCw,
  Sparkles,
  Zap,
  Radio,
  Coins,
  HeartPulse,
} from 'lucide-react';
import { api, type DashboardSummaryData } from '@/lib/api';
import type { TokenUsageSummary, HeartbeatRun, HealthReport } from '@/lib/types';
import type { NavTab } from '@/components/layout/Sidebar';
import { PageHeader } from '@/components/ui/PageHeader';
import { QuickStartPanel } from '@/components/features/dashboard/QuickStartPanel';
import { SystemHealthStrip } from '@/components/features/dashboard/SystemHealthStrip';

export interface DashboardPageProps {
  onNavigateTab: (tab: NavTab) => void;
  onOpenChat: (agentID?: string) => void;
  onEditAgent?: (agentID: string) => void;
}

export function DashboardPage({ onNavigateTab, onOpenChat, onEditAgent }: DashboardPageProps) {
  const { t } = useTranslation('dashboard');
  const { error } = useToast();
  const [data, setData] = useState<DashboardSummaryData | null>(null);
  const [tokenStats, setTokenStats] = useState<TokenUsageSummary | null>(null);
  const [heartbeatRuns, setHeartbeatRuns] = useState<HeartbeatRun[]>([]);
  const [health, setHealth] = useState<HealthReport | null>(null);
  const [loading, setLoading] = useState(true);

  // QuickStart Checklist State stored in localStorage
  const [quickstartDismissed, setQuickstartDismissed] = useState<boolean>(() => {
    return localStorage.getItem('actonos_quickstart_dismissed') === 'true';
  });

  const [completedSteps, setCompletedSteps] = useState<Record<string, boolean>>(() => {
    try {
      const saved = localStorage.getItem('actonos_quickstart_steps');
      return saved ? JSON.parse(saved) : {};
    } catch {
      return {};
    }
  });

  const loadSummary = async () => {
    try {
      setLoading(true);
      const [summary, tokens, hb, supervisor] = await Promise.allSettled([
        api.getDashboardSummary(),
        api.getTokenUsage(),
        api.getHeartbeatHistory(),
        api.getHealth(),
      ]);

      if (summary.status === 'fulfilled') {
        setData(summary.value);
        // Auto-detect completed steps based on server state
        setCompletedSteps((prev) => {
          const next = { ...prev };
          if (summary.value.agents_count > 1) next['agent'] = true;
          if (summary.value.tools_count > 4) next['skills'] = true;
          localStorage.setItem('actonos_quickstart_steps', JSON.stringify(next));
          return next;
        });
      }
      if (tokens.status === 'fulfilled') {
        setTokenStats(tokens.value);
      }
      if (hb.status === 'fulfilled') {
        setHeartbeatRuns(hb.value);
      }
      if (supervisor.status === 'fulfilled') {
        setHealth(supervisor.value);
      }
    } catch (err) {
      error('Failed to load dashboard summary', getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadSummary();
    const interval = setInterval(loadSummary, 5000);
    return () => clearInterval(interval);
  }, []);

  const toggleStep = (stepId: string) => {
    setCompletedSteps((prev) => {
      const next = { ...prev, [stepId]: !prev[stepId] };
      localStorage.setItem('actonos_quickstart_steps', JSON.stringify(next));
      return next;
    });
  };

  const handleDismissQuickstart = () => {
    setQuickstartDismissed(true);
    localStorage.setItem('actonos_quickstart_dismissed', 'true');
  };

  const handleRestoreQuickstart = () => {
    setQuickstartDismissed(false);
    localStorage.setItem('actonos_quickstart_dismissed', 'false');
  };

  const cpuPercent = data?.metrics?.cpu?.usage_percent || 0;
  const ramUsed = data?.metrics?.memory?.used_mb || 0;
  const ramTotal = data?.metrics?.memory?.total_mb || 1;
  const ramPercent = Math.min(100, Math.round((ramUsed / ramTotal) * 100));
  const diskUsed = data?.metrics?.disk?.used_gb || 0;
  const diskTotal = data?.metrics?.disk?.total_gb || 1;
  const diskPercent = Math.min(100, Math.round((diskUsed / diskTotal) * 100));
  const uptimeMins = Math.floor((data?.metrics?.uptime_seconds || 0) / 60);

  return (
    <div className="relative min-h-[calc(100vh-64px)]">
      <BlobBackdrop />

      <PageContainer maxWidth="wide">
        <PageHeader
          eyebrow={t('eyebrow')}
          title={t('title')}
          description={t('subtitle')}
          badge={<Badge variant="success" className="font-mono">{t('liveBadge')}</Badge>}
          actions={(
            <>
              {quickstartDismissed && (
                <Button variant="ghost" size="sm" icon={<Sparkles className="h-3.5 w-3.5" />} onClick={handleRestoreQuickstart}>
                  {t('quickstart.show')}
                </Button>
              )}
              <Button variant="ghost" size="sm" icon={<RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />} onClick={loadSummary}>
                {t('actions.refresh')}
              </Button>
              <Button variant="primary" size="sm" icon={<MessageSquare className="h-3.5 w-3.5" />} onClick={() => onOpenChat('agent_system_core')}>
                {t('launchpad.chatCore')}
              </Button>
            </>
          )}
        />

        {!quickstartDismissed && (
          <QuickStartPanel
            completedSteps={completedSteps}
            onToggleStep={toggleStep}
            onDismiss={handleDismissQuickstart}
            onNavigate={onNavigateTab}
          />
        )}

        <SystemHealthStrip data={data} health={health} />

        {/* 4 Live Hardware Gauges */}
        <div className="hidden grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8" aria-hidden="true">
          {/* CPU Gauge */}
          <Card className="p-5 border border-onyx/10 bg-canvas/90 shadow-xs hover:border-onyx/20 transition-all">
            <div className="flex items-center justify-between mb-2">
              <span className="text-caption font-semibold uppercase text-slate">{t('gauges.cpu', 'CPU')}</span>
              <div className="w-8 h-8 rounded-full bg-soft-meadow flex items-center justify-center text-deep-ink">
                <Cpu className="w-4 h-4" />
              </div>
            </div>
            <div className="text-heading font-serif text-deep-ink flex items-baseline gap-2">
              <span>{cpuPercent.toFixed(1)}%</span>
              <span className="text-caption text-slate font-mono font-normal">
                {t('gauges.cores', { count: data?.metrics?.cpu?.cores || 1 })}
              </span>
            </div>
            <div className="w-full bg-onyx/10 h-2 rounded-full mt-3 overflow-hidden">
              <div
                className="bg-deep-ink h-full rounded-full transition-all duration-500"
                style={{ width: `${Math.min(100, cpuPercent)}%` }}
              />
            </div>
            <div className="text-[11px] font-mono text-slate mt-2 flex items-center justify-between">
              <span>{t('gauges.temperature', { value: data?.metrics?.cpu?.temperature_celsius || 42 })}</span>
              <span className="text-emerald-700 font-semibold">{t('gauges.active', 'Active')}</span>
            </div>
          </Card>

          {/* Memory Gauge */}
          <Card className="p-5 border border-onyx/10 bg-canvas/90 shadow-xs hover:border-onyx/20 transition-all">
            <div className="flex items-center justify-between mb-2">
              <span className="text-caption font-semibold uppercase text-slate">{t('gauges.memory', 'Memory')}</span>
              <div className="w-8 h-8 rounded-full bg-soft-meadow flex items-center justify-center text-deep-ink">
                <HardDrive className="w-4 h-4" />
              </div>
            </div>
            <div className="text-heading font-serif text-deep-ink flex items-baseline gap-2">
              <span>{t('units.megabytes', { value: ramUsed })}</span>
              <span className="text-caption text-slate font-mono font-normal">
                {t('gauges.ofMegabytes', { value: ramTotal })}
              </span>
            </div>
            <div className="w-full bg-onyx/10 h-2 rounded-full mt-3 overflow-hidden">
              <div
                className="bg-emerald-600 h-full rounded-full transition-all duration-500"
                style={{ width: `${ramPercent}%` }}
              />
            </div>
            <div className="text-[11px] font-mono text-slate mt-2 flex items-center justify-between">
              <span>{t('gauges.daemonMemory', { value: data?.metrics?.memory?.actond_mb || 14 })}</span>
              <span>{t('gauges.usedPercent', { value: ramPercent })}</span>
            </div>
          </Card>

          {/* Disk Storage Gauge */}
          <Card className="p-5 border border-onyx/10 bg-canvas/90 shadow-xs hover:border-onyx/20 transition-all">
            <div className="flex items-center justify-between mb-2">
              <span className="text-caption font-semibold uppercase text-slate">{t('gauges.storage', 'Disk Storage')}</span>
              <div className="w-8 h-8 rounded-full bg-soft-meadow flex items-center justify-center text-deep-ink">
                <Layers className="w-4 h-4" />
              </div>
            </div>
            <div className="text-heading font-serif text-deep-ink flex items-baseline gap-2">
              <span>{t('units.gigabytes', { value: diskUsed.toFixed(1) })}</span>
              <span className="text-caption text-slate font-mono font-normal">
                {t('gauges.ofGigabytes', { value: diskTotal.toFixed(0) })}
              </span>
            </div>
            <div className="w-full bg-onyx/10 h-2 rounded-full mt-3 overflow-hidden">
              <div
                className="bg-amber-600 h-full rounded-full transition-all duration-500"
                style={{ width: `${diskPercent}%` }}
              />
            </div>
            <div className="text-[11px] font-mono text-slate mt-2 flex items-center justify-between">
              <span>{t('gauges.dataSize', { value: data?.metrics?.disk?.data_dir_gb?.toFixed(2) || '0.05' })}</span>
              <span>{t('gauges.allocatedPercent', { value: diskPercent })}</span>
            </div>
          </Card>

          {/* Kernel Uptime & Status */}
          <Card className="p-5 border border-onyx/10 bg-canvas/90 shadow-xs hover:border-onyx/20 transition-all">
            <div className="flex items-center justify-between mb-2">
              <span className="text-caption font-semibold uppercase text-slate">{t('gauges.uptime', 'Uptime')}</span>
              <div className="w-8 h-8 rounded-full bg-soft-meadow flex items-center justify-center text-deep-ink">
                <Clock className="w-4 h-4" />
              </div>
            </div>
            <div className="text-heading font-serif text-deep-ink">
              {t('gauges.uptimeMinutes', { value: uptimeMins })}
            </div>
            <div className="flex items-center gap-2 text-caption font-mono text-emerald-700 mt-3">
              <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse shrink-0" />
              <span>{t('gauges.processStatus', { status: data?.metrics?.cpu?.cores ? t('gauges.halRunning') : t('gauges.kernelStable') })}</span>
            </div>
            <div className="text-[11px] font-mono text-slate mt-2">
              {t('gauges.sync', { time: new Date(data?.timestamp || Date.now()).toLocaleTimeString() })}
            </div>
          </Card>
        </div>

        {/* 4 Core Pillar Metrics */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
          {/* Autonomous Agents Pillar */}
          <div
            onClick={() => onNavigateTab('agents')}
            className="p-5 rounded-[24px] border border-onyx/10 bg-soft-meadow hover:shadow-md hover:border-onyx/25 transition-all cursor-pointer group"
          >
            <div className="flex items-center justify-between mb-3">
              <div className="w-10 h-10 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center shadow-xs">
                <Bot className="w-5 h-5" />
              </div>
              <ArrowUpRight className="w-4 h-4 text-slate group-hover:text-deep-ink transition-colors" />
            </div>
            <div className="text-heading-sm font-serif text-deep-ink mb-1">
              {data?.agents_count || 1} {t('metrics.agents', 'Agents')}
            </div>
            <p className="text-caption text-slate">
              {t('metrics.agentsDesc', '{{active}} active of {{total}} total', {
                active: data?.agents_active || 1,
                total: data?.agents_count || 1,
              })}
            </p>
          </div>

          {/* Tools Hub Pillar */}
          <div
            onClick={() => onNavigateTab('tools')}
            className="p-5 rounded-[24px] border border-onyx/10 bg-soft-meadow hover:shadow-md hover:border-onyx/25 transition-all cursor-pointer group"
          >
            <div className="flex items-center justify-between mb-3">
              <div className="w-10 h-10 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center shadow-xs">
                <Wrench className="w-5 h-5" />
              </div>
              <ArrowUpRight className="w-4 h-4 text-slate group-hover:text-deep-ink transition-colors" />
            </div>
            <div className="text-heading-sm font-serif text-deep-ink mb-1">
              {data?.tools_count || 6} {t('metrics.tools', 'Tools')}
            </div>
            <p className="text-caption text-slate">
              {t('metrics.toolBreakdown', {
                native: data?.tools_native || 4,
                mcp: data?.tools_mcp || 0,
                wasm: data?.tools_wasm || 0,
              })}
            </p>
          </div>

          {/* Cron Automations Pillar */}
          <div
            onClick={() => onNavigateTab('automations')}
            className="p-5 rounded-[24px] border border-onyx/10 bg-soft-meadow hover:shadow-md hover:border-onyx/25 transition-all cursor-pointer group"
          >
            <div className="flex items-center justify-between mb-3">
              <div className="w-10 h-10 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center shadow-xs">
                <Calendar className="w-5 h-5" />
              </div>
              <ArrowUpRight className="w-4 h-4 text-slate group-hover:text-deep-ink transition-colors" />
            </div>
            <div className="text-heading-sm font-serif text-deep-ink mb-1">
              {data?.cron_count || 0} {t('metrics.automations', 'Automations')}
            </div>
            <p className="text-caption text-slate">
              {t('metrics.automationsDesc', '{{total}} scheduled tasks', { total: data?.cron_count || 0 })}
            </p>
          </div>

          {/* Tailscale Mesh Pillar */}
          <div
            onClick={() => onNavigateTab('settings')}
            className="p-5 rounded-[24px] border border-onyx/10 bg-soft-meadow hover:shadow-md hover:border-onyx/25 transition-all cursor-pointer group"
          >
            <div className="flex items-center justify-between mb-3">
              <div className="w-10 h-10 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center shadow-xs">
                <Shield className="w-5 h-5" />
              </div>
              <ArrowUpRight className="w-4 h-4 text-slate group-hover:text-deep-ink transition-colors" />
            </div>
            <div className="text-heading-sm font-serif text-deep-ink mb-1">
              {t('metrics.mesh', 'Tailscale VPN')}
            </div>
            <p className="text-caption text-slate">
              {data?.tailscale?.connected
                ? `Node: ${data.tailscale.ip || 'Connected'}`
                : 'Standalone Local Mode'}
            </p>
          </div>
        </div>

        {/* Token Traffic & Autonomous Heartbeat Banner */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 mb-8">
          {/* Token Usage Card */}
          <Card className="lg:col-span-2 p-5 border border-onyx/10 bg-canvas/90 shadow-xs hover:border-onyx/20 transition-all flex flex-col justify-between">
            <div>
              <div className="flex items-center justify-between mb-4">
                <div className="flex items-center gap-2.5">
                  <div className="w-8 h-8 rounded-full bg-emerald-500/10 text-emerald-600 flex items-center justify-center">
                    <Coins className="w-4 h-4" />
                  </div>
                  <div>
                    <h3 className="font-serif text-heading-sm text-deep-ink font-semibold">
                      {t('tokens.title', 'Token Consumption & Cost Ledger')}
                    </h3>
                    <p className="text-[12px] text-slate">
                      {t('tokens.subtitle', 'Live tracking across ReAct loops, Crons, and autonomous Heartbeats')}
                    </p>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant="neutral" className="font-mono">
                    {t('units.usd', { value: (tokenStats?.total_cost_usd || 0).toFixed(4) })}
                  </Badge>
                </div>
              </div>

              <div className="grid grid-cols-3 gap-3 pt-1">
                <div className="p-3 bg-soft-meadow rounded-2xl border border-onyx/5">
                  <span className="text-[11px] font-semibold uppercase text-slate block mb-1">{t('tokens.today')}</span>
                  <span className="text-body font-serif font-bold text-deep-ink">
                    {(tokenStats?.today_tokens || 0).toLocaleString()}
                  </span>
                  <span className="text-[10px] text-slate block font-mono">
                    ${(tokenStats?.today_cost_usd || 0).toFixed(4)}
                  </span>
                </div>
                <div className="p-3 bg-soft-meadow rounded-2xl border border-onyx/5">
                  <span className="text-[11px] font-semibold uppercase text-slate block mb-1">{t('tokens.month')}</span>
                  <span className="text-body font-serif font-bold text-deep-ink">
                    {(tokenStats?.month_tokens || 0).toLocaleString()}
                  </span>
                  <span className="text-[10px] text-slate block font-mono">
                    ${(tokenStats?.month_cost_usd || 0).toFixed(4)}
                  </span>
                </div>
                <div className="p-3 bg-soft-meadow rounded-2xl border border-onyx/5">
                  <span className="text-[11px] font-semibold uppercase text-slate block mb-1">{t('tokens.lifetime')}</span>
                  <span className="text-body font-serif font-bold text-deep-ink">
                    {(tokenStats?.total_tokens || 0).toLocaleString()}
                  </span>
                  <span className="text-[10px] text-slate block font-mono">
                    ${(tokenStats?.total_cost_usd || 0).toFixed(4)}
                  </span>
                </div>
              </div>
            </div>

            {/* Model Breakdown Bars */}
            <div className="mt-4 pt-3 border-t border-onyx/5 flex flex-wrap items-center justify-between gap-2">
              <div className="flex flex-wrap gap-2 items-center">
                <span className="text-[11px] font-semibold text-slate uppercase mr-1">{t('tokens.models')}</span>
                {tokenStats?.by_model && tokenStats.by_model.length > 0 ? (
                  tokenStats.by_model.slice(0, 4).map((m) => (
                    <span key={m.model} className="text-[11px] bg-canvas border border-onyx/10 px-2 py-0.5 rounded-full font-mono text-deep-ink">
                      {m.model}: <strong className="text-emerald-700">{m.percentage.toFixed(0)}%</strong>
                    </span>
                  ))
                ) : (
                  <span className="text-[11px] text-slate font-mono">{t('tokens.noUsage')}</span>
                )}
              </div>
              <button
                type="button"
                onClick={() => onNavigateTab('costs')}
                className="text-[11px] font-semibold text-deep-ink hover:text-emerald-700 underline cursor-pointer"
              >
                {t('tokens.inspect')}
              </button>
            </div>
          </Card>

          {/* Autonomous Heartbeat Status Card */}
          <Card className="p-5 border border-onyx/10 bg-canvas/90 shadow-xs flex flex-col justify-between">
            <div>
              <div className="flex items-center justify-between mb-3">
                <div className="flex items-center gap-2">
                  <div className="w-8 h-8 rounded-full bg-rose-500/10 text-rose-600 flex items-center justify-center">
                    <HeartPulse className="w-4 h-4" />
                  </div>
                  <h3 className="font-serif text-heading-sm text-deep-ink font-semibold">
                    {t('heartbeat.title', 'Heartbeat Pulse')}
                  </h3>
                </div>
                <span className="w-2.5 h-2.5 rounded-full bg-emerald-500 animate-pulse" />
              </div>
              <p className="text-caption text-slate mb-3">
                {t('heartbeat.desc', 'Autonomous 5m cognitive loop reviewing pending tasks and alerts.')}
              </p>
            </div>

            <div className="p-3 bg-soft-meadow rounded-2xl border border-onyx/5 text-[11px]">
              <div className="flex justify-between items-center mb-1 text-slate">
                <span>{t('heartbeat.lastStatus')}</span>
                <span className="font-semibold text-emerald-700 font-mono">
                  {heartbeatRuns[0]?.status || 'Nominal (Zero Noise)'}
                </span>
              </div>
              <div className="text-slate font-mono truncate">
                {heartbeatRuns[0]?.summary || t('heartbeat.nominalSummary')}
              </div>
            </div>
          </Card>
        </div>

        {/* Quick Launchpad & Activity Feed Grid */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Quick Action Launchpad */}
          <Card className="p-6 border border-onyx/10 bg-canvas/90 space-y-4">
            <div className="flex items-center gap-2.5">
              <Zap className="w-5 h-5 text-deep-ink" />
              <h3 className="font-serif text-heading-sm text-deep-ink">
                {t('launchpad.title', 'Quick Actions')}
              </h3>
            </div>

            <div className="space-y-2">
              <button
                onClick={() => onOpenChat('agent_system_core')}
                className="w-full flex items-center justify-between p-3 rounded-2xl bg-soft-meadow hover:bg-black/5 text-deep-ink font-semibold text-body-sm transition-all text-left group cursor-pointer"
              >
                <div className="flex items-center gap-2.5">
                  <Sparkles className="w-4 h-4 text-hi-yellow fill-hi-yellow" />
                  <span>{t('launchpad.chatCore', 'Chat with Assistant')}</span>
                </div>
                <ArrowUpRight className="w-4 h-4 text-slate group-hover:text-deep-ink transition-colors" />
              </button>

              <button
                onClick={() => {
                  if (onEditAgent) onEditAgent('new');
                  else onNavigateTab('agents');
                }}
                className="w-full flex items-center justify-between p-3 rounded-2xl bg-soft-meadow hover:bg-black/5 text-deep-ink font-semibold text-body-sm transition-all text-left group cursor-pointer"
              >
                <div className="flex items-center gap-2.5">
                  <Plus className="w-4 h-4 text-deep-ink" />
                  <span>{t('launchpad.newAgent', 'New Agent')}</span>
                </div>
                <ArrowUpRight className="w-4 h-4 text-slate group-hover:text-deep-ink transition-colors" />
              </button>

              <button
                onClick={() => onNavigateTab('skills')}
                className="w-full flex items-center justify-between p-3 rounded-2xl bg-soft-meadow hover:bg-black/5 text-deep-ink font-semibold text-body-sm transition-all text-left group cursor-pointer"
              >
                <div className="flex items-center gap-2.5">
                  <Sparkles className="w-4 h-4 text-deep-ink" />
                  <span>{t('launchpad.exploreSkills', 'Explore Skills')}</span>
                </div>
                <ArrowUpRight className="w-4 h-4 text-slate group-hover:text-deep-ink transition-colors" />
              </button>

              <button
                onClick={() => onNavigateTab('tools')}
                className="w-full flex items-center justify-between p-3 rounded-2xl bg-soft-meadow hover:bg-black/5 text-deep-ink font-semibold text-body-sm transition-all text-left group cursor-pointer"
              >
                <div className="flex items-center gap-2.5">
                  <Wrench className="w-4 h-4 text-deep-ink" />
                  <span>{t('launchpad.exploreTools', 'Manage Tools')}</span>
                </div>
                <ArrowUpRight className="w-4 h-4 text-slate group-hover:text-deep-ink transition-colors" />
              </button>

              <button
                onClick={() => onNavigateTab('plugins')}
                className="w-full flex items-center justify-between p-3 rounded-2xl bg-soft-meadow hover:bg-black/5 text-deep-ink font-semibold text-body-sm transition-all text-left group cursor-pointer"
              >
                <div className="flex items-center gap-2.5">
                  <Radio className="w-4 h-4 text-deep-ink" />
                  <span>{t('launchpad.pairChannel', 'Plugins & Channels')}</span>
                </div>
                <ArrowUpRight className="w-4 h-4 text-slate group-hover:text-deep-ink transition-colors" />
              </button>

              <button
                onClick={() => onNavigateTab('workspace')}
                className="w-full flex items-center justify-between p-3 rounded-2xl bg-soft-meadow hover:bg-black/5 text-deep-ink font-semibold text-body-sm transition-all text-left group cursor-pointer"
              >
                <div className="flex items-center gap-2.5">
                  <Layers className="w-4 h-4 text-deep-ink" />
                  <span>{t('launchpad.openWorkspace', 'Files & Storage')}</span>
                </div>
                <ArrowUpRight className="w-4 h-4 text-slate group-hover:text-deep-ink transition-colors" />
              </button>
            </div>
          </Card>

          {/* Live System & Audit Feed */}
          <Card className="lg:col-span-2 p-6 border border-onyx/10 bg-canvas/90 flex flex-col justify-between">
            <div>
              <div className="flex items-center justify-between mb-4">
                <div className="flex items-center gap-2.5">
                  <Activity className="w-5 h-5 text-deep-ink" />
                  <div>
                    <h3 className="font-serif text-heading-sm text-deep-ink">
                      {t('activity.title', 'Recent Activity')}
                    </h3>
                    <p className="text-caption text-slate">
                      {t('activity.subtitle', 'Real-time log of tool executions and agent actions.')}
                    </p>
                  </div>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => onNavigateTab('settings')}
                >
                  {t('activity.viewAll', 'View All')}
                </Button>
              </div>

              {!data?.recent_audit || data.recent_audit.length === 0 ? (
                <div className="py-12 text-center text-slate font-sans text-caption">
                  {t('activity.empty', 'No recent logs.')}
                </div>
              ) : (
                <div className="divide-y divide-onyx/5">
                  {data.recent_audit.map((entry, idx) => (
                    <div key={entry.trace_id || idx} className="py-3 flex items-start justify-between gap-4 text-body-sm">
                      <div className="space-y-0.5 flex-1">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold text-deep-ink font-mono text-caption">
                            {entry.tool_name}
                          </span>
                          <Badge
                            variant={
                              entry.risk_level === 'High'
                                ? 'accent'
                                : entry.risk_level === 'Medium'
                                  ? 'stopped'
                                  : 'neutral'
                            }
                            className="text-[10px]"
                          >
                            {t('activity.risk', { level: entry.risk_level })}
                          </Badge>
                          <span className="text-caption font-mono text-slate">
                            {t('activity.statusDuration', { status: entry.status, duration: entry.execution_time_ms })}
                          </span>
                        </div>
                        <div className="text-caption font-mono text-slate truncate max-w-md">
                          {t('activity.detail', { agent: entry.agent_id, trace: entry.trace_id })}
                        </div>
                      </div>

                      <span className="text-caption font-mono text-slate shrink-0">
                        {new Date(entry.timestamp).toLocaleTimeString()}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* Storage Footprint Footer */}
            <div className="pt-4 mt-4 border-t border-onyx/5 flex items-center justify-between text-caption font-mono text-slate">
              <span>
                {t('storage.detail', { sqlite: ((data?.storage?.storage_bytes || 0) / (1024 * 1024)).toFixed(2), vectors: ((data?.storage?.vectors_bytes || 0) / (1024 * 1024)).toFixed(2) })}
              </span>
              <span className="font-semibold text-deep-ink">
                {t('storage.total', { value: ((data?.storage?.total_bytes || 0) / (1024 * 1024)).toFixed(2) })}
              </span>
            </div>
          </Card>
        </div>

      </PageContainer>
    </div>
  );
}
