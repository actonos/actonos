import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { useToast } from '@/components/ui/Toast';
import { api } from '@/lib/api';
import type { ProactiveConfig } from '@/lib/types';
import { ShieldAlert, Activity } from 'lucide-react';

export interface ProactiveConfigModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSaved?: () => void;
}

export function ProactiveConfigModal({ isOpen, onClose, onSaved }: ProactiveConfigModalProps) {
  const { t } = useTranslation('operations');
  const { success, error } = useToast();

  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [config, setConfig] = useState<ProactiveConfig>({
    enabled: true,
    scan_interval_minutes: 15,
    auto_create_tasks: false,
    disk_threshold_percent: 80,
    global_kill_switch: false,
  });

  useEffect(() => {
    if (!isOpen) return;
    let cancelled = false;
    setLoading(true);
    api
      .getProactiveConfig()
      .then((res) => {
        if (!cancelled && res) {
          setConfig(res);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          error(t('anomalies.actionFailed'), err instanceof Error ? err.message : String(err));
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [isOpen, error, t]);

  const handleSave = async () => {
    setSaving(true);
    try {
      await api.updateProactiveConfig(config);
      success(t('anomalies.configSaved'));
      onSaved?.();
      onClose();
    } catch (err) {
      error(t('anomalies.actionFailed'), err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('anomalies.configTitle')}
      maxWidth="max-w-xl"
    >
      <div className="space-y-5">
        <p className="text-caption text-slate leading-relaxed">
          {t('anomalies.configSubtitle')}
        </p>

        {loading ? (
          <div className="py-10 text-center text-caption text-slate animate-pulse">
            {t('anomalies.saving')}
          </div>
        ) : (
          <div className="space-y-4">
            {/* Enabled Switch */}
            <div className="flex items-center justify-between p-3.5 rounded-[18px] bg-soft-meadow border border-onyx/10">
              <div className="flex items-center gap-3">
                <Activity className="w-5 h-5 text-deep-ink" />
                <div>
                  <label className="text-body-sm font-semibold text-deep-ink block cursor-pointer" htmlFor="proactive-enable">
                    {t('anomalies.enabled')}
                  </label>
                </div>
              </div>
              <input
                id="proactive-enable"
                type="checkbox"
                checked={config.enabled}
                onChange={(e) => setConfig({ ...config, enabled: e.target.checked })}
                className="w-5 h-5 rounded accent-deep-ink cursor-pointer"
              />
            </div>

            {/* Scan interval input */}
            <div>
              <label className="block text-caption font-semibold text-deep-ink mb-1">
                {t('anomalies.scanInterval')}
              </label>
              <Input
                type="number"
                min={1}
                max={1440}
                value={config.scan_interval_minutes}
                onChange={(e) =>
                  setConfig({
                    ...config,
                    scan_interval_minutes: Math.max(1, parseInt(e.target.value, 10) || 15),
                  })
                }
                disabled={!config.enabled}
              />
            </div>

            {/* Disk threshold input */}
            <div>
              <label className="block text-caption font-semibold text-deep-ink mb-1">
                {t('anomalies.diskThreshold')}
              </label>
              <Input
                type="number"
                min={10}
                max={99}
                value={config.disk_threshold_percent}
                onChange={(e) =>
                  setConfig({
                    ...config,
                    disk_threshold_percent: Math.min(99, Math.max(10, parseFloat(e.target.value) || 80)),
                  })
                }
                disabled={!config.enabled}
              />
            </div>

            {/* Auto Create Missions */}
            <div className="p-3.5 rounded-[18px] bg-soft-meadow border border-onyx/10 flex items-start justify-between gap-3">
              <div className="flex-1">
                <label className="text-body-sm font-semibold text-deep-ink block cursor-pointer" htmlFor="proactive-autotask">
                  {t('anomalies.autoCreateTasks')}
                </label>
                <p className="text-[11px] text-slate mt-0.5">
                  {t('anomalies.autoCreateTasksDesc')}
                </p>
              </div>
              <input
                id="proactive-autotask"
                type="checkbox"
                checked={config.auto_create_tasks}
                onChange={(e) => setConfig({ ...config, auto_create_tasks: e.target.checked })}
                disabled={!config.enabled}
                className="w-5 h-5 rounded accent-deep-ink cursor-pointer mt-0.5"
              />
            </div>

            {/* Global Kill Switch */}
            <div className="p-3.5 rounded-[18px] bg-hi-yellow/15 border border-onyx/15 flex items-start justify-between gap-3">
              <div className="flex-1">
                <div className="flex items-center gap-2">
                  <ShieldAlert className="w-4 h-4 text-deep-ink" />
                  <label className="text-body-sm font-bold text-deep-ink block cursor-pointer" htmlFor="proactive-killswitch">
                    {t('anomalies.globalKillSwitch')}
                  </label>
                </div>
                <p className="text-[11px] text-slate mt-0.5">
                  {t('anomalies.globalKillSwitchDesc')}
                </p>
              </div>
              <input
                id="proactive-killswitch"
                type="checkbox"
                checked={config.global_kill_switch}
                onChange={(e) => setConfig({ ...config, global_kill_switch: e.target.checked })}
                className="w-5 h-5 rounded accent-deep-ink cursor-pointer mt-0.5"
              />
            </div>
          </div>
        )}

        <div className="flex justify-end gap-2 pt-2 border-t border-onyx/10">
          <Button variant="ghost" onClick={onClose} disabled={saving}>
            {t('anomalies.cancel')}
          </Button>
          <Button variant="primary" onClick={handleSave} disabled={saving || loading}>
            {saving ? t('anomalies.saving') : t('anomalies.saveConfig')}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
