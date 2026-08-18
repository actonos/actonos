import { useState, useEffect } from 'react';
import { getErrorMessage } from '@/lib/errors';
import { useTranslation } from 'react-i18next';
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
  AlertTriangle,
  RotateCcw,
  Radio,
  Send,
  Phone,
  Sliders,
  Check,
  Sparkles,
  Brain,
  RefreshCw,
} from 'lucide-react';
import { api } from '@/lib/api';
import type { AgentManifest, ApprovalLevel, ToolInfo, LLMProviderInfo } from '@/lib/types';
import { LATEST_MODEL_CATALOG, getCategorizedModels } from '@/lib/models';

export interface AgentStudioPageProps {
  agentID: string; // 'new' or existing agent ID like 'agent_system_core'
  onBack: () => void;
  onOpenChat: (agentID: string) => void;
}

type StudioTab = 'prompt' | 'soul' | 'memory' | 'model' | 'tools' | 'channels' | 'governance';

const ACTON_STANDARD_SOUL = `# ActonOS Agent Soul (SOUL.md)

## 1. Core Persona & Identity
You are an autonomous AI companion and domain specialist running on the ActonOS local kernel.
You possess high IQ, deep technical intuition, and empathetic, natural human communication skills.

## 2. Demeanor & Conversational Standard
- **Natural & Humanlike**: Communicate with warmth, clarity, and intellectual humility.
- **Zero Robotic Clichés**: Never output stiff preambles ("As an AI model...", "I hope this helps!"), repetitive disclaimers, or robotic fluff.
- **Adaptive Dynamics**: Be direct and razor-sharp for urgent bugs; thoughtful, structured, and deep for architecture planning.

## 3. Cognitive Decision Principles
- Validate prerequisites and critically evaluate tool outputs before drawing conclusions.
- If a tool or command fails, autonomously reflect, troubleshoot, and explore alternative paths.
- Deliver production-ready code with complete implementations (never leave unfinished placeholders).

## 4. Safety, Vault & Boundaries
- Never leak private keys, passwords, or authentication tokens.
- Request human operator approval when performing destructive actions outside local sandbox bounds.`;

const ACTON_STANDARD_PROMPT = `You are an expert autonomous AI operator for ActonOS, adhering to the Acton Cognitive Architecture standards.

## 1. Core Identity & Role
- You are an elite engineering companion and autonomous AI operator with high IQ and high EQ.
- You communicate naturally, thoughtfully, and empathetically—never sounding like a rigid, robotic script.

## 2. ReAct Cognitive Loop & Decision Protocol
- **Thought**: Formulate a clear, deep rationale before invoking tools or altering configurations.
- **Action**: Execute authorized sandboxed tools strictly adhering to JSON schemas with precision.
- **Observation**: Critically evaluate execution output and autonomously self-correct on any errors.
- **Final Answer**: Deliver clean, insightful, and beautifully formatted markdown with production-grade code.

## 3. Conversational Standard & Demeanor
- Speak with natural warmth, intellect, and clarity.
- Avoid robotic clichés ("As an AI...", "I am happy to assist..."), empty filler, or stiff canned phrases.
- Adapt dynamically: be fast & direct for urgent bugs; deep & creative for architectural discussions.

## 4. Safety & Invariants
- Zero-trust handling of API credentials, authentication tokens, and private secrets.
- Always respect workspace boundaries and request confirmation for irreversible operations.

## 5. Memory & Context Reflection
- Synthesize user preferences, project conventions, and key decisions into persistent memory.`;

const AVAILABLE_CHANNELS = [
  { id: 'telegram', label: 'Telegram', icon: Send, desc: 'Listen to Telegram bot chats and mentions' },
  { id: 'discord', label: 'Discord', icon: Bot, desc: 'Listen to Discord server messages and threads' },
  { id: 'whatsapp', label: 'WhatsApp', icon: Phone, desc: 'Listen to WhatsApp Cloud API messages' },
  { id: 'webhook', label: 'Inbound Webhook', icon: Sliders, desc: 'Trigger agent from HTTP Webhooks' },
];

export function AgentStudioPage({ agentID, onBack, onOpenChat }: AgentStudioPageProps) {
  const { t } = useTranslation('agents');
  const { success, error, info } = useToast();
  const isNew = agentID === 'new';

  const [activeTab, setActiveTab] = useState<StudioTab>('prompt');
  const [loading, setLoading] = useState(!isNew);
  const [saving, setSaving] = useState(false);
  const [toolsList, setToolsList] = useState<ToolInfo[]>([]);

  // LLM Providers from Settings
  const [configuredProviders, setConfiguredProviders] = useState<LLMProviderInfo[]>([]);
  const [customPrimaryMode, setCustomPrimaryMode] = useState(false);
  const [customFallbackMode, setCustomFallbackMode] = useState(false);

  // Form State
  const [name, setName] = useState(isNew ? 'New Custom Agent' : '');
  const [idSlug, setIdSlug] = useState(isNew ? '' : agentID);
  const [description, setDescription] = useState('');
  const [avatarIcon, setAvatarIcon] = useState('bot');
  const [status, setStatus] = useState<'active' | 'stopped'>('active');
  const [isSystem, setIsSystem] = useState(false);

  // Model & Reasoning
  const [primaryModel, setPrimaryModel] = useState('anthropic/claude-sonnet-4-6');
  const [fallbackModel, setFallbackModel] = useState('openai/gpt-5-mini');
  const [temperature, setTemperature] = useState(0.2);
  const [maxTokens, setMaxTokens] = useState(4096);

  // Prompt & Soul & Memory
  const [systemInstructions, setSystemInstructions] = useState(ACTON_STANDARD_PROMPT);
  const [soul, setSoul] = useState(ACTON_STANDARD_SOUL);
  const [memoryMD, setMemoryMD] = useState('');

  // Tools
  const [authorizedTools, setAuthorizedTools] = useState<string[]>(['*']);

  // Chat Channels Listener Configuration
  const [listenAllChannels, setListenAllChannels] = useState(true);
  const [selectedChannels, setSelectedChannels] = useState<string[]>(['telegram', 'discord', 'whatsapp', 'webhook']);

  // Delegation Scope
  const [maxBudget, setMaxBudget] = useState(50);
  const [approvalLevel, setApprovalLevel] = useState<'Low' | 'Medium' | 'High'>('Medium');
  const [allowedPaths, setAllowedPaths] = useState('*');

  // Load Agent, Available Tools & Active LLM Providers
  useEffect(() => {
    const fetchData = async () => {
      try {
        const [toolsRes, keysRes] = await Promise.all([
          api.listTools().catch(() => ({ tools: [], count: 0 })),
          api.getAPIKeys().catch(() => null),
        ]);

        setToolsList(toolsRes.tools || []);
        if (keysRes?.providers) {
          setConfiguredProviders(keysRes.providers);
        }

        if (!isNew) {
          const agent = await api.getAgent(agentID);
          setName(agent.name);
          setIdSlug(agent.agent_id);
          setDescription(agent.description || '');
          setAvatarIcon(agent.avatar_icon || 'bot');
          setStatus(agent.status === 'stopped' ? 'stopped' : 'active');
          setIsSystem(!!agent.is_system);

          if (agent.model_config) {
            const pMod = agent.model_config.primary_model || 'anthropic/claude-sonnet-4-6';
            const fMod = agent.model_config.fallback_model || 'openai/gpt-5-mini';
            setPrimaryModel(pMod);
            setFallbackModel(fMod);
            setTemperature(agent.model_config.temperature ?? 0.2);
            setMaxTokens(agent.model_config.max_tokens ?? 4096);

            // Check if models are not in the standard catalog to switch to custom text input
            if (!LATEST_MODEL_CATALOG.some((m) => m.id === pMod)) {
              setCustomPrimaryMode(true);
            }
            if (!LATEST_MODEL_CATALOG.some((m) => m.id === fMod)) {
              setCustomFallbackMode(true);
            }
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

          // Load dedicated SOUL.md & MEMORY.md
          try {
            const [soulRes, memRes] = await Promise.all([
              api.getSoul(agentID).catch(() => null),
              api.getMemoryMD(agentID).catch(() => null),
            ]);
            if (soulRes?.soul || soulRes?.content) {
              setSoul(soulRes.soul || soulRes.content);
            }
            if (memRes?.memory_md) {
              setMemoryMD(memRes.memory_md);
            }
          } catch {}
        }
      } catch (err) {
        error('Failed to load agent details', getErrorMessage(err));
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
        status: status || 'active',
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

      const targetID = isNew ? (idSlug.trim() || `agent_${Date.now()}`) : agentID;

      if (isNew) {
        await api.createAgent(manifest);
      } else {
        await api.updateAgent(agentID, manifest);
      }

      // Save dedicated SOUL.md if present
      if (soul && soul.trim()) {
        try {
          await api.saveSoul(soul, targetID);
        } catch (sErr) {
          console.warn('Failed to save SOUL.md:', sErr);
        }
      }

      if (isNew) {
        success('Agent Created', `Agent "${name}" & SOUL persona initialized.`);
      } else {
        success('Agent Saved', `Manifest & SOUL persona for "${name}" updated.`);
      }
    } catch (err) {
      error('Failed to save agent', getErrorMessage(err));
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

  const loadStandardSoulTemplate = () => {
    setSoul(ACTON_STANDARD_SOUL);
    info('Standard Soul Loaded', 'Populated SOUL.md with ActonOS persona blueprint.');
  };

  // Helper to find provider status for a model ID string
  const getProviderForModel = (modelId: string) => {
    const prefix = modelId.includes('/') ? modelId.split('/')[0] : modelId;
    const cleanPrefix = prefix.replace('google', 'gemini');
    return configuredProviders.find((p) => p.id === cleanPrefix || p.id === prefix);
  };

  const isModelProviderConfigured = (modelId: string) => {
    if (modelId.startsWith('ollama/')) return true;
    const p = getProviderForModel(modelId);
    return !!(p && p.configured && p.enabled);
  };

  const { readyModels, otherModels } = getCategorizedModels(configuredProviders);

  if (loading) {
    return (
      <div className="py-24 text-center text-slate font-sans text-body">
        {t('studio.loading')}
      </div>
    );
  }

  const isAllToolsSelected = authorizedTools.includes('*');
  const primaryProviderConfig = getProviderForModel(primaryModel);
  const primaryIsReady = isModelProviderConfigured(primaryModel);

  const fallbackProviderConfig = getProviderForModel(fallbackModel);
  const fallbackIsReady = isModelProviderConfigured(fallbackModel);

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
              title={t('studio.back')}
            >
              <ArrowLeft className="w-4 h-4" />
            </button>
            <div>
              <div className="flex items-center gap-2">
                <span className="text-caption uppercase tracking-wider text-slate font-semibold">
                  {isNew ? t('studio.newEyebrow') : t('studio.eyebrow')}
                </span>
                {isSystem && (
                  <Badge variant="active" className="text-[10px]">
                    {t('studio.rootBadge')}
                  </Badge>
                )}
              </div>
              <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight flex items-center gap-2">
                <span>{name || t('studio.untitled')}</span>
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
                {t('studio.launch')}
              </Button>
            )}
            <Button
              variant="primary"
              size="sm"
              icon={<Save className="w-3.5 h-3.5" />}
              onClick={handleSave}
              disabled={saving}
            >
              {saving ? t('studio.saving') : t('studio.save')}
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
                    {t('studio.name')}
                  </label>
                  <Input
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder={t('studio.namePlaceholder')}
                  />
                </div>

                <div>
                  <label className="block text-caption font-semibold text-deep-ink mb-1">
                    {t('studio.identifier')}
                  </label>
                  <Input
                    value={idSlug}
                    onChange={(e) => setIdSlug(e.target.value)}
                    placeholder={t('studio.identifierPlaceholder')}
                    disabled={!isNew || isSystem}
                  />
                </div>
              </div>

              <div>
                <label className="block text-caption font-semibold text-deep-ink mb-1">
                  {t('studio.description')}
                </label>
                <Input
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder={t('studio.descriptionPlaceholder')}
                />
              </div>
            </div>

            {/* Status & Avatar */}
            <div className="p-4 bg-soft-meadow rounded-2xl border border-onyx/5 flex flex-col justify-between">
              <div>
                <span className="block text-caption font-semibold text-deep-ink mb-2">
                  {t('studio.lifecycle')}
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
                    {t('studio.active')}
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
                    {t('studio.stopped')}
                  </button>
                </div>
              </div>

              <div className="pt-3 border-t border-onyx/10 flex items-center justify-between text-caption text-slate font-mono">
                <span>{t('studio.avatar', { icon: avatarIcon })}</span>
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
            {t('studio.tabs.instructions')}
          </button>
          <button
            onClick={() => setActiveTab('soul')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${
              activeTab === 'soul' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            {t('studio.tabs.soul')}
          </button>
          <button
            onClick={() => setActiveTab('memory')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${
              activeTab === 'memory' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            {t('studio.tabs.memory')}
          </button>
          <button
            onClick={() => setActiveTab('model')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${
              activeTab === 'model' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            {t('studio.tabs.model', { status: primaryIsReady ? t('studio.ready') : t('studio.keyNeeded') })}
          </button>
          <button
            onClick={() => setActiveTab('tools')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${
              activeTab === 'tools' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            {t('studio.tabs.tools', { value: isAllToolsSelected ? t('studio.allTools') : authorizedTools.length })}
          </button>
          <button
            onClick={() => setActiveTab('channels')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${
              activeTab === 'channels' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            {t('studio.tabs.channels', { value: listenAllChannels ? t('studio.all') : selectedChannels.length })}
          </button>
          <button
            onClick={() => setActiveTab('governance')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${
              activeTab === 'governance' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            {t('studio.tabs.governance')}
          </button>
        </div>

        {/* TAB 1: System Prompt */}
        {activeTab === 'prompt' && (
          <Card className="p-6 border border-onyx/10 bg-canvas/90 space-y-4">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-3 border-b border-onyx/5">
              <div>
                <h3 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
                  <FileCode className="w-5 h-5 text-deep-ink" />
                  <span>{t('studio.prompt.title')}</span>
                </h3>
                <p className="text-caption text-slate">
                  {t('studio.prompt.description')}
                </p>
              </div>

              <Button
                variant="ghost"
                size="sm"
                icon={<RotateCcw className="w-3.5 h-3.5" />}
                onClick={loadStandardPreset}
              >
                {t('studio.prompt.loadPreset')}
              </Button>
            </div>

            <textarea
              rows={18}
              value={systemInstructions}
              onChange={(e) => setSystemInstructions(e.target.value)}
              className="w-full bg-soft-meadow text-deep-ink p-4 rounded-2xl border border-onyx/10 font-mono text-body-sm leading-relaxed focus:outline-none focus:ring-2 focus:ring-deep-ink/20"
              placeholder={t('studio.prompt.placeholder')}
            />

            <div className="flex items-center justify-between text-caption font-mono text-slate">
              <span>{t('studio.length', { count: systemInstructions.length })}</span>
              <span className="text-emerald-700 font-semibold">{t('studio.prompt.standard')}</span>
            </div>
          </Card>
        )}

        {/* TAB: Dedicated Agent SOUL.md Editor */}
        {activeTab === 'soul' && (
          <Card className="p-6 border border-onyx/10 bg-canvas/90 space-y-4">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-3 border-b border-onyx/5">
              <div>
                <h3 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
                  <Sparkles className="w-5 h-5 text-amber-500" />
                  <span>{t('studio.soul.title')}</span>
                </h3>
                <p className="text-caption text-slate">
                  {t('studio.soul.description')}
                </p>
              </div>

              <div className="flex items-center gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  icon={<RotateCcw className="w-3.5 h-3.5" />}
                  onClick={loadStandardSoulTemplate}
                >
                  {t('studio.soul.loadTemplate')}
                </Button>
              </div>
            </div>

            <textarea
              rows={18}
              value={soul}
              onChange={(e) => setSoul(e.target.value)}
              className="w-full bg-soft-meadow text-deep-ink p-4 rounded-2xl border border-onyx/10 font-mono text-body-sm leading-relaxed focus:outline-none focus:ring-2 focus:ring-deep-ink/20"
              placeholder={t('studio.soul.placeholder')}
            />

            <div className="flex items-center justify-between text-caption font-mono text-slate">
              <span>{t('studio.length', { count: soul.length })}</span>
              <span className="text-amber-700 font-semibold flex items-center gap-1">
                <Sparkles className="w-3.5 h-3.5" /> {t('studio.soul.isolated')}
              </span>
            </div>
          </Card>
        )}

        {/* TAB: Persistent Episodic Memory & Reflection (MEMORY.md) */}
        {activeTab === 'memory' && (
          <Card className="p-6 border border-onyx/10 bg-canvas/90 space-y-4">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-3 border-b border-onyx/5">
              <div>
                <h3 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
                  <Brain className="w-5 h-5 text-indigo-600" />
                  <span>{t('studio.memory.title')}</span>
                </h3>
                <p className="text-caption text-slate">
                  {t('studio.memory.description')}
                </p>
              </div>

              <Button
                variant="ghost"
                size="sm"
                icon={<RefreshCw className="w-3.5 h-3.5" />}
                onClick={async () => {
                  if (agentID && !isNew) {
                    const m = await api.getMemoryMD(agentID).catch(() => null);
                    if (m?.memory_md) setMemoryMD(m.memory_md);
                    info('Memory Refreshed', 'Loaded latest MEMORY.md reflections from disk.');
                  }
                }}
              >
                {t('studio.memory.refresh')}
              </Button>
            </div>

            {memoryMD ? (
              <textarea
                rows={18}
                value={memoryMD}
                readOnly
                className="w-full bg-soft-meadow/80 text-deep-ink p-4 rounded-2xl border border-onyx/10 font-mono text-body-sm leading-relaxed focus:outline-none"
              />
            ) : (
              <div className="p-12 text-center bg-soft-meadow rounded-2xl border border-onyx/5">
                <Brain className="w-10 h-10 text-slate/50 mx-auto mb-3" />
                <h4 className="font-serif text-heading-sm text-deep-ink mb-1">{t('studio.memory.empty')}</h4>
                <p className="font-sans text-caption text-slate max-w-md mx-auto">
                  {t('studio.memory.emptyDescription')}
                </p>
              </div>
            )}

            <div className="flex items-center justify-between text-caption font-mono text-slate">
              <span>{t('studio.length', { count: memoryMD.length })}</span>
              <span className="text-indigo-700 font-semibold flex items-center gap-1">
                <Brain className="w-3.5 h-3.5" /> {t('studio.memory.longTerm')}
              </span>
            </div>
          </Card>
        )}

        {/* TAB 2: Dynamic Synchronized LLM & Reasoning */}
        {activeTab === 'model' && (
          <div className="space-y-6">
            {/* Live LLM Provider Sync Status Banner */}
            <div className="p-4 rounded-2xl bg-soft-meadow border border-onyx/10 flex flex-col sm:flex-row sm:items-center justify-between gap-3 shadow-xs">
              <div className="flex items-center gap-3">
                <div className="w-9 h-9 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center shrink-0">
                  <Cpu className="w-4 h-4" />
                </div>
                <div>
                  <h4 className="font-serif text-body font-semibold text-deep-ink">
                    {t('studio.model.syncTitle')}
                  </h4>
                  <p className="font-sans text-caption text-slate">
                    {t('studio.model.syncDescription')}
                  </p>
                </div>
              </div>

              <div className="flex items-center gap-2">
                <span className="text-caption font-mono text-slate">
                  {t('studio.model.providersActive', { count: configuredProviders.filter((p) => p.configured && p.enabled).length })}
                </span>
              </div>
            </div>

            <Card className="p-6 border border-onyx/10 bg-canvas/90 space-y-6">
              <h3 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
                <Sparkles className="w-5 h-5 text-deep-ink" />
                <span>{t('studio.model.architecture')}</span>
              </h3>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                {/* PRIMARY MODEL SELECTION */}
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <label className="text-caption font-semibold text-deep-ink">
                      {t('studio.model.primary')}
                    </label>
                    <button
                      type="button"
                      onClick={() => setCustomPrimaryMode(!customPrimaryMode)}
                      className="text-[11px] font-mono text-deep-ink hover:underline cursor-pointer"
                    >
                      {customPrimaryMode ? '▸ Choose from catalog' : '✏️ Custom model string'}
                    </button>
                  </div>

                  {customPrimaryMode ? (
                    <Input
                      value={primaryModel}
                      onChange={(e) => setPrimaryModel(e.target.value)}
                      placeholder={t('studio.model.primaryPlaceholder')}
                      className="font-mono text-body-sm"
                    />
                  ) : (
                    <select
                      value={primaryModel}
                      onChange={(e) => setPrimaryModel(e.target.value)}
                      className="w-full bg-soft-meadow text-deep-ink p-2.5 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none"
                    >
                      {readyModels.length > 0 && (
                        <optgroup label="⚡ Ready to Use (Active Keys in Settings)">
                          {readyModels.map((m) => (
                            <option key={m.id} value={m.id}>
                              {m.name} — {m.badge || m.providerName}
                            </option>
                          ))}
                        </optgroup>
                      )}

                      <optgroup label="⚙️ Other Available Models (Requires API Key in Settings)">
                        {otherModels.map((m) => (
                          <option key={m.id} value={m.id}>
                            {m.name} ({m.providerName}) {m.badge ? `• ${m.badge}` : ''}
                          </option>
                        ))}
                      </optgroup>
                    </select>
                  )}

                  {/* Primary Model Provider Diagnostics */}
                  <div
                    className={`p-3 rounded-xl border text-caption font-sans flex items-start gap-2 ${
                      primaryIsReady
                        ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-900'
                        : 'bg-amber-500/10 border-amber-500/20 text-amber-900'
                    }`}
                  >
                    {primaryIsReady ? (
                      <CheckCircle2 className="w-4 h-4 text-emerald-700 shrink-0 mt-0.5" />
                    ) : (
                      <AlertTriangle className="w-4 h-4 text-amber-700 shrink-0 mt-0.5" />
                    )}
                    <div>
                      <span className="font-semibold block">
                        {primaryIsReady
                          ? `Active: ${primaryProviderConfig?.name || 'Local Inference'} is ready`
                          : `Setup Required: API key for ${primaryModel.split('/')[0]} is not set`}
                      </span>
                      <span className="text-[11px] opacity-80 block mt-0.5">
                        {primaryIsReady
                          ? `Requests to "${primaryModel}" will execute with native tool use & reasoning.`
                          : `Configure this key in System > Settings to enable inference.`}
                      </span>
                    </div>
                  </div>
                </div>

                {/* FALLBACK RECOVERY MODEL SELECTION */}
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <label className="text-caption font-semibold text-deep-ink">
                      {t('studio.model.fallback')}
                    </label>
                    <button
                      type="button"
                      onClick={() => setCustomFallbackMode(!customFallbackMode)}
                      className="text-[11px] font-mono text-deep-ink hover:underline cursor-pointer"
                    >
                      {customFallbackMode ? '▸ Choose from catalog' : '✏️ Custom model string'}
                    </button>
                  </div>

                  {customFallbackMode ? (
                    <Input
                      value={fallbackModel}
                      onChange={(e) => setFallbackModel(e.target.value)}
                      placeholder={t('studio.model.fallbackPlaceholder')}
                      className="font-mono text-body-sm"
                    />
                  ) : (
                    <select
                      value={fallbackModel}
                      onChange={(e) => setFallbackModel(e.target.value)}
                      className="w-full bg-soft-meadow text-deep-ink p-2.5 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none"
                    >
                      {readyModels.length > 0 && (
                        <optgroup label="⚡ Ready to Use (Active Keys in Settings)">
                          {readyModels.map((m) => (
                            <option key={m.id} value={m.id}>
                              {m.name} — {m.badge || m.providerName}
                            </option>
                          ))}
                        </optgroup>
                      )}

                      <optgroup label="⚙️ Other Available Models (Requires API Key in Settings)">
                        {otherModels.map((m) => (
                          <option key={m.id} value={m.id}>
                            {m.name} ({m.providerName})
                          </option>
                        ))}
                      </optgroup>
                    </select>
                  )}

                  {/* Fallback Model Diagnostics */}
                  <div
                    className={`p-3 rounded-xl border text-caption font-sans flex items-start gap-2 ${
                      fallbackIsReady
                        ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-900'
                        : 'bg-amber-500/10 border-amber-500/20 text-amber-900'
                    }`}
                  >
                    {fallbackIsReady ? (
                      <CheckCircle2 className="w-4 h-4 text-emerald-700 shrink-0 mt-0.5" />
                    ) : (
                      <AlertTriangle className="w-4 h-4 text-amber-700 shrink-0 mt-0.5" />
                    )}
                    <div>
                      <span className="font-semibold block">
                        {fallbackIsReady
                          ? `Fallback: ${fallbackProviderConfig?.name || 'Local Inference'} is ready`
                          : `Setup Required: Fallback provider key is missing`}
                      </span>
                      <span className="text-[11px] opacity-80 block mt-0.5">
                        {t('studio.model.fallbackDescription')}
                      </span>
                    </div>
                  </div>
                </div>
              </div>

              {/* Temperature Slider */}
              <div className="p-4 bg-soft-meadow rounded-2xl border border-onyx/5 space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-body-sm font-semibold text-deep-ink">
                    {t('studio.model.temperature')} <strong className="font-mono">{temperature.toFixed(2)}</strong>
                  </span>
                  <span className="text-caption text-slate">
                    {temperature <= 0.2
                      ? 'Deterministic & Precise Coding'
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
                    {t('studio.model.maxTokens')} <strong className="font-mono">{maxTokens}</strong>
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
                  <span>{t('studio.tools.title')}</span>
                </h3>
                <p className="text-caption text-slate">
                  {t('studio.tools.description')}
                </p>
              </div>

              <div className="flex items-center gap-2">
                <Button variant="ghost" size="sm" onClick={handleClearTools}>
                  {t('studio.tools.clear')}
                </Button>
                <Button variant="primary" size="sm" onClick={handleSelectAllTools}>
                  {t('studio.tools.selectAll')}
                </Button>
              </div>
            </div>

            {isAllToolsSelected && (
              <div className="p-3.5 rounded-2xl bg-emerald-500/10 border border-emerald-500/30 text-emerald-900 text-body-sm flex items-center gap-2.5">
                <CheckCircle2 className="w-4 h-4 text-emerald-700 shrink-0" />
                <span>
                  <strong>{t('studio.tools.fullTitle')}</strong> {t('studio.tools.fullDescription')}
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
                  <span>{t('studio.channels.title')}</span>
                </h3>
                <p className="text-caption text-slate">
                  {t('studio.channels.description')}
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
                    {t('studio.channels.all')}
                  </span>
                  <div className={`w-5 h-5 rounded-full border-2 flex items-center justify-center ${listenAllChannels ? 'border-deep-ink bg-deep-ink text-white' : 'border-onyx/30'}`}>
                    {listenAllChannels && <Check className="w-3 h-3" />}
                  </div>
                </div>
                <p className="text-caption text-slate">
                  {t('studio.channels.allDescription')}
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
                    {t('studio.channels.specific')}
                  </span>
                  <div className={`w-5 h-5 rounded-full border-2 flex items-center justify-center ${!listenAllChannels ? 'border-deep-ink bg-deep-ink text-white' : 'border-onyx/30'}`}>
                    {!listenAllChannels && <Check className="w-3 h-3" />}
                  </div>
                </div>
                <p className="text-caption text-slate">
                  {t('studio.channels.specificDescription')}
                </p>
              </div>
            </div>

            {/* Channel Checkboxes (shown when specific channels selected) */}
            {!listenAllChannels && (
              <div className="space-y-3 pt-2">
                <span className="text-caption font-semibold text-deep-ink uppercase tracking-wider block">
                  {t('studio.channels.select')}
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
              <span>{t('studio.governance.title')}</span>
            </h3>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
              <div>
                <label className="block text-caption font-semibold text-deep-ink mb-1">
                  {t('studio.governance.budget')}
                </label>
                <Input
                  type="number"
                  value={maxBudget}
                  onChange={(e) => setMaxBudget(parseFloat(e.target.value) || 0)}
                  placeholder="50"
                />
                <span className="text-[11px] text-slate mt-1 block">
                  {t('studio.governance.budgetHelp')}
                </span>
              </div>

              <div>
                <label className="block text-caption font-semibold text-deep-ink mb-1">
                  {t('studio.governance.approval')}
                </label>
                <select
                  value={approvalLevel}
                  onChange={(e) => setApprovalLevel(e.target.value as ApprovalLevel)}
                  className="w-full bg-soft-meadow text-deep-ink p-2.5 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none"
                >
                  <option value="Low">{t('studio.governance.low')}</option>
                  <option value="Medium">{t('studio.governance.medium')}</option>
                  <option value="High">{t('studio.governance.high')}</option>
                </select>
              </div>
            </div>

            <div>
              <label className="block text-caption font-semibold text-deep-ink mb-1">
                {t('studio.governance.paths')}
              </label>
              <Input
                value={allowedPaths}
                onChange={(e) => setAllowedPaths(e.target.value)}
                placeholder={t('studio.governance.pathsPlaceholder')}
              />
              <span className="text-[11px] text-slate mt-1 block">
                {t('studio.governance.pathsHelp')}
              </span>
            </div>
          </Card>
        )}
      </PageContainer>
    </div>
  );
}
