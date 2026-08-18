import { Clock, Cpu, HardDrive, MemoryStick, Thermometer } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { DashboardSummaryData } from '@/lib/api';
import { MetricCard } from '@/components/ui/MetricCard';

export function SystemHealthStrip({ data }: { data: DashboardSummaryData | null }) {
  const { t } = useTranslation('dashboard');
  const cpu = data?.metrics?.cpu;
  const memory = data?.metrics?.memory;
  const disk = data?.metrics?.disk;
  const ramPercent = Math.min(100, Math.round(((memory?.used_mb ?? 0) / Math.max(memory?.total_mb ?? 1, 1)) * 100));
  const diskPercent = Math.min(100, Math.round(((disk?.used_gb ?? 0) / Math.max(disk?.total_gb ?? 1, 1)) * 100));
  const uptimeMins = Math.floor((data?.metrics?.uptime_seconds ?? 0) / 60);
  return (
    <section aria-labelledby="system-health-title" className="mb-8">
      <h2 id="system-health-title" className="sr-only">{t('health.title')}</h2>
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
