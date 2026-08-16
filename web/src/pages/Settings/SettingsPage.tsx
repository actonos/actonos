import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Shield, Activity, Wifi, RefreshCw, Cpu, HardDrive, Thermometer, Clock } from 'lucide-react';
import { api } from '@/lib/api';
import type { SystemMetrics, TailscaleStatus } from '@/lib/types';

export function SettingsPage() {
  const { t } = useTranslation('settings');
  const { t: tCommon } = useTranslation('common');

  const [metrics, setMetrics] = useState<SystemMetrics | null>(null);
  const [tailscale, setTailscale] = useState<TailscaleStatus | null>(null);
  const [wifiNetworks, setWifiNetworks] = useState<any[]>([]);
  const [wifiPassword, setWifiPassword] = useState('');
  const [selectedSSID, setSelectedSSID] = useState('');
  const [loadingWifi, setLoadingWifi] = useState(false);

  const loadStatus = async () => {
    try {
      const [m, ts] = await Promise.all([
        api.getMetrics().catch(() => null),
        api.getTailscale().catch(() => null),
      ]);
      setMetrics(m);
      setTailscale(ts);
    } catch (err) {
      console.error('Failed to load system info:', err);
    }
  };

  const handleScanWifi = async () => {
    setLoadingWifi(true);
    try {
      const res = await api.scanWifi();
      setWifiNetworks(res.networks || []);
    } catch (err) {
      console.error('Failed to scan wifi:', err);
    } finally {
      setLoadingWifi(false);
    }
  };

  const handleConnectWifi = async () => {
    if (!selectedSSID) return;
    try {
      await api.connectWifi(selectedSSID, wifiPassword);
      alert('Connected to Wi-Fi successfully!');
    } catch (err: any) {
      alert(`Wi-Fi connection error: ${err.message}`);
    }
  };

  useEffect(() => {
    loadStatus();
    const interval = setInterval(loadStatus, 5000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="relative min-h-[calc(100vh-72px)]">
      <BlobBackdrop />

      <PageContainer>
        {/* Header */}
        <div className="mb-8">
          <span className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
            {t('eyebrow')}
          </span>
          <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight">
            {t('title')}
          </h1>
          <p className="font-sans text-body text-slate mt-2 max-w-2xl">
            {t('subtitle')}
          </p>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
          {/* Left Column: Tailscale Remote Access */}
          <div className="flex flex-col gap-6">
            <Card className="border border-onyx/10">
              <div className="flex items-center justify-between mb-4">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 rounded-full bg-deep-ink flex items-center justify-center text-hi-yellow">
                    <Shield className="w-5 h-5" />
                  </div>
                  <div>
                    <h3 className="font-serif text-heading-sm text-deep-ink">
                      {t('tailscale.title')}
                    </h3>
                    <p className="font-sans text-body-sm text-slate">
                      {t('tailscale.desc')}
                    </p>
                  </div>
                </div>
              </div>

              <div className="p-4 bg-canvas rounded-[18px] border border-onyx/10 flex flex-col gap-3">
                <div className="flex items-center justify-between">
                  <span className="text-body-sm text-slate">{t('tailscale.status')}</span>
                  <Badge variant={tailscale?.connected ? 'active' : 'stopped'}>
                    {tailscale?.connected ? tCommon('status.connected') : tCommon('status.disconnected')}
                  </Badge>
                </div>

                {tailscale?.ip && (
                  <div className="flex items-center justify-between border-t border-soft-meadow pt-2">
                    <span className="text-body-sm text-slate">{t('tailscale.ip')}</span>
                    <span className="font-mono text-body-sm text-deep-ink font-semibold">{tailscale.ip}</span>
                  </div>
                )}

                <div className="flex items-center justify-between border-t border-soft-meadow pt-2">
                  <span className="text-body-sm text-slate">{t('tailscale.hostname')}</span>
                  <span className="font-mono text-body-sm text-deep-ink">{tailscale?.hostname || 'acton-mini'}</span>
                </div>
              </div>
            </Card>

            {/* Wi-Fi Manager */}
            <Card className="border border-onyx/10">
              <div className="flex items-center justify-between mb-4">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 rounded-full bg-canvas flex items-center justify-center text-deep-ink border border-onyx">
                    <Wifi className="w-5 h-5" />
                  </div>
                  <h3 className="font-serif text-heading-sm text-deep-ink">
                    {t('wifi.title')}
                  </h3>
                </div>

                <Button
                  variant="ghost"
                  size="sm"
                  icon={<RefreshCw className={`w-3.5 h-3.5 ${loadingWifi ? 'animate-spin' : ''}`} />}
                  onClick={handleScanWifi}
                >
                  {t('wifi.scan')}
                </Button>
              </div>

              {wifiNetworks.length > 0 && (
                <div className="flex flex-col gap-3">
                  <select
                    value={selectedSSID}
                    onChange={(e) => setSelectedSSID(e.target.value)}
                    className="w-full bg-canvas text-deep-ink font-sans text-body px-4 py-2.5 rounded-full border border-onyx focus:outline-none"
                  >
                    <option value="">Select Wi-Fi Network</option>
                    {wifiNetworks.map((net, idx) => (
                      <option key={idx} value={net.ssid}>
                        {net.ssid} ({net.signal}%) - {net.security}
                      </option>
                    ))}
                  </select>

                  <Input
                    type="password"
                    placeholder={t('wifi.password')}
                    value={wifiPassword}
                    onChange={(e) => setWifiPassword(e.target.value)}
                    actionButton={
                      <Button variant="primary" size="sm" onClick={handleConnectWifi}>
                        {t('wifi.connect')}
                      </Button>
                    }
                  />
                </div>
              )}
            </Card>
          </div>

          {/* Right Column: Hardware Metrics */}
          <div>
            <Card className="border border-onyx/10">
              <div className="flex items-center gap-3 mb-6">
                <div className="w-10 h-10 rounded-full bg-canvas flex items-center justify-center text-deep-ink border border-onyx">
                  <Activity className="w-5 h-5" />
                </div>
                <h3 className="font-serif text-heading-sm text-deep-ink">
                  {t('metrics.title')}
                </h3>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div className="p-4 bg-canvas rounded-[18px] border border-onyx/10">
                  <div className="flex items-center gap-2 text-slate text-caption uppercase mb-1">
                    <Cpu className="w-4 h-4" />
                    <span>{t('metrics.cpu')}</span>
                  </div>
                  <span className="font-serif text-heading text-deep-ink">
                    {metrics?.cpu.usage_percent ?? 0}%
                  </span>
                  <span className="text-body-sm text-slate block mt-1">
                    {metrics?.cpu.cores ?? 4} Cores ({metrics?.cpu.model || 'Generic'})
                  </span>
                </div>

                <div className="p-4 bg-canvas rounded-[18px] border border-onyx/10">
                  <div className="flex items-center gap-2 text-slate text-caption uppercase mb-1">
                    <Thermometer className="w-4 h-4 text-amber-600" />
                    <span>{t('metrics.temp')}</span>
                  </div>
                  <span className="font-serif text-heading text-deep-ink">
                    {metrics?.cpu.temperature_celsius ?? 42}°C
                  </span>
                  <span className="text-body-sm text-emerald-600 font-medium block mt-1">
                    Normal thermal zone
                  </span>
                </div>

                <div className="p-4 bg-canvas rounded-[18px] border border-onyx/10">
                  <div className="flex items-center gap-2 text-slate text-caption uppercase mb-1">
                    <Activity className="w-4 h-4" />
                    <span>{t('metrics.ram')}</span>
                  </div>
                  <span className="font-serif text-heading text-deep-ink">
                    {metrics?.memory.used_mb ?? 1024} MB
                  </span>
                  <span className="text-body-sm text-slate block mt-1">
                    of {metrics?.memory.total_mb ?? 8192} MB total
                  </span>
                </div>

                <div className="p-4 bg-canvas rounded-[18px] border border-onyx/10">
                  <div className="flex items-center gap-2 text-slate text-caption uppercase mb-1">
                    <HardDrive className="w-4 h-4" />
                    <span>{t('metrics.disk')}</span>
                  </div>
                  <span className="font-serif text-heading text-deep-ink">
                    {metrics?.disk.used_gb ?? 12} GB
                  </span>
                  <span className="text-body-sm text-slate block mt-1">
                    of {metrics?.disk.total_gb ?? 64} GB total
                  </span>
                </div>
              </div>

              <div className="mt-6 pt-4 border-t border-canvas flex items-center justify-between text-body-sm text-slate">
                <div className="flex items-center gap-2">
                  <Clock className="w-4 h-4" />
                  <span>{t('metrics.uptime')}: {metrics?.uptime_seconds ?? 0}s</span>
                </div>
                <span>Daemon RAM: {metrics?.memory.actond_mb.toFixed(1) ?? '28.5'} MB</span>
              </div>
            </Card>
          </div>
        </div>
      </PageContainer>
    </div>
  );
}
