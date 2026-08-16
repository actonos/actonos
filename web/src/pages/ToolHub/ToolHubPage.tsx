import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Modal } from '@/components/ui/Modal';
import { ToolCard } from '@/components/features/tools/ToolCard';
import { McpServerModal } from '@/components/features/tools/McpServerModal';
import { ToolTestModal } from '@/components/features/tools/ToolTestModal';
import {
  Plus,
  RefreshCw,
  Search,
  Upload,
  FolderPlus,
  Wrench,
} from 'lucide-react';
import { api } from '@/lib/api';
import type { ToolInfo } from '@/lib/types';

export function ToolHubPage() {
  const { t } = useTranslation('tools');
  const [tools, setTools] = useState<ToolInfo[]>([]);
  const [activeCategory, setActiveCategory] = useState<string>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [isMcpModalOpen, setIsMcpModalOpen] = useState(false);
  const [isSkillModalOpen, setIsSkillModalOpen] = useState(false);
  const [testingTool, setTestingTool] = useState<ToolInfo | null>(null);
  const [loading, setLoading] = useState(true);

  // New Skill form state
  const [skillName, setSkillName] = useState('');
  const [skillDesc, setSkillDesc] = useState('');
  const [skillContent, setSkillContent] = useState('');
  const [creatingSkill, setCreatingSkill] = useState(false);

  const wasmInputRef = useRef<HTMLInputElement>(null);

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

  const handleCreateSkill = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!skillName.trim()) return;

    setCreatingSkill(true);
    try {
      await api.createSkill({
        name: skillName.trim().toLowerCase().replace(/\s+/g, '_'),
        description: skillDesc.trim(),
        content: skillContent.trim(),
      });
      alert('Skill-as-a-Folder created and hot-reloaded successfully!');
      setIsSkillModalOpen(false);
      setSkillName('');
      setSkillDesc('');
      setSkillContent('');
      loadTools();
    } catch (err: any) {
      alert(`Create skill failed: ${err.message}`);
    } finally {
      setCreatingSkill(false);
    }
  };

  const handleUploadWASM = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    try {
      await api.uploadWASM(file);
      alert(`Uploaded WASM plugin ${file.name} to sandbox runner!`);
      loadTools();
    } catch (err: any) {
      alert(`Upload failed: ${err.message}`);
    }
  };

  const categories = [
    { id: 'all', label: t('tabs.all', { count: tools.length }) },
    { id: 'native', label: t('tabs.native') },
    { id: 'mcp', label: t('tabs.mcp') },
    { id: 'wasm', label: t('tabs.wasm') },
    { id: 'skill', label: t('tabs.skill') },
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

      <PageContainer>
        {/* Header section */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
          <div className="flex-1">
            <span className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
              {t('eyebrow', 'Dynamic Tooling Hub')}
            </span>
            <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight">
              {t('title', 'Tool Registry & Integrations')}
            </h1>
            <p className="font-sans text-body text-slate mt-1 max-w-2xl">
              {t('subtitle', 'Discover and test Native POSIX tools, Model Context Protocol (MCP) servers, WASM sandboxed binaries, and hot-reloading Skill-as-a-Folder packages.')}
            </p>
          </div>

          <div className="flex flex-wrap items-center gap-2.5 shrink-0 self-start sm:self-center">
            <Button
              variant="ghost"
              size="sm"
              icon={<RefreshCw className="w-3.5 h-3.5" />}
              onClick={loadTools}
            >
              {t('actions.refresh', 'Refresh')}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              icon={<FolderPlus className="w-3.5 h-3.5" />}
              onClick={() => setIsSkillModalOpen(true)}
            >
              New Skill
            </Button>
            <Button
              variant="ghost"
              size="sm"
              icon={<Upload className="w-3.5 h-3.5" />}
              onClick={() => wasmInputRef.current?.click()}
            >
              Upload WASM
            </Button>
            <input
              type="file"
              ref={wasmInputRef}
              onChange={handleUploadWASM}
              accept=".wasm"
              className="hidden"
            />
            <Button
              variant="primary"
              size="sm"
              icon={<Plus className="w-3.5 h-3.5" />}
              onClick={() => setIsMcpModalOpen(true)}
            >
              {t('actions.addMCP', 'Connect MCP')}
            </Button>
          </div>
        </div>

        {/* Toolbar: Category Tabs & Search */}
        <div className="flex flex-col md:flex-row items-stretch md:items-center justify-between gap-4 mb-8">
          {/* Category Tabs */}
          <div className="flex flex-wrap gap-1.5 p-1 bg-soft-meadow rounded-full w-fit border border-onyx/10">
            {categories.map((c) => (
              <button
                key={c.id}
                onClick={() => setActiveCategory(c.id)}
                className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer ${
                  activeCategory === c.id
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
              placeholder="Search tools..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full bg-soft-meadow text-deep-ink pl-10 pr-4 py-1.5 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink"
            />
          </div>
        </div>

        {/* Tools Grid */}
        {loading ? (
          <div className="py-20 text-center text-slate font-sans">Loading tools registry...</div>
        ) : filteredTools.length === 0 ? (
          <div className="bg-soft-meadow rounded-[24px] p-12 text-center max-w-md mx-auto border border-onyx/10">
            <Wrench className="w-12 h-12 text-deep-ink mx-auto mb-3 opacity-40" />
            <p className="font-sans text-body-sm text-slate mb-4">No tools found matching your query.</p>
            <Button variant="primary" size="sm" onClick={() => setIsMcpModalOpen(true)}>
              {t('actions.addMCP', 'Connect MCP Server')}
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

      {/* Create Skill-as-a-Folder Modal */}
      <Modal
        isOpen={isSkillModalOpen}
        onClose={() => setIsSkillModalOpen(false)}
        title="Create Skill-as-a-Folder"
      >
        <form onSubmit={handleCreateSkill} className="space-y-4">
          <div>
            <label className="text-caption uppercase text-slate font-semibold block mb-1">
              Skill Name (Slug)
            </label>
            <Input
              placeholder="e.g. market_researcher"
              value={skillName}
              onChange={(e) => setSkillName(e.target.value)}
              required
            />
          </div>

          <div>
            <label className="text-caption uppercase text-slate font-semibold block mb-1">
              Description
            </label>
            <Input
              placeholder="Describe when agents should invoke this skill..."
              value={skillDesc}
              onChange={(e) => setSkillDesc(e.target.value)}
            />
          </div>

          <div>
            <label className="text-caption uppercase text-slate font-semibold block mb-1">
              Skill Instructions (SKILL.md Markdown Content)
            </label>
            <textarea
              rows={6}
              value={skillContent}
              onChange={(e) => setSkillContent(e.target.value)}
              placeholder="Provide procedural instructions, workflow steps, and execution guidelines..."
              className="w-full bg-canvas text-deep-ink font-mono text-body-sm p-4 rounded-[16px] border border-onyx/20 focus:outline-none focus:ring-2 focus:ring-deep-ink"
            />
          </div>

          <Button
            type="submit"
            variant="primary"
            disabled={creatingSkill || !skillName.trim()}
            className="w-full justify-center"
          >
            {creatingSkill ? 'Creating Skill...' : 'Create & Register Skill'}
          </Button>
        </form>
      </Modal>

      {/* Test Execution Modal */}
      <ToolTestModal
        tool={testingTool}
        isOpen={!!testingTool}
        onClose={() => setTestingTool(null)}
      />
    </div>
  );
}
