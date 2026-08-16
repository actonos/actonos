import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Button } from '@/components/ui/Button';
import { ToolCard } from '@/components/features/tools/ToolCard';
import { McpServerModal } from '@/components/features/tools/McpServerModal';
import { ToolTestModal } from '@/components/features/tools/ToolTestModal';
import { Plus, RefreshCw } from 'lucide-react';
import { api } from '@/lib/api';
import type { ToolInfo } from '@/lib/types';

export function ToolHubPage() {
  const { t } = useTranslation('tools');
  const [tools, setTools] = useState<ToolInfo[]>([]);
  const [activeCategory, setActiveCategory] = useState<string>('all');
  const [isMcpModalOpen, setIsMcpModalOpen] = useState(false);
  const [testingTool, setTestingTool] = useState<ToolInfo | null>(null);
  const [loading, setLoading] = useState(true);

  const loadTools = async () => {
    try {
      setLoading(true);
      const res = await api.listTools(activeCategory === 'all' ? undefined : activeCategory);
      setTools(res.tools || []);
    } catch (err) {
      console.error('Failed to load tools:', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadTools();
  }, [activeCategory]);

  const handleConnectMCP = async (cfg: { id: string; command: string; args?: string[] }) => {
    await api.connectMCP(cfg);
    loadTools();
  };

  const categories = [
    { id: 'all', label: t('tabs.all', { count: tools.length }) },
    { id: 'native', label: t('tabs.native') },
    { id: 'mcp', label: t('tabs.mcp') },
    { id: 'wasm', label: t('tabs.wasm') },
    { id: 'skill', label: t('tabs.skill') },
  ];

  return (
    <div className="relative min-h-[calc(100vh-72px)]">
      <BlobBackdrop />

      <PageContainer>
        {/* Header section */}
        <div className="flex flex-col md:flex-row md:items-end justify-between gap-4 mb-8">
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

          <div className="flex items-center gap-3">
            <Button
              variant="ghost"
              size="sm"
              icon={<RefreshCw className="w-4 h-4" />}
              onClick={loadTools}
            >
              {t('actions.refresh')}
            </Button>
            <Button
              variant="primary"
              size="sm"
              icon={<Plus className="w-4 h-4" />}
              onClick={() => setIsMcpModalOpen(true)}
            >
              {t('actions.addMCP')}
            </Button>
          </div>
        </div>

        {/* Category Tabs */}
        <div className="flex flex-wrap gap-2 mb-8 p-1.5 bg-soft-meadow rounded-full w-fit border border-onyx/10">
          {categories.map((c) => (
            <button
              key={c.id}
              onClick={() => setActiveCategory(c.id)}
              className={`px-5 py-2 rounded-full text-body-sm font-sans font-medium transition-all cursor-pointer ${
                activeCategory === c.id
                  ? 'bg-deep-ink text-white font-semibold shadow-xs'
                  : 'text-deep-ink hover:text-slate'
              }`}
            >
              {c.label}
            </button>
          ))}
        </div>

        {/* Tools Grid */}
        {loading ? (
          <div className="py-20 text-center text-slate font-sans">Loading tools registry...</div>
        ) : tools.length === 0 ? (
          <div className="bg-soft-meadow rounded-[24px] p-12 text-center max-w-md mx-auto">
            <p className="font-sans text-body-sm text-slate mb-4">No tools found in this category.</p>
            <Button variant="primary" size="sm" onClick={() => setIsMcpModalOpen(true)}>
              {t('actions.addMCP')}
            </Button>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {tools.map((tool) => (
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
