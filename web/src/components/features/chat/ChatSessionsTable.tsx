import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Pin,
  PinOff,
  Copy,
  Check,
  Search,
  X,
  Plus,
  Trash2,
  Edit3,
  Globe,
  Send,
  MessageSquare,
  Gamepad2,
  Target,
  Zap,
  Sparkles,
  Bot,
  Filter,
  Clock,
  Layers,
  ArrowRight,
} from 'lucide-react';
import type { AgentManifest, ConversationItem } from '@/lib/types';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { IconButton } from '@/components/ui/IconButton';
import { formatRelativeTime } from '@/pages/Chat/chatTypes';

export interface ChatSessionsTableProps {
  conversations: ConversationItem[];
  agents: AgentManifest[];
  search: string;
  onSearchChange: (val: string) => void;
  selectedAgentID: string;
  onSelectAgentID: (id: string) => void;
  selectedChannel: string;
  onSelectChannel: (channel: string) => void;
  pinnedOnly: boolean;
  onTogglePinnedOnly: (val: boolean) => void;
  onViewSession: (convID: string) => void;
  onRenameSession: (conv: ConversationItem) => void;
  onTogglePin: (convID: string, currentPinned: boolean) => void;
  onDeleteSession: (convID: string) => void;
  onNewSession: () => void;
}

export function ChatSessionsTable({
  conversations,
  agents,
  search,
  onSearchChange,
  selectedAgentID,
  onSelectAgentID,
  selectedChannel,
  onSelectChannel,
  pinnedOnly,
  onTogglePinnedOnly,
  onViewSession,
  onRenameSession,
  onTogglePin,
  onDeleteSession,
  onNewSession,
}: ChatSessionsTableProps) {
  const { t, i18n } = useTranslation('chat');
  const [copiedID, setCopiedID] = useState<string | null>(null);

  const handleCopyID = (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    navigator.clipboard.writeText(id);
    setCopiedID(id);
    setTimeout(() => setCopiedID(null), 2000);
  };

  // Helper to get channel icon & localized name
  const renderChannelBadge = (channel?: string) => {
    const norm = (channel || 'web').toLowerCase();
    let icon = <Globe className="w-3.5 h-3.5" />;
    let label = t('channels.web');
    let colorClass = 'bg-canvas text-deep-ink border border-onyx/10';

    if (norm === 'telegram') {
      icon = <Send className="w-3.5 h-3.5 text-sky-600" />;
      label = t('channels.telegram');
      colorClass = 'bg-sky-50 text-sky-800 border border-sky-200';
    } else if (norm === 'whatsapp') {
      icon = <MessageSquare className="w-3.5 h-3.5 text-emerald-600" />;
      label = t('channels.whatsapp');
      colorClass = 'bg-emerald-50 text-emerald-800 border border-emerald-200';
    } else if (norm === 'discord') {
      icon = <Gamepad2 className="w-3.5 h-3.5 text-indigo-600" />;
      label = t('channels.discord');
      colorClass = 'bg-indigo-50 text-indigo-800 border border-indigo-200';
    } else if (norm === 'mission') {
      icon = <Target className="w-3.5 h-3.5 text-amber-600" />;
      label = t('channels.mission');
      colorClass = 'bg-amber-50 text-amber-800 border border-amber-200';
    } else if (norm === 'system' || norm === 'webhook') {
      icon = <Zap className="w-3.5 h-3.5 text-purple-600" />;
      label = norm === 'webhook' ? t('channels.webhook') : t('channels.system');
      colorClass = 'bg-purple-50 text-purple-800 border border-purple-200';
    }

    return (
      <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-medium shrink-0 ${colorClass}`}>
        {icon}
        <span>{label}</span>
      </span>
    );
  };

  // Filter conversations
  const filtered = conversations.filter((conv) => {
    if (pinnedOnly && !conv.is_pinned) {
      return false;
    }
    if (selectedAgentID !== 'all' && conv.agent_id !== selectedAgentID) {
      return false;
    }
    if (selectedChannel !== 'all') {
      const convChannel = (conv.channel || 'web').toLowerCase();
      if (convChannel !== selectedChannel.toLowerCase()) {
        return false;
      }
    }
    if (search.trim()) {
      const q = search.toLowerCase();
      const matchTitle = conv.title.toLowerCase().includes(q);
      const matchMsg = conv.last_message?.toLowerCase().includes(q);
      const matchID = conv.id.toLowerCase().includes(q);
      return matchTitle || matchMsg || matchID;
    }
    return true;
  });

  // Calculate statistics
  const totalCount = conversations.length;
  const pinnedCount = conversations.filter((c) => c.is_pinned).length;
  const uniqueAgentsCount = new Set(conversations.map((c) => c.agent_id)).size;
  const uniqueChannelsCount = new Set(conversations.map((c) => c.channel || 'web')).size;

  const currentLang = i18n.resolvedLanguage || i18n.language || 'en';

  return (
    <div className="space-y-4">
      {/* Quick Summary Metric Cards */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Card className="p-3.5 bg-soft-meadow/80 border border-onyx/10">
          <div className="flex items-center gap-2 text-slate text-caption mb-1">
            <MessageSquare className="w-4 h-4 text-deep-ink" />
            <span>{t('stats.totalSessions')}</span>
          </div>
          <div className="font-serif text-heading-md text-deep-ink">{totalCount}</div>
        </Card>

        <Card className="p-3.5 bg-soft-meadow/80 border border-onyx/10">
          <div className="flex items-center gap-2 text-slate text-caption mb-1">
            <Pin className="w-4 h-4 text-amber-500" />
            <span>{t('stats.pinnedSessions')}</span>
          </div>
          <div className="font-serif text-heading-md text-deep-ink">{pinnedCount}</div>
        </Card>

        <Card className="p-3.5 bg-soft-meadow/80 border border-onyx/10">
          <div className="flex items-center gap-2 text-slate text-caption mb-1">
            <Bot className="w-4 h-4 text-deep-ink" />
            <span>{t('stats.activeAgents')}</span>
          </div>
          <div className="font-serif text-heading-md text-deep-ink">{uniqueAgentsCount}</div>
        </Card>

        <Card className="p-3.5 bg-soft-meadow/80 border border-onyx/10">
          <div className="flex items-center gap-2 text-slate text-caption mb-1">
            <Layers className="w-4 h-4 text-deep-ink" />
            <span>{t('stats.connectedChannels')}</span>
          </div>
          <div className="font-serif text-heading-md text-deep-ink">{uniqueChannelsCount}</div>
        </Card>
      </div>

      {/* Toolbar / Search & Filter Controls */}
      <Card className="p-4 bg-canvas border border-onyx/10 shadow-xs">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          {/* Left: Search input */}
          <div className="relative flex-1 min-w-0 max-w-md">
            <Search className="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-slate" />
            <input
              type="text"
              value={search}
              onChange={(e) => onSearchChange(e.target.value)}
              placeholder={t('filters.searchPlaceholder')}
              className="w-full bg-soft-meadow text-deep-ink pl-10 pr-8 py-2 rounded-full border border-onyx/15 text-body-sm font-sans placeholder:text-slate focus:outline-none focus:border-deep-ink focus:ring-1 focus:ring-deep-ink"
            />
            {search && (
              <button
                type="button"
                onClick={() => onSearchChange('')}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-slate hover:text-deep-ink"
                aria-label={t('filters.searchPlaceholder')}
              >
                <X className="w-3.5 h-3.5" />
              </button>
            )}
          </div>

          {/* Right: Filters & Action Button */}
          <div className="flex flex-wrap items-center gap-2">
            {/* Agent Select Filter */}
            <div className="flex items-center gap-1.5 bg-soft-meadow px-3 py-1.5 rounded-full border border-onyx/10 text-caption">
              <Bot className="w-3.5 h-3.5 text-slate" />
              <select
                value={selectedAgentID}
                onChange={(e) => onSelectAgentID(e.target.value)}
                className="bg-transparent text-deep-ink font-medium focus:outline-none cursor-pointer"
              >
                <option value="all">{t('filters.allAgents')}</option>
                {agents.map((ag) => (
                  <option key={ag.agent_id} value={ag.agent_id}>
                    {ag.name}
                  </option>
                ))}
              </select>
            </div>

            {/* Channel Select Filter */}
            <div className="flex items-center gap-1.5 bg-soft-meadow px-3 py-1.5 rounded-full border border-onyx/10 text-caption">
              <Filter className="w-3.5 h-3.5 text-slate" />
              <select
                value={selectedChannel}
                onChange={(e) => onSelectChannel(e.target.value)}
                className="bg-transparent text-deep-ink font-medium focus:outline-none cursor-pointer"
              >
                <option value="all">{t('filters.allChannels')}</option>
                <option value="web">{t('channels.web')}</option>
                <option value="telegram">{t('channels.telegram')}</option>
                <option value="whatsapp">{t('channels.whatsapp')}</option>
                <option value="discord">{t('channels.discord')}</option>
                <option value="mission">{t('channels.mission')}</option>
                <option value="system">{t('channels.system')}</option>
              </select>
            </div>

            {/* Pinned filter toggle button */}
            <button
              type="button"
              onClick={() => onTogglePinnedOnly(!pinnedOnly)}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-full text-caption font-medium transition-all cursor-pointer border ${
                pinnedOnly
                  ? 'bg-deep-ink text-hi-yellow border-deep-ink'
                  : 'bg-soft-meadow text-slate hover:text-deep-ink border-onyx/10'
              }`}
            >
              <Pin className="w-3.5 h-3.5" />
              <span>{t('filters.pinnedOnly')}</span>
              {pinnedCount > 0 && (
                <span className={`text-[10px] px-1.5 py-0.2 rounded-full ${pinnedOnly ? 'bg-hi-yellow text-deep-ink font-bold' : 'bg-canvas text-slate'}`}>
                  {pinnedCount}
                </span>
              )}
            </button>

            {/* New Session CTA button */}
            <Button
              variant="primary"
              size="sm"
              icon={<Plus className="w-4 h-4" />}
              onClick={onNewSession}
              className="ml-auto lg:ml-2 font-semibold"
            >
              {t('newSession')}
            </Button>
          </div>
        </div>
      </Card>

      {/* Sessions Table Card */}
      <Card className="p-0 border border-onyx/10 bg-canvas overflow-hidden shadow-xs">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-body-sm font-sans border-collapse">
            <thead>
              <tr className="border-b border-onyx/10 bg-soft-meadow/90 text-[11px] uppercase tracking-wider text-slate font-semibold">
                <th className="py-3 px-3.5 w-10 text-center">{t('table.pin')}</th>
                <th className="py-3 px-3.5 w-28">{t('table.id')}</th>
                <th className="py-3 px-3.5 min-w-[180px]">{t('table.title')}</th>
                <th className="py-3 px-3.5 min-w-[140px]">{t('table.agent')}</th>
                <th className="py-3 px-3.5 min-w-[120px]">{t('table.channel')}</th>
                <th className="py-3 px-3.5 min-w-[240px]">{t('table.latestMessage')}</th>
                <th className="py-3 px-3.5 min-w-[130px]">{t('table.updatedAt')}</th>
                <th className="py-3 px-3.5 w-36 text-right">{t('table.actions')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-onyx/5">
              {filtered.length === 0 ? (
                <tr>
                  <td colSpan={8} className="py-12 px-4 text-center">
                    <div className="max-w-md mx-auto space-y-2">
                      <div className="w-12 h-12 mx-auto rounded-full bg-soft-meadow flex items-center justify-center text-slate">
                        <MessageSquare className="w-6 h-6 opacity-60" />
                      </div>
                      <h4 className="font-serif text-heading-sm text-deep-ink">
                        {search || selectedAgentID !== 'all' || selectedChannel !== 'all' || pinnedOnly
                          ? t('noFilterMatches')
                          : t('noSessions')}
                      </h4>
                      <p className="text-caption text-slate">
                        {search || selectedAgentID !== 'all' || selectedChannel !== 'all' || pinnedOnly
                          ? t('noFilterMatches')
                          : t('noSessionsDesc')}
                      </p>
                      <Button
                        variant="primary"
                        size="sm"
                        icon={<Plus className="w-4 h-4" />}
                        onClick={onNewSession}
                        className="mt-3 font-semibold"
                      >
                        {t('newSession')}
                      </Button>
                    </div>
                  </td>
                </tr>
              ) : (
                filtered.map((conv) => {
                  const agent = agents.find((a) => a.agent_id === conv.agent_id);
                  const isPinned = !!conv.is_pinned;
                  const shortID = conv.id.length > 18 ? conv.id.slice(0, 16) + '...' : conv.id;
                  const relativeUpdated = formatRelativeTime(conv.updated_at || conv.created_at, currentLang);
                  const exactCreatedAt = new Date(conv.created_at).toLocaleString(currentLang);
                  const exactUpdatedAt = new Date(conv.updated_at).toLocaleString(currentLang);

                  return (
                    <tr
                      key={conv.id}
                      onClick={() => onViewSession(conv.id)}
                      className={`group hover:bg-soft-meadow/60 transition-colors cursor-pointer ${
                        isPinned ? 'bg-amber-500/[0.03]' : ''
                      }`}
                    >
                      {/* 1. Pin Column */}
                      <td className="py-3 px-3.5 text-center">
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation();
                            onTogglePin(conv.id, isPinned);
                          }}
                          className={`p-1.5 rounded-full transition-all cursor-pointer ${
                            isPinned
                              ? 'text-amber-500 bg-amber-100 hover:bg-amber-200'
                              : 'text-slate/40 hover:text-slate hover:bg-soft-meadow'
                          }`}
                          title={isPinned ? t('actions.unpin') : t('actions.pin')}
                        >
                          {isPinned ? <Pin className="w-4 h-4 fill-amber-500" /> : <PinOff className="w-4 h-4" />}
                        </button>
                      </td>

                      {/* 2. ID Column */}
                      <td className="py-3 px-3.5">
                        <div className="flex items-center gap-1">
                          <span
                            className="font-mono text-[11px] text-slate bg-soft-meadow px-2 py-0.5 rounded-md border border-onyx/5 truncate"
                            title={conv.id}
                          >
                            {shortID}
                          </span>
                          <button
                            type="button"
                            onClick={(e) => handleCopyID(conv.id, e)}
                            className="p-1 text-slate hover:text-deep-ink rounded transition-colors"
                            title={copiedID === conv.id ? t('actions.copiedId') : t('actions.copyId')}
                          >
                            {copiedID === conv.id ? (
                              <Check className="w-3 h-3 text-emerald-600" />
                            ) : (
                              <Copy className="w-3 h-3" />
                            )}
                          </button>
                        </div>
                      </td>

                      {/* 3. Title Column */}
                      <td className="py-3 px-3.5">
                        <div className="flex items-center gap-2">
                          <span
                            className="font-semibold text-deep-ink hover:underline group-hover:text-deep-ink line-clamp-1 max-w-[260px]"
                            title={conv.title}
                          >
                            {conv.title || 'Untitled Session'}
                          </span>
                          {conv.message_count !== undefined && conv.message_count > 0 && (
                            <span className="text-[10px] text-slate font-mono shrink-0">
                              ({conv.message_count})
                            </span>
                          )}
                        </div>
                      </td>

                      {/* 4. Agent Column */}
                      <td className="py-3 px-3.5">
                        <div className="flex items-center gap-2 truncate">
                          <div className="w-6 h-6 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center shrink-0 shadow-2xs">
                            {agent?.avatar_icon === 'sparkles' || agent?.is_system ? (
                              <Sparkles className="w-3 h-3" />
                            ) : (
                              <Bot className="w-3 h-3" />
                            )}
                          </div>
                          <div className="truncate">
                            <span className="font-medium text-deep-ink truncate block text-caption">
                              {agent?.name || conv.agent_id}
                            </span>
                          </div>
                          {agent?.is_system && (
                            <Badge variant="accent" className="text-[9px] px-1.5 py-0">
                              {t('rootBadge')}
                            </Badge>
                          )}
                        </div>
                      </td>

                      {/* 5. Channel Column */}
                      <td className="py-3 px-3.5">
                        {renderChannelBadge(conv.channel)}
                      </td>

                      {/* 6. Latest Message Preview Column */}
                      <td className="py-3 px-3.5">
                        <div
                          className="text-slate text-caption line-clamp-1 max-w-[320px] font-sans"
                          title={conv.last_message || t('noMessages')}
                        >
                          {conv.last_message || (
                            <span className="italic text-slate/60">{t('noMessages')}</span>
                          )}
                        </div>
                      </td>

                      {/* 7. Updated Time Column */}
                      <td className="py-3 px-3.5">
                        <div
                          className="flex items-center gap-1.5 text-slate text-caption"
                          title={`${t('table.createdAt')}: ${exactCreatedAt} \n${t('table.updatedAt')}: ${exactUpdatedAt}`}
                        >
                          <Clock className="w-3.5 h-3.5 text-slate/70 shrink-0" />
                          <span className="truncate">{relativeUpdated}</span>
                        </div>
                      </td>

                      {/* 8. Actions Column */}
                      <td className="py-3 px-3.5 text-right">
                        <div className="flex items-center justify-end gap-1" onClick={(e) => e.stopPropagation()}>
                          {/* View Button */}
                          <Button
                            variant="primary"
                            size="sm"
                            onClick={() => onViewSession(conv.id)}
                            className="px-2.5 py-1 text-caption font-semibold h-7"
                            title={t('actions.view')}
                          >
                            <span>{t('actions.view')}</span>
                            <ArrowRight className="w-3 h-3 ml-0.5" />
                          </Button>

                          {/* Rename Button */}
                          <IconButton
                            size="sm"
                            label={t('actions.rename')}
                            icon={<Edit3 className="w-3.5 h-3.5" />}
                            onClick={() => onRenameSession(conv)}
                          />

                          {/* Delete Button */}
                          <IconButton
                            size="sm"
                            tone="danger"
                            label={t('actions.delete')}
                            icon={<Trash2 className="w-3.5 h-3.5" />}
                            onClick={() => onDeleteSession(conv.id)}
                          />
                        </div>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  );
}
