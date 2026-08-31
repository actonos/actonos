import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { useToast } from '@/components/ui/Toast';
import { getGroupedTimezones } from '@/lib/timezones';
import { api } from '@/lib/api';
import type { NotificationPreferences } from '@/lib/types';
import {
  Moon,
  Sparkles,
  Bell,
  Save,
  Zap,
} from 'lucide-react';

export interface NotificationPreferencesModalProps {
  isOpen: boolean;
  onClose: () => void;
  onPreferencesUpdated?: () => void;
}

export function NotificationPreferencesModal({
  isOpen,
  onClose,
  onPreferencesUpdated,
}: NotificationPreferencesModalProps) {
  const { t } = useTranslation('notifications');
  const { success, error, info } = useToast();

  const [prefs, setPrefs] = useState<NotificationPreferences>({
    quiet_hours_enabled: false,
    quiet_hours_start: '22:00',
    quiet_hours_end: '07:00',
    quiet_hours_timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
    daily_digest_enabled: false,
    daily_digest_time: '08:00',
    min_push_severity: 'info',
  });

  const [saving, setSaving] = useState(false);
  const [triggeringDigest, setTriggeringDigest] = useState(false);

  const timezones = getGroupedTimezones();

  useEffect(() => {
    if (!isOpen) return;
    const loadPrefs = async () => {
      try {
        const data = await api.getNotificationPreferences();
        setPrefs(data);
      } catch {
        // Defaults preserved
      }
    };
    loadPrefs();
  }, [isOpen]);

  const handleSave = async () => {
    setSaving(true);
    try {
      await api.saveNotificationPreferences(prefs);
      success(
        t('preferences.savedTitle', 'Preferences Saved'),
        t('preferences.savedDesc', 'Smart notification routing and quiet hours schedule updated.')
      );
      onPreferencesUpdated?.();
      onClose();
    } catch (err) {
      error('Failed to save preferences', String(err));
    } finally {
      setSaving(false);
    }
  };

  const handleTriggerTestDigest = async () => {
    setTriggeringDigest(true);
    try {
      await api.triggerDailyDigest();
      info(
        t('preferences.digestTriggered', 'Daily Digest Generated'),
        t('preferences.digestTriggeredDesc', 'A 24-hour executive summary notification was added to your inbox.')
      );
      onPreferencesUpdated?.();
    } catch (err) {
      error('Digest Generation Failed', String(err));
    } finally {
      setTriggeringDigest(false);
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('preferences.modalTitle', 'Smart Notifications & Fatigue Control')}
      maxWidth="max-w-lg"
    >
      <div className="space-y-3.5">
        <p className="text-caption text-slate -mt-2">
          {t('preferences.modalDesc', 'Configure intelligent filtering, quiet hours push suppression, and automated 24-hour executive digest.')}
        </p>

        {/* 1. Quiet Hours Section */}
        <div className="p-3.5 rounded-2xl border border-onyx/10 bg-soft-meadow/30 space-y-2.5">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <div className="p-1.5 rounded-lg bg-soft-meadow border border-onyx/10 text-deep-ink">
                <Moon className="w-3.5 h-3.5" />
              </div>
              <div>
                <h4 className="text-body-sm font-bold text-deep-ink leading-tight">
                  {t('preferences.quietHoursTitle', 'Quiet Hours Schedule')}
                </h4>
                <p className="text-[11px] text-slate">
                  {t('preferences.quietHoursDesc', 'Mutes non-critical push alerts during focus/sleep.')}
                </p>
              </div>
            </div>

            <input
              type="checkbox"
              id="quietHoursToggle"
              checked={prefs.quiet_hours_enabled}
              onChange={(e) => setPrefs({ ...prefs, quiet_hours_enabled: e.target.checked })}
              className="rounded border-onyx/20 text-deep-ink focus:ring-deep-ink cursor-pointer w-4 h-4"
            />
          </div>

          {prefs.quiet_hours_enabled && (
            <div className="pt-2 border-t border-onyx/5 grid grid-cols-2 sm:grid-cols-3 gap-2">
              <div>
                <label className="text-[10px] uppercase font-semibold text-slate block mb-1">
                  {t('preferences.quietStart', 'Start Time')}
                </label>
                <Input
                  type="time"
                  value={prefs.quiet_hours_start}
                  onChange={(e) => setPrefs({ ...prefs, quiet_hours_start: e.target.value })}
                  className="text-xs h-8 py-1"
                />
              </div>

              <div>
                <label className="text-[10px] uppercase font-semibold text-slate block mb-1">
                  {t('preferences.quietEnd', 'End Time')}
                </label>
                <Input
                  type="time"
                  value={prefs.quiet_hours_end}
                  onChange={(e) => setPrefs({ ...prefs, quiet_hours_end: e.target.value })}
                  className="text-xs h-8 py-1"
                />
              </div>

              <div className="col-span-2 sm:col-span-1">
                <label className="text-[10px] uppercase font-semibold text-slate block mb-1">
                  {t('preferences.timezone', 'Timezone')}
                </label>
                <select
                  value={prefs.quiet_hours_timezone}
                  onChange={(e) => setPrefs({ ...prefs, quiet_hours_timezone: e.target.value })}
                  className="w-full text-xs font-mono bg-canvas border border-onyx/15 rounded-xl px-2 py-1.5 text-deep-ink focus:outline-none focus:ring-1 focus:ring-deep-ink cursor-pointer h-8"
                >
                  {timezones.map((g) => (
                    <optgroup key={g.region} label={g.region}>
                      {g.zones.map((z) => (
                        <option key={z.value} value={z.value}>
                          {z.label}
                        </option>
                      ))}
                    </optgroup>
                  ))}
                </select>
              </div>
            </div>
          )}
        </div>

        {/* 2. Daily Digest Section */}
        <div className="p-3.5 rounded-2xl border border-onyx/10 bg-soft-meadow/30 space-y-2.5">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <div className="p-1.5 rounded-lg bg-soft-meadow border border-onyx/10 text-deep-ink">
                <Sparkles className="w-3.5 h-3.5" />
              </div>
              <div>
                <h4 className="text-body-sm font-bold text-deep-ink leading-tight">
                  {t('preferences.dailyDigestTitle', 'Daily Executive Digest')}
                </h4>
                <p className="text-[11px] text-slate">
                  {t('preferences.dailyDigestDesc', 'Aggregates 24-hour tasks, errors, and tokens into 1 briefing.')}
                </p>
              </div>
            </div>

            <input
              type="checkbox"
              id="dailyDigestToggle"
              checked={prefs.daily_digest_enabled}
              onChange={(e) => setPrefs({ ...prefs, daily_digest_enabled: e.target.checked })}
              className="rounded border-onyx/20 text-deep-ink focus:ring-deep-ink cursor-pointer w-4 h-4"
            />
          </div>

          {prefs.daily_digest_enabled && (
            <div className="flex items-center justify-between pt-2 border-t border-onyx/5 gap-2">
              <div className="flex items-center gap-2">
                <label className="text-[11px] font-semibold text-deep-ink whitespace-nowrap">
                  {t('preferences.digestTime', 'Delivery Time')}:
                </label>
                <Input
                  type="time"
                  value={prefs.daily_digest_time}
                  onChange={(e) => setPrefs({ ...prefs, daily_digest_time: e.target.value })}
                  className="text-xs w-28 h-8 py-1"
                />
              </div>

              <Button
                variant="ghost"
                size="sm"
                onClick={handleTriggerTestDigest}
                disabled={triggeringDigest}
                icon={<Zap className="w-3.5 h-3.5 text-amber-600" />}
                className="text-xs py-1 px-2.5 h-8 font-medium"
              >
                {triggeringDigest ? 'Generating...' : t('preferences.testDigest', 'Generate Now')}
              </Button>
            </div>
          )}
        </div>

        {/* 3. Minimum Push Severity */}
        <div className="p-3.5 rounded-2xl border border-onyx/10 bg-soft-meadow/30 space-y-2">
          <div className="flex items-center gap-2">
            <div className="p-1.5 rounded-lg bg-soft-meadow border border-onyx/10 text-deep-ink">
              <Bell className="w-3.5 h-3.5" />
            </div>
            <div>
              <h4 className="text-body-sm font-bold text-deep-ink leading-tight">
                {t('preferences.minSeverityTitle', 'Minimum Push Severity')}
              </h4>
              <p className="text-[11px] text-slate">
                {t('preferences.minSeverityDesc', 'Filters background Service Worker push alerts.')}
              </p>
            </div>
          </div>

          <div className="grid grid-cols-3 gap-2 pt-1.5">
            {[
              { id: 'info' as const, label: t('preferences.severityInfo', 'All (Info+)'), desc: 'All pulses' },
              { id: 'warning' as const, label: t('preferences.severityWarning', 'Warnings+'), desc: 'Approvals' },
              { id: 'critical' as const, label: t('preferences.severityCritical', 'Critical'), desc: 'Failures' },
            ].map((sev) => (
              <button
                key={sev.id}
                type="button"
                onClick={() => setPrefs({ ...prefs, min_push_severity: sev.id })}
                className={`p-2 rounded-xl border text-left transition-all cursor-pointer ${
                  prefs.min_push_severity === sev.id
                    ? 'border-deep-ink bg-canvas ring-1 ring-deep-ink/20 shadow-2xs font-semibold'
                    : 'border-onyx/10 bg-canvas/60 hover:bg-canvas text-slate'
                }`}
              >
                <div className="text-[11px] text-deep-ink font-semibold">{sev.label}</div>
                <div className="text-[10px] text-slate">{sev.desc}</div>
              </button>
            ))}
          </div>
        </div>

        {/* Footer Actions */}
        <div className="flex items-center justify-end gap-2.5 pt-2 border-t border-onyx/10">
          <Button variant="ghost" size="sm" onClick={onClose} className="px-3">
            {t('common.cancel', 'Cancel')}
          </Button>

          <Button
            variant="primary"
            size="sm"
            onClick={handleSave}
            disabled={saving}
            icon={<Save className="w-3.5 h-3.5" />}
            className="px-4 shadow-xs"
          >
            {saving ? t('preferences.saving', 'Saving...') : t('preferences.saveBtn', 'Save Preferences')}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
