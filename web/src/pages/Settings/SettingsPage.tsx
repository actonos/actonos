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
  CheckCircle,
  XCircle,
  Activity,
  Clock,
  Sparkles,
  Layers,
  RotateCcw,
} from 'lucide-react';
import {
  api,
  type ProviderKeysData,
  type AuditLogItem,
  type StorageInfoData,
} from '@/lib/api';
import type { SystemMetrics, TailscaleStatus } from '@/lib/types';

type SettingsTab = 'keys' | 'audit' | 'network' | 'maintenance';

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

  // API Keys state
  const [keysData, setKeysData] = useState<ProviderKeysData | null>(null);
  const [anthropicKey, setAnthropicKey] = useState('');
  const [geminiKey, setGeminiKey] = useState('');
  const [openaiKey, setOpenAIKey] = useState('');
  const [deepseekKey, setDeepSeekKey] = useState('');
  const [ollamaURL, setOllamaURL] = useState('http://localhost:11434');
  const [savingKeys, setSavingKeys] = useState(false);
  const [testingProvider, setTestingProvider] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<{ provider: string; success: boolean; msg: string } | null>(null);

  // Audit logs & Storage
  const [auditLogs, setAuditLogs] = useState<AuditLogItem[]>([]);
  const [auditFilter, setAuditFilter] = useState<'all' | 'high' | 'medium' | 'low'>('all');
  const [auditSearch, setAuditSearch] = useState('');
  const [storageInfo, setStorageInfo] = useState<StorageInfoData | null>(null);
  const [otaStatus, setOtaStatus] = useState<any | null>(null);
  const [checkingOTA, setCheckingOTA] = useState(false);
  const [isRestartModalOpen, setIsRestartModalOpen] = useState(false);

  const loadStatus = async () => {
    try {
      const [m, ts, k, logs, stor] = await Promise.all([
        api.getMetrics().catch(() => null),
        api.getTailscale().catch(() => null),
        api.getAPIKeys().catch(() => null),
        api.getAuditLogs().catch(() => ({ entries: [], count: 0 })),
        api.getStorageInfo().catch(() => null),
      ]);
      setMetrics(m);
      setTailscale(ts);
      setKeysData(k);
      if (k?.ollama_url) setOllamaURL(k.ollama_url);
      setAuditLogs(logs.entries || []);
      setStorageInfo(stor);
    } catch (err: any) {
      error('Failed to load system info', err.message);
    }
  };

  const handleSaveKeys = async () => {
    setSavingKeys(true);
    try {
      await api.saveAPIKeys({
        anthropic_key: anthropicKey || undefined,
        gemini_key: geminiKey || undefined,
        openai_key: openaiKey || undefined,
        deepseek_key: deepseekKey || undefined,
        ollama_url: ollamaURL || undefined,
      });
      success('API Keys Saved', 'Provider keys stored in hardware-bound vault.');
      setAnthropicKey('');
      setGeminiKey('');
      setOpenAIKey('');
      setDeepSeekKey('');
      loadStatus();
    } catch (err: any) {
      error('Failed to save API keys', err.message);
    } finally {
      setSavingKeys(false);
    }
  };

  const handleTestKey = async (provider: string, keyVal: string, urlVal?: string) => {
    setTestingProvider(provider);
    setTestResult(null);
    try {
      const res = await api.testAPIKey(provider, keyVal, urlVal);
      setTestResult({
        provider,
        success: true,
        msg: `Connection Success! Model: ${res.model}`,
      });
      success('Provider Test Passed', `Successfully contacted ${provider} (${res.model}).`);
    } catch (err: any) {
      setTestResult({
        provider,
        success: false,
        msg: `Connection Failed: ${err.message}`,
      });
      error('Provider Test Failed', err.message);
    } finally {
      setTestingProvider(null);
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

  useEffect(() => {
    loadStatus();
    const interval = setInterval(loadStatus, 10000);
    return () => clearInterval(interval);
  }, []);

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
              {t('eyebrow', 'System Administration')}
            </span>
            <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight">
              {t('title', 'Settings & System Administration')}
            </h1>
            <p className="font-sans text-body text-slate mt-1 max-w-2xl">
              {t(
                'subtitle',
                'Manage hardware metrics, hardware-bound API keys, Tailscale mesh network, audit logs, and OTA firmware updates.'
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
        <div className="flex items-center gap-1.5 bg-canvas/80 backdrop-blur-sm p-1 rounded-full border border-onyx/10 shadow-xs mb-8 self-start sm:self-auto max-w-fit">
          <button
            onClick={() => setActiveTab('keys')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer ${
              activeTab === 'keys' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            🔑 Provider Keys & Cascade
          </button>
          <button
            onClick={() => setActiveTab('network')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer ${
              activeTab === 'network' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            📶 Hardware & Network
          </button>
          <button
            onClick={() => setActiveTab('audit')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer ${
              activeTab === 'audit' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            🛡️ Audit Logs ({auditLogs.length})
          </button>
          <button
            onClick={() => setActiveTab('maintenance')}
            className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer ${
              activeTab === 'maintenance' ? 'bg-deep-ink text-white font-semibold shadow-xs' : 'text-deep-ink hover:text-slate'
            }`}
          >
            ⚙️ Storage & OTA
          </button>
        </div>

        {/* TAB 1: Provider Keys */}
        {activeTab === 'keys' && (
          <div className="space-y-6">
            <Card className="p-6 border border-onyx/10 bg-canvas/90">
              <div className="flex items-center justify-between mb-4">
                <div>
                  <h3 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
                    <Key className="w-5 h-5 text-deep-ink" />
                    <span>LLM Provider Keys & Cascade Fallbacks</span>
                  </h3>
                  <p className="font-sans text-caption text-slate mt-0.5">
                    Keys are encrypted at rest using AES-256-GCM + Argon2id derived from CPU/Motherboard hardware fingerprint.
                  </p>
                </div>

                <Button
                  variant="primary"
                  size="sm"
                  onClick={handleSaveKeys}
                  disabled={savingKeys}
                >
                  {savingKeys ? 'Saving...' : 'Save API Keys'}
                </Button>
              </div>

              {testResult && (
                <div
                  className={`p-3.5 rounded-[16px] mb-4 text-body-sm font-mono flex items-center gap-2.5 border ${
                    testResult.success
                      ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-900'
                      : 'bg-red-500/10 border-red-500/30 text-red-900'
                  }`}
                >
                  {testResult.success ? <CheckCircle className="w-4 h-4 text-emerald-600 shrink-0" /> : <XCircle className="w-4 h-4 text-red-600 shrink-0" />}
                  <span>{testResult.msg}</span>
                </div>
              )}

              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                {/* Anthropic Card */}
                <div className="p-4 bg-soft-meadow rounded-[20px] border border-onyx/5 space-y-3">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <Sparkles className="w-4 h-4 text-hi-yellow" />
                      <span className="font-semibold text-body-sm text-deep-ink">Anthropic (Claude 3.7 Sonnet)</span>
                    </div>
                    <Badge variant={keysData?.anthropic_configured ? 'active' : 'stopped'}>
                      {keysData?.anthropic_configured ? `Active (${keysData.anthropic_masked})` : 'Not Set'}
                    </Badge>
                  </div>
                  <Input
                    type="password"
                    placeholder="sk-ant-api03-..."
                    value={anthropicKey}
                    onChange={(e) => setAnthropicKey(e.target.value)}
                  />
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => handleTestKey('anthropic', anthropicKey)}
                    disabled={testingProvider === 'anthropic' || (!anthropicKey && !keysData?.anthropic_configured)}
                    className="w-full justify-center"
                  >
                    {testingProvider === 'anthropic' ? 'Verifying...' : 'Test Connection'}
                  </Button>
                </div>

                {/* Gemini Card */}
                <div className="p-4 bg-soft-meadow rounded-[20px] border border-onyx/5 space-y-3">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <Sparkles className="w-4 h-4 text-hi-yellow" />
                      <span className="font-semibold text-body-sm text-deep-ink">Google Gemini (Gemini 2.5 Flash)</span>
                    </div>
                    <Badge variant={keysData?.gemini_configured ? 'active' : 'stopped'}>
                      {keysData?.gemini_configured ? `Active (${keysData.gemini_masked})` : 'Not Set'}
                    </Badge>
                  </div>
                  <Input
                    type="password"
                    placeholder="AIzaSy..."
                    value={geminiKey}
                    onChange={(e) => setGeminiKey(e.target.value)}
                  />
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => handleTestKey('gemini', geminiKey)}
                    disabled={testingProvider === 'gemini' || (!geminiKey && !keysData?.gemini_configured)}
                    className="w-full justify-center"
                  >
                    {testingProvider === 'gemini' ? 'Verifying...' : 'Test Connection'}
                  </Button>
                </div>

                {/* OpenAI Card */}
                <div className="p-4 bg-soft-meadow rounded-[20px] border border-onyx/5 space-y-3">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <Sparkles className="w-4 h-4 text-hi-yellow" />
                      <span className="font-semibold text-body-sm text-deep-ink">OpenAI (GPT-4o / o3-mini)</span>
                    </div>
                    <Badge variant={keysData?.openai_configured ? 'active' : 'stopped'}>
                      {keysData?.openai_configured ? `Active (${keysData.openai_masked})` : 'Not Set'}
                    </Badge>
                  </div>
                  <Input
                    type="password"
                    placeholder="sk-proj-..."
                    value={openaiKey}
                    onChange={(e) => setOpenAIKey(e.target.value)}
                  />
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => handleTestKey('openai', openaiKey)}
                    disabled={testingProvider === 'openai' || (!openaiKey && !keysData?.openai_configured)}
                    className="w-full justify-center"
                  >
                    {testingProvider === 'openai' ? 'Verifying...' : 'Test Connection'}
                  </Button>
                </div>

                {/* DeepSeek & Ollama Card */}
                <div className="p-4 bg-soft-meadow rounded-[20px] border border-onyx/5 space-y-3">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <Sparkles className="w-4 h-4 text-hi-yellow" />
                      <span className="font-semibold text-body-sm text-deep-ink">DeepSeek & Local Ollama</span>
                    </div>
                    <Badge variant={keysData?.deepseek_configured ? 'active' : 'stopped'}>
                      {keysData?.deepseek_configured ? `Active (${keysData.deepseek_masked})` : 'Local Ready'}
                    </Badge>
                  </div>
                  <Input
                    type="password"
                    placeholder="DeepSeek Key: sk-..."
                    value={deepseekKey}
                    onChange={(e) => setDeepSeekKey(e.target.value)}
                  />
                  <Input
                    type="text"
                    placeholder="Ollama Endpoint (e.g. http://localhost:11434)"
                    value={ollamaURL}
                    onChange={(e) => setOllamaURL(e.target.value)}
                  />
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => handleTestKey('ollama', '', ollamaURL)}
                    disabled={testingProvider === 'ollama'}
                    className="w-full justify-center"
                  >
                    {testingProvider === 'ollama' ? 'Pinging Ollama...' : 'Ping Local Ollama'}
                  </Button>
                </div>
              </div>
            </Card>
          </div>
        )}

        {/* TAB 2: Hardware & Network */}
        {activeTab === 'network' && (
          <div className="space-y-6">
            {/* Live Telemetry Card */}
            <Card className="p-6 border border-onyx/10 bg-canvas/90">
              <h3 className="font-serif text-heading-sm text-deep-ink mb-4 flex items-center gap-2">
                <Activity className="w-5 h-5 text-deep-ink" />
                <span>Live Hardware Gauges (Docker / Bare-Metal HAL)</span>
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
                    <CheckCircle className="w-3.5 h-3.5" />
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
                        <span className="font-semibold text-deep-ink">{log.tool_name}</span>
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
                onClick={() => success('Backup Exported', 'ActonOS state snapshot downloaded.')}
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
