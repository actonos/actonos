import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { useToast } from '@/components/ui/Toast';
import { AgentCard } from '@/components/features/agents/AgentCard';
import { SoulEditorModal } from '@/components/features/agents/SoulEditorModal';
import {
  Plus,
  Search,
  Download,
  Upload,
  Sparkles,
  RefreshCw,
} from 'lucide-react';
import { api } from '@/lib/api';
import type { AgentManifest } from '@/lib/types';
import type { NavTab } from '@/components/layout/Sidebar';

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
  const [isSoulModalOpen, setIsSoulModalOpen] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const loadData = async () => {
    try {
      setLoading(true);
      const agentRes = await api.listAgents();
      setAgents(agentRes.agents || []);
    } catch (err: any) {
      error('Failed to load agents', err.message);
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
    } catch (err: any) {
      error('Failed to delete agent', err.message);
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
    } catch (err: any) {
      error('Failed to toggle agent status', err.message);
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
    } catch (err: any) {
      error('Manifest Import Failed', err.message);
    }
  };

  // Filter & search
  const filteredAgents = agents.filter((ag) => {
    const matchesSearch =
      ag.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      ag.description?.toLowerCase().includes(searchQuery.toLowerCase()) ||
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

      <PageContainer>
        {/* Header section */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
          <div className="flex-1">
            <span className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
              {t('eyebrow', 'Universal Agent Engine')}
            </span>
            <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight">
              {t('title', 'Autonomous Agents')}
            </h1>
            <p className="font-sans text-body text-slate mt-1 max-w-2xl">
              {t(
                'subtitle',
                'Create, customize, and orchestrate autonomous AI agents with SOUL.md personality and proactive background cron automations.'
              )}
            </p>
          </div>

          <div className="flex flex-wrap items-center gap-2.5 shrink-0 self-start sm:self-center">
            <Button
              variant="ghost"
              size="sm"
              icon={<Sparkles className="w-3.5 h-3.5" />}
              onClick={() => setIsSoulModalOpen(true)}
              title="Edit Agent Soul (SOUL.md)"
            >
              Agent Soul
            </Button>
            <Button
              variant="ghost"
              size="sm"
              icon={<Download className="w-3.5 h-3.5" />}
              onClick={handleExportAgents}
              title="Export agents as JSON"
            >
              Export
            </Button>
            <Button
              variant="ghost"
              size="sm"
              icon={<Upload className="w-3.5 h-3.5" />}
              onClick={() => fileInputRef.current?.click()}
              title="Import agents from JSON"
            >
              Import
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
              Refresh
            </Button>
            <Button
              variant="primary"
              size="sm"
              icon={<Plus className="w-3.5 h-3.5" />}
              onClick={() => onEditAgent('new')}
            >
              {t('actions.createNew', 'Create New Agent')}
            </Button>
          </div>
        </div>

        {/* Filter and Search Bar */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
          {/* Pill filters */}
          <div className="flex items-center gap-1.5 bg-canvas/80 backdrop-blur-sm p-1 rounded-full border border-onyx/10 shadow-xs self-start sm:self-auto overflow-x-auto">
            {(['all', 'system', 'active', 'stopped'] as const).map((filter) => {
              const label =
                filter === 'all'
                  ? `All Agents (${agents.length})`
                  : filter === 'system'
                    ? '⭐ Root System'
                    : filter === 'active'
                      ? 'Active'
                      : 'Stopped';

              const isActive = activeFilter === filter;
              return (
                <button
                  key={filter}
                  onClick={() => setActiveFilter(filter)}
                  className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${isActive
                      ? 'bg-deep-ink text-white font-semibold shadow-xs'
                      : 'text-deep-ink hover:text-slate'
                    }`}
                >
                  {label}
                </button>
              );
            })}
          </div>

          {/* Search box */}
          <div className="relative w-full sm:w-72">
            <Search className="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-slate" />
            <input
              type="text"
              placeholder="Search agents, tools, models..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full bg-canvas/80 backdrop-blur-sm text-deep-ink pl-10 pr-4 py-2 rounded-full border border-onyx/10 text-body-sm font-sans placeholder:text-slate focus:outline-none focus:border-deep-ink transition-colors shadow-xs"
            />
          </div>
        </div>

        {/* Agents Grid */}
        {loading ? (
          <div className="py-24 text-center text-slate font-sans text-body">Loading agents...</div>
        ) : filteredAgents.length === 0 ? (
          <Card className="p-12 text-center border border-dashed border-onyx/20 bg-canvas/60">
            <p className="text-body text-slate mb-4">
              {searchQuery ? 'No agents match your search filter.' : t('card.noAgents')}
            </p>
            <Button
              variant="primary"
              size="sm"
              icon={<Plus className="w-3.5 h-3.5" />}
              onClick={() => onEditAgent('new')}
            >
              {t('actions.createNew')}
            </Button>
          </Card>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {filteredAgents.map((agent) => (
              <AgentCard
                key={agent.agent_id}
                agent={agent}
                onChat={() => onOpenChat(agent.agent_id)}
                onEdit={() => onEditAgent(agent.agent_id)}
                onDelete={() => setDeletingAgentId(agent.agent_id)}
                onToggleStatus={() => handleToggleStatus(agent.agent_id, agent.status)}
              />
            ))}
          </div>
        )}
      </PageContainer>

      {/* Soul Editor Modal */}
      <SoulEditorModal
        isOpen={isSoulModalOpen}
        onClose={() => setIsSoulModalOpen(false)}
      />

      {/* Delete Agent Confirmation Modal */}
      <ConfirmModal
        isOpen={!!deletingAgentId}
        onClose={() => setDeletingAgentId(null)}
        onConfirm={handleConfirmDeleteAgent}
        title="Delete Agent Manifest"
        description={`Are you sure you want to delete "${deletingAgentId}"? This will permanently remove its state and execution manifests from ActonOS.`}
        confirmLabel="Delete Agent"
        variant="danger"
      />
    </div>
  );
}
