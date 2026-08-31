import { useTranslation } from 'react-i18next';

export type AgentStudioSection = 'prompt' | 'soul' | 'model' | 'tools' | 'channels' | 'heartbeat' | 'governance' | 'review';

export function AgentStudioNav({
  value,
  modelReady,
  allTools,
  toolCount,
  allChannels,
  channelCount,
  heartbeatActive,
  onChange,
}: {
  value: AgentStudioSection;
  modelReady: boolean;
  allTools: boolean;
  toolCount: number;
  allChannels: boolean;
  channelCount: number;
  heartbeatActive?: boolean;
  onChange: (section: AgentStudioSection) => void;
}) {
  const { t } = useTranslation('agents');
  const items: Array<{ value: AgentStudioSection; label: string }> = [
    { value: 'prompt', label: t('studio.tabs.instructions') },
    { value: 'soul', label: t('studio.tabs.soul') },
    { value: 'model', label: t('studio.tabs.model', { status: modelReady ? t('studio.ready') : t('studio.keyNeeded') }) },
    { value: 'tools', label: t('studio.tabs.tools', { value: allTools ? t('studio.allTools') : toolCount }) },
    { value: 'channels', label: t('studio.tabs.channels', { value: allChannels ? t('studio.all') : channelCount }) },
    { value: 'heartbeat', label: t('studio.tabs.heartbeat', { status: heartbeatActive ? t('studio.active') : t('studio.stopped') }) },
    { value: 'governance', label: t('studio.tabs.governance') },
    { value: 'review', label: t('studio.tabs.review') },
  ];
  return (
    <nav
      role="tablist"
      aria-label={t('studio.tabs.label')}
      className="sticky top-20 z-20 mb-8 flex max-w-full items-center gap-1.5 overflow-x-auto rounded-full border border-onyx/10 bg-canvas/95 p-1 backdrop-blur-sm"
    >
      {items.map((item) => (
        <button
          key={item.value}
          type="button"
          role="tab"
          aria-selected={value === item.value}
          onClick={() => onChange(item.value)}
          className={`shrink-0 whitespace-nowrap rounded-full px-4 py-2 text-caption font-medium transition-colors ${
            value === item.value ? 'bg-deep-ink font-semibold text-white' : 'text-deep-ink hover:bg-soft-meadow'
          }`}
        >
          {item.label}
        </button>
      ))}
    </nav>
  );
}
