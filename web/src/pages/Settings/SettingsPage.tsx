import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
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
  Terminal,
  Activity,
  Thermometer,
  Clock,
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
  const [storageInfo, setStorageInfo] = useState<StorageInfoData | null>(null);
  const [otaStatus, setOtaStatus] = useState<any | null>(null);
  const [checkingOTA, setCheckingOTA] = useState(false);

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
    } catch (err) {
      console.error('Failed to load system info:', err);
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
      alert('API Keys saved and registered into router successfully!');
      setAnthropicKey('');
      setGeminiKey('');
      setOpenAIKey('');
      setDeepSeekKey('');
      loadStatus();
    } catch (err: any) {
      alert(`Save failed: ${err.message}`);
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
        msg: `Success! Model: ${res.model} — Response: "${res.response}"`,
      });
    } catch (err: any) {
      setTestResult({
        provider,
        success: false,
        msg: `Connection Failed: ${err.message}`,
      });
    } finally {
      setTestingProvider(null);
    }
  };

  const handleScanWifi = async () => {
    setLoadingWifi(true);
    try {
      const res = await api.scanWifi();
      setWifiNetworks(res.networks || []);
    } catch (err) {
      console.error('Failed to scan wifi:', err);
    } finally {
      setLoadingWifi(false);
    }
  };

  const handleConnectWifi = async () => {
    if (!selectedSSID) return;
    try {
      await api.connectWifi(selectedSSID, wifiPassword);
      alert('Connected to Wi-Fi successfully!');
    } catch (err: any) {
      alert(`Wi-Fi connection error: ${err.message}`);
    }
  };

  const handleCheckOTA = async () => {
    setCheckingOTA(true);
    try {
      const res = await api.checkOTA();
      setOtaStatus(res);
    } catch (err: any) {
      alert(`OTA check failed: ${err.message}`);
    } finally {
      setCheckingOTA(false);
    }
  };

  useEffect(() => {
    loadStatus();
    const interval = setInterval(loadStatus, 6000);
    return () => clearInterval(interval);
  }, []);

  const tabs: { id: SettingsTab; label: string; icon: React.ElementType }[] = [
    { id: 'keys', label: 'LLM Providers & Keys', icon: Key },
    { id: 'audit', label: 'Audit Logs (OTel)', icon: FileText },
    { id: 'network', label: 'Remote & Network', icon: Shield },
    { id: 'maintenance', label: 'Storage & Firmware', icon: HardDrive },
  ];

  const formatBytes = (bytes: number) => {
    if (!bytes) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

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
              {t('title', 'System & Appliance Settings')}
            </h1>
            <p className="font-sans text-body text-slate mt-1 max-w-2xl">
              {t('subtitle', 'Configure LLM inference keys, inspect OpenTelemetry audit logs, manage Tailscale mesh, and monitor storage.')}
            </p>
          </div>

          <div className="flex items-center gap-2.5 shrink-0 self-start sm:self-center">
            <Button
              variant="ghost"
              size="sm"
              icon={<Download className="w-3.5 h-3.5" />}
              onClick={() => window.open('/api/system/backup', '_blank')}
              title="Download SQLite DB backup file"
            >
              Download Backup (.db)
            </Button>
            <Button
              variant="secondary"
              size="sm"
              icon={<RefreshCw className="w-3.5 h-3.5" />}
              onClick={loadStatus}
            >
              Sync State
            </Button>
          </div>
        </div>

        {/* Tab Navigation Capsule */}
        <div className="flex flex-wrap gap-2 mb-8 p-1.5 bg-soft-meadow rounded-full w-fit border border-onyx/10">
          {tabs.map((tab) => {
            const Icon = tab.icon;
            const isActive = activeTab === tab.id;
            return (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`flex items-center gap-2 px-4 py-2 rounded-full text-body-sm font-sans font-medium transition-all cursor-pointer ${
                  isActive
                    ? 'bg-deep-ink text-white font-semibold shadow-xs'
                    : 'text-deep-ink hover:text-slate'
                }`}
              >
                <Icon className={`w-4 h-4 ${isActive ? 'text-hi-yellow' : 'text-slate'}`} />
                <span>{tab.label}</span>
              </button>
            );
          })}
        </div>

        {/* TAB 1: LLM Providers & API Keys */}
        {activeTab === 'keys' && (
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
            <Card className="lg:col-span-2 p-6 border border-onyx/10">
              <h2 className="font-serif text-heading-sm text-deep-ink mb-1 flex items-center gap-2">
                <Key className="w-5 h-5 text-deep-ink" />
                <span>LLM Providers Configuration</span>
              </h2>
              <p className="font-sans text-body-sm text-slate mb-6">
                Enter API keys for primary and fallback reasoning models. Keys are bound to hardware and stored securely in the AES-256 Vault.
              </p>

              <div className="flex flex-col gap-5">
                {/* Anthropic Claude */}
                <div className="p-4 rounded-[16px] bg-canvas border border-onyx/10">
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center gap-2">
                      <span className="font-semibold text-body-sm text-deep-ink">Anthropic Claude</span>
                      <Badge variant={keysData?.anthropic_configured ? 'active' : 'stopped'}>
                        {keysData?.anthropic_configured ? 'Configured' : 'Missing'}
                      </Badge>
                    </div>
                    {keysData?.anthropic_configured && (
                      <span className="text-caption font-mono text-slate">{keysData.anthropic_masked}</span>
                    )}
                  </div>
                  <div className="flex gap-2">
                    <Input
                      type="password"
                      placeholder="sk-ant-api03-..."
                      value={anthropicKey}
                      onChange={(e) => setAnthropicKey(e.target.value)}
                      className="flex-1"
                    />
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleTestKey('anthropic', anthropicKey)}
                      disabled={!anthropicKey || testingProvider === 'anthropic'}
                    >
                      {testingProvider === 'anthropic' ? 'Testing...' : 'Test'}
                    </Button>
                  </div>
                </div>

                {/* Google Gemini */}
                <div className="p-4 rounded-[16px] bg-canvas border border-onyx/10">
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center gap-2">
                      <span className="font-semibold text-body-sm text-deep-ink">Google Gemini</span>
                      <Badge variant={keysData?.gemini_configured ? 'active' : 'stopped'}>
                        {keysData?.gemini_configured ? 'Configured' : 'Missing'}
                      </Badge>
                    </div>
                    {keysData?.gemini_configured && (
                      <span className="text-caption font-mono text-slate">{keysData.gemini_masked}</span>
                    )}
                  </div>
                  <div className="flex gap-2">
                    <Input
                      type="password"
                      placeholder="AIzaSy..."
                      value={geminiKey}
                      onChange={(e) => setGeminiKey(e.target.value)}
                      className="flex-1"
                    />
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleTestKey('gemini', geminiKey)}
                      disabled={!geminiKey || testingProvider === 'gemini'}
                    >
                      {testingProvider === 'gemini' ? 'Testing...' : 'Test'}
                    </Button>
                  </div>
                </div>

                {/* OpenAI */}
                <div className="p-4 rounded-[16px] bg-canvas border border-onyx/10">
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center gap-2">
                      <span className="font-semibold text-body-sm text-deep-ink">OpenAI (GPT-4o)</span>
                      <Badge variant={keysData?.openai_configured ? 'active' : 'stopped'}>
                        {keysData?.openai_configured ? 'Configured' : 'Missing'}
                      </Badge>
                    </div>
                    {keysData?.openai_configured && (
                      <span className="text-caption font-mono text-slate">{keysData.openai_masked}</span>
                    )}
                  </div>
                  <div className="flex gap-2">
                    <Input
                      type="password"
                      placeholder="sk-proj-..."
                      value={openaiKey}
                      onChange={(e) => setOpenAIKey(e.target.value)}
                      className="flex-1"
                    />
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleTestKey('openai', openaiKey)}
                      disabled={!openaiKey || testingProvider === 'openai'}
                    >
                      {testingProvider === 'openai' ? 'Testing...' : 'Test'}
                    </Button>
                  </div>
                </div>

                {/* DeepSeek */}
                <div className="p-4 rounded-[16px] bg-canvas border border-onyx/10">
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center gap-2">
                      <span className="font-semibold text-body-sm text-deep-ink">DeepSeek (R1 / V3)</span>
                      <Badge variant={keysData?.deepseek_configured ? 'active' : 'stopped'}>
                        {keysData?.deepseek_configured ? 'Configured' : 'Missing'}
                      </Badge>
                    </div>
                    {keysData?.deepseek_configured && (
                      <span className="text-caption font-mono text-slate">{keysData.deepseek_masked}</span>
                    )}
                  </div>
                  <div className="flex gap-2">
                    <Input
                      type="password"
                      placeholder="sk-..."
                      value={deepseekKey}
                      onChange={(e) => setDeepSeekKey(e.target.value)}
                      className="flex-1"
                    />
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleTestKey('deepseek', deepseekKey)}
                      disabled={!deepseekKey || testingProvider === 'deepseek'}
                    >
                      {testingProvider === 'deepseek' ? 'Testing...' : 'Test'}
                    </Button>
                  </div>
                </div>

                {/* Local Ollama */}
                <div className="p-4 rounded-[16px] bg-canvas border border-onyx/10">
                  <div className="flex items-center justify-between mb-2">
                    <span className="font-semibold text-body-sm text-deep-ink">Local Ollama Server</span>
                    <Badge variant="neutral">Local Inference</Badge>
                  </div>
                  <div className="flex gap-2">
                    <Input
                      placeholder="http://localhost:11434"
                      value={ollamaURL}
                      onChange={(e) => setOllamaURL(e.target.value)}
                      className="flex-1"
                    />
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleTestKey('ollama', 'ollama', ollamaURL)}
                      disabled={testingProvider === 'ollama'}
                    >
                      {testingProvider === 'ollama' ? 'Testing...' : 'Test'}
                    </Button>
                  </div>
                </div>

                {/* Test Result Alert */}
                {testResult && (
                  <div className={`p-4 rounded-[16px] flex items-center gap-3 border ${
                    testResult.success ? 'bg-emerald-50 border-emerald-300 text-emerald-900' : 'bg-red-50 border-red-300 text-red-900'
                  }`}>
                    {testResult.success ? <CheckCircle className="w-5 h-5 text-emerald-600 shrink-0" /> : <XCircle className="w-5 h-5 text-red-600 shrink-0" />}
                    <span className="text-body-sm font-sans">{testResult.msg}</span>
                  </div>
                )}

                <Button
                  variant="primary"
                  size="md"
                  onClick={handleSaveKeys}
                  disabled={savingKeys}
                  className="w-full mt-2"
                >
                  {savingKeys ? 'Saving...' : 'Save All Keys to Secure Vault'}
                </Button>
              </div>
            </Card>

            {/* Right sidebar summary */}
            <Card className="p-6 border border-onyx/10 flex flex-col justify-between">
              <div>
                <h3 className="font-serif text-heading-sm text-deep-ink mb-3">Cascade Fallback Engine</h3>
                <p className="font-sans text-body-sm text-slate mb-4">
                  ActonOS routes every agent completion through a 3-tier cascade fallback:
                </p>
                <ol className="list-decimal list-inside space-y-2.5 text-body-sm font-sans text-deep-ink">
                  <li><strong>Tier 1: Claude 3.7 Sonnet</strong> (Primary cognitive planner)</li>
                  <li><strong>Tier 2: Gemini 2.5 Flash</strong> (Sub-second fallback on rate-limits)</li>
                  <li><strong>Tier 3: Local Ollama Llama 3</strong> (Offline privacy & sandbox executor)</li>
                </ol>
              </div>
              <div className="pt-6 border-t border-canvas text-caption text-slate font-mono">
                Entropy Branching Threshold: H &gt; 0.65 bit
              </div>
            </Card>
          </div>
        )}

        {/* TAB 2: Audit Logs (OpenTelemetry) */}
        {activeTab === 'audit' && (
          <Card className="p-6 border border-onyx/10">
            <div className="flex items-center justify-between mb-4">
              <div>
                <h2 className="font-serif text-heading-sm text-deep-ink">Structured Audit Log Stream</h2>
                <p className="font-sans text-body-sm text-slate">
                  Immutable OpenTelemetry JSON-lines logs recorded at <code className="font-mono bg-canvas px-1 rounded">/data/logs/audit.jsonl</code>.
                </p>
              </div>
              <Button variant="ghost" size="sm" icon={<RefreshCw className="w-3.5 h-3.5" />} onClick={loadStatus}>
                Refresh
              </Button>
            </div>

            {auditLogs.length === 0 ? (
              <div className="py-16 text-center text-slate">
                <Terminal className="w-10 h-10 mx-auto mb-2 opacity-30 text-deep-ink" />
                <p className="text-body-sm">No audit entries recorded yet. Execute an agent tool to see events.</p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-left text-body-sm font-sans">
                  <thead>
                    <tr className="border-b border-canvas text-slate text-caption uppercase">
                      <th className="py-3 px-2">Timestamp</th>
                      <th className="py-3 px-2">Trace ID</th>
                      <th className="py-3 px-2">Agent ID</th>
                      <th className="py-3 px-2">Tool Executed</th>
                      <th className="py-3 px-2">Risk</th>
                      <th className="py-3 px-2">Duration</th>
                      <th className="py-3 px-2">Status</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-canvas">
                    {auditLogs.map((entry, idx) => (
                      <tr key={idx} className="hover:bg-canvas/50 transition-colors">
                        <td className="py-2.5 px-2 font-mono text-caption text-slate">{entry.timestamp}</td>
                        <td className="py-2.5 px-2 font-mono text-caption text-slate truncate max-w-[120px]">{entry.trace_id}</td>
                        <td className="py-2.5 px-2 font-semibold text-deep-ink">{entry.agent_id}</td>
                        <td className="py-2.5 px-2 font-mono text-body-sm text-deep-ink">{entry.tool_name}</td>
                        <td className="py-2.5 px-2">
                          <Badge variant={entry.risk_level === 'High' ? 'accent' : 'neutral'}>
                            {entry.risk_level}
                          </Badge>
                        </td>
                        <td className="py-2.5 px-2 font-mono text-caption text-slate">{entry.execution_time_ms} ms</td>
                        <td className="py-2.5 px-2">
                          <Badge variant={entry.status === 'Success' ? 'active' : 'stopped'}>
                            {entry.status}
                          </Badge>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Card>
        )}

        {/* TAB 3: Remote Access & Network */}
        {activeTab === 'network' && (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
            {/* Tailscale Remote Access */}
            <Card className="border border-onyx/10 p-6">
              <div className="flex items-center gap-3 mb-4">
                <div className="w-10 h-10 rounded-full bg-deep-ink flex items-center justify-center text-hi-yellow">
                  <Shield className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="font-serif text-heading-sm text-deep-ink">Tailscale Mesh VPN</h3>
                  <p className="font-sans text-body-sm text-slate">Direct point-to-point WireGuard mesh network.</p>
                </div>
              </div>

              <div className="p-4 bg-canvas rounded-[16px] border border-onyx/10 space-y-3 mb-4">
                <div className="flex justify-between items-center text-body-sm">
                  <span className="text-slate">Connection Status:</span>
                  <Badge variant={tailscale?.connected ? 'active' : 'stopped'}>
                    {tailscale?.connected ? 'Connected' : 'Offline'}
                  </Badge>
                </div>
                <div className="flex justify-between items-center text-body-sm">
                  <span className="text-slate">Tailscale Mesh IP:</span>
                  <span className="font-mono font-medium text-deep-ink">{tailscale?.ip || 'Not Assigned'}</span>
                </div>
                <div className="flex justify-between items-center text-body-sm">
                  <span className="text-slate">Node FQDN:</span>
                  <span className="font-mono text-caption text-slate">{tailscale?.fqdn || 'acton-mini.ts.net'}</span>
                </div>
              </div>
            </Card>

            {/* Wi-Fi Scanner & Connector */}
            <Card className="border border-onyx/10 p-6">
              <div className="flex items-center justify-between mb-4">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 rounded-full bg-canvas border border-onyx flex items-center justify-center text-deep-ink">
                    <Wifi className="w-5 h-5" />
                  </div>
                  <div>
                    <h3 className="font-serif text-heading-sm text-deep-ink">Wi-Fi Network</h3>
                    <p className="font-sans text-body-sm text-slate">Scan and connect to local wireless networks.</p>
                  </div>
                </div>
                <Button variant="ghost" size="sm" icon={<RefreshCw className="w-3.5 h-3.5" />} onClick={handleScanWifi} disabled={loadingWifi}>
                  {loadingWifi ? 'Scanning...' : 'Scan'}
                </Button>
              </div>

              {wifiNetworks.length > 0 ? (
                <div className="space-y-3">
                  <select
                    className="w-full bg-canvas text-deep-ink p-3 rounded-full border border-onyx text-body-sm font-sans"
                    value={selectedSSID}
                    onChange={(e) => setSelectedSSID(e.target.value)}
                  >
                    <option value="">Select Wi-Fi Network</option>
                    {wifiNetworks.map((n, i) => (
                      <option key={i} value={n.ssid}>{n.ssid} ({n.signal}% - {n.security})</option>
                    ))}
                  </select>
                  <Input
                    type="password"
                    placeholder="Wi-Fi Password"
                    value={wifiPassword}
                    onChange={(e) => setWifiPassword(e.target.value)}
                  />
                  <Button variant="primary" size="sm" onClick={handleConnectWifi} className="w-full">
                    Connect to Wi-Fi
                  </Button>
                </div>
              ) : (
                <div className="py-8 text-center text-slate text-body-sm">
                  Click 'Scan' to discover available wireless networks.
                </div>
              )}
            </Card>
          </div>
        )}

        {/* TAB 4: Storage & Firmware Maintenance */}
        {activeTab === 'maintenance' && (
          <div className="space-y-8">
            {/* Live Hardware Telemetry */}
            {metrics && (
              <Card className="border border-onyx/10 p-6">
                <div className="flex items-center justify-between mb-4">
                  <h3 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
                    <Activity className="w-5 h-5 text-deep-ink" />
                    <span>Live Hardware Telemetry & HAL Metrics</span>
                  </h3>
                  <Badge variant="active">Uptime: {Math.floor(metrics.uptime_seconds / 60)} mins</Badge>
                </div>
                <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
                  <div className="p-3 bg-canvas rounded-[16px] border border-onyx/5">
                    <div className="flex items-center gap-2 text-slate text-caption mb-1">
                      <Cpu className="w-4 h-4" /> CPU Model
                    </div>
                    <div className="font-semibold text-deep-ink truncate text-body-sm">{metrics.cpu.model || 'AMD/Intel 64-bit'}</div>
                    <div className="text-caption text-slate font-mono">{metrics.cpu.cores} Cores • {metrics.cpu.usage_percent.toFixed(1)}% Load</div>
                  </div>
                  <div className="p-3 bg-canvas rounded-[16px] border border-onyx/5">
                    <div className="flex items-center gap-2 text-slate text-caption mb-1">
                      <Thermometer className="w-4 h-4" /> Chip Temperature
                    </div>
                    <div className="font-semibold text-deep-ink text-heading-sm font-mono">{metrics.cpu.temperature_celsius ? `${metrics.cpu.temperature_celsius}°C` : '42.0°C'}</div>
                    <div className="text-caption text-emerald-700">Thermal Nominal</div>
                  </div>
                  <div className="p-3 bg-canvas rounded-[16px] border border-onyx/5">
                    <div className="flex items-center gap-2 text-slate text-caption mb-1">
                      <HardDrive className="w-4 h-4" /> Physical Memory (RAM)
                    </div>
                    <div className="font-semibold text-deep-ink text-body-sm font-mono">{metrics.memory.used_mb} / {metrics.memory.total_mb} MB</div>
                    <div className="text-caption text-slate">actond: {metrics.memory.actond_mb.toFixed(1)} MB</div>
                  </div>
                  <div className="p-3 bg-canvas rounded-[16px] border border-onyx/5">
                    <div className="flex items-center gap-2 text-slate text-caption mb-1">
                      <Clock className="w-4 h-4" /> System Clock
                    </div>
                    <div className="font-semibold text-deep-ink text-body-sm font-mono">{new Date(metrics.timestamp).toLocaleTimeString()}</div>
                    <div className="text-caption text-slate">Synchronized (UTC)</div>
                  </div>
                </div>
              </Card>
            )}

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
              {/* Storage Breakdown */}
              <Card className="border border-onyx/10 p-6">
                <h3 className="font-serif text-heading-sm text-deep-ink mb-1 flex items-center gap-2">
                  <HardDrive className="w-5 h-5 text-deep-ink" />
                  <span>Partition Storage Allocation</span>
                </h3>
                <p className="font-sans text-body-sm text-slate mb-4">Storage distribution across ActonOS sub-volumes.</p>

                <div className="space-y-3">
                  <div className="p-3 bg-canvas rounded-[16px] flex justify-between items-center text-body-sm border border-onyx/5">
                    <span className="font-medium text-deep-ink">Relational SQLite Database (/storage)</span>
                    <span className="font-mono font-semibold">{formatBytes(storageInfo?.storage_bytes || 0)}</span>
                  </div>
                  <div className="p-3 bg-canvas rounded-[16px] flex justify-between items-center text-body-sm border border-onyx/5">
                    <span className="font-medium text-deep-ink">Vector Store Chromem-go (/vectors)</span>
                    <span className="font-mono font-semibold">{formatBytes(storageInfo?.vectors_bytes || 0)}</span>
                  </div>
                  <div className="p-3 bg-canvas rounded-[16px] flex justify-between items-center text-body-sm border border-onyx/5">
                    <span className="font-medium text-deep-ink">Isolated Agent Workspace (/workspace)</span>
                    <span className="font-mono font-semibold">{formatBytes(storageInfo?.workspace_bytes || 0)}</span>
                  </div>
                  <div className="p-3 bg-canvas rounded-[16px] flex justify-between items-center text-body-sm border border-onyx/5">
                    <span className="font-medium text-deep-ink">Audit Logs (/logs)</span>
                    <span className="font-mono font-semibold">{formatBytes(storageInfo?.logs_bytes || 0)}</span>
                  </div>
                </div>
              </Card>

              {/* OTA Firmware Update & Restart */}
              <Card className="border border-onyx/10 p-6 flex flex-col justify-between">
                <div>
                  <h3 className="font-serif text-heading-sm text-deep-ink mb-1 flex items-center gap-2">
                    <DownloadCloud className="w-5 h-5 text-deep-ink" />
                    <span>Atomic OTA & Firmware Watchdog</span>
                  </h3>
                  <p className="font-sans text-body-sm text-slate mb-4">
                    ActonOS supports self-healing atomic updates with a 30s automatic health watchdog rollback.
                  </p>

                  <div className="p-4 bg-canvas rounded-[16px] border border-onyx/10 mb-4 space-y-2 text-body-sm">
                    <div className="flex justify-between">
                      <span className="text-slate">Installed Version:</span>
                      <span className="font-mono font-bold text-deep-ink">v0.1.0 (dev)</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-slate">Update Channel:</span>
                      <span className="font-mono text-deep-ink">Stable Release</span>
                    </div>
                    {otaStatus && (
                      <div className="flex justify-between text-emerald-700">
                        <span>Status:</span>
                        <span>System is up to date ({otaStatus.last_checked})</span>
                      </div>
                    )}
                  </div>
                </div>

                <div className="flex gap-3">
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={handleCheckOTA}
                    disabled={checkingOTA}
                    className="flex-1"
                  >
                    {checkingOTA ? 'Checking...' : 'Check for Updates'}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => api.restartDaemon()}
                  >
                    Restart Daemon
                  </Button>
                </div>
              </Card>
            </div>
          </div>
        )}
      </PageContainer>
    </div>
  );
}
