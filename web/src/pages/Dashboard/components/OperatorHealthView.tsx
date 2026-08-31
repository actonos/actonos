import { useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { useToast } from '@/components/ui/Toast';
import { api } from '@/lib/api';
import type {
  SystemMetrics,
  SystemAnomaly,
  AgentManifest,
  HeartbeatConfigData,
  ProviderHealthReport,
} from '@/lib/types';
import {
  Activity,
  HeartPulse,
  Cpu,
  Bot,
  Zap,
  ShieldAlert,
  Sparkles,
} from 'lucide-react';

export interface OperatorHealthViewProps {
  metrics?: SystemMetrics | null;
  anomalies?: SystemAnomaly[];
  agents?: AgentManifest[];
  agentsCount?: number;
  agentsActive?: number;
  heartbeatConfig?: HeartbeatConfigData | null;
  providerReports?: Record<string, ProviderHealthReport>;
  onNavigateTab?: (tab: any) => void;
  onRefresh?: () => void;
}

export function OperatorHealthView({
  metrics,
  anomalies = [],
  agents = [],
  agentsCount,
  agentsActive,
  heartbeatConfig,
  providerReports = {},
  onNavigateTab,
  onRefresh,
}: OperatorHealthViewProps) {
  const { t } = useTranslation('dashboard');
  const { success, error } = useToast();
  const [actingAnomalyId, setActingAnomalyId] = useState<string | null>(null);

  // Active anomalies
  const activeAnomalies = useMemo(() => {
    return anomalies.filter((a) => a.status === 'active');
  }, [anomalies]);

  const memPercent = useMemo(() => {
    if (!metrics?.memory?.total_mb) return 0;
    return Math.round((metrics.memory.used_mb / metrics.memory.total_mb) * 100);
  }, [metrics]);

  const diskPercent = useMemo(() => {
    if (!metrics?.disk?.total_gb) return 0;
    return Math.round((metrics.disk.used_gb / metrics.disk.total_gb) * 100);
  }, [metrics]);

  const cpuPercent = useMemo(() => {
    return metrics?.cpu?.usage_percent ? Math.round(metrics.cpu.usage_percent) : 0;
  }, [metrics]);

  // Compute overall health score (0 to 100)
  const healthScore = useMemo(() => {
    let score = 100;
    // Deduct for active anomalies
    const criticals = activeAnomalies.filter((a) => a.severity === 'critical').length;
    const warnings = activeAnomalies.filter((a) => a.severity === 'warning').length;
    score -= criticals * 25;
    score -= warnings * 10;

    // Deduct for tripped circuit breakers
    for (const p of Object.values(providerReports)) {
      if (p.status === 'circuit_tripped') score -= 20;
      else if (p.status === 'degraded') score -= 10;
    }

    // Deduct if high memory or disk usage
    if (memPercent > 85) score -= 10;
    if (diskPercent > 90) score -= 15;

    return Math.max(0, Math.min(100, score));
  }, [activeAnomalies, providerReports, memPercent, diskPercent]);

  const handleActAnomaly = async (anomaly: SystemAnomaly, action: 'auto_task' | 'resolve' | 'ignore') => {
    setActingAnomalyId(anomaly.id);
    try {
      await api.actOnAnomaly(anomaly.id, action);
      success(
        t('anomalies.actedTitle', 'Anomaly Handled'),
        action === 'auto_task'
          ? t('anomalies.autoTaskCreated', 'Autonomous diagnostic task spawned.')
          : t('anomalies.resolved', 'Anomaly marked as resolved.')
      );
      onRefresh?.();
    } catch (err) {
      error('Action Failed', String(err));
    } finally {
      setActingAnomalyId(null);
    }
  };

  const isHealthy = healthScore >= 85;
  const isDegraded = healthScore >= 60 && healthScore < 85;

  return (
    <div className="space-y-6">
      {/* Operator Health Overview Strip */}
      <Card className="p-6 border border-onyx/10 bg-canvas/95 shadow-xs relative overflow-hidden">
        {/* Subtle accent backdrop */}
        <div
          className={`absolute top-0 right-0 w-64 h-64 rounded-full blur-3xl -mr-20 -mt-20 pointer-events-none opacity-20 ${isHealthy ? 'bg-emerald-500' : isDegraded ? 'bg-amber-500' : 'bg-accent-coral'
            }`}
        />

        <div className="flex flex-col md:flex-row md:items-center justify-between gap-6 relative z-10">
          {/* Health Score Gauge */}
          <div className="flex items-center gap-5">
            <div
              className={`w-18 h-18 rounded-2xl flex flex-col items-center justify-center border text-center shadow-xs shrink-0 ${isHealthy
                ? 'bg-emerald-500/10 border-emerald-500/25 text-emerald-700'
                : isDegraded
                  ? 'bg-amber-500/10 border-amber-500/25 text-amber-700'
                  : 'bg-accent-coral/10 border-accent-coral/25 text-accent-coral'
                }`}
            >
              <span className="text-2xl font-bold font-serif leading-none">{healthScore}</span>
              <span className="text-[10px] font-mono tracking-wider font-semibold uppercase mt-0.5">/ 100</span>
            </div>

            <div>
              <div className="flex items-center gap-2">
                <h3 className="text-heading font-serif font-bold text-deep-ink">
                  {isHealthy
                    ? t('operator.nominalStatus', 'Operator Nominal')
                    : isDegraded
                      ? t('operator.degradedStatus', 'System Degraded')
                      : t('operator.criticalStatus', 'Action Required')}
                </h3>
                <Badge
                  variant={isHealthy ? 'active' : isDegraded ? 'stopped' : 'accent'}
                  className="text-[11px]"
                >
                  {isHealthy ? 'Healthy' : isDegraded ? 'Degraded' : 'Attention'}
                </Badge>
              </div>
              <p className="text-body-sm text-slate mt-1 max-w-xl">
                {activeAnomalies.length > 0
                  ? t('operator.anomaliesDetected', '{{count}} proactive anomalies require operator oversight.', { count: activeAnomalies.length })
                  : t('operator.nominalDescription', 'Autonomous Swarm, Memory Layer, and 24/7 Heartbeat operating at peak efficiency.')}
              </p>
            </div>
          </div>

          {/* Quick Action Buttons */}
          <div className="flex flex-wrap items-center gap-3 shrink-0">
            {onNavigateTab && (
              <>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => onNavigateTab('missions')}
                  icon={<Zap className="w-3.5 h-3.5" />}
                >
                  {t('operator.missionControl', 'Mission Control')}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => onNavigateTab('audit-logs')}
                  icon={<Activity className="w-3.5 h-3.5" />}
                >
                  {t('operator.auditLogs', 'Audit Ledger')}
                </Button>
              </>
            )}
          </div>
        </div>

        {/* Proactive Anomaly Quick Banner (If anomalies present) */}
        {activeAnomalies.length > 0 && (
          <div className="mt-5 pt-4 border-t border-onyx/10 space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-caption font-semibold text-deep-ink uppercase tracking-wider flex items-center gap-1.5">
                <ShieldAlert className="w-4 h-4 text-amber-600" />
                {t('anomalies.title', 'Proactive System Anomalies')}
              </span>
              <span className="text-[11px] text-slate font-mono">{activeAnomalies.length} active</span>
            </div>

            {activeAnomalies.slice(0, 2).map((anomaly) => (
              <div
                key={anomaly.id}
                className="p-3.5 rounded-xl bg-amber-500/10 border border-amber-500/20 flex flex-col sm:flex-row sm:items-center justify-between gap-3"
              >
                <div className="space-y-0.5 min-w-0">
                  <div className="flex items-center gap-2">
                    <Badge
                      variant={anomaly.severity === 'critical' ? 'accent' : 'stopped'}
                      className="text-[10px]"
                    >
                      {anomaly.severity}
                    </Badge>
                    <h4 className="text-body-sm font-semibold text-deep-ink truncate">
                      {anomaly.title}
                    </h4>
                  </div>
                  <p className="text-caption text-slate line-clamp-1">
                    {anomaly.description}
                  </p>
                </div>

                <div className="flex items-center gap-2 shrink-0">
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={() => handleActAnomaly(anomaly, 'auto_task')}
                    disabled={actingAnomalyId === anomaly.id}
                    icon={<Sparkles className="w-3.5 h-3.5" />}
                    className="text-xs py-1 px-2.5 h-7"
                  >
                    {t('anomalies.autoFix', 'Auto-Fix Task')}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => handleActAnomaly(anomaly, 'resolve')}
                    disabled={actingAnomalyId === anomaly.id}
                    className="text-xs py-1 px-2 h-7"
                  >
                    {t('anomalies.dismiss', 'Dismiss')}
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>

      {/* Core Vitals 4-Card Strip */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {/* Heartbeat 24/7 Status */}
        <Card className="p-4 border border-onyx/10 bg-canvas/90 shadow-xs flex items-center justify-between">
          <div className="space-y-1">
            <span className="text-caption font-semibold text-slate uppercase">{t('vitals.pulse', 'Heartbeat Pulse')}</span>
            <div className="font-serif text-heading-sm font-bold text-deep-ink flex items-center gap-2">
              <span className={`w-2.5 h-2.5 rounded-full ${heartbeatConfig?.enabled ? 'bg-emerald-500 animate-pulse' : 'bg-slate'}`} />
              {heartbeatConfig?.enabled ? `${heartbeatConfig.interval_minutes}m interval` : 'Disabled'}
            </div>
            <span className="text-[11px] text-slate font-mono">{heartbeatConfig?.target_channel || 'all'} channel</span>
          </div>
          <div className="p-3 rounded-2xl bg-soft-meadow border border-onyx/10 text-deep-ink">
            <HeartPulse className="w-5 h-5" />
          </div>
        </Card>

        {/* LLM Provider Status */}
        <Card className="p-4 border border-onyx/10 bg-canvas/90 shadow-xs flex items-center justify-between">
          <div className="space-y-1">
            <span className="text-caption font-semibold text-slate uppercase">{t('vitals.llmHealth', 'LLM Provider Mesh')}</span>
            <div className="font-serif text-heading-sm font-bold text-deep-ink">
              {Object.keys(providerReports).length > 0 ? `${Object.keys(providerReports).length} Connected` : 'Ready'}
            </div>
            <span className="text-[11px] text-emerald-700 font-mono">0 circuit trips</span>
          </div>
          <div className="p-3 rounded-2xl bg-soft-meadow border border-onyx/10 text-deep-ink">
            <Zap className="w-5 h-5" />
          </div>
        </Card>

        {/* Memory & Host Telemetry */}
        <Card className="p-4 border border-onyx/10 bg-canvas/90 shadow-xs flex items-center justify-between">
          <div className="space-y-1">
            <span className="text-caption font-semibold text-slate uppercase">{t('vitals.hostMemory', 'Host RAM & CPU')}</span>
            <div className="font-serif text-heading-sm font-bold text-deep-ink">
              {memPercent > 0 ? `${memPercent}% RAM` : 'Normal'}
            </div>
            <span className="text-[11px] text-slate font-mono">{cpuPercent > 0 ? `${cpuPercent}% CPU` : 'Active'}</span>
          </div>
          <div className="p-3 rounded-2xl bg-soft-meadow border border-onyx/10 text-deep-ink">
            <Cpu className="w-5 h-5" />
          </div>
        </Card>

        {/* Active Agents */}
        <Card className="p-4 border border-onyx/10 bg-canvas/90 shadow-xs flex items-center justify-between">
          <div className="space-y-1">
            <span className="text-caption font-semibold text-slate uppercase">{t('vitals.agentFleet', 'Agent Fleet')}</span>
            <div className="font-serif text-heading-sm font-bold text-deep-ink">
              {agentsActive !== undefined ? agentsActive : agents.filter((a) => a.status === 'active').length} <span className="text-caption font-sans font-normal text-slate">/ {agentsCount !== undefined ? agentsCount : agents.length} active</span>
            </div>
            <span className="text-[11px] text-emerald-700 font-mono">Swarm Nominal</span>
          </div>
          <div className="p-3 rounded-2xl bg-soft-meadow border border-onyx/10 text-deep-ink">
            <Bot className="w-5 h-5" />
          </div>
        </Card>
      </div>
    </div>
  );
}
