import { useMemo, useState } from 'react';
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
  MessageSquare,
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
import { useInstalledChannels } from '@/lib/installed-channels';
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
  const { channels: pluginChannels } = useInstalledChannels();
  const [copiedID, setCopiedID] = useState<string | null>(null);

  const handleCopyID = (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    navigator.clipboard.writeText(id);
    setCopiedID(id);
    setTimeout(() => setCopiedID(null), 2000);
  };

  const channelLabel = (id: string) => {
    const plugin = pluginChannels.find((channel) => channel.id === id);
    if (plugin) return plugin.label;
    const key = `channels.${id}`;
    const labeled = t(key);
    return labeled === key ? id : labeled;
  };

  const channelFilterOptions = useMemo(() => {
    const seen = new Set<string>();
    const options: { id: string; label: string }[] = [];
    const add = (id: string, label: string) => {
      if (!id || seen.has(id)) return;
      seen.add(id);
      options.push({ id, label });
    };
    add('web', t('channels.web'));
    pluginChannels.forEach((channel) => add(channel.id, channel.label));
    add('mission', t('channels.mission'));
    add('webhook', t('channels.webhook'));
    return options;
  }, [pluginChannels, t]);

  // Helper to get channel icon & localized name
  const renderChannelBadge = (channel?: string) => {
    const norm = (channel || 'web').toLowerCase();
    let icon = <Globe className="w-3 h-3" />;
    let label = t('channels.web');
    let colorClass = 'bg-canvas text-deep-ink border border-onyx/10';

    if (norm === 'mission') {
      icon = <Target className="w-3 h-3 text-amber-600" />;
      label = t('channels.mission');
      colorClass = 'bg-amber-50 text-amber-800 border border-amber-200';
    } else if (norm === 'system' || norm === 'webhook') {
      icon = <Zap className="w-3 h-3 text-purple-600" />;
      label = norm === 'webhook' ? t('channels.webhook') : t('channels.system');
      colorClass = 'bg-purple-50 text-purple-800 border border-purple-200';
    } else if (norm !== 'web') {
      icon = <MessageSquare className="w-3 h-3 text-sky-600" />;
      label = channelLabel(norm);
      colorClass = 'bg-sky-50 text-sky-800 border border-sky-200';
    }

    return (
      <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-medium shrink-0 ${colorClass}`}>
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
        <Card className="p-3.5 border border-onyx/10 bg-canvas/80 shadow-xs flex items-center gap-3">
          <div className="w-10 h-10 rounded-2xl bg-hi-yellow/20 text-deep-ink flex items-center justify-center shrink-0">
            <MessageSquare className="w-5 h-5" />
          </div>
          <div className="min-w-0">
            <div className="text-caption text-slate">{t('stats.totalSessions')}</div>
            <div className="font-serif text-heading-md font-bold text-deep-ink">{totalCount}</div>
          </div>
        </Card>

        <Card className="p-3.5 border border-onyx/10 bg-canvas/80 shadow-xs flex items-center gap-3">
          <div className="w-10 h-10 rounded-2xl bg-soft-meadow text-deep-ink flex items-center justify-center shrink-0">
            <Bot className="w-5 h-5" />
          </div>
          <div className="min-w-0">
            <div className="text-caption text-slate">{t('stats.activeAgents')}</div>
            <div className="font-serif text-heading-md font-bold text-deep-ink">{uniqueAgentsCount}</div>
          </div>
        </Card>

        <Card className="p-3.5 border border-onyx/10 bg-canvas/80 shadow-xs flex items-center gap-3">
          <div className="w-10 h-10 rounded-2xl bg-sky-100 text-sky-900 flex items-center justify-center shrink-0">
            <Layers className="w-5 h-5" />
          </div>
          <div className="min-w-0">
            <div className="text-caption text-slate">{t('stats.connectedChannels')}</div>
            <div className="font-serif text-heading-md font-bold text-deep-ink">{uniqueChannelsCount}</div>
          </div>
        </Card>

        <Card className="p-3.5 border border-onyx/10 bg-canvas/80 shadow-xs flex items-center gap-3">
          <div className="w-10 h-10 rounded-2xl bg-amber-100 text-amber-900 flex items-center justify-center shrink-0">
            <Pin className="w-5 h-5 fill-amber-500 text-amber-500" />
          </div>
          <div className="min-w-0">
            <div className="text-caption text-slate">{t('stats.pinnedSessions')}</div>
            <div className="font-serif text-heading-md font-bold text-deep-ink">{pinnedCount}</div>
          </div>
        </Card>
      </div>

      {/* Filter and Search Bar */}
      <Card className="p-3 border border-onyx/10 bg-canvas/90 shadow-xs">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          {/* Search Bar */}
          <div className="relative flex-1 min-w-[240px]">
            <Search className="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-slate" />
            <input
              type="text"
              value={search}
              onChange={(e) => onSearchChange(e.target.value)}
              placeholder={t('filters.searchPlaceholder')}
              className="w-full pl-9 pr-8 py-2 rounded-full border border-onyx/15 bg-soft-meadow/60 text-body-sm text-deep-ink placeholder:text-slate focus:outline-none focus:ring-2 focus:ring-deep-ink transition-all"
            />
            {search && (
              <button
                type="button"
                onClick={() => onSearchChange('')}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-slate hover:text-deep-ink p-0.5 rounded-full"
                title="Clear search"
              >
                <X className="w-3.5 h-3.5" />
              </button>
            )}
          </div>

          {/* Filter Selectors */}
          <div className="flex flex-wrap items-center gap-2">
            {/* Agent Filter */}
            <div className="flex items-center gap-1.5 bg-soft-meadow px-3 py-1.5 rounded-full border border-onyx/10 text-caption">
              <Bot className="w-3.5 h-3.5 text-slate" />
              <select
                value={selectedAgentID}
                onChange={(e) => onSelectAgentID(e.target.value)}
                className="bg-transparent border-0 text-deep-ink font-medium focus:outline-none cursor-pointer pr-1 text-caption"
              >
                <option value="all">{t('filters.allAgents')}</option>
                {agents.map((agent) => (
                  <option key={agent.agent_id} value={agent.agent_id}>
                    {agent.name} {agent.is_system ? `(${t('root')})` : ''}
                  </option>
                ))}
              </select>
            </div>

            {/* Channel Filter */}
            <div className="flex items-center gap-1.5 bg-soft-meadow px-3 py-1.5 rounded-full border border-onyx/10 text-caption">
              <Filter className="w-3.5 h-3.5 text-slate" />
              <select
                value={selectedChannel}
                onChange={(e) => onSelectChannel(e.target.value)}
                className="bg-transparent border-0 text-deep-ink font-medium focus:outline-none cursor-pointer pr-1 text-caption"
              >
                <option value="all">{t('filters.allChannels')}</option>
                {channelFilterOptions.map((item) => (
                  <option key={item.id} value={item.id}>{item.label}</option>
                ))}
              </select>
            </div>

            {/* Pinned Filter Pill */}
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

      {/* Condensed Sessions Table Card (4 Unified Columns) */}
      <Card className="p-0 border border-onyx/10 bg-canvas overflow-hidden shadow-xs">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-body-sm font-sans border-collapse">
            <thead>
              <tr className="border-b border-onyx/10 bg-soft-meadow/90 text-[11px] uppercase tracking-wider text-slate font-semibold">
                <th className="py-3 px-4 min-w-[280px]">{t('table.conversation')}</th>
                <th className="py-3 px-4 min-w-[160px] w-56">{t('table.assistantAndChannel')}</th>
                <th className="py-3 px-4 min-w-[110px] w-36">{t('table.activity')}</th>
                <th className="py-3 px-4 w-32 text-right">{t('table.actions')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-onyx/5">
              {filtered.length === 0 ? (
                <tr>
                  <td colSpan={4} className="py-12 px-4 text-center">
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
                  const shortID = conv.id.length > 14 ? conv.id.slice(0, 12) + '...' : conv.id;
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
                      {/* Column 1: Conversation Info (Pin + Title + Count + ID + Latest Message Preview) */}
                      <td className="py-3 px-4">
                        <div className="space-y-1">
                          {/* Row 1: Pin + Title + Count + ID badge */}
                          <div className="flex items-center gap-2 flex-wrap">
                            <button
                              type="button"
                              onClick={(e) => {
                                e.stopPropagation();
                                onTogglePin(conv.id, isPinned);
                              }}
                              className={`p-1 rounded-full transition-all cursor-pointer shrink-0 ${
                                isPinned
                                  ? 'text-amber-500 bg-amber-100 hover:bg-amber-200'
                                  : 'text-slate/30 hover:text-slate hover:bg-soft-meadow'
                              }`}
                              title={isPinned ? t('actions.unpin') : t('actions.pin')}
                            >
                              {isPinned ? <Pin className="w-3.5 h-3.5 fill-amber-500" /> : <PinOff className="w-3.5 h-3.5" />}
                            </button>

                            <span
                              className="font-semibold text-deep-ink group-hover:underline line-clamp-1 max-w-[320px] text-body-sm"
                              title={conv.title}
                            >
                              {conv.title || 'Untitled Session'}
                            </span>

                            {conv.message_count !== undefined && conv.message_count > 0 && (
                              <span className="text-[10px] text-slate font-mono bg-soft-meadow px-1.5 py-0.5 rounded-full border border-onyx/5 shrink-0">
                                {t('table.messagesCount', { count: conv.message_count })}
                              </span>
                            )}

                            {/* ID badge with 1-click copy */}
                            <div className="flex items-center gap-0.5 bg-onyx/5 px-1.5 py-0.5 rounded text-[10px] font-mono text-slate shrink-0">
                              <span title={conv.id}>{shortID}</span>
                              <button
                                type="button"
                                onClick={(e) => handleCopyID(conv.id, e)}
                                className="p-0.5 text-slate hover:text-deep-ink rounded transition-colors"
                                title={copiedID === conv.id ? t('actions.copiedId') : t('actions.copyId')}
                              >
                                {copiedID === conv.id ? (
                                  <Check className="w-2.5 h-2.5 text-emerald-600" />
                                ) : (
                                  <Copy className="w-2.5 h-2.5" />
                                )}
                              </button>
                            </div>
                          </div>

                          {/* Row 2: Latest Message Snippet */}
                          <div
                            className="text-slate text-caption line-clamp-1 max-w-[480px] font-sans pl-6"
                            title={conv.last_message || t('noMessages')}
                          >
                            {conv.last_message ? (
                              <span>{conv.last_message}</span>
                            ) : (
                              <span className="italic text-slate/50">{t('noMessages')}</span>
                            )}
                          </div>
                        </div>
                      </td>

                      {/* Column 2: Assistant & Channel */}
                      <td className="py-3 px-4">
                        <div className="space-y-1">
                          {/* Row 1: Agent Avatar + Name */}
                          <div className="flex items-center gap-1.5 truncate">
                            <div className="w-5 h-5 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center shrink-0 shadow-2xs">
                              {agent?.avatar_icon === 'sparkles' || agent?.is_system ? (
                                <Sparkles className="w-2.5 h-2.5" />
                              ) : (
                                <Bot className="w-2.5 h-2.5" />
                              )}
                            </div>
                            <span className="font-medium text-deep-ink truncate text-caption">
                              {agent?.name || conv.agent_id}
                            </span>
                            {agent?.is_system && (
                              <Badge variant="accent" className="text-[8px] px-1 py-0">
                                {t('rootBadge')}
                              </Badge>
                            )}
                          </div>

                          {/* Row 2: Channel badge */}
                          <div>
                            {renderChannelBadge(conv.channel)}
                          </div>
                        </div>
                      </td>

                      {/* Column 3: Activity / Time */}
                      <td className="py-3 px-4">
                        <div
                          className="flex items-center gap-1 text-slate text-caption"
                          title={`${t('table.createdAt')}: ${exactCreatedAt}\n${t('table.updatedAt')}: ${exactUpdatedAt}`}
                        >
                          <Clock className="w-3.5 h-3.5 text-slate/60 shrink-0" />
                          <span className="truncate">{relativeUpdated}</span>
                        </div>
                      </td>

                      {/* Column 4: Actions */}
                      <td className="py-3 px-4 text-right">
                        <div className="flex items-center justify-end gap-1" onClick={(e) => e.stopPropagation()}>
                          <IconButton
                            icon={<ArrowRight className="w-4 h-4 text-deep-ink" />}
                            label={t('actions.view')}
                            size="sm"
                            tone="default"
                            onClick={() => onViewSession(conv.id)}
                            className="hover:bg-soft-meadow"
                          />
                          <IconButton
                            icon={<Edit3 className="w-3.5 h-3.5 text-slate" />}
                            label={t('actions.rename')}
                            size="sm"
                            tone="default"
                            onClick={() => onRenameSession(conv)}
                            className="hover:bg-soft-meadow"
                          />
                          <IconButton
                            icon={<Trash2 className="w-3.5 h-3.5 text-slate hover:text-red-600" />}
                            label={t('actions.delete')}
                            size="sm"
                            tone="danger"
                            onClick={() => onDeleteSession(conv.id)}
                            className="hover:bg-red-50"
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
