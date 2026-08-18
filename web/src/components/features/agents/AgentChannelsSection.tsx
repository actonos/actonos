import { Bot, Check, Phone, Radio, Send, Sliders } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { SegmentedControl } from '@/components/ui/SegmentedControl';

const channels = [
  { id: 'telegram', icon: Send },
  { id: 'discord', icon: Bot },
  { id: 'whatsapp', icon: Phone },
  { id: 'webhook', icon: Sliders },
] as const;

export function AgentChannelsSection({
  listenAll,
  selectedChannels,
  onListenModeChange,
  onToggleChannel,
}: {
  listenAll: boolean;
  selectedChannels: string[];
  onListenModeChange: (listenAll: boolean) => void;
  onToggleChannel: (channelID: string) => void;
}) {
  const { t } = useTranslation('agents');
  return (
    <Card className="space-y-6 border border-onyx/10 bg-canvas/90 p-6">
      <div className="border-b border-onyx/5 pb-3">
        <h3 className="flex items-center gap-2 font-serif text-heading-sm text-deep-ink"><Radio className="h-5 w-5" />{t('studio.channels.title')}</h3>
        <p className="text-caption text-slate">{t('studio.channels.description')}</p>
      </div>
      <SegmentedControl
        value={listenAll ? 'all' : 'specific'}
        onChange={(value) => onListenModeChange(value === 'all')}
        label={t('studio.channels.modeLabel')}
        options={[
          { value: 'all', label: t('studio.channels.all') },
          { value: 'specific', label: t('studio.channels.specific') },
        ]}
      />
      <p className="text-body-sm text-slate">{listenAll ? t('studio.channels.allDescription') : t('studio.channels.specificDescription')}</p>
      {!listenAll && (
        <div>
          <p className="mb-3 text-caption font-semibold uppercase tracking-wide text-deep-ink">{t('studio.channels.select')}</p>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {channels.map(({ id, icon: Icon }) => {
              const selected = selectedChannels.includes(id);
              return (
                <button
                  key={id}
                  type="button"
                  aria-pressed={selected}
                  onClick={() => onToggleChannel(id)}
                  className={`flex items-center justify-between rounded-[18px] border p-3.5 text-left transition-colors ${
                    selected ? 'border-deep-ink/30 bg-soft-meadow' : 'border-onyx/10 bg-canvas text-slate'
                  }`}
                >
                  <span className="flex min-w-0 items-center gap-3">
                    <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-deep-ink text-hi-yellow"><Icon className="h-4 w-4" /></span>
                    <span className="min-w-0">
                      <span className="block text-body-sm font-semibold text-deep-ink">{t(`studio.channels.catalog.${id}.name`)}</span>
                      <span className="block text-[11px] text-slate">{t(`studio.channels.catalog.${id}.description`)}</span>
                    </span>
                  </span>
                  <span className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full border-2 ${selected ? 'border-success bg-success text-white' : 'border-onyx/20'}`}>
                    {selected && <Check className="h-3 w-3" />}
                  </span>
                </button>
              );
            })}
          </div>
        </div>
      )}
    </Card>
  );
}
