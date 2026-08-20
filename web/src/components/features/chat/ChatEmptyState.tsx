import { Sparkles, Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export function ChatEmptyState({
  agentName,
  prompts,
  onPrompt,
}: {
  agentName: string;
  prompts: string[];
  onPrompt: (prompt: string) => void;
}) {
  const { t } = useTranslation('chat');
  return (
    <div className="py-16 text-center text-slate">
      <div className="mx-auto mb-3 flex h-14 w-14 items-center justify-center rounded-full border border-onyx/10 bg-soft-meadow">
        <Sparkles className="h-7 w-7 text-hi-yellow" aria-hidden="true" />
      </div>
      <h4 className="mb-1 font-serif text-heading-sm text-deep-ink">
        {t('startConversation', { name: agentName })}
      </h4>
      <p className="mx-auto mb-6 max-w-md font-sans text-body-sm text-slate">{t('startDescription')}</p>
      <div className="mx-auto flex max-w-lg flex-wrap justify-center gap-2">
        {prompts.map((prompt) => (
          <button
            key={prompt}
            type="button"
            onClick={() => onPrompt(prompt)}
            className="flex items-center gap-1.5 rounded-full border border-onyx/10 bg-soft-meadow px-3.5 py-1.5 text-caption font-medium text-deep-ink transition-colors hover:bg-canvas"
          >
            <Zap className="h-3 w-3 text-hi-yellow" aria-hidden="true" />
            <span>{prompt}</span>
          </button>
        ))}
      </div>
    </div>
  );
}
