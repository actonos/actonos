import { Clock, Cpu, HardDrive, HeartPulse, MemoryStick, Thermometer } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { DashboardSummaryData } from '@/lib/api';
import type { HealthReport } from '@/lib/types';
import { Badge } from '@/components/ui/Badge';
import { MetricCard } from '@/components/ui/MetricCard';
import { SUPERVISOR_COMPONENTS, componentStatus, supervisorTone } from '@/lib/health';

export function SystemHealthStrip({
  data,
  health,
}: {
  data: DashboardSummaryData | null;
  health?: HealthReport | null;
}) {
  const { t } = useTranslation('dashboard');
  const cpu = data?.metrics?.cpu;
  const memory = data?.metrics?.memory;
  const disk = data?.metrics?.disk;
  const ramPercent = Math.min(100, Math.round(((memory?.used_mb ?? 0) / Math.max(memory?.total_mb ?? 1, 1)) * 100));
  const diskPercent = Math.min(100, Math.round(((disk?.used_gb ?? 0) / Math.max(disk?.total_gb ?? 1, 1)) * 100));
  const uptimeMins = Math.floor((data?.metrics?.uptime_seconds ?? 0) / 60);
  const overallTone = supervisorTone(health?.status);
  return (
    <section aria-labelledby="system-health-title" className="mb-8">
      <h2 id="system-health-title" className="sr-only">{t('health.title')}</h2>
      {health && (
        <div className="mb-4 flex flex-wrap items-center gap-2 rounded-[24px] border border-onyx/10 bg-soft-meadow p-3">
          <HeartPulse className="h-4 w-4 text-deep-ink" aria-hidden="true" />
          <Badge variant={overallTone === 'success' ? 'success' : overallTone === 'warning' ? 'warning' : overallTone === 'danger' ? 'danger' : 'neutral'}>
            {t(`health.status.${health.status}`, health.status)}
          </Badge>
          {SUPERVISOR_COMPONENTS.map((key) => {
            const status = componentStatus(health, key);
            const tone = supervisorTone(status);
            return (
              <Badge
                key={key}
                variant={tone === 'success' ? 'success' : tone === 'warning' ? 'warning' : tone === 'danger' ? 'danger' : 'neutral'}
              >
                {t(`health.components.${key}`)}: {t(`health.status.${status}`, status)}
              </Badge>
            );
          })}
        </div>
      )}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <MetricCard
          label={t('gauges.cpu')}
          value={`${(cpu?.usage_percent ?? 0).toFixed(1)}%`}
          detail={t('gauges.cores', { count: cpu?.cores ?? 0 })}
          icon={Cpu}
          progress={cpu?.usage_percent ?? 0}
          tone={(cpu?.usage_percent ?? 0) > 85 ? 'danger' : 'neutral'}
        />
        <MetricCard
          label={t('gauges.memory')}
          value={t('units.megabytes', { value: memory?.used_mb ?? 0 })}
          detail={t('gauges.ofMegabytes', { value: memory?.total_mb ?? 0 })}
          icon={MemoryStick}
          progress={ramPercent}
          tone={ramPercent > 85 ? 'warning' : 'success'}
        />
        <MetricCard
          label={t('gauges.temperature')}
          value={`${(cpu?.temperature_celsius ?? 0).toFixed(1)}°C`}
          detail={t('gauges.cores', { count: cpu?.cores ?? 0 })}
          icon={Thermometer}
          progress={cpu?.temperature_celsius ?? 0}
          tone={(cpu?.temperature_celsius ?? 0) > 80 ? 'danger' : 'success'}
        />
        <MetricCard
          label={t('gauges.uptime')}
          value={t('gauges.uptimeMinutes', { value: uptimeMins })}
          detail={t('gauges.dataSize', { value: disk?.data_dir_gb?.toFixed(2) ?? '0' })}
          icon={disk ? HardDrive : Clock}
          progress={diskPercent}
          tone={diskPercent > 90 ? 'warning' : 'neutral'}
        />
      </div>
    </section>
  );
}
