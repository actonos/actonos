import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Button } from '@/components/ui/Button';
import { AgentCard } from '@/components/features/agents/AgentCard';
import { AgentFormModal } from '@/components/features/agents/AgentFormModal';
import { QuickstartGuide } from '@/components/features/onboarding/QuickstartGuide';
import {
  Plus,
  MessageSquare,
  Search,
  Download,
  Upload,
  Bot,
} from 'lucide-react';
import { api } from '@/lib/api';
import type { AgentManifest, ToolInfo } from '@/lib/types';
import type { NavTab } from '@/components/layout/Navbar';

export interface AgentsPageProps {
  onOpenChat: (agentID: string) => void;
  onNavigateTab: (tab: NavTab) => void;
  isCreateModalOpen: boolean;
  setIsCreateModalOpen: (open: boolean) => void;
}

type AgentFilter = 'all' | 'system' | 'active' | 'stopped';

export function AgentsPage({
  onOpenChat,
  onNavigateTab,
  isCreateModalOpen,
  setIsCreateModalOpen,
}: AgentsPageProps) {
  const { t } = useTranslation('agents');
  const [agents, setAgents] = useState<AgentManifest[]>([]);
  const [tools, setTools] = useState<ToolInfo[]>([]);
  const [editingAgent, setEditingAgent] = useState<AgentManifest | null>(null);
  const [loading, setLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  const [activeFilter, setActiveFilter] = useState<AgentFilter>('all');
  const fileInputRef = useRef<HTMLInputElement>(null);

  const loadData = async () => {
    try {
      setLoading(true);
      const [agentRes, toolRes] = await Promise.all([
        api.listAgents(),
        api.listTools(),
      ]);
      setAgents(agentRes.agents || []);
      setTools(toolRes.tools || []);
    } catch (err) {
      console.error('Failed to load agents or tools:', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const handleCreateOrUpdate = async (data: Partial<AgentManifest>) => {
    if (editingAgent) {
      await api.updateAgent(editingAgent.agent_id, data);
    } else {
      await api.createAgent(data);
    }
    setEditingAgent(null);
    loadData();
  };

  const handleDelete = async (id: string) => {
    if (window.confirm(t('actions.deleteConfirm', 'Are you sure you want to delete this agent?'))) {
      await api.deleteAgent(id);
      loadData();
    }
  };

  const handleToggleStatus = async (id: string, currentStatus: string) => {
    if (currentStatus === 'active') {
      await api.stopAgent(id);
    } else {
      await api.startAgent(id);
    }
    loadData();
  };

  const handleExportAgents = () => {
    const dataStr = 'data:text/json;charset=utf-8,' + encodeURIComponent(JSON.stringify(agents, null, 2));
    const downloadAnchor = document.createElement('a');
    downloadAnchor.setAttribute('href', dataStr);
    downloadAnchor.setAttribute('download', 'actonos-agents-manifest.json');
    document.body.appendChild(downloadAnchor);
    downloadAnchor.click();
    downloadAnchor.remove();
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
      alert(`Imported ${items.length} agent manifest(s) successfully!`);
    } catch (err: any) {
      alert(`Failed to import manifest: ${err.message}`);
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
        {/* Onboarding Quickstart Banner */}
        <QuickstartGuide onNavigateTab={onNavigateTab} onOpenChat={onOpenChat} />

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
              {t('subtitle', 'Create, customize, and orchestrate autonomous AI agents running on the ActonOS kernel.')}
            </p>
          </div>

          <div className="flex flex-wrap items-center gap-2.5 shrink-0 self-start sm:self-center">
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
              variant="secondary"
              size="sm"
              icon={<MessageSquare className="w-4 h-4" />}
              onClick={() => onOpenChat('agent_system_core')}
            >
              {t('quickChatSystem', 'Chat with Root')}
            </Button>
            <Button
              variant="primary"
              size="sm"
              icon={<Plus className="w-4 h-4" />}
              onClick={() => {
                setEditingAgent(null);
                setIsCreateModalOpen(true);
              }}
            >
              {t('actions.createNew', 'Create New Agent')}
            </Button>
          </div>
        </div>

        {/* Search & Filter Toolbar */}
        <div className="flex flex-col md:flex-row items-stretch md:items-center justify-between gap-3 mb-6">
          {/* Search Input */}
          <div className="relative flex-1 max-w-md">
            <Search className="w-4 h-4 text-slate absolute left-3.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              placeholder="Search agents by name, role, or authorized tools..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full bg-soft-meadow text-deep-ink pl-10 pr-4 py-2 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink"
            />
          </div>

          {/* Filter Pills */}
          <div className="flex items-center gap-1.5 p-1 bg-soft-meadow rounded-full border border-onyx/10 shrink-0">
            {(['all', 'system', 'active', 'stopped'] as AgentFilter[]).map((f) => (
              <button
                key={f}
                onClick={() => setActiveFilter(f)}
                className={`px-3.5 py-1 rounded-full text-caption font-medium capitalize transition-all ${
                  activeFilter === f
                    ? 'bg-deep-ink text-white font-semibold shadow-xs'
                    : 'text-deep-ink hover:text-slate'
                }`}
              >
                {f === 'system' ? '⭐ Root' : f}
              </button>
            ))}
          </div>
        </div>

        {/* Agent Cards Grid */}
        {loading ? (
          <div className="py-20 text-center text-slate font-sans">Loading agents...</div>
        ) : filteredAgents.length === 0 ? (
          <div className="bg-soft-meadow rounded-[24px] p-12 text-center max-w-lg mx-auto border border-onyx/10">
            <div className="w-14 h-14 rounded-full bg-canvas flex items-center justify-center text-deep-ink mx-auto mb-4 border border-onyx">
              <Bot className="w-7 h-7 text-hi-yellow" />
            </div>
            <h3 className="font-serif text-heading-sm text-deep-ink mb-2">No Matching Agents</h3>
            <p className="font-sans text-body-sm text-slate mb-6">
              {searchQuery ? 'No agents match your search filter.' : t('card.noAgents', 'Create your first agent to get started.')}
            </p>
            <Button variant="primary" onClick={() => setIsCreateModalOpen(true)}>
              {t('actions.createNew', 'Create New Agent')}
            </Button>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {filteredAgents.map((agent) => (
              <AgentCard
                key={agent.agent_id}
                agent={agent}
                onChat={onOpenChat}
                onEdit={(ag) => {
                  setEditingAgent(ag);
                  setIsCreateModalOpen(true);
                }}
                onDelete={handleDelete}
                onToggleStatus={handleToggleStatus}
              />
            ))}
          </div>
        )}
      </PageContainer>

      {/* Create / Edit Modal */}
      <AgentFormModal
        isOpen={isCreateModalOpen}
        onClose={() => {
          setIsCreateModalOpen(false);
          setEditingAgent(null);
        }}
        onSubmit={handleCreateOrUpdate}
        initialAgent={editingAgent}
        availableTools={tools}
      />
    </div>
  );
}
