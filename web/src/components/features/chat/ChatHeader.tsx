import { useTranslation } from 'react-i18next';
import type { AgentManifest } from '@/lib/types';
import { Badge } from '@/components/ui/Badge';
import { PageHeader } from '@/components/ui/PageHeader';
import { Button } from '@/components/ui/Button';
import { MessagesSquare } from 'lucide-react';

export function ChatHeader({ agent, onOpenSessions }: { agent?: AgentManifest; onOpenSessions: () => void }) {
  const { t } = useTranslation('chat');
  return (
    <PageHeader
      eyebrow={t('selectAgent')}
      title={t('title')}
      description={t('subtitle')}
      actions={(
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="sm" icon={<MessagesSquare className="h-4 w-4" />} onClick={onOpenSessions} className="lg:hidden">
            {t('sessions')}
          </Button>
          <Badge variant="neutral">{agent?.name || t('selectAgent')}</Badge>
        </div>
      )}
    />
  );
}
