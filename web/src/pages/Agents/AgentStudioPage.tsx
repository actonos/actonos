import { useState, useEffect } from 'react';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { useToast } from '@/components/ui/Toast';
import {
  Bot,
  ArrowLeft,
  Save,
  MessageSquare,
  Wrench,
  Cpu,
  Shield,
  FileCode,
  CheckCircle2,
  RotateCcw,
  Radio,
  Send,
  Phone,
  Sliders,
  Check,
} from 'lucide-react';
import { api } from '@/lib/api';
import type { AgentManifest, ToolInfo } from '@/lib/types';

export interface AgentStudioPageProps {
  agentID: string; // 'new' or existing agent ID like 'agent_system_core'
  onBack: () => void;
  onOpenChat: (agentID: string) => void;
}

type StudioTab = 'prompt' | 'model' | 'tools' | 'channels' | 'governance';

const ACTON_STANDARD_PROMPT = `You are an expert autonomous AI operator for ActonOS, adhering to the Acton Cognitive Architecture standards.

## 1. Core Identity & Role
- You operate with high technical rigor, explicit reasoning steps, and absolute reliability.
- You maintain a professional, objective, and solution-driven demeanor.

## 2. ReAct Cognitive Loop
- **Thought**: Before invoking tools or modifying files, explicitly analyze the objective and formulate your plan.
- **Action**: Execute tools adhering strictly to their JSON parameter schemas. Inspect inputs before running destructive commands.
- **Observation**: Thoroughly evaluate tool execution output. If an error occurs, perform root-cause analysis and self-correct.
- **Final Answer**: Deliver clear, concise, actionable solutions formatted in GitHub-flavored markdown.

## 3. Sandboxed Tool & Workspace Directives
- Always check if a file exists before overwriting.
- Maintain workspace cleanliness and respect isolated directory permissions.
- Validate code syntax and ensure unit tests pass whenever modifying software components.

## 4. Safety Invariants & Confidentiality
- Never expose API keys, credentials, or private pairing codes.
- Do not execute unverified commands outside the sandboxed runtime.
- Request user confirmation for high-risk operations.

## 5. Memory Reflection & Knowledge Sync
- Identify reusable patterns, architectural rules, and project decisions.
- Synchronize persistent context into episodic memory and project documentation.`;

const AVAILABLE_CHANNELS = [
  { id: 'telegram', label: 'Telegram', icon: Send, desc: 'Listen to Telegram bot chats and mentions' },
  { id: 'discord', label: 'Discord', icon: Bot, desc: 'Listen to Discord server messages and threads' },
  { id: 'whatsapp', label: 'WhatsApp', icon: Phone, desc: 'Listen to WhatsApp Cloud API messages' },
  { id: 'webhook', label: 'Inbound Webhook', icon: Sliders, desc: 'Trigger agent from HTTP Webhooks' },
];

export function AgentStudioPage({ agentID, onBack, onOpenChat }: AgentStudioPageProps) {
  const { success, error, info } = useToast();
  const isNew = agentID === 'new';

  const [activeTab, setActiveTab] = useState<StudioTab>('prompt');
  const [loading, setLoading] = useState(!isNew);
  const [saving, setSaving] = useState(false);
  const [toolsList, setToolsList] = useState<ToolInfo[]>([]);

  // Form State
  const [name, setName] = useState(isNew ? 'New Custom Agent' : '');
  const [idSlug, setIdSlug] = useState(isNew ? '' : agentID);
  const [description, setDescription] = useState('');
  const [avatarIcon, setAvatarIcon] = useState('bot');
  const [status, setStatus] = useState<'active' | 'stopped'>('active');
  const [isSystem, setIsSystem] = useState(false);

  // Model & Reasoning
  const [primaryModel, setPrimaryModel] = useState('anthropic/claude-3-7-sonnet');
  const [fallbackModel, setFallbackModel] = useState('google/gemini-2.5-flash');
  const [temperature, setTemperature] = useState(0.2);
  const [maxTokens, setMaxTokens] = useState(4096);

  // Prompt
  const [systemInstructions, setSystemInstructions] = useState(ACTON_STANDARD_PROMPT);

  // Tools
  const [authorizedTools, setAuthorizedTools] = useState<string[]>(['*']);

  // Chat Channels Listener Configuration
  const [listenAllChannels, setListenAllChannels] = useState(true);
  const [selectedChannels, setSelectedChannels] = useState<string[]>(['telegram', 'discord', 'whatsapp', 'webhook']);

  // Delegation Scope
  const [maxBudget, setMaxBudget] = useState(50);
  const [approvalLevel, setApprovalLevel] = useState<'Low' | 'Medium' | 'High'>('Medium');
  const [allowedPaths, setAllowedPaths] = useState('*');

  // Load Agent & Available Tools
  useEffect(() => {
    const fetchData = async () => {
      try {
        const toolsRes = await api.listTools().catch(() => ({ tools: [], count: 0 }));
        setToolsList(toolsRes.tools || []);

        if (!isNew) {
          const agent = await api.getAgent(agentID);
          setName(agent.name);
          setIdSlug(agent.agent_id);
          setDescription(agent.description || '');
          setAvatarIcon(agent.avatar_icon || 'bot');
          setStatus(agent.status === 'stopped' ? 'stopped' : 'active');
          setIsSystem(!!agent.is_system);

          if (agent.model_config) {
            setPrimaryModel(agent.model_config.primary_model || 'anthropic/claude-3-7-sonnet');
            setFallbackModel(agent.model_config.fallback_model || 'google/gemini-2.5-flash');
            setTemperature(agent.model_config.temperature ?? 0.2);
            setMaxTokens(agent.model_config.max_tokens ?? 4096);
          }

          setSystemInstructions(agent.system_instructions || ACTON_STANDARD_PROMPT);
          setAuthorizedTools(agent.authorized_tools || ['*']);

          // Load listen_channels
          const channels = agent.listen_channels || ['*'];
          if (channels.includes('*') || channels.length === 0) {
            setListenAllChannels(true);
            setSelectedChannels(['telegram', 'discord', 'whatsapp', 'webhook']);
          } else {
            setListenAllChannels(false);
            setSelectedChannels(channels);
          }

          if (agent.delegation_scope) {
            setMaxBudget(agent.delegation_scope.max_monthly_budget_usd ?? 50);
            setApprovalLevel(agent.delegation_scope.require_human_approval_level || 'Medium');
            setAllowedPaths(agent.delegation_scope.allowed_workspace_paths?.join(', ') || '*');
          }
        }
      } catch (err: any) {
        error('Failed to load agent details', err.message);
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, [agentID, isNew]);

  const handleSave = async () => {
    if (!name.trim()) {
      error('Validation Error', 'Agent name cannot be empty.');
      return;
    }

    setSaving(true);
    try {
      const listen_channels = listenAllChannels ? ['*'] : selectedChannels;

      const manifest: Partial<AgentManifest> = {
        agent_id: idSlug || undefined,
        name: name.trim(),
        description: description.trim(),
        avatar_icon: avatarIcon,
        status,
        model_config: {
          primary_model: primaryModel,
          fallback_model: fallbackModel,
          temperature,
          max_tokens: maxTokens,
        },
        system_instructions: systemInstructions,
        authorized_tools: authorizedTools,
        listen_channels,
        delegation_scope: {
          max_monthly_budget_usd: Number(maxBudget),
          allowed_workspace_paths: allowedPaths.split(',').map((p) => p.trim()).filter(Boolean),
          require_human_approval_level: approvalLevel,
        },
      };

      if (isNew) {
        await api.createAgent(manifest);
        success('Agent Created', `Agent "${name}" initialized successfully.`);
      } else {
        await api.updateAgent(agentID, manifest);
        success('Agent Saved', `Manifest for "${name}" updated in SQLite storage.`);
      }
    } catch (err: any) {
      error('Failed to save agent', err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleToggleTool = (toolName: string) => {
    if (authorizedTools.includes('*')) {
      const allNames = toolsList.map((t) => t.name).filter((n) => n !== toolName);
      setAuthorizedTools(allNames);
      return;
    }

    if (authorizedTools.includes(toolName)) {
      setAuthorizedTools(authorizedTools.filter((t) => t !== toolName));
    } else {
      setAuthorizedTools([...authorizedTools, toolName]);
    }
  };

  const handleSelectAllTools = () => {
    setAuthorizedTools(['*']);
    info('All Tools Selected', 'Agent is authorized to call all registered tools.');
  };

  const handleClearTools = () => {
    setAuthorizedTools([]);
  };

  const handleToggleChannel = (chId: string) => {
    if (selectedChannels.includes(chId)) {
      setSelectedChannels(selectedChannels.filter((c) => c !== chId));
    } else {
      setSelectedChannels([...selectedChannels, chId]);
    }
  };

  const loadStandardPreset = () => {
    setSystemInstructions(ACTON_STANDARD_PROMPT);
    info('Standard Preset Loaded', 'System instructions populated with ActonOS ReAct cognitive architecture prompt.');
  };

  if (loading) {
    return (
      <div className="py-24 text-center text-slate font-sans text-body">
        Loading Agent Studio...
      </div>
    );
  }

  const isAllToolsSelected = authorizedTools.includes('*');

  return (
    <div className="relative min-h-[calc(100vh-64px)]">
      <BlobBackdrop />

      <PageContainer>
        {/* Top Breadcrumb & Action Bar */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
          <div className="flex items-center gap-3">
            <button
              onClick={onBack}
              className="p-2 rounded-full bg-soft-meadow hover:bg-black/5 text-deep-ink border border-onyx/10 transition-all cursor-pointer"
              title="Back to agents list"
            >
              <ArrowLeft className="w-4 h-4" />
            </button>
            <div>
              <div className="flex items-center gap-2">
                <span className="text-caption uppercase tracking-wider text-slate font-semibold">
                  {isNew ? 'New Agent Studio' : 'Agent Studio Manifest'}
                </span>
                {isSystem && (
                  <Badge variant="active" className="text-[10px]">
                    ⭐ Root System Agent
                  </Badge>
                )}
              </div>
              <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight flex items-center gap-2">
                <span>{name || 'Untitled Agent'}</span>
              </h1>
            </div>
          </div>

          <div className="flex items-center gap-2.5 shrink-0 self-start sm:self-center">
            {!isNew && (
              <Button
                variant="ghost"
                size="sm"
                icon={<MessageSquare className="w-3.5 h-3.5" />}
                onClick={() => onOpenChat(agentID)}
              >
                Launch Session
              </Button>
            )}
            <Button
              variant="primary"
              size="sm"
              icon={<Save className="w-3.5 h-3.5" />}
              onClick={handleSave}
              disabled={saving}
            >
              {saving ? 'Saving...' : 'Save Agent'}
            </Button>
          </div>
        </div>

        {/* Identity & Basic Header Card */}
        <Card className="p-6 border border-onyx/10 bg-canvas/90 shadow-xs mb-8 space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            <div className="space-y-3 md:col-span-2">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <label className="block text-caption font-semibold text-deep-ink mb-1">
                    Agent Name
                  </label>
                  <Input
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="e.g., Senior Full-Stack Architect"
                  />
                </div>

                <div>
                  <label className="block text-caption font-semibold text-deep-ink mb-1">
                    Identifier (Slug)
                  </label>
                  <Input
                    value={idSlug}
                    onChange={(e) => setIdSlug(e.target.value)}
                    placeholder="e.g., agent_fullstack_lead"
                    disabled={!isNew || isSystem}
                  />
                </div>
              </div>

              <div>
                <label className="block text-caption font-semibold text-deep-ink mb-1">
                  Description & Role
                </label>
                <Input
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Describe the agent's responsibilities, expertise, and operational boundaries..."
                />
              </div>
            </div>

            {/* Status & Avatar */}
            <div className="p-4 bg-soft-meadow rounded-2xl border border-onyx/5 flex flex-col justify-between">
              <div>
                <span className="block text-caption font-semibold text-deep-ink mb-2">
                  Lifecycle Status
                </span>
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => setStatus('active')}
                    className={`px-3 py-1.5 rounded-full text-caption font-sans transition-all cursor-pointer ${
                      status === 'active'
                        ? 'bg-emerald-600 text-white font-semibold shadow-xs'
                        : 'bg-canvas text-deep-ink hover:bg-black/5'
                    }`}
                  >
                    Active
                  </button>
                  <button
                    type="button"
                    onClick={() => setStatus('stopped')}
                    className={`px-3 py-1.5 rounded-full text-caption font-sans transition-all cursor-pointer ${
                      status === 'stopped'
                        ? 'bg-red-600 text-white font-semibold shadow-xs'
                        : 'bg-canvas text-deep-ink hover:bg-black/5'
                    }`}
                  >
                    Stopped
                  </button>
                </div>
              </div>

              <div className="pt-3 border-t border-onyx/10 flex items-center justify-between text-caption text-slate font-mono">
                <span>Avatar: {avatarIcon}</span>
                <div className="w-8 h-8 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center">
                  <Bot className="w-4 h-4" />
                </div>
              </div>
            </div>
          </div>
        </Card>

        {/* Tab Navigation Capsule */}
        <div className="flex items-center gap-1.5 bg-canvas/80 backdrop-blur-sm p-1 rounded-full border border-onyx/10 shadow-xs mb-8 self-start sm:self-auto max-w-fit overflow-x-auto">
          <button
            onClick={() => setActiveTab('prompt')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${
              activeTab === 'prompt' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            📝 Instructions
          </button>
          <button
            onClick={() => setActiveTab('model')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${
              activeTab === 'model' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            🧠 LLM & Model
          </button>
          <button
            onClick={() => setActiveTab('tools')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${
              activeTab === 'tools' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            🧰 Tools ({isAllToolsSelected ? 'All (*)' : authorizedTools.length})
          </button>
          <button
            onClick={() => setActiveTab('channels')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${
              activeTab === 'channels' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            📡 Channels ({listenAllChannels ? 'All' : selectedChannels.length})
          </button>
          <button
            onClick={() => setActiveTab('governance')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${
              activeTab === 'governance' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            🛡️ Governance
          </button>
        </div>

        {/* TAB 1: System Prompt */}
        {activeTab === 'prompt' && (
          <Card className="p-6 border border-onyx/10 bg-canvas/90 space-y-4">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-3 border-b border-onyx/5">
              <div>
                <h3 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
                  <FileCode className="w-5 h-5 text-deep-ink" />
                  <span>Cognitive System Instructions</span>
                </h3>
                <p className="text-caption text-slate">
                  Defines the agent's core identity, reasoning loops, and directives.
                </p>
              </div>

              <Button
                variant="ghost"
                size="sm"
                icon={<RotateCcw className="w-3.5 h-3.5" />}
                onClick={loadStandardPreset}
              >
                Load Standard Preset
              </Button>
            </div>

            <textarea
              rows={18}
              value={systemInstructions}
              onChange={(e) => setSystemInstructions(e.target.value)}
              className="w-full bg-soft-meadow text-deep-ink p-4 rounded-2xl border border-onyx/10 font-mono text-body-sm leading-relaxed focus:outline-none focus:ring-2 focus:ring-deep-ink/20"
              placeholder="Enter system instructions..."
            />

            <div className="flex items-center justify-between text-caption font-mono text-slate">
              <span>Length: {systemInstructions.length} characters</span>
              <span className="text-emerald-700 font-semibold">ActonOS Standard</span>
            </div>
          </Card>
        )}

        {/* TAB 2: LLM & Reasoning */}
        {activeTab === 'model' && (
          <div className="space-y-6">
            <Card className="p-6 border border-onyx/10 bg-canvas/90 space-y-6">
              <h3 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
                <Cpu className="w-5 h-5 text-deep-ink" />
                <span>Model Configuration</span>
              </h3>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div>
                  <label className="block text-caption font-semibold text-deep-ink mb-1">
                    Primary LLM Model
                  </label>
                  <select
                    value={primaryModel}
                    onChange={(e) => setPrimaryModel(e.target.value)}
                    className="w-full bg-soft-meadow text-deep-ink p-2.5 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none"
                  >
                    <option value="anthropic/claude-3-7-sonnet">Anthropic Claude 3.7 Sonnet</option>
                    <option value="google/gemini-2.5-flash">Google Gemini 2.5 Flash</option>
                    <option value="openai/gpt-4o">OpenAI GPT-4o</option>
                    <option value="deepseek/deepseek-chat">DeepSeek-V3 Chat</option>
                    <option value="ollama/llama3.3">Local Ollama (Llama 3.3)</option>
                  </select>
                </div>

                <div>
                  <label className="block text-caption font-semibold text-deep-ink mb-1">
                    Fallback Recovery Model
                  </label>
                  <select
                    value={fallbackModel}
                    onChange={(e) => setFallbackModel(e.target.value)}
                    className="w-full bg-soft-meadow text-deep-ink p-2.5 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none"
                  >
                    <option value="google/gemini-2.5-flash">Google Gemini 2.5 Flash (Fast Fallback)</option>
                    <option value="anthropic/claude-3-7-sonnet">Anthropic Claude 3.7 Sonnet</option>
                    <option value="openai/gpt-4o">OpenAI GPT-4o</option>
                    <option value="deepseek/deepseek-chat">DeepSeek-V3 Chat</option>
                  </select>
                </div>
              </div>

              {/* Temperature Slider */}
              <div className="p-4 bg-soft-meadow rounded-2xl border border-onyx/5 space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-body-sm font-semibold text-deep-ink">
                    Creativity Temperature: <strong className="font-mono">{temperature.toFixed(2)}</strong>
                  </span>
                  <span className="text-caption text-slate">
                    {temperature <= 0.2
                      ? 'Deterministic & Code'
                      : temperature <= 0.7
                      ? 'Balanced Reasoning'
                      : 'Creative'}
                  </span>
                </div>
                <input
                  type="range"
                  min="0.0"
                  max="1.0"
                  step="0.05"
                  value={temperature}
                  onChange={(e) => setTemperature(parseFloat(e.target.value))}
                  className="w-full accent-deep-ink cursor-pointer"
                />
              </div>

              {/* Max Tokens Slider */}
              <div className="p-4 bg-soft-meadow rounded-2xl border border-onyx/5 space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-body-sm font-semibold text-deep-ink">
                    Max Generation Tokens: <strong className="font-mono">{maxTokens}</strong>
                  </span>
                  <span className="text-caption text-slate font-mono">1,024 - 16,384</span>
                </div>
                <input
                  type="range"
                  min="1024"
                  max="16384"
                  step="512"
                  value={maxTokens}
                  onChange={(e) => setMaxTokens(parseInt(e.target.value, 10))}
                  className="w-full accent-deep-ink cursor-pointer"
                />
              </div>
            </Card>
          </div>
        )}

        {/* TAB 3: Authorized Tools */}
        {activeTab === 'tools' && (
          <Card className="p-6 border border-onyx/10 bg-canvas/90 space-y-6">
            <div className="flex items-center justify-between pb-3 border-b border-onyx/5">
              <div>
                <h3 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
                  <Wrench className="w-5 h-5 text-deep-ink" />
                  <span>Authorized Tools</span>
                </h3>
                <p className="text-caption text-slate">
                  Select native tools, MCP servers, and skills this agent is permitted to execute.
                </p>
              </div>

              <div className="flex items-center gap-2">
                <Button variant="ghost" size="sm" onClick={handleClearTools}>
                  Clear All
                </Button>
                <Button variant="primary" size="sm" onClick={handleSelectAllTools}>
                  Select All (*)
                </Button>
              </div>
            </div>

            {isAllToolsSelected && (
              <div className="p-3.5 rounded-2xl bg-emerald-500/10 border border-emerald-500/30 text-emerald-900 text-body-sm flex items-center gap-2.5">
                <CheckCircle2 className="w-4 h-4 text-emerald-700 shrink-0" />
                <span>
                  <strong>Full Authorization:</strong> Agent can use all available tools and plugins.
                </span>
              </div>
            )}

            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
              {toolsList.map((tool) => {
                const isSelected = isAllToolsSelected || authorizedTools.includes(tool.name);

                return (
                  <div
                    key={tool.name}
                    onClick={() => handleToggleTool(tool.name)}
                    className={`p-4 rounded-2xl border transition-all cursor-pointer select-none flex flex-col justify-between ${
                      isSelected
                        ? 'bg-soft-meadow border-deep-ink/30 shadow-xs'
                        : 'bg-canvas border-onyx/10 hover:border-onyx/20 opacity-70'
                    }`}
                  >
                    <div>
                      <div className="flex items-center justify-between mb-1.5">
                        <span className="font-mono text-body-sm font-semibold text-deep-ink truncate">
                          {tool.name}
                        </span>
                        <Badge variant="neutral" className="text-[10px] uppercase">
                          {tool.category}
                        </Badge>
                      </div>
                      <p className="text-caption text-slate line-clamp-2">
                        {tool.description || 'No tool description provided.'}
                      </p>
                    </div>

                    <div className="pt-3 mt-3 border-t border-onyx/5 flex items-center justify-between text-caption font-mono">
                      <span className={isSelected ? 'text-emerald-700 font-semibold' : 'text-slate'}>
                        {isSelected ? '✓ Authorized' : 'Disabled'}
                      </span>
                    </div>
                  </div>
                );
              })}
            </div>
          </Card>
        )}

        {/* TAB 4: Chat Channels Listeners */}
        {activeTab === 'channels' && (
          <Card className="p-6 border border-onyx/10 bg-canvas/90 space-y-6">
            <div className="flex items-center justify-between pb-3 border-b border-onyx/5">
              <div>
                <h3 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
                  <Radio className="w-5 h-5 text-deep-ink" />
                  <span>Chat Channels Inbound Listeners</span>
                </h3>
                <p className="text-caption text-slate">
                  Specify which messaging channels trigger this agent when messages or mentions are received.
                </p>
              </div>
            </div>

            {/* Mode selection radio cards */}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div
                onClick={() => setListenAllChannels(true)}
                className={`p-5 rounded-2xl border transition-all cursor-pointer ${
                  listenAllChannels
                    ? 'bg-soft-meadow border-deep-ink shadow-xs ring-1 ring-deep-ink'
                    : 'bg-canvas border-onyx/10 hover:border-onyx/20'
                }`}
              >
                <div className="flex items-center justify-between mb-2">
                  <span className="font-semibold text-body-sm text-deep-ink">
                    All Chat Channels (Default)
                  </span>
                  <div className={`w-5 h-5 rounded-full border-2 flex items-center justify-center ${listenAllChannels ? 'border-deep-ink bg-deep-ink text-white' : 'border-onyx/30'}`}>
                    {listenAllChannels && <Check className="w-3 h-3" />}
                  </div>
                </div>
                <p className="text-caption text-slate">
                  Agent listens to incoming messages across Telegram, Discord, WhatsApp, and Webhook.
                </p>
              </div>

              <div
                onClick={() => setListenAllChannels(false)}
                className={`p-5 rounded-2xl border transition-all cursor-pointer ${
                  !listenAllChannels
                    ? 'bg-soft-meadow border-deep-ink shadow-xs ring-1 ring-deep-ink'
                    : 'bg-canvas border-onyx/10 hover:border-onyx/20'
                }`}
              >
                <div className="flex items-center justify-between mb-2">
                  <span className="font-semibold text-body-sm text-deep-ink">
                    Specific Channels Only
                  </span>
                  <div className={`w-5 h-5 rounded-full border-2 flex items-center justify-center ${!listenAllChannels ? 'border-deep-ink bg-deep-ink text-white' : 'border-onyx/30'}`}>
                    {!listenAllChannels && <Check className="w-3 h-3" />}
                  </div>
                </div>
                <p className="text-caption text-slate">
                  Restrict this agent to only respond to specific channels chosen below.
                </p>
              </div>
            </div>

            {/* Channel Checkboxes (shown when specific channels selected) */}
            {!listenAllChannels && (
              <div className="space-y-3 pt-2">
                <span className="text-caption font-semibold text-deep-ink uppercase tracking-wider block">
                  Select Channels to Listen
                </span>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  {AVAILABLE_CHANNELS.map((ch) => {
                    const Icon = ch.icon;
                    const isChecked = selectedChannels.includes(ch.id);

                    return (
                      <div
                        key={ch.id}
                        onClick={() => handleToggleChannel(ch.id)}
                        className={`p-3.5 rounded-2xl border transition-all cursor-pointer flex items-center justify-between ${
                          isChecked
                            ? 'bg-soft-meadow border-onyx/20 shadow-xs'
                            : 'bg-canvas border-onyx/10 opacity-60'
                        }`}
                      >
                        <div className="flex items-center gap-3">
                          <div className="w-8 h-8 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center shrink-0">
                            <Icon className="w-4 h-4" />
                          </div>
                          <div>
                            <span className="font-semibold text-body-sm text-deep-ink block">
                              {ch.label}
                            </span>
                            <span className="text-[11px] text-slate">{ch.desc}</span>
                          </div>
                        </div>

                        <div className={`w-5 h-5 rounded-full border-2 flex items-center justify-center shrink-0 ${isChecked ? 'bg-emerald-500 border-emerald-500 text-white' : 'border-onyx/20'}`}>
                          {isChecked && <Check className="w-3 h-3" />}
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}
          </Card>
        )}

        {/* TAB 5: Governance & Scope */}
        {activeTab === 'governance' && (
          <Card className="p-6 border border-onyx/10 bg-canvas/90 space-y-6">
            <h3 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
              <Shield className="w-5 h-5 text-deep-ink" />
              <span>Governance & Permissions</span>
            </h3>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
              <div>
                <label className="block text-caption font-semibold text-deep-ink mb-1">
                  Monthly Budget Cap ($ USD)
                </label>
                <Input
                  type="number"
                  value={maxBudget}
                  onChange={(e) => setMaxBudget(parseFloat(e.target.value) || 0)}
                  placeholder="50"
                />
                <span className="text-[11px] text-slate mt-1 block">
                  Agent halts when monthly token cost reaches this limit.
                </span>
              </div>

              <div>
                <label className="block text-caption font-semibold text-deep-ink mb-1">
                  Human Approval Level
                </label>
                <select
                  value={approvalLevel}
                  onChange={(e) => setApprovalLevel(e.target.value as any)}
                  className="w-full bg-soft-meadow text-deep-ink p-2.5 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none"
                >
                  <option value="Low">Low (Autonomous Auto-Execute)</option>
                  <option value="Medium">Medium (Workspace Writes Allowed)</option>
                  <option value="High">High (Confirm on Sensitive Actions)</option>
                </select>
              </div>
            </div>

            <div>
              <label className="block text-caption font-semibold text-deep-ink mb-1">
                Allowed Workspace Paths
              </label>
              <Input
                value={allowedPaths}
                onChange={(e) => setAllowedPaths(e.target.value)}
                placeholder="e.g., *, /data/workspace/project/*"
              />
              <span className="text-[11px] text-slate mt-1 block">
                Comma-separated file paths or wildcards (*) that this agent is permitted to access.
              </span>
            </div>
          </Card>
        )}
      </PageContainer>
    </div>
  );
}
