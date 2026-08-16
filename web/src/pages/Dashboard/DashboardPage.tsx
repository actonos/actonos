import { useState, useEffect } from 'react';
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
  CheckCircle2,
  Circle,
  X,
  Key,
  Radio,
} from 'lucide-react';
import { api, type DashboardSummaryData } from '@/lib/api';
import type { NavTab } from '@/components/layout/Sidebar';

export interface DashboardPageProps {
  onNavigateTab: (tab: NavTab) => void;
  onOpenChat: (agentID?: string) => void;
  onEditAgent?: (agentID: string) => void;
}

export function DashboardPage({ onNavigateTab, onOpenChat, onEditAgent }: DashboardPageProps) {
  const { t } = useTranslation('dashboard');
  const { error } = useToast();
  const [data, setData] = useState<DashboardSummaryData | null>(null);
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
      const summary = await api.getDashboardSummary();
      setData(summary);

      // Auto-detect completed steps based on server state
      setCompletedSteps((prev) => {
        const next = { ...prev };
        if (summary.agents_count > 1) next['agent'] = true;
        if (summary.tools_count > 4) next['skills'] = true;
        localStorage.setItem('actonos_quickstart_steps', JSON.stringify(next));
        return next;
      });
    } catch (err: any) {
      error('Failed to load dashboard summary', err.message);
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

  const quickstartSteps = [
    {
      id: 'keys',
      title: t('quickstart.steps.keys.title', 'Add API Keys'),
      desc: t('quickstart.steps.keys.desc', 'Configure Anthropic, OpenAI, or Gemini keys.'),
      actionText: t('quickstart.steps.keys.action', 'Configure'),
      icon: Key,
      tab: 'settings' as NavTab,
    },
    {
      id: 'channel',
      title: t('quickstart.steps.channel.title', 'Connect a Chat Channel'),
      desc: t('quickstart.steps.channel.desc', 'Link Telegram, Discord, or WhatsApp to chat with agents.'),
      actionText: t('quickstart.steps.channel.action', 'Connect'),
      icon: Radio,
      tab: 'channels' as NavTab,
    },
    {
      id: 'agent',
      title: t('quickstart.steps.agent.title', 'Create an AI Agent'),
      desc: t('quickstart.steps.agent.desc', 'Create and customize your autonomous agent.'),
      actionText: t('quickstart.steps.agent.action', 'Create'),
      icon: Bot,
      tab: 'agents' as NavTab,
    },
    {
      id: 'chat',
      title: t('quickstart.steps.chat.title', 'Try Chatting'),
      desc: t('quickstart.steps.chat.desc', 'Send a prompt to see the agent think and use tools.'),
      actionText: t('quickstart.steps.chat.action', 'Chat'),
      icon: MessageSquare,
      tab: 'chat' as NavTab,
    },
    {
      id: 'skills',
      title: t('quickstart.steps.skills.title', 'Explore Skills'),
      desc: t('quickstart.steps.skills.desc', 'Install community skills to give your agents new capabilities.'),
      actionText: t('quickstart.steps.skills.action', 'Browse'),
      icon: Sparkles,
      tab: 'skills' as NavTab,
    },
  ];

  const completedCount = quickstartSteps.filter((s) => completedSteps[s.id]).length;
  const progressPercent = Math.round((completedCount / quickstartSteps.length) * 100);

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

      <PageContainer>
        {/* Header */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
          <div className="flex-1">
            <span className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
              {t('eyebrow', 'Overview')}
            </span>
            <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight flex items-center gap-3">
              <span>{t('title', 'Dashboard')}</span>
              <Badge variant="active" className="text-caption font-mono">
                v0.1.0 Live
              </Badge>
            </h1>
            <p className="font-sans text-body text-slate mt-1 max-w-2xl">
              {t(
                'subtitle',
                'Overview of system status, active agents, and recent activities.'
              )}
            </p>
          </div>

          <div className="flex items-center gap-2.5 shrink-0 self-start sm:self-center">
            {quickstartDismissed && (
              <Button
                variant="ghost"
                size="sm"
                icon={<Sparkles className="w-3.5 h-3.5 text-hi-yellow" />}
                onClick={handleRestoreQuickstart}
              >
                {t('quickstart.show', 'Show Guide')}
              </Button>
            )}
            <Button
              variant="ghost"
              size="sm"
              icon={<RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />}
              onClick={loadSummary}
            >
              Refresh
            </Button>
            <Button
              variant="primary"
              size="sm"
              icon={<MessageSquare className="w-3.5 h-3.5" />}
              onClick={() => onOpenChat('agent_system_core')}
            >
              {t('launchpad.chatCore', 'Chat with Assistant')}
            </Button>
          </div>
        </div>

        {/* QuickStart Guide Checklist */}
        {!quickstartDismissed && (
          <Card className="p-6 border-2 border-deep-ink/15 bg-canvas/95 shadow-sm mb-8 relative overflow-hidden">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-4">
              <div>
                <div className="flex items-center gap-2">
                  <div className="w-7 h-7 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center shadow-xs">
                    <Sparkles className="w-3.5 h-3.5" />
                  </div>
                  <h2 className="font-serif text-heading-sm text-deep-ink font-semibold">
                    {t('quickstart.title', 'Quick Start Guide')}
                  </h2>
                </div>
                <p className="font-sans text-caption text-slate mt-1">
                  {t('quickstart.subtitle', 'Complete these quick steps to get the most out of ActonOS.')}
                </p>
              </div>

              <div className="flex items-center gap-3">
                <div className="text-right">
                  <span className="text-caption font-mono text-deep-ink font-semibold">
                    {t('quickstart.progress', { completed: completedCount, total: quickstartSteps.length })}
                  </span>
                  <div className="w-32 bg-onyx/10 h-1.5 rounded-full mt-1 overflow-hidden">
                    <div
                      className="bg-emerald-500 h-full rounded-full transition-all duration-300"
                      style={{ width: `${progressPercent}%` }}
                    />
                  </div>
                </div>
                <button
                  onClick={handleDismissQuickstart}
                  className="p-1.5 rounded-full hover:bg-black/5 text-slate hover:text-deep-ink transition-colors cursor-pointer"
                  title={t('quickstart.dismiss', 'Hide Guide')}
                >
                  <X className="w-4 h-4" />
                </button>
              </div>
            </div>

            {/* Checklist Grid */}
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-3 pt-2">
              {quickstartSteps.map((step, idx) => {
                const Icon = step.icon;
                const isDone = !!completedSteps[step.id];

                return (
                  <div
                    key={step.id}
                    className={`p-3.5 rounded-[18px] border transition-all flex flex-col justify-between ${
                      isDone
                        ? 'bg-soft-meadow/50 border-emerald-500/30'
                        : 'bg-soft-meadow border-onyx/10 hover:border-onyx/20'
                    }`}
                  >
                    <div>
                      <div className="flex items-center justify-between mb-2">
                        <span className="text-[11px] font-mono font-semibold text-slate">
                          0{idx + 1}
                        </span>
                        <button
                          onClick={() => toggleStep(step.id)}
                          className="cursor-pointer transition-colors"
                        >
                          {isDone ? (
                            <CheckCircle2 className="w-4 h-4 text-emerald-600" />
                          ) : (
                            <Circle className="w-4 h-4 text-slate/40 hover:text-slate" />
                          )}
                        </button>
                      </div>
                      <div className="flex items-center gap-2 mb-1">
                        <Icon className="w-4 h-4 text-deep-ink shrink-0" />
                        <h4 className={`text-body-sm font-semibold text-deep-ink truncate ${isDone ? 'line-through opacity-70' : ''}`}>
                          {step.title}
                        </h4>
                      </div>
                      <p className="text-[11px] text-slate line-clamp-2 mb-3">
                        {step.desc}
                      </p>
                    </div>

                    <Button
                      variant={isDone ? 'ghost' : 'primary'}
                      size="sm"
                      onClick={() => onNavigateTab(step.tab)}
                      className="w-full justify-center text-caption py-1 h-7"
                    >
                      {step.actionText}
                    </Button>
                  </div>
                );
              })}
            </div>
          </Card>
        )}

        {/* 4 Live Hardware Gauges */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
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
                {data?.metrics?.cpu?.cores || 1} Cores
              </span>
            </div>
            <div className="w-full bg-onyx/10 h-2 rounded-full mt-3 overflow-hidden">
              <div
                className="bg-deep-ink h-full rounded-full transition-all duration-500"
                style={{ width: `${Math.min(100, cpuPercent)}%` }}
              />
            </div>
            <div className="text-[11px] font-mono text-slate mt-2 flex items-center justify-between">
              <span>Temp: {data?.metrics?.cpu?.temperature_celsius || 42}°C</span>
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
              <span>{ramUsed} MB</span>
              <span className="text-caption text-slate font-mono font-normal">
                / {ramTotal} MB
              </span>
            </div>
            <div className="w-full bg-onyx/10 h-2 rounded-full mt-3 overflow-hidden">
              <div
                className="bg-emerald-600 h-full rounded-full transition-all duration-500"
                style={{ width: `${ramPercent}%` }}
              />
            </div>
            <div className="text-[11px] font-mono text-slate mt-2 flex items-center justify-between">
              <span>Daemon: {data?.metrics?.memory?.actond_mb || 14} MB</span>
              <span>{ramPercent}% used</span>
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
              <span>{diskUsed.toFixed(1)} GB</span>
              <span className="text-caption text-slate font-mono font-normal">
                / {diskTotal.toFixed(0)} GB
              </span>
            </div>
            <div className="w-full bg-onyx/10 h-2 rounded-full mt-3 overflow-hidden">
              <div
                className="bg-amber-600 h-full rounded-full transition-all duration-500"
                style={{ width: `${diskPercent}%` }}
              />
            </div>
            <div className="text-[11px] font-mono text-slate mt-2 flex items-center justify-between">
              <span>Data: {data?.metrics?.disk?.data_dir_gb?.toFixed(2) || '0.05'} GB</span>
              <span>{diskPercent}% allocated</span>
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
              {uptimeMins}m <span className="text-body-sm font-sans font-normal text-slate">online</span>
            </div>
            <div className="flex items-center gap-2 text-caption font-mono text-emerald-700 mt-3">
              <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse shrink-0" />
              <span>PID {data?.metrics?.cpu?.cores ? 'HAL Running' : 'Kernel Stable'}</span>
            </div>
            <div className="text-[11px] font-mono text-slate mt-2">
              Sync: {new Date(data?.timestamp || Date.now()).toLocaleTimeString()}
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
              {data?.tools_native || 4} Native • {data?.tools_mcp || 0} MCP • {data?.tools_wasm || 0} WASM
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
                onClick={() => onNavigateTab('channels')}
                className="w-full flex items-center justify-between p-3 rounded-2xl bg-soft-meadow hover:bg-black/5 text-deep-ink font-semibold text-body-sm transition-all text-left group cursor-pointer"
              >
                <div className="flex items-center gap-2.5">
                  <Radio className="w-4 h-4 text-deep-ink" />
                  <span>{t('launchpad.pairChannel', 'Chat Channels')}</span>
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
                            {entry.risk_level} Risk
                          </Badge>
                          <span className="text-caption font-mono text-slate">
                            {entry.status} ({entry.execution_time_ms}ms)
                          </span>
                        </div>
                        <div className="text-caption font-mono text-slate truncate max-w-md">
                          Agent: {entry.agent_id} • Trace: {entry.trace_id}
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
                SQLite: {((data?.storage?.storage_bytes || 0) / (1024 * 1024)).toFixed(2)} MB • Vectors: {((data?.storage?.vectors_bytes || 0) / (1024 * 1024)).toFixed(2)} MB
              </span>
              <span className="font-semibold text-deep-ink">
                Storage: {((data?.storage?.total_bytes || 0) / (1024 * 1024)).toFixed(2)} MB
              </span>
            </div>
          </Card>
        </div>
      </PageContainer>
    </div>
  );
}
