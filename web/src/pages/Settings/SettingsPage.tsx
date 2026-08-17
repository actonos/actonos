import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { useToast } from '@/components/ui/Toast';
import {
  Shield,
  Wifi,
  RefreshCw,
  Cpu,
  HardDrive,
  Key,
  FileText,
  DownloadCloud,
  Download,
  CheckCircle2,
  XCircle,
  Activity,
  Clock,
  Sparkles,
  Layers,
  RotateCcw,
  Eye,
  EyeOff,
  Zap,
  Save,
  Server,
  Terminal,
  User,
  Coins,
  TrendingUp,
  Bot,
  Database,
} from 'lucide-react';
import {
  api,
  type AuditLogItem,
  type StorageInfoData,
  type UserIdentityProfile,
} from '@/lib/api';
import type { SystemMetrics, TailscaleStatus, LLMProviderInfo, TokenUsageSummary, TokenUsageRecord } from '@/lib/types';

type SettingsTab = 'identity' | 'keys' | 'tokens' | 'network' | 'audit' | 'maintenance';

interface ProviderMeta {
  id: string;
  name: string;
  category: string;
  description: string;
  defaultBaseURL: string;
  modelPresets: { id: string; label: string }[];
  accentColor: string;
  icon: React.ElementType;
}

const PROVIDER_METAS: ProviderMeta[] = [
  {
    id: 'anthropic',
    name: 'Anthropic Claude',
    category: 'Cloud Frontier',
    description: 'Premier coding, hybrid reasoning, and ReAct loop capabilities.',
    defaultBaseURL: 'https://api.anthropic.com',
    modelPresets: [
      { id: 'claude-3-7-sonnet', label: 'Claude 3.7 Sonnet (Hybrid Reasoning Flagship)' },
      { id: 'claude-opus-4-8', label: 'Claude Opus 4.8 (Supreme Intelligence)' },
      { id: 'claude-sonnet-4-6', label: 'Claude Sonnet 4.6 (Frontier Coding Specialist)' },
      { id: 'claude-haiku-4-5', label: 'Claude Haiku 4.5 (Ultra Fast Worker)' },
      { id: 'claude-3-5-sonnet', label: 'Claude 3.5 Sonnet' },
    ],
    accentColor: '#D97706',
    icon: Sparkles,
  },
  {
    id: 'openai',
    name: 'OpenAI',
    category: 'Cloud Frontier',
    description: 'General intelligence, multimodal reasoning, and structured output.',
    defaultBaseURL: 'https://api.openai.com/v1',
    modelPresets: [
      { id: 'gpt-5.6', label: 'GPT-5.6 (Omni Flagship 2026)' },
      { id: 'gpt-5.5', label: 'GPT-5.5 (General Multimodal)' },
      { id: 'gpt-5.4-pro', label: 'GPT-5.4 Pro (Enterprise Reasoning)' },
      { id: 'gpt-5.4-mini', label: 'GPT-5.4 Mini (Light Fast)' },
      { id: 'o3', label: 'o3 (Next-Gen Deliberate Reasoning)' },
      { id: 'o3-mini', label: 'o3-mini (High STEM & Coding Reasoning)' },
      { id: 'gpt-4o', label: 'GPT-4o (Omni Multimodal Standard)' },
      { id: 'gpt-4o-mini', label: 'GPT-4o Mini (Fast & Cheap)' },
    ],
    accentColor: '#10B981',
    icon: Zap,
  },
  {
    id: 'gemini',
    name: 'Google Gemini',
    category: 'Cloud Frontier',
    description: 'Massive 2M+ token context window, ultra-fast latency, and native tool use.',
    defaultBaseURL: 'https://generativelanguage.googleapis.com',
    modelPresets: [
      { id: 'gemini-3.1-pro', label: 'Gemini 3.1 Pro (2M+ Context Flagship)' },
      { id: 'gemini-3-flash', label: 'Gemini 3 Flash (1M+ Realtime Streaming)' },
      { id: 'gemini-2.5-pro', label: 'Gemini 2.5 Pro (Deep Multimodal Code)' },
      { id: 'gemini-2.5-flash', label: 'Gemini 2.5 Flash (1M+ Context Recommended)' },
    ],
    accentColor: '#3B82F6',
    icon: Sparkles,
  },
  {
    id: 'deepseek',
    name: 'DeepSeek',
    category: 'Open Weights & Cloud',
    description: 'Cost-effective high-performance reasoning and code intelligence.',
    defaultBaseURL: 'https://api.deepseek.com/v1',
    modelPresets: [
      { id: 'deepseek-v4-pro', label: 'DeepSeek-V4 Pro (1M MoE Architecture)' },
      { id: 'deepseek-v4-flash', label: 'DeepSeek-V4 Flash (High Throughput MoE)' },
      { id: 'deepseek-r1', label: 'DeepSeek-R1 (Open Reasoning Benchmark Leader)' },
      { id: 'deepseek-v3.2', label: 'DeepSeek-V3.2 Chat' },
    ],
    accentColor: '#6366F1',
    icon: Cpu,
  },
  {
    id: 'groq',
    name: 'Groq Cloud',
    category: 'Ultra-Fast Inference',
    description: 'LPU inference engine delivering 500+ tokens/sec for open models.',
    defaultBaseURL: 'https://api.groq.com/openai/v1',
    modelPresets: [
      { id: 'llama-3.3-70b-versatile', label: 'Llama 3.3 70B Versatile (Groq LPU)' },
      { id: 'deepseek-r1-distill-llama-70b', label: 'DeepSeek R1 Distill 70B (Groq)' },
      { id: 'qwen3-coder', label: 'Qwen3 Coder (Groq LPU)' },
      { id: 'llama-3.1-8b-instant', label: 'Llama 3.1 8B Instant (800+ tok/s)' },
    ],
    accentColor: '#F97316',
    icon: Zap,
  },
  {
    id: 'openrouter',
    name: 'OpenRouter',
    category: 'Unified Aggregator',
    description: 'One API key to access 100+ open-source and proprietary models.',
    defaultBaseURL: 'https://openrouter.ai/api/v1',
    modelPresets: [
      { id: 'anthropic/claude-3.7-sonnet', label: 'Claude 3.7 Sonnet (via OpenRouter)' },
      { id: 'openai/gpt-4o', label: 'GPT-4o (via OpenRouter)' },
      { id: 'openai/o3-mini', label: 'o3-mini (via OpenRouter)' },
      { id: 'google/gemini-2.5-flash', label: 'Gemini 2.5 Flash (via OpenRouter)' },
      { id: 'deepseek/deepseek-r1', label: 'DeepSeek R1 (via OpenRouter)' },
      { id: 'meta-llama/llama-3.3-70b-instruct', label: 'Llama 3.3 70B (via OpenRouter)' },
    ],
    accentColor: '#8B5CF6',
    icon: Server,
  },
  {
    id: 'mistral',
    name: 'Mistral AI',
    category: 'European AI',
    description: 'State-of-the-art multilingual and European code models.',
    defaultBaseURL: 'https://api.mistral.ai/v1',
    modelPresets: [
      { id: 'mistral-large-latest', label: 'Mistral Large 2 (128k Context)' },
      { id: 'codestral-latest', label: 'Codestral 2501 (Code Specialist)' },
      { id: 'mistral-small-latest', label: 'Mistral Small 3' },
      { id: 'pixtral-large-latest', label: 'Pixtral Large (Vision & Documents)' },
    ],
    accentColor: '#EC4899',
    icon: Sparkles,
  },
  {
    id: 'ollama',
    name: 'Local Ollama / vLLM',
    category: 'On-Premise / Offline',
    description: 'Run completely private models on local GPU without internet connection.',
    defaultBaseURL: 'http://localhost:11434',
    modelPresets: [
      { id: 'llama3.3', label: 'Llama 3.3 (70B Local)' },
      { id: 'deepseek-r1:70b', label: 'DeepSeek R1 70B (Local Reasoning)' },
      { id: 'deepseek-r1:14b', label: 'DeepSeek R1 14B (Local Reasoning)' },
      { id: 'deepseek-r1:8b', label: 'DeepSeek R1 8B (Fast Local Reasoning)' },
      { id: 'qwen2.5-coder:32b', label: 'Qwen 2.5 Coder 32B (Local)' },
      { id: 'phi4', label: 'Phi-4 (14B High Quality)' },
      { id: 'mistral-nemo', label: 'Mistral NeMo 12B' },
    ],
    accentColor: '#64748B',
    icon: Terminal,
  },
  {
    id: 'custom_openai',
    name: 'Custom OpenAI-Compatible',
    category: 'Self-Hosted / Gateway',
    description: 'Connect LM Studio, LocalAI, Azure OpenAI, or custom enterprise gateway.',
    defaultBaseURL: 'http://localhost:8000/v1',
    modelPresets: [
      { id: 'default-model', label: 'Default Model' },
      { id: 'custom-model', label: 'Custom Model Tag' },
    ],
    accentColor: '#0EA5E9',
    icon: Server,
  },
];

export function SettingsPage() {
  const { t } = useTranslation('settings');
  const { success, error, info } = useToast();
  const [activeTab, setActiveTab] = useState<SettingsTab>('keys');

  // Metrics & Tailscale
  const [metrics, setMetrics] = useState<SystemMetrics | null>(null);
  const [tailscale, setTailscale] = useState<TailscaleStatus | null>(null);
  const [wifiNetworks, setWifiNetworks] = useState<any[]>([]);
  const [wifiPassword, setWifiPassword] = useState('');
  const [selectedSSID, setSelectedSSID] = useState('');
  const [loadingWifi, setLoadingWifi] = useState(false);

  // Provider Keys State (Per Provider forms)
  const [providersData, setProvidersData] = useState<Record<string, LLMProviderInfo>>({});
  const [inputKeys, setInputKeys] = useState<Record<string, string>>({});
  const [inputURLs, setInputURLs] = useState<Record<string, string>>({});
  const [inputModels, setInputModels] = useState<Record<string, string>>({});
  const [inputEnabled, setInputEnabled] = useState<Record<string, boolean>>({});
  const [showKeyMap, setShowKeyMap] = useState<Record<string, boolean>>({});

  // Testing state
  const [testingId, setTestingId] = useState<string | null>(null);
  const [testResults, setTestResults] = useState<Record<string, { success: boolean; latency: number; msg: string }>>({});
  const [savingId, setSavingId] = useState<string | null>(null);

  // Audit logs, Storage & Identity
  const [auditLogs, setAuditLogs] = useState<AuditLogItem[]>([]);
  const [auditFilter, setAuditFilter] = useState<'all' | 'high' | 'medium' | 'low'>('all');
  const [auditSearch, setAuditSearch] = useState('');
  const [storageInfo, setStorageInfo] = useState<StorageInfoData | null>(null);
  const [otaStatus, setOtaStatus] = useState<any | null>(null);
  const [checkingOTA, setCheckingOTA] = useState(false);
  const [isRestartModalOpen, setIsRestartModalOpen] = useState(false);

  // Token Analytics & Ledger state
  const [tokenStats, setTokenStats] = useState<TokenUsageSummary | null>(null);
  const [tokenHistory, setTokenHistory] = useState<TokenUsageRecord[]>([]);
  const [tokenAgentFilter, setTokenAgentFilter] = useState<string>('all');
  const [tokenSourceFilter, setTokenSourceFilter] = useState<string>('all');
  const [agentsList, setAgentsList] = useState<any[]>([]);

  // Identity & Persona state
  const [identityProfile, setIdentityProfile] = useState<UserIdentityProfile>({
    user_name: 'Operator',
    user_role: 'System Administrator & Architect',
    language: 'vi',
    timezone: 'Asia/Ho_Chi_Minh',
    communication_style: 'concise',
    bio: 'Owner of the ActonOS local intelligence kernel.',
    custom_instructions: 'Provide structured, high-accuracy, and verified responses.',
    soul: '',
  });
  const [savingIdentity, setSavingIdentity] = useState(false);

  const loadStatus = async () => {
    try {
      const [m, ts, k, logs, stor, ident, tokSum, tokHist, ags] = await Promise.all([
        api.getMetrics().catch(() => null),
        api.getTailscale().catch(() => null),
        api.getAPIKeys().catch(() => null),
        api.getAuditLogs().catch(() => ({ entries: [], count: 0 })),
        api.getStorageInfo().catch(() => null),
        api.getIdentity().catch(() => null),
        api.getTokenUsage().catch(() => null),
        api.getTokenHistory({
          agent_id: tokenAgentFilter !== 'all' ? tokenAgentFilter : undefined,
          source: tokenSourceFilter !== 'all' ? tokenSourceFilter : undefined,
        }).catch(() => []),
        api.listAgents().catch(() => ({ agents: [], count: 0 })),
      ]);
      setMetrics(m);
      setTailscale(ts);
      setAuditLogs(logs.entries || []);
      setStorageInfo(stor);
      if (tokSum) setTokenStats(tokSum);
      setTokenHistory(tokHist || []);
      if (ags && ags.agents) setAgentsList(ags.agents);
      if (ident) {
        setIdentityProfile((prev) => ({ ...prev, ...ident }));
      }

      if (k) {
        const pMap: Record<string, LLMProviderInfo> = {};
        const urlMap: Record<string, string> = {};
        const modelMap: Record<string, string> = {};
        const enMap: Record<string, boolean> = {};

        if (k.providers) {
          k.providers.forEach((p: LLMProviderInfo) => {
            pMap[p.id] = p;
            urlMap[p.id] = p.base_url || '';
            modelMap[p.id] = p.default_model || '';
            enMap[p.id] = p.enabled;
          });
        }
        setProvidersData(pMap);
        setInputURLs((prev) => ({ ...urlMap, ...prev }));
        setInputModels((prev) => ({ ...modelMap, ...prev }));
        setInputEnabled((prev) => ({ ...enMap, ...prev }));
      }
    } catch (err: any) {
      error('Failed to load system info', err.message);
    }
  };

  useEffect(() => {
    loadStatus();
    const interval = setInterval(loadStatus, 10000);
    return () => clearInterval(interval);
  }, []);

  const handleSaveSingleProvider = async (meta: ProviderMeta) => {
    setSavingId(meta.id);
    try {
      const keyVal = inputKeys[meta.id];
      const urlVal = inputURLs[meta.id] || meta.defaultBaseURL;
      const modelVal = inputModels[meta.id] || meta.modelPresets[0]?.id;
      const enabledVal = inputEnabled[meta.id] ?? true;

      await api.saveProviderKey({
        provider: meta.id,
        api_key: keyVal || undefined,
        base_url: urlVal || undefined,
        default_model: modelVal || undefined,
        enabled: enabledVal,
      });

      success('Provider Saved', `${meta.name} configuration stored in hardware vault.`);
      setInputKeys((prev) => ({ ...prev, [meta.id]: '' }));
      loadStatus();
    } catch (err: any) {
      error('Save Failed', err.message);
    } finally {
      setSavingId(null);
    }
  };

  const handleTestProvider = async (meta: ProviderMeta) => {
    setTestingId(meta.id);
    try {
      const keyVal = inputKeys[meta.id];
      const urlVal = inputURLs[meta.id] || meta.defaultBaseURL;
      const modelVal = inputModels[meta.id] || meta.modelPresets[0]?.id;

      const res = await api.testAPIKey(meta.id, keyVal || '', urlVal, modelVal);
      setTestResults((prev) => ({
        ...prev,
        [meta.id]: {
          success: true,
          latency: res.latency_ms,
          msg: `Connected • ${res.latency_ms}ms (${res.model})`,
        },
      }));
      success('Provider Validated', `${meta.name} connection confirmed (${res.latency_ms}ms).`);
      loadStatus();
    } catch (err: any) {
      setTestResults((prev) => ({
        ...prev,
        [meta.id]: {
          success: false,
          latency: 0,
          msg: `Failed: ${err.message}`,
        },
      }));
      error('Connection Test Failed', err.message);
    } finally {
      setTestingId(null);
    }
  };

  const handleSaveIdentity = async () => {
    setSavingIdentity(true);
    try {
      await api.saveIdentity(identityProfile);
      success('Identity Saved', 'Owner profile and Soul persona updated across all cognitive layers.');
      loadStatus();
    } catch (err: any) {
      error('Failed to save identity', err.message);
    } finally {
      setSavingIdentity(false);
    }
  };

  const handleScanWifi = async () => {
    setLoadingWifi(true);
    try {
      const res = await api.scanWifi();
      setWifiNetworks(res.networks || []);
      info('Wi-Fi Scan Complete', `Found ${res.networks?.length || 0} wireless network(s).`);
    } catch (err: any) {
      error('Wi-Fi scan failed', err.message);
    } finally {
      setLoadingWifi(false);
    }
  };

  const handleConnectWifi = async () => {
    if (!selectedSSID) return;
    try {
      await api.connectWifi(selectedSSID, wifiPassword);
      success('Wi-Fi Connected', `Joined wireless network "${selectedSSID}".`);
    } catch (err: any) {
      error('Wi-Fi connection error', err.message);
    }
  };

  const handleCheckOTA = async () => {
    setCheckingOTA(true);
    try {
      const res = await api.checkOTA();
      setOtaStatus(res);
      if (res.update_available) {
        info('Update Available', `ActonOS v${res.latest_version} is available for installation.`);
      } else {
        success('System Up-to-Date', `Running latest version v${res.current_version}.`);
      }
    } catch (err: any) {
      error('OTA check failed', err.message);
    } finally {
      setCheckingOTA(false);
    }
  };

  const handleRestartDaemon = async () => {
    try {
      await api.restartDaemon();
      success('Daemon Restart Initiated', 'ActonOS kernel is rebooting. Web UI will reconnect shortly.');
    } catch (err: any) {
      error('Restart failed', err.message);
    }
  };

  const configuredCount = PROVIDER_METAS.filter((m) => providersData[m.id]?.configured).length;

  const filteredAuditLogs = auditLogs.filter((entry) => {
    if (auditFilter === 'high' && entry.risk_level?.toLowerCase() !== 'high') return false;
    if (auditFilter === 'medium' && entry.risk_level?.toLowerCase() !== 'medium') return false;
    if (auditFilter === 'low' && entry.risk_level?.toLowerCase() !== 'low') return false;

    if (auditSearch) {
      const q = auditSearch.toLowerCase();
      return (
        entry.tool_name?.toLowerCase().includes(q) ||
        entry.agent_id?.toLowerCase().includes(q) ||
        entry.trace_id?.toLowerCase().includes(q)
      );
    }
    return true;
  });

  return (
    <div className="relative min-h-[calc(100vh-64px)]">
      <BlobBackdrop />

      <PageContainer>
        {/* Header */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
          <div className="flex-1">
            <span className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
              {t('eyebrow', 'System')}
            </span>
            <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight flex items-center gap-3">
              <span>{t('title', 'Settings')}</span>
              <Badge variant="neutral" className="text-caption font-mono">
                {configuredCount}/{PROVIDER_METAS.length} LLMs Active
              </Badge>
            </h1>
            <p className="font-sans text-body text-slate mt-1 max-w-2xl">
              {t(
                'subtitle',
                'System hardware, API keys, network, and Tailscale VPN.'
              )}
            </p>
          </div>

          <div className="flex items-center gap-2.5 shrink-0 self-start sm:self-center">
            <Button
              variant="ghost"
              size="sm"
              icon={<RefreshCw className="w-3.5 h-3.5" />}
              onClick={loadStatus}
            >
              Refresh
            </Button>
            <Button
              variant="danger"
              size="sm"
              icon={<RotateCcw className="w-3.5 h-3.5" />}
              onClick={() => setIsRestartModalOpen(true)}
            >
              Restart Kernel
            </Button>
          </div>
        </div>

        {/* Tab Navigation Capsule */}
        <div className="flex items-center gap-1.5 bg-canvas/80 backdrop-blur-sm p-1 rounded-full border border-onyx/10 shadow-xs mb-8 self-start sm:self-auto max-w-fit overflow-x-auto">
          <button
            onClick={() => setActiveTab('identity')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${
              activeTab === 'identity' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            👤 Identity & Soul
          </button>
          <button
            onClick={() => setActiveTab('keys')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${
              activeTab === 'keys' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            🔑 LLM Providers ({configuredCount})
          </button>
          <button
            onClick={() => setActiveTab('tokens')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${
              activeTab === 'tokens' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            🪙 Token Ledger & Cost
          </button>
          <button
            onClick={() => setActiveTab('network')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${
              activeTab === 'network' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            📶 Hardware & Tailscale
          </button>
          <button
            onClick={() => setActiveTab('audit')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${
              activeTab === 'audit' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            🛡️ Audit Logs ({auditLogs.length})
          </button>
          <button
            onClick={() => setActiveTab('maintenance')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer whitespace-nowrap ${
              activeTab === 'maintenance' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            ⚙️ Storage & OTA
          </button>
        </div>

        {/* TAB: Identity & Owner Profile */}
        {activeTab === 'identity' && (
          <div className="space-y-6">
            <div className="flex items-center justify-between">
              <div>
                <h3 className="font-serif text-heading-sm font-semibold text-deep-ink">
                  Operator Identity & Profile
                </h3>
                <p className="font-sans text-caption text-slate mt-0.5">
                  Configure your operator identity, working style, and global standing directives for ActonOS.
                </p>
              </div>

              <Button
                variant="primary"
                size="sm"
                icon={savingIdentity ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Save className="w-3.5 h-3.5" />}
                disabled={savingIdentity}
                onClick={handleSaveIdentity}
              >
                {savingIdentity ? 'Saving...' : 'Save Profile'}
              </Button>
            </div>

            <Card className="p-8 border border-onyx/15 shadow-sm max-w-3xl">
              <div className="flex items-center gap-2 border-b border-onyx/10 pb-4 mb-6">
                <User className="w-5 h-5 text-deep-ink" />
                <h4 className="font-serif text-subheading font-semibold text-deep-ink">
                  Owner Identity & Preferences
                </h4>
              </div>

              <div className="space-y-4 text-body-sm">
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-caption uppercase text-slate font-semibold mb-1.5">
                      Owner Name / Handle
                    </label>
                    <Input
                      value={identityProfile.user_name}
                      onChange={(e) => setIdentityProfile({ ...identityProfile, user_name: e.target.value })}
                      placeholder="e.g. Alex, Operator"
                    />
                  </div>

                  <div>
                    <label className="block text-caption uppercase text-slate font-semibold mb-1.5">
                      Professional Role / Title
                    </label>
                    <Input
                      value={identityProfile.user_role || ''}
                      onChange={(e) => setIdentityProfile({ ...identityProfile, user_role: e.target.value })}
                      placeholder="e.g. System Architect & Lead Developer"
                    />
                  </div>
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-caption uppercase text-slate font-semibold mb-1.5">
                      Preferred Language
                    </label>
                    <select
                      value={identityProfile.language}
                      onChange={(e) => setIdentityProfile({ ...identityProfile, language: e.target.value })}
                      className="w-full px-4 py-2.5 bg-canvas border border-onyx/15 rounded-full text-body-sm text-deep-ink font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink"
                    >
                      <option value="en">English (US / Global)</option>
                      <option value="vi">Tiếng Việt (Vietnamese)</option>
                    </select>
                  </div>

                  <div>
                    <label className="block text-caption uppercase text-slate font-semibold mb-1.5">
                      Timezone
                    </label>
                    <Input
                      value={identityProfile.timezone || 'Asia/Ho_Chi_Minh'}
                      onChange={(e) => setIdentityProfile({ ...identityProfile, timezone: e.target.value })}
                      placeholder="e.g. Asia/Ho_Chi_Minh, UTC"
                    />
                  </div>
                </div>

                <div>
                  <label className="block text-caption uppercase text-slate font-semibold mb-1.5">
                    Communication & Collaboration Tone
                  </label>
                  <Input
                    value={identityProfile.communication_style || ''}
                    onChange={(e) => setIdentityProfile({ ...identityProfile, communication_style: e.target.value })}
                    placeholder="e.g. adaptive, natural, empathetic & sharp"
                  />
                </div>

                <div>
                  <label className="block text-caption uppercase text-slate font-semibold mb-1.5">
                    Owner Bio & Domain Context
                  </label>
                  <textarea
                    rows={2}
                    value={identityProfile.bio || ''}
                    onChange={(e) => setIdentityProfile({ ...identityProfile, bio: e.target.value })}
                    placeholder="Brief background about yourself to give agents domain context..."
                    className="w-full p-3 bg-canvas border border-onyx/15 rounded-2xl text-body-sm text-deep-ink font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink resize-none leading-relaxed"
                  />
                </div>

                <div>
                  <label className="block text-caption uppercase text-slate font-semibold mb-1.5">
                    Universal Standing Directives & Rules
                  </label>
                  <textarea
                    rows={4}
                    value={identityProfile.custom_instructions || ''}
                    onChange={(e) => setIdentityProfile({ ...identityProfile, custom_instructions: e.target.value })}
                    placeholder="Rules all agents must obey (e.g. Write clean code, explain architectural trade-offs, avoid placeholders)..."
                    className="w-full p-3 bg-canvas border border-onyx/15 rounded-2xl text-body-sm text-deep-ink font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink resize-none leading-relaxed"
                  />
                </div>

                <div className="pt-2">
                  <Button
                    variant="primary"
                    size="sm"
                    icon={savingIdentity ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Save className="w-3.5 h-3.5" />}
                    disabled={savingIdentity}
                    onClick={handleSaveIdentity}
                    className="px-6 font-semibold"
                  >
                    {savingIdentity ? 'Saving Profile...' : 'Save Profile Settings'}
                  </Button>
                </div>
              </div>
            </Card>
          </div>
        )}

        {/* TAB 1: LLM Providers Management */}
        {activeTab === 'keys' && (
          <div className="space-y-6">
            {/* Vault Encryption Info Card */}
            <div className="p-4 rounded-2xl bg-canvas/90 border border-onyx/10 flex flex-col sm:flex-row sm:items-center justify-between gap-3 shadow-xs">
              <div className="flex items-center gap-3">
                <div className="w-9 h-9 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center shrink-0">
                  <Key className="w-4 h-4" />
                </div>
                <div>
                  <h3 className="font-serif text-body font-semibold text-deep-ink">
                    Hardware-Bound LLM Vault
                  </h3>
                  <p className="font-sans text-caption text-slate">
                    Keys are encrypted at rest using AES-256-GCM with hardware fingerprint derivation. Changes apply immediately to all active agent swarms.
                  </p>
                </div>
              </div>

              <Badge variant="active" className="text-[11px] font-mono shrink-0 self-start sm:self-center">
                AES-256-GCM Active
              </Badge>
            </div>

            {/* Providers Grid */}
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
              {PROVIDER_METAS.map((meta) => {
                const pData = providersData[meta.id];
                const isConfigured = pData?.configured;
                const Icon = meta.icon;
                const isTesting = testingId === meta.id;
                const isSaving = savingId === meta.id;
                const testInfo = testResults[meta.id];
                const isEnabled = inputEnabled[meta.id] ?? pData?.enabled ?? true;
                const showKey = !!showKeyMap[meta.id];

                const currentURL = inputURLs[meta.id] !== undefined ? inputURLs[meta.id] : (pData?.base_url || meta.defaultBaseURL);
                const currentModel = inputModels[meta.id] !== undefined ? inputModels[meta.id] : (pData?.default_model || meta.modelPresets[0]?.id);

                return (
                  <Card
                    key={meta.id}
                    className={`p-6 border transition-all flex flex-col justify-between ${
                      isConfigured
                        ? 'border-emerald-500/30 bg-canvas/95 shadow-sm'
                        : 'border-onyx/10 bg-canvas/80'
                    }`}
                  >
                    <div>
                      {/* Provider Header */}
                      <div className="flex items-center justify-between mb-3">
                        <div className="flex items-center gap-2.5">
                          <div
                            className="w-10 h-10 rounded-full flex items-center justify-center text-white shadow-xs"
                            style={{ backgroundColor: meta.accentColor }}
                          >
                            <Icon className="w-5 h-5" />
                          </div>
                          <div>
                            <span className="font-serif text-heading-sm text-deep-ink font-semibold block">
                              {meta.name}
                            </span>
                            <span className="text-caption text-slate">{meta.category}</span>
                          </div>
                        </div>

                        <div className="flex items-center gap-2">
                          <button
                            type="button"
                            onClick={() =>
                              setInputEnabled((prev) => ({ ...prev, [meta.id]: !isEnabled }))
                            }
                            className={`px-2.5 py-1 rounded-full text-[11px] font-semibold transition-all cursor-pointer ${
                              isEnabled
                                ? 'bg-emerald-500/10 text-emerald-700 border border-emerald-500/20'
                                : 'bg-onyx/5 text-slate border border-onyx/10'
                            }`}
                          >
                            {isEnabled ? 'Active' : 'Disabled'}
                          </button>
                          <Badge variant={isConfigured ? 'active' : 'stopped'}>
                            {isConfigured ? 'Configured' : 'Not Set'}
                          </Badge>
                        </div>
                      </div>

                      <p className="text-caption text-slate mb-4">
                        {meta.description}
                      </p>

                      {/* Config Form */}
                      <div className="space-y-3 p-4 rounded-2xl bg-soft-meadow border border-onyx/5 mb-4">
                        {/* API Key Input */}
                        {meta.id !== 'ollama' && (
                          <div>
                            <div className="flex items-center justify-between mb-1">
                              <label className="text-caption font-semibold text-deep-ink">
                                API Key
                              </label>
                              {isConfigured && pData?.masked_key && (
                                <span className="text-[11px] font-mono text-slate">
                                  Current: {pData.masked_key}
                                </span>
                              )}
                            </div>
                            <div className="relative">
                              <Input
                                type={showKey ? 'text' : 'password'}
                                placeholder={isConfigured ? 'Enter new key to update...' : 'sk-...'}
                                value={inputKeys[meta.id] || ''}
                                onChange={(e) =>
                                  setInputKeys((prev) => ({ ...prev, [meta.id]: e.target.value }))
                                }
                                className="pr-10 font-mono text-body-sm"
                              />
                              <button
                                type="button"
                                onClick={() =>
                                  setShowKeyMap((prev) => ({ ...prev, [meta.id]: !prev[meta.id] }))
                                }
                                className="absolute right-3 top-1/2 -translate-y-1/2 text-slate hover:text-deep-ink transition-colors cursor-pointer"
                              >
                                {showKey ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                              </button>
                            </div>
                          </div>
                        )}

                        {/* Base URL Input */}
                        <div>
                          <label className="text-caption font-semibold text-deep-ink block mb-1">
                            Endpoint / Base URL
                          </label>
                          <Input
                            placeholder={meta.defaultBaseURL}
                            value={currentURL}
                            onChange={(e) =>
                              setInputURLs((prev) => ({ ...prev, [meta.id]: e.target.value }))
                            }
                            className="font-mono text-caption"
                          />
                        </div>

                        {/* Default Model Selector */}
                        <div>
                          <label className="text-caption font-semibold text-deep-ink block mb-1">
                            Default Model
                          </label>
                          <select
                            value={currentModel}
                            onChange={(e) =>
                              setInputModels((prev) => ({ ...prev, [meta.id]: e.target.value }))
                            }
                            className="w-full bg-canvas text-deep-ink p-2.5 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none"
                          >
                            {meta.modelPresets.map((mp) => (
                              <option key={mp.id} value={mp.id}>
                                {mp.label}
                              </option>
                            ))}
                          </select>
                        </div>

                        {/* Test Status Banner */}
                        {testInfo && (
                          <div
                            className={`flex items-center gap-1.5 pt-1 text-[11px] font-mono ${
                              testInfo.success ? 'text-emerald-700 font-semibold' : 'text-red-600'
                            }`}
                          >
                            {testInfo.success ? <CheckCircle2 className="w-3.5 h-3.5" /> : <XCircle className="w-3.5 h-3.5" />}
                            <span>{testInfo.msg}</span>
                          </div>
                        )}
                      </div>
                    </div>

                    {/* Footer Actions */}
                    <div className="pt-3 border-t border-onyx/10 flex items-center justify-between">
                      <Button
                        variant="ghost"
                        size="sm"
                        icon={<RefreshCw className={`w-3.5 h-3.5 ${isTesting ? 'animate-spin' : ''}`} />}
                        onClick={() => handleTestProvider(meta)}
                        disabled={isTesting || (!inputKeys[meta.id] && !isConfigured && meta.id !== 'ollama')}
                      >
                        {isTesting ? 'Testing...' : 'Test Connection'}
                      </Button>

                      <Button
                        variant="primary"
                        size="sm"
                        icon={<Save className="w-3.5 h-3.5" />}
                        onClick={() => handleSaveSingleProvider(meta)}
                        disabled={isSaving}
                      >
                        {isSaving ? 'Saving...' : 'Save Settings'}
                      </Button>
                    </div>
                  </Card>
                );
              })}
            </div>
          </div>
        )}

        {/* TAB: Token Consumption & Cost Ledger */}
        {activeTab === 'tokens' && (
          <div className="space-y-6">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div>
                <h3 className="font-serif text-heading-sm font-semibold text-deep-ink flex items-center gap-2">
                  <Coins className="w-5 h-5 text-emerald-600" />
                  <span>Token Consumption & Cost Ledger</span>
                </h3>
                <p className="font-sans text-caption text-slate mt-0.5">
                  Audited live token traffic, cost estimation per model, and comprehensive execution transaction ledger.
                </p>
              </div>

              <Button
                variant="ghost"
                size="sm"
                icon={<RefreshCw className="w-3.5 h-3.5" />}
                onClick={loadStatus}
              >
                Refresh Ledger
              </Button>
            </div>

            {/* 4 Summary Metric Cards */}
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
              <Card className="p-4 bg-canvas border border-onyx/10 space-y-1">
                <span className="text-[11px] font-semibold uppercase text-slate block">Today Tokens</span>
                <div className="text-heading-sm font-serif font-bold text-deep-ink">
                  {(tokenStats?.today_tokens || 0).toLocaleString()}
                </div>
                <span className="text-[11px] font-mono text-emerald-700 font-semibold block">
                  ${(tokenStats?.today_cost_usd || 0).toFixed(4)} USD
                </span>
              </Card>

              <Card className="p-4 bg-canvas border border-onyx/10 space-y-1">
                <span className="text-[11px] font-semibold uppercase text-slate block">This Month</span>
                <div className="text-heading-sm font-serif font-bold text-deep-ink">
                  {(tokenStats?.month_tokens || 0).toLocaleString()}
                </div>
                <span className="text-[11px] font-mono text-emerald-700 font-semibold block">
                  ${(tokenStats?.month_cost_usd || 0).toFixed(4)} USD
                </span>
              </Card>

              <Card className="p-4 bg-canvas border border-onyx/10 space-y-1">
                <span className="text-[11px] font-semibold uppercase text-slate block">Lifetime Total</span>
                <div className="text-heading-sm font-serif font-bold text-deep-ink">
                  {(tokenStats?.total_tokens || 0).toLocaleString()}
                </div>
                <span className="text-[11px] font-mono text-slate block">
                  {(tokenStats?.total_prompt_tokens || 0).toLocaleString()} in / {(tokenStats?.total_completion_tokens || 0).toLocaleString()} out
                </span>
              </Card>

              <Card className="p-4 bg-canvas border border-onyx/10 space-y-1">
                <span className="text-[11px] font-semibold uppercase text-slate block">Estimated Cost Burn</span>
                <div className="text-heading-sm font-serif font-bold text-emerald-700">
                  ${(tokenStats?.total_cost_usd || 0).toFixed(4)} USD
                </div>
                <span className="text-[11px] font-mono text-slate block">
                  Official Pricing Catalog
                </span>
              </Card>
            </div>

            {/* 14-Day Trend Chart */}
            <Card className="p-6 border border-onyx/10 bg-canvas/90 space-y-3">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <TrendingUp className="w-4 h-4 text-deep-ink" />
                  <h4 className="font-serif text-body-sm font-semibold text-deep-ink">
                    14-Day Token Traffic & Cost Trend
                  </h4>
                </div>
                <span className="text-[11px] text-slate font-mono">Daily Volume</span>
              </div>

              {tokenStats?.daily_trend && tokenStats.daily_trend.length > 0 ? (
                <div className="space-y-2 pt-2">
                  <div className="flex items-end gap-1.5 h-36 pt-4 px-2 bg-soft-meadow/50 rounded-xl border border-onyx/5">
                    {tokenStats.daily_trend.map((d) => {
                      const maxDaily = Math.max(...tokenStats.daily_trend.map((x) => x.total_tokens), 1);
                      const heightPercent = Math.max(8, (d.total_tokens / maxDaily) * 100);
                      return (
                        <div key={d.date} className="flex-1 flex flex-col items-center gap-1 group relative h-full justify-end">
                          <div className="absolute -top-10 bg-deep-ink text-white text-[10px] py-1 px-2 rounded-lg opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none whitespace-nowrap z-10 font-mono shadow-md">
                            {d.date}: {d.total_tokens.toLocaleString()} tokens (${d.cost_usd.toFixed(4)})
                          </div>
                          <div
                            className="w-full bg-deep-ink rounded-t-md hover:bg-emerald-600 transition-all duration-300 min-h-[4px]"
                            style={{ height: `${heightPercent}%` }}
                          />
                          <span className="text-[9px] text-slate font-mono truncate w-full text-center">
                            {d.date.slice(5)}
                          </span>
                        </div>
                      );
                    })}
                  </div>
                </div>
              ) : (
                <div className="p-8 text-center text-caption text-slate bg-soft-meadow rounded-xl">
                  No token consumption recorded yet.
                </div>
              )}
            </Card>

            {/* Model & Agent Breakdown Grid */}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              {/* Model Breakdown */}
              <Card className="p-6 border border-onyx/10 bg-canvas space-y-4">
                <div className="flex items-center gap-2">
                  <Layers className="w-4 h-4 text-deep-ink" />
                  <h4 className="font-serif text-body-sm font-semibold text-deep-ink">
                    Consumption by Model
                  </h4>
                </div>

                {tokenStats?.by_model && tokenStats.by_model.length > 0 ? (
                  <div className="space-y-3">
                    {tokenStats.by_model.map((m) => (
                      <div key={m.model} className="space-y-1">
                        <div className="flex items-center justify-between text-caption">
                          <span className="font-mono font-semibold text-deep-ink truncate">{m.model}</span>
                          <span className="font-mono text-slate text-[11px]">
                            {m.total_tokens.toLocaleString()} tok (${m.cost_usd.toFixed(4)}) • <strong className="text-emerald-700">{m.percentage.toFixed(1)}%</strong>
                          </span>
                        </div>
                        <div className="w-full bg-onyx/10 h-1.5 rounded-full overflow-hidden">
                          <div
                            className="bg-deep-ink h-full rounded-full transition-all duration-500"
                            style={{ width: `${m.percentage}%` }}
                          />
                        </div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-caption text-slate">No model metrics available yet.</p>
                )}
              </Card>

              {/* Agent Breakdown */}
              <Card className="p-6 border border-onyx/10 bg-canvas space-y-4">
                <div className="flex items-center gap-2">
                  <Bot className="w-4 h-4 text-deep-ink" />
                  <h4 className="font-serif text-body-sm font-semibold text-deep-ink">
                    Consumption by Agent
                  </h4>
                </div>

                {tokenStats?.by_agent && tokenStats.by_agent.length > 0 ? (
                  <div className="space-y-3">
                    {tokenStats.by_agent.map((a) => (
                      <div key={a.agent_id} className="space-y-1">
                        <div className="flex items-center justify-between text-caption">
                          <span className="font-mono font-semibold text-deep-ink truncate">{a.agent_id}</span>
                          <span className="font-mono text-slate text-[11px]">
                            {a.total_tokens.toLocaleString()} tok (${a.cost_usd.toFixed(4)}) • <strong className="text-emerald-700">{a.percentage.toFixed(1)}%</strong>
                          </span>
                        </div>
                        <div className="w-full bg-onyx/10 h-1.5 rounded-full overflow-hidden">
                          <div
                            className="bg-emerald-600 h-full rounded-full transition-all duration-500"
                            style={{ width: `${a.percentage}%` }}
                          />
                        </div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-caption text-slate">No agent metrics available yet.</p>
                )}
              </Card>
            </div>

            {/* Live Transaction Ledger Table */}
            <Card className="p-6 border border-onyx/10 bg-canvas space-y-4">
              <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                <div>
                  <h4 className="font-serif text-heading-sm font-semibold text-deep-ink flex items-center gap-2">
                    <Database className="w-4 h-4 text-deep-ink" />
                    <span>Transaction Ledger & Audit Trail</span>
                  </h4>
                  <p className="text-caption text-slate mt-0.5">
                    Individual inference events recorded per cognitive cycle.
                  </p>
                </div>

                {/* Filters */}
                <div className="flex items-center gap-2">
                  <select
                    value={tokenAgentFilter}
                    onChange={(e) => setTokenAgentFilter(e.target.value)}
                    className="bg-soft-meadow text-deep-ink text-[12px] font-sans px-3 py-1.5 rounded-full border border-onyx/10 focus:outline-none"
                  >
                    <option value="all">All Agents</option>
                    {agentsList.map((ag: any) => (
                      <option key={ag.agent_id} value={ag.agent_id}>
                        {ag.name || ag.agent_id}
                      </option>
                    ))}
                  </select>

                  <select
                    value={tokenSourceFilter}
                    onChange={(e) => setTokenSourceFilter(e.target.value)}
                    className="bg-soft-meadow text-deep-ink text-[12px] font-sans px-3 py-1.5 rounded-full border border-onyx/10 focus:outline-none"
                  >
                    <option value="all">All Sources</option>
                    <option value="chat">Chat</option>
                    <option value="stream">Chat Stream</option>
                    <option value="cron">Cron Automation</option>
                    <option value="heartbeat">Heartbeat Loop</option>
                    <option value="channel">External Channel</option>
                  </select>
                </div>
              </div>

              <div className="border border-onyx/10 rounded-2xl overflow-hidden">
                <div className="max-h-96 overflow-y-auto">
                  <table className="w-full text-left border-collapse text-body-sm font-sans">
                    <thead>
                      <tr className="border-b border-onyx/10 bg-soft-meadow/60 text-[11px] font-semibold uppercase tracking-wider text-slate sticky top-0 bg-soft-meadow z-10">
                        <th className="py-2.5 px-3">Time</th>
                        <th className="py-2.5 px-3">Agent</th>
                        <th className="py-2.5 px-3">Model</th>
                        <th className="py-2.5 px-3">Source</th>
                        <th className="py-2.5 px-3 text-right">Prompt</th>
                        <th className="py-2.5 px-3 text-right">Completion</th>
                        <th className="py-2.5 px-3 text-right">Total</th>
                        <th className="py-2.5 px-3 text-right">Cost (USD)</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-onyx/5 font-mono text-[12px]">
                      {tokenHistory.length > 0 ? (
                        tokenHistory.map((rec) => (
                          <tr key={rec.id} className="hover:bg-soft-meadow/30 transition-colors">
                            <td className="py-2.5 px-3 whitespace-nowrap text-slate text-[11px]">
                              {new Date(rec.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                            </td>
                            <td className="py-2.5 px-3 font-semibold text-deep-ink max-w-[120px] truncate">
                              {rec.agent_id}
                            </td>
                            <td className="py-2.5 px-3 text-deep-ink max-w-[130px] truncate">
                              {rec.model}
                            </td>
                            <td className="py-2.5 px-3">
                              <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-semibold bg-onyx/5 border border-onyx/10 text-deep-ink">
                                {rec.source}
                              </span>
                            </td>
                            <td className="py-2.5 px-3 text-right text-slate">
                              {rec.prompt_tokens.toLocaleString()}
                            </td>
                            <td className="py-2.5 px-3 text-right text-slate">
                              {rec.completion_tokens.toLocaleString()}
                            </td>
                            <td className="py-2.5 px-3 text-right font-semibold text-deep-ink">
                              {rec.total_tokens.toLocaleString()}
                            </td>
                            <td className="py-2.5 px-3 text-right text-emerald-700 font-semibold">
                              ${rec.estimated_cost_usd.toFixed(5)}
                            </td>
                          </tr>
                        ))
                      ) : (
                        <tr>
                          <td colSpan={8} className="py-8 text-center text-caption text-slate font-sans">
                            No token ledger events recorded yet. Start interacting with agents to generate live usage statistics.
                          </td>
                        </tr>
                      )}
                    </tbody>
                  </table>
                </div>
              </div>
            </Card>
          </div>
        )}

        {/* TAB: Hardware & Network */}
        {activeTab === 'network' && (
          <div className="space-y-6">
            {/* Live Telemetry Card */}
            <Card className="p-6 border border-onyx/10 bg-canvas/90">
              <h3 className="font-serif text-heading-sm text-deep-ink mb-4 flex items-center gap-2">
                <Activity className="w-5 h-5 text-deep-ink" />
                <span>Live Hardware Gauges</span>
              </h3>

              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                {/* CPU Gauge */}
                <div className="p-4 bg-soft-meadow rounded-[20px] border border-onyx/5">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-caption font-semibold uppercase text-slate">CPU Utilization</span>
                    <Cpu className="w-4 h-4 text-deep-ink" />
                  </div>
                  <div className="text-heading font-serif text-deep-ink">
                    {metrics?.cpu?.usage_percent?.toFixed(1) || '0.0'}%
                  </div>
                  <div className="w-full bg-onyx/10 h-2 rounded-full mt-3 overflow-hidden">
                    <div
                      className="bg-deep-ink h-full rounded-full transition-all"
                      style={{ width: `${Math.min(100, metrics?.cpu?.usage_percent || 0)}%` }}
                    />
                  </div>
                  <div className="text-[11px] font-mono text-slate mt-2">
                    {metrics?.cpu?.cores || 1} Cores Active • {metrics?.cpu?.model || 'Hardware HAL'}
                  </div>
                </div>

                {/* RAM Gauge */}
                <div className="p-4 bg-soft-meadow rounded-[20px] border border-onyx/5">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-caption font-semibold uppercase text-slate">System Memory</span>
                    <HardDrive className="w-4 h-4 text-deep-ink" />
                  </div>
                  <div className="text-heading font-serif text-deep-ink">
                    {metrics?.memory?.used_mb || 0} MB
                  </div>
                  <div className="w-full bg-onyx/10 h-2 rounded-full mt-3 overflow-hidden">
                    <div
                      className="bg-emerald-600 h-full rounded-full transition-all"
                      style={{ width: `${Math.min(100, ((metrics?.memory?.used_mb || 0) / (metrics?.memory?.total_mb || 1)) * 100)}%` }}
                    />
                  </div>
                  <div className="text-[11px] font-mono text-slate mt-2">
                    Total: {metrics?.memory?.total_mb || 0} MB (Daemon: {metrics?.memory?.actond_mb || 0} MB)
                  </div>
                </div>

                {/* Storage Gauge */}
                <div className="p-4 bg-soft-meadow rounded-[20px] border border-onyx/5">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-caption font-semibold uppercase text-slate">Disk Storage</span>
                    <Layers className="w-4 h-4 text-deep-ink" />
                  </div>
                  <div className="text-heading font-serif text-deep-ink">
                    {((metrics?.disk?.used_gb || 0)).toFixed(1)} GB
                  </div>
                  <div className="w-full bg-onyx/10 h-2 rounded-full mt-3 overflow-hidden">
                    <div
                      className="bg-amber-600 h-full rounded-full transition-all"
                      style={{ width: `${Math.min(100, ((metrics?.disk?.used_gb || 0) / (metrics?.disk?.total_gb || 1)) * 100)}%` }}
                    />
                  </div>
                  <div className="text-[11px] font-mono text-slate mt-2">
                    Total: {metrics?.disk?.total_gb || 0} GB (Data Dir: {metrics?.disk?.data_dir_gb || 0} GB)
                  </div>
                </div>

                {/* Uptime & Thermal */}
                <div className="p-4 bg-soft-meadow rounded-[20px] border border-onyx/5">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-caption font-semibold uppercase text-slate">Uptime & Health</span>
                    <Clock className="w-4 h-4 text-deep-ink" />
                  </div>
                  <div className="text-heading font-serif text-deep-ink">
                    {Math.floor((metrics?.uptime_seconds || 0) / 60)}m
                  </div>
                  <div className="flex items-center gap-1.5 text-caption font-mono text-emerald-700 mt-3">
                    <CheckCircle2 className="w-3.5 h-3.5" />
                    <span>Kernel Normal</span>
                  </div>
                  <div className="text-[11px] font-mono text-slate mt-2">
                    Temp: {metrics?.cpu?.temperature_celsius || 42}°C
                  </div>
                </div>
              </div>
            </Card>

            {/* Tailscale & Wi-Fi */}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              {/* Tailscale Card */}
              <Card className="p-6 border border-onyx/10 bg-canvas/90">
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2.5">
                    <Shield className="w-5 h-5 text-deep-ink" />
                    <h4 className="font-serif text-heading-sm text-deep-ink">Tailscale Mesh VPN (tsnet)</h4>
                  </div>
                  <Badge variant={tailscale?.connected ? 'active' : 'stopped'}>
                    {tailscale?.connected ? 'Mesh Active' : 'Disabled'}
                  </Badge>
                </div>
                <p className="font-sans text-caption text-slate mb-4">
                  End-to-end WireGuard mesh tunnel for secure remote access without port forwarding.
                </p>

                {tailscale?.connected && (
                  <div className="space-y-1.5 font-mono text-caption text-slate bg-soft-meadow p-3 rounded-xl border border-onyx/5">
                    <div>Node IP: <strong className="text-deep-ink">{tailscale.ip}</strong></div>
                    <div>Hostname: <strong className="text-deep-ink">{tailscale.hostname}</strong></div>
                    <div>Peers Count: <strong className="text-deep-ink">{tailscale.peers_count}</strong></div>
                  </div>
                )}
              </Card>

              {/* Wi-Fi Scanner */}
              <Card className="p-6 border border-onyx/10 bg-canvas/90">
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2.5">
                    <Wifi className="w-5 h-5 text-deep-ink" />
                    <h4 className="font-serif text-heading-sm text-deep-ink">Wireless Network (Wi-Fi)</h4>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={handleScanWifi}
                    disabled={loadingWifi}
                  >
                    {loadingWifi ? 'Scanning...' : 'Scan Wi-Fi'}
                  </Button>
                </div>

                <div className="space-y-2">
                  <select
                    value={selectedSSID}
                    onChange={(e) => setSelectedSSID(e.target.value)}
                    className="w-full bg-soft-meadow text-deep-ink p-2.5 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none"
                  >
                    <option value="">Select Wi-Fi Network ({wifiNetworks.length} found)</option>
                    {wifiNetworks.map((net) => (
                      <option key={net.ssid} value={net.ssid}>
                        {net.ssid} ({net.signal_strength || 80}%)
                      </option>
                    ))}
                  </select>

                  <Input
                    type="password"
                    placeholder="Wi-Fi Password..."
                    value={wifiPassword}
                    onChange={(e) => setWifiPassword(e.target.value)}
                  />

                  <Button
                    variant="primary"
                    size="sm"
                    onClick={handleConnectWifi}
                    disabled={!selectedSSID}
                    className="w-full justify-center"
                  >
                    Join Network
                  </Button>
                </div>
              </Card>
            </div>
          </div>
        )}

        {/* TAB 3: Audit Logs */}
        {activeTab === 'audit' && (
          <Card className="p-6 border border-onyx/10 bg-canvas/90">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-4">
              <div>
                <h3 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
                  <FileText className="w-5 h-5 text-deep-ink" />
                  <span>Security & Action Audit Logs</span>
                </h3>
                <p className="font-sans text-caption text-slate mt-0.5">
                  Immutable log of all tool executions, API access, and agent operations.
                </p>
              </div>

              <div className="flex items-center gap-2">
                {/* Risk Filter */}
                <div className="flex items-center gap-1 bg-soft-meadow p-1 rounded-full border border-onyx/10">
                  {(['all', 'high', 'medium', 'low'] as const).map((r) => (
                    <button
                      key={r}
                      onClick={() => setAuditFilter(r)}
                      className={`px-3 py-1 rounded-full text-caption font-medium capitalize cursor-pointer ${
                        auditFilter === r ? 'bg-deep-ink text-white font-semibold' : 'text-deep-ink hover:text-slate'
                      }`}
                    >
                      {r}
                    </button>
                  ))}
                </div>

                <Input
                  placeholder="Search logs..."
                  value={auditSearch}
                  onChange={(e) => setAuditSearch(e.target.value)}
                  className="max-w-[180px] py-1 text-caption"
                />
              </div>
            </div>

            {filteredAuditLogs.length === 0 ? (
              <div className="py-16 text-center text-slate font-sans text-body-sm">
                No audit log entries found matching criteria.
              </div>
            ) : (
              <div className="divide-y divide-onyx/5 max-h-[500px] overflow-y-auto">
                {filteredAuditLogs.map((log, idx) => (
                  <div key={log.trace_id || idx} className="py-3 flex items-start justify-between gap-4 text-body-sm">
                    <div className="space-y-0.5 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="font-semibold text-deep-ink font-mono text-caption">{log.tool_name}</span>
                        <Badge
                          variant={
                            log.risk_level === 'High'
                              ? 'accent'
                              : log.risk_level === 'Medium'
                              ? 'stopped'
                              : 'neutral'
                          }
                          className="text-[10px]"
                        >
                          {log.risk_level} Risk
                        </Badge>
                        <span className="text-caption font-mono text-slate">Status: {log.status}</span>
                      </div>
                      <div className="text-caption font-mono text-slate">
                        Agent: {log.agent_id} • Trace: {log.trace_id} • Exec: {log.execution_time_ms}ms
                      </div>
                      {log.error && (
                        <div className="text-[11px] font-mono text-red-600 bg-red-500/10 p-1.5 rounded-md mt-1 inline-block">
                          {log.error}
                        </div>
                      )}
                    </div>

                    <span className="text-caption font-mono text-slate shrink-0">
                      {new Date(log.timestamp).toLocaleTimeString()}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </Card>
        )}

        {/* TAB 4: Storage & Maintenance */}
        {activeTab === 'maintenance' && (
          <div className="space-y-6">
            {/* About ActonOS Appliance Banner */}
            <Card className="p-6 border border-onyx/10 bg-canvas/90 shadow-xs flex flex-col sm:flex-row sm:items-center justify-between gap-6">
              <div className="flex items-center gap-5">
                <img
                  src="/actonos_logo.png"
                  alt="ActonOS"
                  className="h-10 w-auto object-contain shrink-0"
                />
                <div className="border-l border-onyx/10 pl-5 hidden sm:block">
                  <div className="font-serif font-bold text-body text-deep-ink">ActonOS Kernel Runtime</div>
                  <p className="font-sans text-caption text-slate">
                    Autonomous Extensible AI Agent Operating System • Dual-Runtime (Bare-metal & Docker)
                  </p>
                </div>
              </div>

              <div className="flex items-center gap-3 shrink-0">
                <Badge variant="active" className="font-mono text-caption">
                  v0.1.0-release
                </Badge>
                <Badge variant="neutral" className="font-mono text-caption">
                  CGO_ENABLED=0
                </Badge>
              </div>
            </Card>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              {/* Storage Info */}
              <Card className="p-6 border border-onyx/10 bg-canvas/90 space-y-4">
                <h4 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
                  <HardDrive className="w-5 h-5 text-deep-ink" />
                  <span>Storage Breakdown & Backup</span>
                </h4>
                <p className="font-sans text-caption text-slate">
                  Database storage usage and state snapshot for disaster recovery.
                </p>

                {storageInfo && (
                  <div className="space-y-2 font-mono text-caption text-slate bg-soft-meadow p-3 rounded-xl border border-onyx/5">
                    <div>Storage (DB): <strong className="text-deep-ink">{(storageInfo.storage_bytes / (1024 * 1024)).toFixed(2)} MB</strong></div>
                    <div>Vectors: <strong className="text-deep-ink">{(storageInfo.vectors_bytes / (1024 * 1024)).toFixed(2)} MB</strong></div>
                    <div>Workspace: <strong className="text-deep-ink">{(storageInfo.workspace_bytes / (1024 * 1024)).toFixed(2)} MB</strong></div>
                    <div>Total Size: <strong className="text-deep-ink">{(storageInfo.total_bytes / (1024 * 1024)).toFixed(2)} MB</strong></div>
                  </div>
                )}

                <Button
                  variant="ghost"
                  size="sm"
                  icon={<Download className="w-3.5 h-3.5" />}
                  onClick={async () => {
                    try {
                      await api.downloadBackup();
                      success('Backup Exported', 'ActonOS state database snapshot downloaded.');
                    } catch (err: any) {
                      error('Backup Failed', err.message);
                    }
                  }}
                  className="w-full justify-center"
                >
                  Download State Snapshot
                </Button>
              </Card>

              {/* OTA Update */}
              <Card className="p-6 border border-onyx/10 bg-canvas/90 space-y-4">
                <div className="flex items-center justify-between">
                  <h4 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
                    <DownloadCloud className="w-5 h-5 text-deep-ink" />
                    <span>OTA Kernel Updates</span>
                  </h4>
                  <Badge variant="neutral">v0.1.0 (Stable)</Badge>
                </div>
                <p className="font-sans text-caption text-slate">
                  Check GitHub releases for atomic A/B kernel partition firmware updates.
                </p>

                {otaStatus && (
                  <div className="space-y-1 font-mono text-caption text-slate bg-soft-meadow p-3 rounded-xl border border-onyx/5">
                    <div>Current: <strong className="text-deep-ink">v{otaStatus.current_version}</strong></div>
                    <div>Latest: <strong className="text-deep-ink">v{otaStatus.latest_version}</strong></div>
                    <div>Update Available: <span className={otaStatus.update_available ? 'text-hi-yellow font-bold' : 'text-emerald-700'}>
                      {otaStatus.update_available ? 'YES' : 'NO'}
                    </span></div>
                  </div>
                )}

                <Button
                  variant="primary"
                  size="sm"
                  onClick={handleCheckOTA}
                  disabled={checkingOTA}
                  className="w-full justify-center"
                >
                  {checkingOTA ? 'Checking Releases...' : 'Check for Updates'}
                </Button>
              </Card>
            </div>
          </div>
        )}
      </PageContainer>

      {/* Restart Daemon Confirmation Modal */}
      <ConfirmModal
        isOpen={isRestartModalOpen}
        onClose={() => setIsRestartModalOpen(false)}
        onConfirm={handleRestartDaemon}
        title="Restart ActonOS Kernel"
        description="Are you sure you want to restart the ActonOS daemon? All active background ReAct tasks will be cleanly re-initialized."
        confirmLabel="Restart Daemon"
        variant="warning"
      />
    </div>
  );
}
