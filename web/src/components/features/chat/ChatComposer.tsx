import type { FormEvent, RefObject } from 'react';
import { Send } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';

export interface ChatComposerProps {
  value: string;
  loading: boolean;
  inputRef: RefObject<HTMLInputElement | null>;
  onChange: (value: string) => void;
  onSubmit: (event: FormEvent) => void;
}

export function ChatComposer({ value, loading, inputRef, onChange, onSubmit }: ChatComposerProps) {
  const { t } = useTranslation('chat');
  return (
    <form onSubmit={onSubmit} className="sticky bottom-0 border-t border-soft-meadow pt-2 backdrop-blur-sm">
      <div className="flex items-center gap-2 rounded-full border border-onyx/15 bg-white p-1.5 shadow-sm focus-within:ring-2 focus-within:ring-deep-ink">
        <input
          ref={inputRef}
          type="text"
          placeholder={t('placeholder')}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          className="min-w-0 flex-1 bg-transparent px-4 py-2 text-body-sm text-deep-ink focus:outline-none"
          disabled={loading}
        />
        <Button
          type="submit"
          variant="primary"
          size="sm"
          disabled={!value.trim() || loading}
          icon={<Send className="h-3.5 w-3.5" />}
          className="shrink-0 px-5 py-2 font-semibold"
        >
          {t('send')}
        </Button>
      </div>
    </form>
  );
}
