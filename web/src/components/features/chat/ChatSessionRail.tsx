import { MessageSquare, Plus, Search, Trash2, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { AgentManifest, ConversationItem } from '@/lib/types';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { IconButton } from '@/components/ui/IconButton';
import { SegmentedControl } from '@/components/ui/SegmentedControl';

export function ChatSessionRail({
  agents,
  conversations,
  activeAgentID,
  activeConversationID,
  search,
  scope,
  onAgentChange,
  onConversationSelect,
  onConversationDelete,
  onSearchChange,
  onScopeChange,
  onNew,
  open,
  onClose,
}: {
  agents: AgentManifest[];
  conversations: ConversationItem[];
  activeAgentID: string;
  activeConversationID: string | null;
  search: string;
  scope: 'all' | 'agent';
  onAgentChange: (agentID: string) => void;
  onConversationSelect: (conversationID: string) => void;
  onConversationDelete: (conversationID: string) => void;
  onSearchChange: (value: string) => void;
  onScopeChange: (scope: 'all' | 'agent') => void;
  onNew: () => void;
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation('chat');
  return (
    <>
    {open && <button type="button" aria-label={t('closeSessions')} onClick={onClose} className="fixed inset-0 z-40 bg-deep-ink/35 lg:hidden" />}
    <Card className={`${open ? 'flex' : 'hidden'} fixed inset-y-3 left-3 z-50 w-[min(22rem,calc(100vw-1.5rem))] min-h-0 flex-col border border-onyx/10 bg-canvas/95 p-4 lg:static lg:inset-auto lg:z-auto lg:col-span-4 xl:col-span-3 lg:flex lg:w-auto`}>
      <div className="mb-2 flex items-center justify-between lg:hidden">
        <strong className="font-serif text-heading-sm text-deep-ink">{t('sessions')}</strong>
        <IconButton size="sm" label={t('closeSessions')} icon={<X className="h-4 w-4" />} onClick={onClose} />
      </div>
      <label className="mb-1 text-caption font-semibold uppercase tracking-wide text-slate" htmlFor="chat-agent-select">{t('selectAgent')}</label>
      <select
        id="chat-agent-select"
        value={activeAgentID}
        onChange={(event) => onAgentChange(event.target.value)}
        className="density-control mb-3 rounded-full border border-onyx/15 bg-soft-meadow px-4 text-body-sm font-semibold text-deep-ink"
      >
        {agents.map((agent) => <option key={agent.agent_id} value={agent.agent_id}>{agent.name}</option>)}
      </select>
      <Button variant="primary" size="sm" icon={<Plus className="h-4 w-4" />} onClick={onNew} className="mb-3 w-full justify-center">
        {t('newSession')}
      </Button>
      <div className="relative mb-2">
        <Search className="absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate" aria-hidden="true" />
        <input
          value={search}
          onChange={(event) => onSearchChange(event.target.value)}
          placeholder={t('searchSessions')}
          className="density-control w-full rounded-full border border-onyx/10 bg-soft-meadow pl-8 pr-3 text-caption"
        />
      </div>
      <SegmentedControl
        value={scope}
        onChange={onScopeChange}
        label={t('sessionScope')}
        options={[
          { value: 'all', label: t('allAgents') },
          { value: 'agent', label: t('currentAgent') },
        ]}
      />
      <div className="mt-3 min-h-0 flex-1 space-y-1.5 overflow-y-auto">
        {conversations.length === 0 ? (
          <div className="rounded-[18px] bg-soft-meadow p-5 text-center text-caption text-slate">
            <MessageSquare className="mx-auto mb-2 h-5 w-5" />
            {t('noSessions')}
          </div>
        ) : conversations.map((conversation) => (
          <div key={conversation.id} className={`group flex items-center gap-2 rounded-[16px] p-2 ${activeConversationID === conversation.id ? 'bg-deep-ink text-white' : 'hover:bg-soft-meadow'}`}>
            <button type="button" onClick={() => { onConversationSelect(conversation.id); onClose(); }} className="min-w-0 flex-1 text-left">
              <span className="block truncate text-body-sm font-semibold">{conversation.title}</span>
              <span className={`block truncate text-[10px] ${activeConversationID === conversation.id ? 'text-white/70' : 'text-slate'}`}>{conversation.last_message || t('noMessages')}</span>
            </button>
            <IconButton
              size="sm"
              tone="danger"
              label={t('deleteSession')}
              icon={<Trash2 className="h-3.5 w-3.5" />}
              onClick={() => onConversationDelete(conversation.id)}
            />
          </div>
        ))}
      </div>
    </Card>
    </>
  );
}
