import { useState, useEffect, useRef } from 'react';
import { getErrorMessage } from '@/lib/errors';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { useToast } from '@/components/ui/Toast';
import {
  Plus,
  Search,
  Download,
  Upload,
  RefreshCw,
  Bot,
  Sparkles,
  MessageSquare,
  Sliders,
  Play,
  Square,
  Trash2,
  Cpu,
  Wrench,
  ShieldCheck,
} from 'lucide-react';
import { api } from '@/lib/api';
import type { AgentManifest } from '@/lib/types';
import type { NavTab } from '@/components/layout/Sidebar';
import { PageHeader } from '@/components/ui/PageHeader';
import { SegmentedControl } from '@/components/ui/SegmentedControl';
import { EmptyState } from '@/components/ui/EmptyState';

export interface AgentsPageProps {
  onOpenChat: (agentID: string) => void;
  onNavigateTab: (tab: NavTab) => void;
  onEditAgent: (agentID: string) => void;
}

type AgentFilter = 'all' | 'system' | 'active' | 'stopped';

export function AgentsPage({
  onOpenChat,
  onNavigateTab: _onNavigateTab,
  onEditAgent,
}: AgentsPageProps) {
  const { t } = useTranslation('agents');
  const { success, error, info } = useToast();
  const [agents, setAgents] = useState<AgentManifest[]>([]);
  const [deletingAgentId, setDeletingAgentId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  const [activeFilter, setActiveFilter] = useState<AgentFilter>('all');
  const fileInputRef = useRef<HTMLInputElement>(null);

  const loadData = async () => {
    try {
      setLoading(true);
      const agentRes = await api.listAgents();
      setAgents(agentRes.agents || []);
    } catch (err) {
      error('Failed to load agents', getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const handleConfirmDeleteAgent = async () => {
    if (!deletingAgentId) return;
    try {
      await api.deleteAgent(deletingAgentId);
      success('Agent Deleted', `Agent ${deletingAgentId} has been removed.`);
      setDeletingAgentId(null);
      loadData();
    } catch (err) {
      error('Failed to delete agent', getErrorMessage(err));
    }
  };

  const handleToggleStatus = async (id: string, currentStatus: string) => {
    try {
      if (currentStatus === 'active') {
        await api.stopAgent(id);
        info('Agent Stopped', `Agent ${id} is now idle.`);
      } else {
        await api.startAgent(id);
        success('Agent Started', `Agent ${id} is now active.`);
      }
      loadData();
    } catch (err) {
      error('Failed to toggle agent status', getErrorMessage(err));
    }
  };

  const handleExportAgents = () => {
    const dataStr = 'data:text/json;charset=utf-8,' + encodeURIComponent(JSON.stringify(agents, null, 2));
    const downloadAnchor = document.createElement('a');
    downloadAnchor.setAttribute('href', dataStr);
    downloadAnchor.setAttribute('download', 'actonos-agents-manifest.json');
    document.body.appendChild(downloadAnchor);
    downloadAnchor.click();
    downloadAnchor.remove();
    success('Export Complete', `${agents.length} agent manifest(s) downloaded.`);
  };

  const handleImportAgents = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    try {
      const text = await file.text();
      const imported = JSON.parse(text);
      const items = Array.isArray(imported) ? imported : [imported];

      for (const item of items) {
        if (item.name) {
          await api.createAgent(item);
        }
      }
      loadData();
      success('Import Succeeded', `Imported ${items.length} agent manifest(s) successfully!`);
    } catch (err) {
      error('Manifest Import Failed', getErrorMessage(err));
    }
  };

  // Filter & search
  const filteredAgents = agents.filter((ag) => {
    const matchesSearch =
      ag.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      ag.description?.toLowerCase().includes(searchQuery.toLowerCase()) ||
      ag.agent_id.toLowerCase().includes(searchQuery.toLowerCase()) ||
      ag.model_config?.primary_model?.toLowerCase().includes(searchQuery.toLowerCase()) ||
      ag.authorized_tools?.some((tl) => tl.toLowerCase().includes(searchQuery.toLowerCase()));

    if (!matchesSearch) return false;

    if (activeFilter === 'system') return ag.is_system || ag.agent_id === 'agent_system_core';
    if (activeFilter === 'active') return ag.status === 'active';
    if (activeFilter === 'stopped') return ag.status === 'stopped';
    return true;
  });

  return (
    <div className="relative min-h-[calc(100vh-64px)]">
      <BlobBackdrop />

      <PageContainer maxWidth="wide">
        <PageHeader
          eyebrow={t('eyebrow')}
          title={t('title')}
          description={t('subtitle')}
          actions={(
            <>
              <Button
                variant="ghost"
                size="sm"
                icon={<Download className="w-3.5 h-3.5" />}
                onClick={handleExportAgents}
                title={t('list.exportTitle')}
              >
                {t('list.export')}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                icon={<Upload className="w-3.5 h-3.5" />}
                onClick={() => fileInputRef.current?.click()}
                title={t('list.importTitle')}
              >
                {t('list.import')}
              </Button>
              <input
                type="file"
                ref={fileInputRef}
                onChange={handleImportAgents}
                accept=".json"
                className="hidden"
              />
              <Button
                variant="ghost"
                size="sm"
                icon={<RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />}
                onClick={loadData}
              >
                {t('list.refresh')}
              </Button>
              <Button
                variant="primary"
                size="sm"
                icon={<Plus className="w-3.5 h-3.5" />}
                onClick={() => onEditAgent('new')}
              >
                {t('actions.createNew', 'Create New Agent')}
              </Button>
            </>
          )}
        />

        {/* Filter and Search Bar */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
          {/* Pill filters */}
          <SegmentedControl
            value={activeFilter}
            onChange={setActiveFilter}
            label={t('list.filterLabel')}
            options={(['all', 'system', 'active', 'stopped'] as const).map((filter) => {
              const count =
                filter === 'all'
                  ? agents.length
                  : filter === 'system'
                    ? agents.filter((a) => a.is_system || a.agent_id === 'agent_system_core').length
                    : agents.filter((a) => a.status === filter).length;

              return {
                value: filter,
                label: `${t(`filters.${filter}`)} (${count})`,
              };
            })}
          />

          {/* Search box */}
          <div className="relative w-full sm:w-72">
            <Search className="w-4 h-4 text-slate absolute left-3.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              placeholder={t('searchPlaceholder', 'Search agents by name, model, tool...')}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full pl-9 pr-4 py-1.5 rounded-full border border-onyx/15 bg-canvas/90 text-body-sm text-deep-ink placeholder:text-slate/60 focus:outline-none focus:ring-1 focus:ring-deep-ink font-sans transition-all"
            />
          </div>
        </div>

        {/* Agents Table View */}
        {loading ? (
          <div className="py-24 text-center text-slate font-sans text-body">{t('list.loading')}</div>
        ) : filteredAgents.length === 0 ? (
          <EmptyState
            icon={<Bot className="h-10 w-10" />}
            title={t('emptyState.title')}
            description={searchQuery ? t('emptyState.noSearchResults') : t('emptyState.description')}
            action={(
              <Button variant="primary" size="sm" icon={<Plus className="w-3.5 h-3.5" />} onClick={() => onEditAgent('new')}>
                {t('actions.createNew')}
              </Button>
            )}
          />
        ) : (
          <Card className="border border-onyx/10 bg-canvas/90 shadow-xs overflow-hidden rounded-2xl">
            <div className="overflow-x-auto">
              <table className="w-full text-left border-collapse">
                <thead>
                  <tr className="border-b border-onyx/10 bg-soft-meadow/70 text-slate text-[11px] uppercase tracking-wider font-semibold font-sans select-none">
                    <th className="py-3.5 px-5">{t('list.columns.identity')}</th>
                    <th className="py-3.5 px-4">{t('list.columns.status')}</th>
                    <th className="py-3.5 px-4">{t('list.columns.model')}</th>
                    <th className="py-3.5 px-4">{t('list.columns.tools')}</th>
                    <th className="py-3.5 px-4">{t('list.columns.governance')}</th>
                    <th className="py-3.5 px-5 text-right">{t('list.columns.actions')}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-onyx/5 font-sans text-body-sm">
                  {filteredAgents.map((agent) => {
                    const isSystem = agent.is_system || agent.agent_id === 'agent_system_core';
                    const isAllTools = agent.authorized_tools?.includes('*');
                    const toolCount = isAllTools ? 'All Tools (*)' : `${agent.authorized_tools?.length || 0} tools`;
                    const listenAll = !agent.listen_channels || agent.listen_channels.includes('*') || agent.listen_channels.length === 0;

                    return (
                      <tr
                        key={agent.agent_id}
                        className="hover:bg-soft-meadow/40 transition-colors group"
                      >
                        {/* Agent & Identity Column */}
                        <td className="py-4 px-5 align-middle">
                          <div className="flex items-center gap-3.5">
                            <div className="w-10 h-10 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center border border-deep-ink shadow-2xs shrink-0">
                              {agent.avatar_icon === 'sparkles' || isSystem ? (
                                <Sparkles className="w-5 h-5" />
                              ) : (
                                <Bot className="w-5 h-5" />
                              )}
                            </div>
                            <div className="min-w-0 max-w-xs sm:max-w-sm">
                              <div className="flex items-center gap-2">
                                <button
                                  onClick={() => onEditAgent(agent.agent_id)}
                                  className="font-serif font-bold text-deep-ink text-body group-hover:text-amber-900 transition-colors truncate hover:underline text-left cursor-pointer"
                                >
                                  {agent.name}
                                </button>
                                {isSystem && (
                                  <Badge variant="accent" className="text-[10px] px-2 py-0.5 shrink-0">
                                    {t('list.root')}
                                  </Badge>
                                )}
                              </div>
                              <div className="flex items-center gap-2 mt-0.5">
                                <span className="font-mono text-[11px] text-slate select-all">
                                  {agent.agent_id}
                                </span>
                              </div>
                              {agent.description && (
                                <p
                                  className="text-caption text-slate truncate mt-1 max-w-[260px] sm:max-w-[320px]"
                                  title={agent.description}
                                >
                                  {agent.description}
                                </p>
                              )}
                            </div>
                          </div>
                        </td>

                        {/* Status Column */}
                        <td className="py-4 px-4 align-middle whitespace-nowrap">
                          <div className="flex items-center gap-2">
                            <button
                              onClick={() => handleToggleStatus(agent.agent_id, agent.status)}
                              className="inline-flex items-center gap-1.5 cursor-pointer"
                              title={`Click to ${agent.status === 'active' ? 'stop' : 'start'} agent`}
                            >
                              <Badge
                                variant={agent.status === 'active' ? 'active' : 'stopped'}
                                className="capitalize cursor-pointer"
                              >
                                <span
                                  className={`w-1.5 h-1.5 rounded-full mr-2 ${agent.status === 'active' ? 'bg-emerald-500 animate-pulse' : 'bg-red-400'
                                    }`}
                                />
                                <span>{t('studio.' + agent.status)}</span>
                              </Badge>
                            </button>
                          </div>
                        </td>

                        {/* Cognitive Model Column */}
                        <td className="py-4 px-4 align-middle whitespace-nowrap">
                          <div className="space-y-1">
                            <div className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-soft-meadow border border-onyx/10 font-mono text-[11px] text-deep-ink font-semibold">
                              <Cpu className="w-3 h-3 text-slate" />
                              <span className="truncate max-w-[170px]">
                                {agent.model_config?.primary_model || 'claude-sonnet-4-6'}
                              </span>
                            </div>
                            <div className="text-[10px] font-mono text-slate pl-1">
                              {t('list.modelDetails', {
                                reasoningEffort: agent.model_config?.reasoning_effort || 'medium',
                                maxTokens: agent.model_config?.max_tokens || 32768,
                              })}
                            </div>
                          </div>
                        </td>

                        {/* Tools & Channels Column */}
                        <td className="py-4 px-4 align-middle whitespace-nowrap">
                          <div className="space-y-1.5">
                            <div className="flex items-center gap-1.5 text-caption font-mono text-deep-ink">
                              <Wrench className="w-3.5 h-3.5 text-slate shrink-0" />
                              <span className={isAllTools ? 'text-emerald-700 font-semibold' : 'text-slate'}>
                                {toolCount}
                              </span>
                            </div>
                            <div className="flex items-center gap-1">
                              {listenAll ? (
                                <span className="text-[10px] font-mono text-slate px-2 py-0.5 bg-soft-meadow rounded-full border border-onyx/5">
                                  {t('list.allChannels')}
                                </span>
                              ) : (
                                agent.listen_channels.map((ch) => (
                                  <span
                                    key={ch}
                                    className="text-[10px] font-mono capitalize px-1.5 py-0.5 bg-soft-meadow rounded-full border border-onyx/5 text-deep-ink"
                                  >
                                    {ch}
                                  </span>
                                ))
                              )}
                            </div>
                          </div>
                        </td>

                        {/* Governance Column */}
                        <td className="py-4 px-4 align-middle whitespace-nowrap">
                          <div className="space-y-1">
                            <div className="flex items-center gap-1.5">
                              <ShieldCheck className="w-3.5 h-3.5 text-slate" />
                              <Badge
                                variant={
                                  agent.delegation_scope?.require_human_approval_level === 'High'
                                    ? 'accent'
                                    : agent.delegation_scope?.require_human_approval_level === 'Low'
                                      ? 'active'
                                      : 'neutral'
                                }
                                className="text-[10px] px-2 py-0.5 font-mono"
                              >
                                {t('list.approval', {
                                  level: agent.delegation_scope?.require_human_approval_level || t('list.medium'),
                                })}
                              </Badge>
                            </div>
                            <div className="text-[11px] font-mono text-slate pl-5">
                              {t('list.monthlyBudget', {
                                amount: agent.delegation_scope?.max_monthly_budget_usd ?? 50,
                              })}
                            </div>
                          </div>
                        </td>

                        {/* Actions Column */}
                        <td className="py-4 px-5 align-middle text-right whitespace-nowrap">
                          <div className="inline-flex items-center gap-1.5">
                            <Button
                              variant="primary"
                              size="sm"
                              icon={<MessageSquare className="w-3.5 h-3.5" />}
                              onClick={() => onOpenChat(agent.agent_id)}
                              className="px-3"
                              title={t('list.launchChat')}
                            >
                              {t('list.chat')}
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              icon={<Sliders className="w-3.5 h-3.5" />}
                              onClick={() => onEditAgent(agent.agent_id)}
                              title={t('list.openStudio')}
                            />
                            <Button
                              variant="ghost"
                              size="sm"
                              icon={
                                agent.status === 'active' ? (
                                  <Square className="w-3.5 h-3.5 text-slate" />
                                ) : (
                                  <Play className="w-3.5 h-3.5 text-emerald-600" />
                                )
                              }
                              onClick={() => handleToggleStatus(agent.agent_id, agent.status)}
                              title={agent.status === 'active' ? 'Stop Agent' : 'Start Agent'}
                            />
                            <Button
                              variant="danger"
                              size="sm"
                              icon={<Trash2 className="w-3.5 h-3.5" />}
                              onClick={() => setDeletingAgentId(agent.agent_id)}
                              disabled={isSystem}
                              title={isSystem ? 'Cannot delete root system agent' : 'Delete agent'}
                            />
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </Card>
        )}
      </PageContainer>

      {/* Delete Agent Confirmation Modal */}
      <ConfirmModal
        isOpen={!!deletingAgentId}
        onClose={() => setDeletingAgentId(null)}
        onConfirm={handleConfirmDeleteAgent}
        title={t('list.deleteTitle')}
        description={t('list.deleteDescription', { agent: deletingAgentId })}
        confirmLabel={t('list.delete')}
        variant="danger"
      />
    </div>
  );
}
