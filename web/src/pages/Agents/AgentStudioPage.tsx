import { useState, useEffect, useMemo, useRef } from 'react';
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
import { AgentStudioNav, type AgentStudioSection } from '@/components/features/agents/AgentStudioNav';
import { AgentIdentitySection } from '@/components/features/agents/AgentIdentitySection';
import { AgentGovernanceSection } from '@/components/features/agents/AgentGovernanceSection';
import { AgentToolsSection } from '@/components/features/agents/AgentToolsSection';
import { AgentChannelsSection } from '@/components/features/agents/AgentChannelsSection';
import { AgentTextSection } from '@/components/features/agents/AgentTextSection';
import { AgentMemorySection } from '@/components/features/agents/AgentMemorySection';
import { AgentReviewSection } from '@/components/features/agents/AgentReviewSection';

export interface AgentStudioPageProps {
  agentID: string; // 'new' or existing agent ID like 'agent_system_core'
  onBack: () => void;
  onOpenChat: (agentID: string) => void;
}

type StudioTab = AgentStudioSection;

const ACTON_STANDARD_SOUL = `You are an autonomous, versatile, and dedicated AI agent running within ActonOS.
You combine high analytical competence (IQ), empathetic communication (EQ), and structured execution.

- Natural & Respectful: Communicate with warmth, clarity, intellectual honesty, and professional polish.
- Zero Robotic Clichés: Never output canned disclaimers ('As an AI...'), repetitive filler, or robotic fluff.
- Adaptive Execution: Be concise and swift for urgent operational tasks; structured, creative, and deep for planning and complex analysis.
- Decision & Action: Understand objectives, verify inputs, and evaluate outcomes critically before concluding.
- Safety & Integrity: Never leak private keys, secrets, or sensitive configuration data.`;

const ACTON_STANDARD_PROMPT = `You are a specialized autonomous AI agent operating within ActonOS.

- Role & Expertise: Embody your assigned role and responsibilities with precision, clarity, and domain expertise.
- Execution Protocol: Clarify objectives and constraints before choosing tools. Use authorized tools purposefully.
- Synthesis: Critically review tool execution observations and deliver polished, structured results directly.
- Conversational Standard: Communicate naturally and contextually. Avoid robotic platitudes and dive straight into high-value solutions.
- Safety Invariants: Safeguard credentials and confine file modifications strictly to authorized workspace paths.`;

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
  const [refreshingMemory, setRefreshingMemory] = useState(false);
  const baselineRef = useRef('');
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
  const [primaryModel, setPrimaryModel] = useState('openai/gpt-5.4-mini');
  const [fallbackModel, setFallbackModel] = useState('openai/gpt-5.4-mini');
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
  const renderLegacyEditors = false as boolean;

  const formSignature = useMemo(() => JSON.stringify({
    name, idSlug, description, avatarIcon, status, isSystem, primaryModel, fallbackModel,
    temperature, maxTokens, systemInstructions, soul, memoryMD, authorizedTools,
    listenAllChannels, selectedChannels, maxBudget, approvalLevel, allowedPaths,
  }), [
    name, idSlug, description, avatarIcon, status, isSystem, primaryModel, fallbackModel,
    temperature, maxTokens, systemInstructions, soul, memoryMD, authorizedTools,
    listenAllChannels, selectedChannels, maxBudget, approvalLevel, allowedPaths,
  ]);
  const isDirty = baselineRef.current !== '' && baselineRef.current !== formSignature;
  const validationErrors = useMemo(() => {
    const issues: string[] = [];
    if (!String(name ?? '').trim()) issues.push(t('studio.review.nameRequired'));
    if (!String(idSlug ?? '').trim()) issues.push(t('studio.review.identifierRequired'));
    if (!String(primaryModel ?? '').trim()) issues.push(t('studio.review.primaryModelRequired'));
    if (authorizedTools.length === 0) issues.push(t('studio.review.toolsRequired'));
    if (!listenAllChannels && selectedChannels.length === 0) issues.push(t('studio.review.channelsRequired'));
    return issues;
  }, [authorizedTools.length, idSlug, listenAllChannels, name, primaryModel, selectedChannels.length, t]);

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
            const pMod = agent.model_config.primary_model || 'openai/gpt-5.4-mini';
            const fMod = agent.model_config.fallback_model || 'openai/gpt-5.4-mini';
            setPrimaryModel(pMod);
            setFallbackModel(fMod);
            setTemperature(agent.model_config.temperature ?? 0.2);
            setMaxTokens(agent.model_config.max_tokens ?? 32768);

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
          } catch { }
        }
      } catch (err) {
        error('Failed to load agent details', getErrorMessage(err));
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, [agentID, isNew]);

  useEffect(() => {
    if (!loading && baselineRef.current === '') baselineRef.current = formSignature;
  }, [loading, formSignature]);

  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => {
      if (!isDirty) return;
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, [isDirty]);

  const handleSave = async () => {
    if (validationErrors.length > 0) {
      setActiveTab('review');
      error(t('studio.review.invalid'), validationErrors[0]);
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
      baselineRef.current = formSignature;
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
    info(t('studio.soul.refreshed', 'Persona Template Loaded'), t('studio.soul.refreshedDesc', 'Loaded the standard persona guidelines.'));
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

      <PageContainer maxWidth="wide">
        {/* Top Breadcrumb & Action Bar */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
          <div className="flex items-center gap-3">
            <button
              onClick={() => {
                if (!isDirty || window.confirm(t('studio.leaveConfirm'))) onBack();
              }}
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
              disabled={saving || validationErrors.length > 0}
            >
              {saving ? t('studio.saving') : isDirty ? t('studio.save') : t('studio.saved')}
            </Button>
          </div>
        </div>

        <AgentIdentitySection
          name={name}
          identifier={idSlug}
          description={description}
          avatarIcon={avatarIcon}
          status={status}
          identifierLocked={!isNew || isSystem}
          onNameChange={setName}
          onIdentifierChange={setIdSlug}
          onDescriptionChange={setDescription}
          onStatusChange={setStatus}
        />

        {/* Legacy identity editor disabled after feature extraction. */}
        <Card className="hidden p-6 border border-onyx/10 bg-canvas/90 shadow-xs mb-8 space-y-4" aria-hidden="true">
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
                    className={`px-3 py-1.5 rounded-full text-caption font-sans transition-all cursor-pointer ${status === 'active'
                      ? 'bg-emerald-600 text-white font-semibold shadow-xs'
                      : 'bg-canvas text-deep-ink hover:bg-black/5'
                      }`}
                  >
                    {t('studio.active')}
                  </button>
                  <button
                    type="button"
                    onClick={() => setStatus('stopped')}
                    className={`px-3 py-1.5 rounded-full text-caption font-sans transition-all cursor-pointer ${status === 'stopped'
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

        <AgentStudioNav
          value={activeTab}
          modelReady={primaryIsReady}
          allTools={isAllToolsSelected}
          toolCount={authorizedTools.length}
          allChannels={listenAllChannels}
          channelCount={selectedChannels.length}
          onChange={setActiveTab}
        />

        {/* Legacy section navigation disabled after feature extraction. */}
        <div role="tablist" aria-label={t('studio.tabs.label')} className="hidden sticky top-20 z-20 items-center gap-1.5 bg-canvas/95 backdrop-blur-sm p-1 rounded-full border border-onyx/10 shadow-xs mb-8 self-start sm:self-auto max-w-full overflow-x-auto" aria-hidden="true">
          <button
            role="tab"
            aria-selected={activeTab === 'prompt'}
            onClick={() => setActiveTab('prompt')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${activeTab === 'prompt' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
              }`}
          >
            {t('studio.tabs.instructions')}
          </button>
          <button
            role="tab"
            aria-selected={activeTab === 'soul'}
            onClick={() => setActiveTab('soul')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${activeTab === 'soul' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
              }`}
          >
            {t('studio.tabs.soul')}
          </button>
          <button
            role="tab"
            aria-selected={activeTab === 'memory'}
            onClick={() => setActiveTab('memory')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${activeTab === 'memory' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
              }`}
          >
            {t('studio.tabs.memory')}
          </button>
          <button
            role="tab"
            aria-selected={activeTab === 'model'}
            onClick={() => setActiveTab('model')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${activeTab === 'model' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
              }`}
          >
            {t('studio.tabs.model', { status: primaryIsReady ? t('studio.ready') : t('studio.keyNeeded') })}
          </button>
          <button
            role="tab"
            aria-selected={activeTab === 'tools'}
            onClick={() => setActiveTab('tools')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${activeTab === 'tools' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
              }`}
          >
            {t('studio.tabs.tools', { value: isAllToolsSelected ? t('studio.allTools') : authorizedTools.length })}
          </button>
          <button
            role="tab"
            aria-selected={activeTab === 'channels'}
            onClick={() => setActiveTab('channels')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${activeTab === 'channels' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
              }`}
          >
            {t('studio.tabs.channels', { value: listenAllChannels ? t('studio.all') : selectedChannels.length })}
          </button>
          <button
            role="tab"
            aria-selected={activeTab === 'governance'}
            onClick={() => setActiveTab('governance')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${activeTab === 'governance' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
              }`}
          >
            {t('studio.tabs.governance')}
          </button>
        </div>

        {activeTab === 'prompt' && (
          <AgentTextSection
            icon={FileCode}
            title={t('studio.prompt.title')}
            description={t('studio.prompt.description')}
            value={systemInstructions}
            placeholder={t('studio.prompt.placeholder')}
            resetLabel={t('studio.prompt.loadPreset')}
            statusLabel={t('studio.prompt.standard')}
            onChange={setSystemInstructions}
            onReset={loadStandardPreset}
          />
        )}
        {/* Legacy prompt editor retained temporarily for source migration only. */}
        {renderLegacyEditors && activeTab === 'prompt' && (
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

        {activeTab === 'soul' && (
          <AgentTextSection
            icon={Sparkles}
            title={t('studio.soul.title')}
            description={t('studio.soul.description')}
            value={soul}
            placeholder={t('studio.soul.placeholder')}
            resetLabel={t('studio.soul.loadTemplate')}
            statusLabel={t('studio.soul.isolated')}
            onChange={setSoul}
            onReset={loadStandardSoulTemplate}
          />
        )}
        {/* Legacy soul editor retained temporarily for source migration only. */}
        {renderLegacyEditors && activeTab === 'soul' && (
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

        {activeTab === 'memory' && (
          <AgentMemorySection
            value={memoryMD}
            refreshing={refreshingMemory}
            onRefresh={async () => {
              if (!agentID || isNew) return;
              setRefreshingMemory(true);
              try {
                const memory = await api.getMemoryMD(agentID);
                setMemoryMD(memory.memory_md || '');
                info(t('studio.memory.refreshed'), t('studio.memory.refreshedDescription'));
              } catch (err) {
                error(t('studio.memory.refreshFailed'), getErrorMessage(err));
              } finally {
                setRefreshingMemory(false);
              }
            }}
          />
        )}
        {/* Legacy memory editor retained temporarily for source migration only. */}
        {renderLegacyEditors && activeTab === 'memory' && (
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
                    info(t('studio.memory.refreshed'), t('studio.memory.refreshedDescription'));
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
                      {customPrimaryMode ? 'Choose from catalog' : 'Custom model string'}
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
                        <optgroup label="Ready to Use (Active Keys in Settings)">
                          {readyModels.map((m) => (
                            <option key={m.id} value={m.id}>
                              {m.name} — {m.badge || m.providerName}
                            </option>
                          ))}
                        </optgroup>
                      )}

                      <optgroup label="Other Available Models (Requires API Key in Settings)">
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
                    className={`p-3 rounded-xl border text-caption font-sans flex items-start gap-2 ${primaryIsReady
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
                      {customFallbackMode ? 'Choose from catalog' : 'Custom model string'}
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
                        <optgroup label="Ready to Use (Active Keys in Settings)">
                          {readyModels.map((m) => (
                            <option key={m.id} value={m.id}>
                              {m.name} — {m.badge || m.providerName}
                            </option>
                          ))}
                        </optgroup>
                      )}

                      <optgroup label="Other Available Models (Requires API Key in Settings)">
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
                    className={`p-3 rounded-xl border text-caption font-sans flex items-start gap-2 ${fallbackIsReady
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
                    {t('studio.model.maxTokens')} <strong className="font-mono">{maxTokens.toLocaleString()}</strong>
                  </span>
                  <span className="text-caption text-slate font-mono">1,024 - 65,536</span>
                </div>
                <input
                  type="range"
                  min="1024"
                  max="65536"
                  step="1024"
                  value={maxTokens}
                  onChange={(e) => setMaxTokens(parseInt(e.target.value, 10))}
                  className="w-full accent-deep-ink cursor-pointer"
                />
              </div>
            </Card>
          </div>
        )}

        {activeTab === 'tools' && (
          <AgentToolsSection
            tools={toolsList}
            authorizedTools={authorizedTools}
            allSelected={isAllToolsSelected}
            onToggle={handleToggleTool}
            onClear={handleClearTools}
            onSelectAll={handleSelectAllTools}
          />
        )}

        {/* Legacy tools editor disabled after feature extraction. */}
        {renderLegacyEditors && activeTab === 'tools' && (
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
                    className={`p-4 rounded-2xl border transition-all cursor-pointer select-none flex flex-col justify-between ${isSelected
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

        {activeTab === 'channels' && (
          <AgentChannelsSection
            listenAll={listenAllChannels}
            selectedChannels={selectedChannels}
            onListenModeChange={setListenAllChannels}
            onToggleChannel={handleToggleChannel}
          />
        )}

        {/* Legacy channels editor disabled after feature extraction. */}
        {renderLegacyEditors && activeTab === 'channels' && (
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
                className={`p-5 rounded-2xl border transition-all cursor-pointer ${listenAllChannels
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
                className={`p-5 rounded-2xl border transition-all cursor-pointer ${!listenAllChannels
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
                        className={`p-3.5 rounded-2xl border transition-all cursor-pointer flex items-center justify-between ${isChecked
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

        {activeTab === 'governance' && (
          <AgentGovernanceSection
            budget={maxBudget}
            approvalLevel={approvalLevel}
            allowedPaths={allowedPaths}
            onBudgetChange={setMaxBudget}
            onApprovalLevelChange={setApprovalLevel}
            onAllowedPathsChange={setAllowedPaths}
          />
        )}

        {activeTab === 'review' && (
          <AgentReviewSection
            errors={validationErrors}
            items={[
              { label: t('studio.review.identity'), value: `${name || t('studio.untitled')} (${idSlug || '-'})` },
              { label: t('studio.review.models'), value: `${primaryModel} / ${fallbackModel}` },
              { label: t('studio.review.tools'), value: isAllToolsSelected ? t('studio.allTools') : String(authorizedTools.length) },
              { label: t('studio.review.channels'), value: listenAllChannels ? t('studio.all') : String(selectedChannels.length) },
              { label: t('studio.review.approval'), value: approvalLevel },
              { label: t('studio.review.budget'), value: `$${maxBudget}` },
            ]}
          />
        )}

        {/* Legacy governance editor disabled after feature extraction. */}
        {renderLegacyEditors && activeTab === 'governance' && (
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
