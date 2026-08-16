import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Button } from '@/components/ui/Button';
import { AgentCard } from '@/components/features/agents/AgentCard';
import { AgentFormModal } from '@/components/features/agents/AgentFormModal';
import { Plus, Sparkles } from 'lucide-react';
import { api } from '@/lib/api';
import type { AgentManifest, ToolInfo } from '@/lib/types';

export interface AgentsPageProps {
  onOpenChat: (agentID: string) => void;
  isCreateModalOpen: boolean;
  setIsCreateModalOpen: (open: boolean) => void;
}

export function AgentsPage({ onOpenChat, isCreateModalOpen, setIsCreateModalOpen }: AgentsPageProps) {
  const { t } = useTranslation('agents');
  const [agents, setAgents] = useState<AgentManifest[]>([]);
  const [tools, setTools] = useState<ToolInfo[]>([]);
  const [editingAgent, setEditingAgent] = useState<AgentManifest | null>(null);
  const [loading, setLoading] = useState(true);

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
    if (window.confirm(t('actions.deleteConfirm'))) {
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

  return (
    <div className="relative min-h-[calc(100vh-72px)]">
      <BlobBackdrop />

      <PageContainer>
        {/* Header section */}
        <div className="flex flex-col md:flex-row md:items-end justify-between gap-4 mb-10">
          <div>
            <span className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
              {t('eyebrow')}
            </span>
            <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight">
              {t('title')}
            </h1>
            <p className="font-sans text-body text-slate mt-2 max-w-2xl">
              {t('subtitle')}
            </p>
          </div>

          <Button
            variant="primary"
            icon={<Plus className="w-4 h-4" />}
            onClick={() => {
              setEditingAgent(null);
              setIsCreateModalOpen(true);
            }}
          >
            {t('actions.createNew')}
          </Button>
        </div>

        {/* Agent Cards Grid */}
        {loading ? (
          <div className="py-20 text-center text-slate font-sans">Loading agents...</div>
        ) : agents.length === 0 ? (
          <div className="bg-soft-meadow rounded-[24px] p-12 text-center max-w-lg mx-auto border border-onyx/10">
            <div className="w-14 h-14 rounded-full bg-canvas flex items-center justify-center text-deep-ink mx-auto mb-4 border border-onyx">
              <Sparkles className="w-7 h-7 text-hi-yellow" />
            </div>
            <h3 className="font-serif text-heading-sm text-deep-ink mb-2">No Agents Configured</h3>
            <p className="font-sans text-body-sm text-slate mb-6">{t('card.noAgents')}</p>
            <Button variant="primary" onClick={() => setIsCreateModalOpen(true)}>
              {t('actions.createNew')}
            </Button>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {agents.map((agent) => (
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
