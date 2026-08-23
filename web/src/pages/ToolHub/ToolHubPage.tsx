import { useState, useEffect } from 'react';
import { getErrorMessage } from '@/lib/errors';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { PageHeader } from '@/components/ui/PageHeader';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Button } from '@/components/ui/Button';
import { useToast } from '@/components/ui/Toast';
import { ToolCard } from '@/components/features/tools/ToolCard';
import { McpServerModal } from '@/components/features/tools/McpServerModal';
import { ToolTestModal } from '@/components/features/tools/ToolTestModal';
import { Card } from '@/components/ui/Card';
import {
  Plus,
  RefreshCw,
  Search,
  Wrench,
} from 'lucide-react';
import { api } from '@/lib/api';
import type { ToolInfo, MCPServerStatus } from '@/lib/types';
import { useActionProgress } from '@/lib/useActionProgress';

export function ToolHubPage() {
  const { t } = useTranslation('tools');
  const { error, success } = useToast();
  const [tools, setTools] = useState<ToolInfo[]>([]);
  const [activeCategory, setActiveCategory] = useState<string>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [isMcpModalOpen, setIsMcpModalOpen] = useState(false);
  const [testingTool, setTestingTool] = useState<ToolInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [servers, setServers] = useState<MCPServerStatus[]>([]);
  const { executeAction, isExecuting } = useActionProgress();

  const loadTools = async () => {
    try {
      setLoading(true);
      const [res, mcp] = await Promise.all([
        api.listTools(activeCategory === 'all' ? undefined : activeCategory),
        api.listMCPServers().catch(() => ({ servers: [] })),
      ]);
      // Filter out skill category since skills have their own dedicated page
      const nonSkillTools = (res.tools || []).filter((tl) => tl.category !== 'skill');
      setTools(nonSkillTools);
      setServers(mcp.servers);
    } catch (err) {
      error('Failed to load tools', getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadTools();
    const handleUpdates = () => {
      loadTools();
    };
    window.addEventListener('actonos:tools-updated', handleUpdates);
    window.addEventListener('actonos:approval-decided', handleUpdates);
    return () => {
      window.removeEventListener('actonos:tools-updated', handleUpdates);
      window.removeEventListener('actonos:approval-decided', handleUpdates);
    };
  }, [activeCategory]);

  const handleConnectMCP = async (cfg: { id: string; transport: string; command?: string; args?: string[]; url?: string; env?: Record<string, string> }) => {
    setIsMcpModalOpen(false);
    await executeAction({
      targetId: cfg.id,
      title: t('actions.connectingMCP', { name: cfg.id, defaultValue: `Connecting MCP Server: ${cfg.id}` }),
      steps: [
        { id: 'auth', label: t('actionProgress.steps.auth', { defaultValue: 'Security Authorization & Approval' }) },
        { id: 'connect', label: t('mcp.establishingConnection', { defaultValue: 'Initializing Transport & Discovering Tools' }) },
      ],
      action: () => api.connectMCP(cfg),
      onSuccess: () => {
        success('MCP Connected', `Server ${cfg.id} registered.`);
        loadTools();
      },
    });
  };

  const handleToggleMCP = (server: MCPServerStatus) => {
    const nextEnabled = !server.enabled;
    executeAction({
      targetId: server.id,
      title: nextEnabled
        ? t('mcp.enablingTitle', { name: server.id, defaultValue: `Enabling MCP Server: ${server.id}` })
        : t('mcp.disablingTitle', { name: server.id, defaultValue: `Disabling MCP Server: ${server.id}` }),
      steps: [
        { id: 'auth', label: t('actionProgress.steps.auth', { defaultValue: 'Security Authorization & Approval' }) },
        { id: 'toggle', label: t('mcp.updatingState', { defaultValue: 'Updating server lifecycle state' }) },
      ],
      action: () => api.toggleMCPServer(server.id, nextEnabled),
      onSuccess: () => {
        success(t('mcp.updated', { defaultValue: 'MCP Status Updated' }), server.id);
        loadTools();
      },
    });
  };

  const categories = [
    { id: 'all', label: t('tabs.all', { count: tools.length }) },
    { id: 'native', label: t('tabs.native', 'Native') },
    { id: 'mcp', label: t('tabs.mcp', 'MCP') },
  ];

  const filteredTools = tools.filter((tl) => {
    const matchesSearch =
      tl.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      tl.description.toLowerCase().includes(searchQuery.toLowerCase()) ||
      tl.category.toLowerCase().includes(searchQuery.toLowerCase());
    return matchesSearch;
  });

  return (
    <div className="relative min-h-[calc(100vh-64px)]">
      <BlobBackdrop />

      <PageContainer maxWidth="wide">
        <PageHeader eyebrow={t('eyebrow')} title={t('title')} description={t('subtitle')} actions={(
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="sm" icon={<RefreshCw className="h-3.5 w-3.5" />} onClick={loadTools}>
              {t('actions.refresh')}
            </Button>
            <Button
              variant="primary"
              size="sm"
              icon={<Plus className="w-3.5 h-3.5" />}
              onClick={() => setIsMcpModalOpen(true)}
            >
              {t('actions.addMCP', 'Connect MCP')}
            </Button>
          </div>
        )} />

        {/* Categories Bar & Search */}
        <div className="flex flex-col sm:flex-row items-center justify-between gap-4 mb-8">
          {/* Category Tabs */}
          <div className="flex flex-wrap items-center gap-1.5 bg-canvas/80 backdrop-blur-sm p-1 rounded-full border border-onyx/10 shadow-xs self-start sm:self-auto">
            {categories.map((c) => (
              <button
                key={c.id}
                onClick={() => setActiveCategory(c.id)}
                className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer ${activeCategory === c.id
                  ? 'bg-deep-ink text-white font-semibold shadow-xs'
                  : 'text-deep-ink hover:text-slate'
                  }`}
              >
                {c.label}
              </button>
            ))}
          </div>

          {/* Search Input */}
          <div className="relative max-w-xs w-full">
            <Search className="w-4 h-4 text-slate absolute left-3.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              placeholder={t('actions.search', 'Search tools...')}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full bg-soft-meadow text-deep-ink pl-10 pr-4 py-1.5 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink"
            />
          </div>
        </div>

        {/* Tools Grid */}
        {servers.length > 0 && (
          <div className="mb-8">
            <h2 className="font-serif text-heading-sm font-bold mb-3">{t('mcp.servers')}</h2>
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">
              {servers.map((server) => (
                <Card key={server.id} className="p-4 border border-onyx/10 flex items-center gap-3">
                  <span className={`w-2.5 h-2.5 rounded-full ${server.connected ? 'bg-deep-ink' : 'bg-slate/30'}`} />
                  <div className="min-w-0 flex-1">
                    <p className="font-semibold truncate">{server.id}</p>
                    <p className="text-caption text-slate truncate">{server.transport || 'stdio'} · {server.command || server.url}</p>
                  </div>
                  <button
                    type="button"
                    role="switch"
                    aria-checked={server.enabled}
                    disabled={isExecuting(server.id)}
                    onClick={() => handleToggleMCP(server)}
                    className={`w-11 h-6 rounded-full p-0.5 transition-colors ${server.enabled ? 'bg-deep-ink' : 'bg-slate/30'} ${isExecuting(server.id) ? 'opacity-50 cursor-not-allowed' : ''}`}
                  >
                    <span className={`block w-5 h-5 rounded-full bg-canvas transition-transform ${server.enabled ? 'translate-x-5' : ''}`} />
                  </button>
                </Card>
              ))}
            </div>
          </div>
        )}

        {loading ? (
          <div className="py-20 text-center text-slate font-sans">{t('loading')}</div>
        ) : filteredTools.length === 0 ? (
          <div className="bg-soft-meadow rounded-[24px] p-12 text-center max-w-md mx-auto border border-onyx/10">
            <Wrench className="w-12 h-12 text-deep-ink mx-auto mb-3 opacity-40" />
            <p className="font-sans text-body-sm text-slate mb-4">{t('empty')}</p>
            <Button variant="primary" size="sm" onClick={() => setIsMcpModalOpen(true)}>
              {t('actions.addMCP', 'Connect MCP')}
            </Button>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {filteredTools.map((tool) => (
              <ToolCard
                key={tool.name}
                tool={tool}
                onTest={(tl) => setTestingTool(tl)}
              />
            ))}
          </div>
        )}
      </PageContainer>

      {/* Connect MCP Modal */}
      <McpServerModal
        isOpen={isMcpModalOpen}
        onClose={() => setIsMcpModalOpen(false)}
        onConnect={handleConnectMCP}
      />

      {/* Test Execution Modal */}
      <ToolTestModal
        tool={testingTool}
        isOpen={!!testingTool}
        onClose={() => setTestingTool(null)}
      />
    </div>
  );
}
