import { useEffect, useRef, type FormEvent, type KeyboardEvent, type RefObject } from 'react';
import { Send } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';

export interface ChatComposerProps {
  value: string;
  loading: boolean;
  inputRef?: RefObject<HTMLTextAreaElement | null>;
  onChange: (value: string) => void;
  onSubmit: (event: FormEvent) => void;
}

export function ChatComposer({ value, loading, inputRef, onChange, onSubmit }: ChatComposerProps) {
  const { t } = useTranslation('chat');
  const internalRef = useRef<HTMLTextAreaElement | null>(null);
  const textareaRef = inputRef || internalRef;

  // Auto-resize textarea as content grows
  useEffect(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = 'auto';
    const nextHeight = Math.min(Math.max(el.scrollHeight, 40), 180);
    el.style.height = `${nextHeight}px`;
  }, [value, textareaRef]);

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      if (value.trim() && !loading) {
        onSubmit(e as unknown as FormEvent);
      }
    }
  };

  return (
    <form onSubmit={onSubmit} className="sticky bottom-0 border-t border-onyx/10 pt-2.5">
      <div className="flex items-end gap-2 rounded-[22px] border border-onyx/15 bg-soft-meadow p-1.5 shadow-xs focus-within:ring-2 focus-within:ring-deep-ink transition-all">
        <textarea
          ref={textareaRef}
          rows={1}
          placeholder={t('placeholder')}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          onKeyDown={handleKeyDown}
          className="min-w-0 flex-1 bg-transparent px-3.5 py-2 text-body-sm text-deep-ink placeholder:text-slate focus:outline-none resize-none leading-relaxed overflow-y-auto"
          disabled={loading}
        />
        <Button
          type="submit"
          variant="primary"
          size="sm"
          disabled={!value.trim() || loading}
          icon={<Send className="h-3.5 w-3.5" />}
          className="shrink-0 rounded-full px-4 py-2 font-semibold mb-0.5"
        >
          {t('send')}
        </Button>
      </div>
    </form>
  );
}
