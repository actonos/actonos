import { Check, Radio } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { SegmentedControl } from '@/components/ui/SegmentedControl';
import { EmptyState } from '@/components/ui/EmptyState';
import { useInstalledChannels } from '@/lib/installed-channels';

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
  const { t } = useTranslation(['agents', 'channels']);
  const { channels } = useInstalledChannels();
  return (
    <Card className="space-y-6 border border-onyx/15 bg-soft-meadow p-6 shadow-xs">
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
          {channels.length === 0 ? (
            <EmptyState
              compact
              icon={<Radio className="h-6 w-6" />}
              title={t('channels:accounts.noPluginsTitle')}
              description={t('channels:accounts.noPluginsDescription')}
            />
          ) : (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {channels.map((channel) => {
              const selected = selectedChannels.includes(channel.id);
              return (
                <button
                  key={channel.id}
                  type="button"
                  aria-pressed={selected}
                  onClick={() => onToggleChannel(channel.id)}
                  className={`flex items-center justify-between rounded-[20px] p-3.5 text-left transition-all cursor-pointer ${
                    selected
                      ? 'border-2 border-deep-ink bg-canvas shadow-xs ring-1 ring-deep-ink/10'
                      : 'border border-onyx/15 bg-canvas/60 text-slate opacity-75 hover:opacity-100 hover:border-onyx/35 hover:bg-canvas'
                  }`}
                >
                  <span className="flex min-w-0 items-center gap-3">
                    <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-deep-ink text-hi-yellow"><Radio className="h-4 w-4" /></span>
                    <span className="min-w-0">
                      <span className="block text-body-sm font-semibold text-deep-ink">{channel.label}</span>
                      <span className="block text-[11px] font-mono text-slate">{channel.id}</span>
                    </span>
                  </span>
                  <span className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full border-2 transition-all ${selected ? 'border-deep-ink bg-deep-ink text-hi-yellow' : 'border-onyx/25'}`}>
                    {selected && <Check className="h-3 w-3 stroke-[3]" />}
                  </span>
                </button>
              );
            })}
          </div>
          )}
        </div>
      )}
    </Card>
  );
}
