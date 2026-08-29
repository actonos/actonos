import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { IconButton } from '@/components/ui/IconButton';
import { useToast } from '@/components/ui/Toast';
import { api } from '@/lib/api';
import type { SystemAnomaly } from '@/lib/types';
import { ProactiveConfigModal } from './ProactiveConfigModal';
import {
  AlertTriangle,
  CheckCircle2,
  XCircle,
  Play,
  RotateCw,
  Sliders,
  ShieldCheck,
  HardDrive,
  Cpu,
  Layers,
  Clock,
  Sparkles,
  Server,
  MessageSquare,
} from 'lucide-react';

export interface ProactiveAnomaliesCardProps {
  onMissionCreated?: () => void;
  className?: string;
}

export function ProactiveAnomaliesCard({ onMissionCreated, className = '' }: ProactiveAnomaliesCardProps) {
  const { t } = useTranslation('operations');
  const { success, error, info } = useToast();

  const [anomalies, setAnomalies] = useState<SystemAnomaly[]>([]);
  const [loading, setLoading] = useState(false);
  const [scanning, setScanning] = useState(false);
  const [actingId, setActingId] = useState<string | null>(null);
  const [isConfigOpen, setIsConfigOpen] = useState(false);

  const loadAnomalies = useCallback(async (isBackground = false) => {
    try {
      if (!isBackground) setLoading(true);
      const res = await api.listAnomalies('active', undefined, 20);
      setAnomalies(res.anomalies || []);
    } catch (err) {
      if (!isBackground) {
        error(t('anomalies.actionFailed'), err instanceof Error ? err.message : String(err));
      }
    } finally {
      if (!isBackground) setLoading(false);
    }
  }, [error, t]);

  useEffect(() => {
    loadAnomalies(false);
    const interval = window.setInterval(() => loadAnomalies(true), 15000);
    return () => window.clearInterval(interval);
  }, [loadAnomalies]);

  const handleScanNow = async () => {
    setScanning(true);
    try {
      const res = await api.triggerAnomalyScan();
      success(t('anomalies.scanSuccess', { count: res.count }));
      await loadAnomalies(false);
    } catch (err) {
      error(t('anomalies.actionFailed'), err instanceof Error ? err.message : String(err));
    } finally {
      setScanning(false);
    }
  };

  const handleAct = async (id: string, action: 'auto_task' | 'resolve' | 'ignore') => {
    setActingId(id);
    try {
      await api.actOnAnomaly(id, action);
      if (action === 'auto_task') {
        success(t('anomalies.actionSuccess', { action: t('anomalies.autoTask') }));
        onMissionCreated?.();
      } else {
        info(t('anomalies.actionSuccess', { action: action === 'resolve' ? t('anomalies.resolve') : t('anomalies.ignore') }));
      }
      await loadAnomalies(true);
    } catch (err) {
      error(t('anomalies.actionFailed'), err instanceof Error ? err.message : String(err));
    } finally {
      setActingId(null);
    }
  };

  const getKindIcon = (kind: string) => {
    switch (kind) {
      case 'disk_usage':
        return <HardDrive className="w-4 h-4 text-slate shrink-0" />;
      case 'embedding_queue':
        return <Layers className="w-4 h-4 text-slate shrink-0" />;
      case 'mcp_error':
        return <Server className="w-4 h-4 text-slate shrink-0" />;
      case 'task_stalled':
        return <Clock className="w-4 h-4 text-slate shrink-0" />;
      case 'token_budget':
        return <Cpu className="w-4 h-4 text-slate shrink-0" />;
      case 'inbound_queue':
        return <MessageSquare className="w-4 h-4 text-slate shrink-0" />;
      default:
        return <AlertTriangle className="w-4 h-4 text-slate shrink-0" />;
    }
  };

  return (
    <>
      <Card className={`p-5 border border-onyx/10 ${className}`}>
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-4">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <Sparkles className="w-4 h-4 text-deep-ink" />
              <h2 className="font-serif text-heading-sm font-bold text-deep-ink">
                {t('anomalies.title')}
              </h2>
              {anomalies.length > 0 && (
                <Badge variant="warning">{t('anomalies.activeCount', { count: anomalies.length })}</Badge>
              )}
            </div>
            <p className="text-caption text-slate mt-0.5">{t('anomalies.subtitle')}</p>
          </div>

          <div className="flex items-center gap-2 shrink-0">
            <Button
              variant="ghost"
              size="sm"
              icon={<RotateCw className={`w-3.5 h-3.5 ${scanning ? 'animate-spin' : ''}`} />}
              onClick={handleScanNow}
              disabled={scanning || loading}
            >
              {scanning ? t('anomalies.scanning') : t('anomalies.scanNow')}
            </Button>
            <IconButton
              size="sm"
              label={t('anomalies.configure')}
              icon={<Sliders className="w-3.5 h-3.5" />}
              onClick={() => setIsConfigOpen(true)}
            />
          </div>
        </div>

        {loading && anomalies.length === 0 ? (
          <div className="py-8 text-center text-caption text-slate animate-pulse">
            {t('anomalies.scanning')}
          </div>
        ) : anomalies.length === 0 ? (
          <div className="p-6 rounded-[20px] bg-soft-meadow border border-onyx/5 flex items-center gap-3.5">
            <div className="w-9 h-9 rounded-full bg-deep-ink/5 flex items-center justify-center shrink-0">
              <ShieldCheck className="w-5 h-5 text-deep-ink" />
            </div>
            <div className="min-w-0">
              <p className="text-body-sm font-semibold text-deep-ink">
                {t('anomalies.noAnomalies')}
              </p>
              <p className="text-[11px] text-slate mt-0.5">
                Proactive background monitors are idle and all health assertions pass.
              </p>
            </div>
          </div>
        ) : (
          <div className="space-y-3">
            {anomalies.map((item) => {
              const isActing = actingId === item.id;
              const severityTone =
                item.severity === 'critical'
                  ? 'stopped'
                  : item.severity === 'warning'
                    ? 'warning'
                    : 'neutral';

              return (
                <div
                  key={item.id}
                  className="p-4 rounded-[20px] bg-soft-meadow border border-onyx/10 flex flex-col md:flex-row md:items-center justify-between gap-3.5 transition-all hover:border-onyx/20"
                >
                  <div className="flex items-start gap-3 min-w-0 flex-1">
                    <div className="mt-0.5">{getKindIcon(item.kind)}</div>
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant={severityTone}>
                          {item.severity.toUpperCase()}
                        </Badge>
                        <h3 className="font-sans font-bold text-body-sm text-deep-ink truncate">
                          {item.title}
                        </h3>
                        <span className="text-[10px] text-slate font-mono">
                          {t('anomalies.detectedAt', { time: new Date(item.detected_at).toLocaleTimeString() })}
                        </span>
                      </div>
                      <p className="text-caption text-slate mt-1 line-clamp-2">
                        {item.description}
                      </p>
                      {item.suggested_action && (
                        <p className="text-[11px] font-mono text-deep-ink/80 mt-1 bg-canvas/60 px-2 py-0.5 rounded-md inline-block">
                          ➔ {item.suggested_action}
                        </p>
                      )}
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="flex items-center gap-1.5 shrink-0 self-end md:self-center">
                    <Button
                      variant="primary"
                      size="sm"
                      icon={<Play className="w-3 h-3" />}
                      disabled={isActing}
                      onClick={() => handleAct(item.id, 'auto_task')}
                    >
                      {t('anomalies.autoTask')}
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      icon={<CheckCircle2 className="w-3 h-3 text-deep-ink" />}
                      disabled={isActing}
                      onClick={() => handleAct(item.id, 'resolve')}
                    >
                      {t('anomalies.resolve')}
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      icon={<XCircle className="w-3 h-3 text-slate" />}
                      disabled={isActing}
                      onClick={() => handleAct(item.id, 'ignore')}
                    >
                      {t('anomalies.ignore')}
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </Card>

      <ProactiveConfigModal
        isOpen={isConfigOpen}
        onClose={() => setIsConfigOpen(false)}
        onSaved={() => loadAnomalies(false)}
      />
    </>
  );
}
