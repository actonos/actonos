import { Heart, Activity, Clock, Send, ShieldAlert, Moon, Sparkles } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Input } from '@/components/ui/Input';
import { SegmentedControl } from '@/components/ui/SegmentedControl';
import { useInstalledChannels } from '@/lib/installed-channels';

export interface AgentHeartbeatSectionProps {
  enabled: boolean;
  directives: string;
  intervalMinutes: number;
  targetChannel: string;
  targetAccountID: string;
  activeHoursStart: string;
  activeHoursEnd: string;
  activeHoursTimezone: string;
  onEnabledChange: (enabled: boolean) => void;
  onDirectivesChange: (directives: string) => void;
  onIntervalChange: (interval: number) => void;
  onTargetChannelChange: (channel: string) => void;
  onTargetAccountIDChange: (accountID: string) => void;
  onActiveHoursStartChange: (start: string) => void;
  onActiveHoursEndChange: (end: string) => void;
  onActiveHoursTimezoneChange: (tz: string) => void;
}

const INTERVAL_OPTIONS = [
  { value: 5, label: '5 minutes' },
  { value: 15, label: '15 minutes' },
  { value: 30, label: '30 minutes' },
  { value: 60, label: '1 hour' },
  { value: 120, label: '2 hours' },
  { value: 360, label: '6 hours' },
  { value: 720, label: '12 hours' },
  { value: 1440, label: '24 hours (Daily)' },
];



export function AgentHeartbeatSection({
  enabled,
  directives,
  intervalMinutes,
  targetChannel,
  targetAccountID,
  activeHoursStart,
  activeHoursEnd,
  activeHoursTimezone,
  onEnabledChange,
  onDirectivesChange,
  onIntervalChange,
  onTargetChannelChange,
  onTargetAccountIDChange,
  onActiveHoursStartChange,
  onActiveHoursEndChange,
  onActiveHoursTimezoneChange,
}: AgentHeartbeatSectionProps) {
  const { t } = useTranslation('agents');
  const { channels: pluginChannels } = useInstalledChannels();
  const targetOptions = [
    { id: 'all', label: t('studio.heartbeat.channelAll') },
    ...pluginChannels.map((channel) => ({ id: channel.id, label: channel.label })),
    { id: 'none', label: t('studio.heartbeat.channelNone') },
  ];

  return (
    <Card className="space-y-6 border border-onyx/15 bg-soft-meadow p-6 shadow-xs">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 border-b border-onyx/5 pb-3">
        <div>
          <h3 className="flex items-center gap-2 font-serif text-heading-sm text-deep-ink">
            <Heart className={`h-5 w-5 ${enabled ? 'text-red-500 fill-red-500/20' : 'text-slate'}`} />
            {t('studio.heartbeat.title', 'Autonomous Agent Heartbeat')}
          </h3>
          <p className="text-caption text-slate">
            {t('studio.heartbeat.description', 'Configure scheduled cognitive pulses, proactive health checks, and standing directives dedicated to this agent.')}
          </p>
        </div>
        <Badge variant={enabled ? 'active' : 'stopped'} className="self-start sm:self-auto">
          {enabled ? t('studio.heartbeat.enabledBadge', 'Heartbeat Active') : t('studio.heartbeat.disabledBadge', 'Heartbeat Inactive')}
        </Badge>
      </div>

      <SegmentedControl
        value={enabled ? 'enabled' : 'disabled'}
        onChange={(val) => onEnabledChange(val === 'enabled')}
        label={t('studio.heartbeat.modeLabel', 'Heartbeat Mode')}
        options={[
          { value: 'enabled', label: t('studio.heartbeat.enabled', 'Enabled (Autonomous Pulse)') },
          { value: 'disabled', label: t('studio.heartbeat.disabled', 'Disabled (Manual / Reactive Only)') },
        ]}
      />

      {enabled ? (
        <div className="space-y-5 animate-in fade-in duration-200">
          {/* Standing Directives */}
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <label className="text-caption font-semibold uppercase tracking-wide text-deep-ink flex items-center gap-1.5">
                <Activity className="h-3.5 w-3.5 text-deep-ink" />
                <span>{t('studio.heartbeat.directivesLabel', 'Standing Directives / Autonomous Orders')}</span>
              </label>
              <span className="text-[11px] text-slate font-mono">Markdown</span>
            </div>
            <p className="text-caption text-slate mb-2">
              {t('studio.heartbeat.directivesHelp', 'Specific instructions this agent evaluates on every pulse. When conditions are normal, the agent returns HEARTBEAT_OK with zero notification noise.')}
            </p>
            <textarea
              value={directives}
              onChange={(e) => onDirectivesChange(e.target.value)}
              placeholder={`# Daily Ops Routine\n- Check database connection pool\n- Check error rate in the last hour\n- Alert team on Telegram if error rate > 5%`}
              rows={6}
              className="w-full rounded-2xl border border-onyx/15 bg-canvas p-3.5 font-mono text-body-sm text-deep-ink placeholder:text-slate/60 focus:border-deep-ink focus:outline-none focus:ring-1 focus:ring-deep-ink"
            />
          </div>

          {/* Pulse Schedule & Destination */}
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {/* Interval */}
            <div className="rounded-2xl border border-onyx/10 bg-canvas p-4 space-y-2">
              <label className="text-caption font-semibold text-deep-ink flex items-center gap-1.5">
                <Clock className="h-4 w-4 text-slate" />
                <span>{t('studio.heartbeat.intervalLabel', 'Pulse Interval')}</span>
              </label>
              <select
                value={intervalMinutes || 60}
                onChange={(e) => onIntervalChange(Number(e.target.value))}
                className="w-full rounded-xl border border-onyx/15 bg-canvas px-3 py-2 text-body-sm text-deep-ink focus:border-deep-ink focus:outline-none"
              >
                {INTERVAL_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
              <p className="text-[11px] text-slate">
                {t('studio.heartbeat.intervalHelp', 'How frequently the daemon wakes this agent for a cognitive evaluation.')}
              </p>
            </div>

            {/* Target Notification Channel */}
            <div className="rounded-2xl border border-onyx/10 bg-canvas p-4 space-y-2">
              <label className="text-caption font-semibold text-deep-ink flex items-center gap-1.5">
                <Send className="h-4 w-4 text-slate" />
                <span>{t('studio.heartbeat.channelLabel', 'Alert Destination Channel')}</span>
              </label>
              <select
                value={targetChannel || 'all'}
                onChange={(e) => onTargetChannelChange(e.target.value)}
                className="w-full rounded-xl border border-onyx/15 bg-canvas px-3 py-2 text-body-sm text-deep-ink focus:border-deep-ink focus:outline-none"
              >
                {targetOptions.map((ch) => (
                  <option key={ch.id} value={ch.id}>
                    {ch.label}
                  </option>
                ))}
              </select>
              <p className="text-[11px] text-slate">
                {t('studio.heartbeat.channelHelp', 'Where non-silent alert summaries and task completion notifications will be sent.')}
              </p>
            </div>
          </div>

          {/* Advanced: Target Account & Active Hours */}
          <div className="rounded-2xl border border-onyx/10 bg-canvas/70 p-4 space-y-4">
            <div className="flex items-center gap-2 text-body-sm font-semibold text-deep-ink">
              <Moon className="h-4 w-4 text-slate" />
              <span>{t('studio.heartbeat.activeHoursTitle', 'Active Hours & Account Scoping (Optional)')}</span>
            </div>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <div>
                <label className="text-[11px] font-semibold text-slate uppercase block mb-1">
                  {t('studio.heartbeat.startHour', 'Active Start (HH:MM)')}
                </label>
                <Input
                  type="time"
                  value={activeHoursStart || ''}
                  onChange={(e) => onActiveHoursStartChange(e.target.value)}
                  placeholder="08:00"
                />
              </div>
              <div>
                <label className="text-[11px] font-semibold text-slate uppercase block mb-1">
                  {t('studio.heartbeat.endHour', 'Active End (HH:MM)')}
                </label>
                <Input
                  type="time"
                  value={activeHoursEnd || ''}
                  onChange={(e) => onActiveHoursEndChange(e.target.value)}
                  placeholder="22:00"
                />
              </div>
              <div>
                <label className="text-[11px] font-semibold text-slate uppercase block mb-1">
                  {t('studio.heartbeat.timezone', 'Timezone')}
                </label>
                <Input
                  value={activeHoursTimezone || ''}
                  onChange={(e) => onActiveHoursTimezoneChange(e.target.value)}
                  placeholder="e.g. Asia/Ho_Chi_Minh"
                />
              </div>
              <div>
                <label className="text-[11px] font-semibold text-slate uppercase block mb-1">
                  {t('studio.heartbeat.targetAccount', 'Specific Account ID')}
                </label>
                <Input
                  value={targetAccountID || ''}
                  onChange={(e) => onTargetAccountIDChange(e.target.value)}
                  placeholder="e.g. tg_support_bot"
                />
              </div>
            </div>

            <div className="flex items-start gap-2 text-caption text-slate bg-soft-meadow p-3 rounded-xl">
              <Sparkles className="h-4 w-4 text-amber-500 shrink-0 mt-0.5" />
              <p className="leading-relaxed">
                {t('studio.heartbeat.zeroNoiseNote', 'Zero-Noise Policy: If all systems are nominal and directives find nothing requiring operator intervention, this agent logs the pulse and remains completely silent.')}
              </p>
            </div>
          </div>
        </div>
      ) : (
        <div className="py-6 text-center bg-canvas/60 rounded-2xl border border-onyx/5">
          <ShieldAlert className="h-8 w-8 text-slate/50 mx-auto mb-2" />
          <p className="text-body-sm text-deep-ink font-semibold mb-1">
            {t('studio.heartbeat.disabledNoticeTitle', 'Heartbeat is currently turned off for this agent')}
          </p>
          <p className="text-caption text-slate max-w-md mx-auto">
            {t('studio.heartbeat.disabledNoticeDesc', 'Enable heartbeat if you want this agent to independently wake up on a schedule, inspect standing directives, and push notifications when needed.')}
          </p>
        </div>
      )}
    </Card>
  );
}
