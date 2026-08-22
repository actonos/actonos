import { useTranslation } from 'react-i18next';
import type { AgentManifest } from '@/lib/types';
import { Badge } from '@/components/ui/Badge';
import { PageHeader } from '@/components/ui/PageHeader';
import { Button } from '@/components/ui/Button';
import { ArrowLeft, Plus } from 'lucide-react';

export interface ChatHeaderProps {
  agent?: AgentManifest;
  viewMode?: 'sessions' | 'chat';
  onBackToSessions?: () => void;
  onNewSession?: () => void;
}

export function ChatHeader({
  agent,
  viewMode = 'sessions',
  onBackToSessions,
  onNewSession,
}: ChatHeaderProps) {
  const { t } = useTranslation('chat');

  return (
    <PageHeader
      eyebrow={viewMode === 'chat' ? (agent?.name || t('selectAgent')) : t('sessionsList')}
      title={t('title')}
      description={t('subtitle')}
      actions={(
        <div className="flex items-center gap-2">
          {viewMode === 'chat' ? (
            <>
              {onBackToSessions && (
                <Button
                  variant="ghost"
                  size="sm"
                  icon={<ArrowLeft className="h-4 w-4" />}
                  onClick={onBackToSessions}
                  className="font-medium"
                >
                  {t('backToSessions')}
                </Button>
              )}
              {onNewSession && (
                <Button
                  variant="primary"
                  size="sm"
                  icon={<Plus className="h-4 w-4" />}
                  onClick={onNewSession}
                  className="font-semibold"
                >
                  {t('newSession')}
                </Button>
              )}
            </>
          ) : (
            <>
              {agent && (
                <Badge variant="neutral">
                  {t('currentAgent')}: {agent.name}
                </Badge>
              )}
              {onNewSession && (
                <Button
                  variant="primary"
                  size="sm"
                  icon={<Plus className="h-4 w-4" />}
                  onClick={onNewSession}
                  className="font-semibold"
                >
                  {t('newSession')}
                </Button>
              )}
            </>
          )}
        </div>
      )}
    />
  );
}

