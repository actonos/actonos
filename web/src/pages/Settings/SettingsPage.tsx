import { useState, useEffect, useMemo } from 'react';
import { getErrorMessage } from '@/lib/errors';
import { useTranslation } from 'react-i18next';
import { getGroupedTimezones } from '@/lib/timezones';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { TokenLedgerPanel } from '@/components/modals/TokenLedgerModal';
import { useToast } from '@/components/ui/Toast';
import {
  Shield,
  Wifi,
  RefreshCw,
  Cpu,
  HardDrive,
  Key,
  DownloadCloud,
  Download,
  CheckCircle2,
  XCircle,
  Activity,
  Clock,
  Layers,
  RotateCcw,
  Eye,
  EyeOff,
  Save,
  User,
  Palette,
} from 'lucide-react';
import {
  api,
  type StorageInfoData,
  type UserIdentityProfile,
  type WifiNetwork,
  type OTAStatus,
} from '@/lib/api';
import { isApprovalRequired, type SystemMetrics, type TailscaleStatus, type LLMProviderInfo } from '@/lib/types';
import { PageHeader } from '@/components/ui/PageHeader';
import { SegmentedControl } from '@/components/ui/SegmentedControl';
import { readHashParams, setHashParam } from '@/lib/url-state';
import { useTheme, type UITheme } from '@/components/providers/ThemeProvider';
import { useDensity, type UIDensity } from '@/components/providers/DensityProvider';

type SettingsTab = 'identity' | 'keys' | 'tokens' | 'network' | 'maintenance';

import { useModelCatalog } from '@/components/providers/ModelProvider';
import type { ProviderMeta } from '@/lib/models';

export function SettingsPage() {
  const { t } = useTranslation('settings');
  const { success, error, info } = useToast();
  const { theme, setTheme } = useTheme();
  const { density, setDensity } = useDensity();
  const { providers: providerMetas } = useModelCatalog();
  const [activeTab, setActiveTab] = useState<SettingsTab>(() => {
    const value = readHashParams().get('view');
    return ['identity', 'keys', 'tokens', 'network', 'maintenance'].includes(value || '')
      ? value as SettingsTab
      : 'keys';
  });
  const changeTab = (tab: SettingsTab) => {
    setActiveTab(tab);
    setHashParam('view', tab === 'keys' ? undefined : tab);
  };
  const { resolvedTheme } = useTheme();

  const timezoneGroups = useMemo(() => getGroupedTimezones(), []);

  // Metrics & Tailscale
  const [metrics, setMetrics] = useState<SystemMetrics | null>(null);
  const [tailscale, setTailscale] = useState<TailscaleStatus | null>(null);
  const [wifiNetworks, setWifiNetworks] = useState<WifiNetwork[]>([]);
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

  // Storage & Identity
  const [storageInfo, setStorageInfo] = useState<StorageInfoData | null>(null);
  const [otaStatus, setOtaStatus] = useState<OTAStatus | null>(null);
  const [checkingOTA, setCheckingOTA] = useState(false);
  const [isRestartModalOpen, setIsRestartModalOpen] = useState(false);

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
  const [daemonVersion, setDaemonVersion] = useState<string>(__APP_VERSION__);

  const loadStatus = async () => {
    try {
      const [m, ts, k, stor, ident, health] = await Promise.all([
        api.getMetrics().catch(() => null),
        api.getTailscale().catch(() => null),
        api.getAPIKeys().catch(() => null),
        api.getStorageInfo().catch(() => null),
        api.getIdentity().catch(() => null),
        api.getHealth().catch(() => null),
      ]);
      setMetrics(m);
      setTailscale(ts);
      setStorageInfo(stor);
      if (ident) {
        setIdentityProfile((prev) => ({ ...prev, ...ident }));
      }
      if ((health as { version?: string } | null)?.version) {
        setDaemonVersion((health as { version: string }).version);
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
    } catch (err) {
      error('Failed to load system info', getErrorMessage(err));
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
    } catch (err) {
      error('Save Failed', getErrorMessage(err));
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
    } catch (err) {
      setTestResults((prev) => ({
        ...prev,
        [meta.id]: {
          success: false,
          latency: 0,
          msg: `Failed: ${getErrorMessage(err)}`,
        },
      }));
      error('Connection Test Failed', getErrorMessage(err));
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
    } catch (err) {
      error('Failed to save identity', getErrorMessage(err));
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
    } catch (err) {
      error('Wi-Fi scan failed', getErrorMessage(err));
    } finally {
      setLoadingWifi(false);
    }
  };

  const handleConnectWifi = async () => {
    if (!selectedSSID) return;
    try {
      await api.connectWifi(selectedSSID, wifiPassword);
      success('Wi-Fi Connected', `Joined wireless network "${selectedSSID}".`);
    } catch (err) {
      error('Wi-Fi connection error', getErrorMessage(err));
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
    } catch (err) {
      error('OTA check failed', getErrorMessage(err));
    } finally {
      setCheckingOTA(false);
    }
  };

  const handleRestartDaemon = async () => {
    try {
      const result = await api.restartDaemon();
      if (isApprovalRequired(result)) {
        info(t('common:approval.queuedTitle'), t('common:approval.queuedDescription'));
        return;
      }
      success('Daemon Restart Initiated', 'ActonOS kernel is rebooting. Web UI will reconnect shortly.');
    } catch (err) {
      error('Restart failed', getErrorMessage(err));
    }
  };

  const configuredCount = providerMetas.filter((m: ProviderMeta) => providersData[m.id]?.configured).length;

  return (
    <div className="relative min-h-[calc(100vh-64px)]">
      <BlobBackdrop />

      <PageContainer maxWidth="wide">
        <PageHeader
          eyebrow={t('eyebrow')}
          title={t('title')}
          description={t('subtitle')}
          badge={<Badge variant="neutral" className="font-mono">{t('header.activeModels', { configured: configuredCount, total: providerMetas.length })}</Badge>}
          actions={(
            <>
              <Button
                variant="ghost"
                size="sm"
                icon={<RefreshCw className="w-3.5 h-3.5" />}
                onClick={loadStatus}
              >
                {t('actions.refresh')}
              </Button>
              <Button
                variant="danger"
                size="sm"
                icon={<RotateCcw className="w-3.5 h-3.5" />}
                onClick={() => setIsRestartModalOpen(true)}
              >
                {t('actions.restart')}
              </Button>
            </>
          )}
        />

        <SegmentedControl
          value={activeTab}
          onChange={changeTab}
          label={t('tabs.label')}
          className="mb-8 w-fit"
          options={[
            { value: 'identity', label: t('tabs.identity') },
            { value: 'keys', label: t('tabs.providers', { count: configuredCount }) },
            { value: 'tokens', label: t('tabs.tokens') },
            { value: 'network', label: t('tabs.network') },
            { value: 'maintenance', label: t('tabs.maintenance') },
          ]}
        />

        {/* TAB: Identity & Owner Profile */}
        {activeTab === 'identity' && (
          <div className="space-y-6">
            <div className="flex items-center justify-between">
              <div>
                <h3 className="font-serif text-heading-sm font-semibold text-deep-ink">
                  {t('identity.title')}
                </h3>
                <p className="font-sans text-caption text-slate mt-0.5">
                  {t('identity.subtitle')}
                </p>
              </div>

              <Button
                variant="primary"
                size="sm"
                icon={savingIdentity ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Save className="w-3.5 h-3.5" />}
                disabled={savingIdentity}
                onClick={handleSaveIdentity}
              >
                {savingIdentity ? t('identity.saving') : t('identity.save')}
              </Button>
            </div>

            {/* Appearance & Display Card */}
            <Card className="p-6 border border-onyx/15 shadow-xs max-w-3xl">
              <div className="flex items-center gap-2 border-b border-onyx/10 pb-3 mb-5">
                <Palette className="w-5 h-5 text-deep-ink" />
                <h4 className="font-serif text-subheading font-semibold text-deep-ink">
                  {t('appearance.title', 'Appearance & Display')}
                </h4>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                {/* Theme Selector */}
                <div>
                  <label className="block text-caption uppercase text-slate font-semibold mb-2">
                    {t('appearance.theme', 'Theme Mode')}
                  </label>
                  <SegmentedControl
                    value={theme}
                    onChange={(val) => setTheme(val as UITheme)}
                    label={t('appearance.theme', 'Theme Mode')}
                    className="w-full"
                    options={[
                      { value: 'light', label: t('appearance.light', 'Light') },
                      { value: 'dark', label: t('appearance.dark', 'Dark') },
                      { value: 'system', label: t('appearance.system', 'System') },
                    ]}
                  />
                  <p className="text-[11px] text-slate mt-2">
                    {theme === 'system'
                      ? t('appearance.systemHint', 'Follows your operating system color preference.')
                      : theme === 'dark'
                        ? t('appearance.darkHint', 'Obsidian dark theme for focused night work.')
                        : t('appearance.lightHint', 'Sunlit Wildflower high-contrast light theme.')}
                  </p>
                </div>

                {/* Density Selector */}
                <div>
                  <label className="block text-caption uppercase text-slate font-semibold mb-2">
                    {t('appearance.density', 'UI Density')}
                  </label>
                  <SegmentedControl
                    value={density}
                    onChange={(val) => setDensity(val as UIDensity)}
                    label={t('appearance.density', 'UI Density')}
                    className="w-full"
                    options={[
                      { value: 'comfortable', label: t('appearance.comfortable', 'Comfortable') },
                      { value: 'compact', label: t('appearance.compact', 'Compact') },
                    ]}
                  />
                  <p className="text-[11px] text-slate mt-2">
                    {density === 'compact'
                      ? t('appearance.compactHint', 'Tighter padding and dense row heights for small screens.')
                      : t('appearance.comfortableHint', 'Generous spacing and airy touch targets.')}
                  </p>
                </div>
              </div>
            </Card>

            <Card className="p-8 border border-onyx/15 shadow-sm max-w-3xl">
              <div className="flex items-center gap-2 border-b border-onyx/10 pb-4 mb-6">
                <User className="w-5 h-5 text-deep-ink" />
                <h4 className="font-serif text-subheading font-semibold text-deep-ink">
                  {t('identity.preferences')}
                </h4>
              </div>

              <div className="space-y-4 text-body-sm">
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-caption uppercase text-slate font-semibold mb-1.5">
                      {t('identity.name')}
                    </label>
                    <Input
                      value={identityProfile.user_name}
                      onChange={(e) => setIdentityProfile({ ...identityProfile, user_name: e.target.value })}
                      placeholder={t('identity.namePlaceholder')}
                    />
                  </div>

                  <div>
                    <label className="block text-caption uppercase text-slate font-semibold mb-1.5">
                      {t('identity.role')}
                    </label>
                    <Input
                      value={identityProfile.user_role || ''}
                      onChange={(e) => setIdentityProfile({ ...identityProfile, user_role: e.target.value })}
                      placeholder={t('identity.rolePlaceholder')}
                    />
                  </div>
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-caption uppercase text-slate font-semibold mb-1.5">
                      {t('identity.language')}
                    </label>
                    <select
                      value={identityProfile.language}
                      onChange={(e) => setIdentityProfile({ ...identityProfile, language: e.target.value })}
                      className="w-full px-4 py-2.5 bg-canvas border border-onyx/15 rounded-full text-body-sm text-deep-ink font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink"
                    >
                      <option value="en">{t('identity.english')}</option>
                      <option value="vi">{t('identity.vietnamese')}</option>
                    </select>
                  </div>

                  <div>
                    <label className="block text-caption uppercase text-slate font-semibold mb-1.5">
                      {t('identity.timezone')}
                    </label>
                    <select
                      value={identityProfile.timezone || 'UTC'}
                      onChange={(e) => setIdentityProfile({ ...identityProfile, timezone: e.target.value })}
                      className="w-full px-4 py-2.5 bg-canvas border border-onyx/15 rounded-full text-body-sm text-deep-ink font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink"
                    >
                      {timezoneGroups.map((group) => (
                        <optgroup key={group.region} label={group.region}>
                          {group.zones.map((tz) => (
                            <option key={tz.value} value={tz.value}>
                              {tz.label}
                            </option>
                          ))}
                        </optgroup>
                      ))}
                    </select>
                  </div>
                </div>

                <div>
                  <label className="block text-caption uppercase text-slate font-semibold mb-1.5">
                    {t('identity.tone')}
                  </label>
                  <Input
                    value={identityProfile.communication_style || ''}
                    onChange={(e) => setIdentityProfile({ ...identityProfile, communication_style: e.target.value })}
                    placeholder={t('identity.tonePlaceholder')}
                  />
                </div>

                <div>
                  <label className="block text-caption uppercase text-slate font-semibold mb-1.5">
                    {t('identity.bio')}
                  </label>
                  <textarea
                    rows={2}
                    value={identityProfile.bio || ''}
                    onChange={(e) => setIdentityProfile({ ...identityProfile, bio: e.target.value })}
                    placeholder={t('identity.bioPlaceholder')}
                    className="w-full p-3 bg-canvas border border-onyx/15 rounded-2xl text-body-sm text-deep-ink font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink resize-none leading-relaxed"
                  />
                </div>

                <div>
                  <label className="block text-caption uppercase text-slate font-semibold mb-1.5">
                    {t('identity.directives')}
                  </label>
                  <textarea
                    rows={4}
                    value={identityProfile.custom_instructions || ''}
                    onChange={(e) => setIdentityProfile({ ...identityProfile, custom_instructions: e.target.value })}
                    placeholder={t('identity.directivesPlaceholder')}
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
                    {savingIdentity ? t('identity.saving') : t('identity.saveSettings')}
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
                    {t('providers.vaultTitle')}
                  </h3>
                  <p className="font-sans text-caption text-slate">
                    {t('providers.vaultDescription')}
                  </p>
                </div>
              </div>

              <Badge variant="active" className="text-[11px] font-mono shrink-0 self-start sm:self-center">
                {t('providers.encryptionActive')}
              </Badge>
            </div>

            {/* Providers Grid */}
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
              {providerMetas.map((meta) => {
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
                    className={`p-6 border transition-all flex flex-col justify-between ${isConfigured
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
                            className={`px-2.5 py-1 rounded-full text-[11px] font-semibold transition-all cursor-pointer ${isEnabled
                              ? 'bg-emerald-500/10 text-emerald-700 border border-emerald-500/20'
                              : 'bg-onyx/5 text-slate border border-onyx/10'
                              }`}
                          >
                            {isEnabled ? t('providers.active') : t('providers.disabled')}
                          </button>
                          <Badge variant={isConfigured ? 'active' : 'stopped'}>
                            {isConfigured ? t('providers.configured') : t('providers.notSet')}
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
                                {t('providers.apiKey')}
                              </label>
                              {isConfigured && pData?.masked_key && (
                                <span className="text-[11px] font-mono text-slate">
                                  {t('providers.current', { value: pData.masked_key })}
                                </span>
                              )}
                            </div>
                            <div className="relative">
                              <Input
                                type={showKey ? 'text' : 'password'}
                                placeholder={isConfigured ? t('providers.updateKeyPlaceholder') : 'sk-...'}
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
                                aria-label={showKey ? t('providers.hideKey') : t('providers.showKey')}
                              >
                                {showKey ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                              </button>
                            </div>
                          </div>
                        )}

                        {/* Base URL Input */}
                        <div>
                          <label className="text-caption font-semibold text-deep-ink block mb-1">
                            {t('providers.endpoint')}
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
                            {t('providers.defaultModel')}
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
                            className={`flex items-center gap-1.5 pt-1 text-[11px] font-mono ${testInfo.success ? 'text-emerald-700 font-semibold' : 'text-red-600'
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
                        {isTesting ? t('providers.testing') : t('providers.test')}
                      </Button>

                      <Button
                        variant="primary"
                        size="sm"
                        icon={<Save className="w-3.5 h-3.5" />}
                        onClick={() => handleSaveSingleProvider(meta)}
                        disabled={isSaving}
                      >
                        {isSaving ? t('providers.saving') : t('providers.save')}
                      </Button>
                    </div>
                  </Card>
                );
              })}
            </div>
          </div>
        )}

        {/* TAB: Token Consumption & Cost Ledger */}
        {activeTab === 'tokens' && <TokenLedgerPanel />}

        {/* TAB: Hardware & Network */}
        {activeTab === 'network' && (
          <div className="space-y-6">
            {/* Live Telemetry Card */}
            <Card className="p-6 border border-onyx/10 bg-canvas/90">
              <h3 className="font-serif text-heading-sm text-deep-ink mb-4 flex items-center gap-2">
                <Activity className="w-5 h-5 text-deep-ink" />
                <span>{t('network.hardwareTitle')}</span>
              </h3>

              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                {/* CPU Gauge */}
                <div className="p-4 bg-soft-meadow rounded-[20px] border border-onyx/5">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-caption font-semibold uppercase text-slate">{t('network.cpu')}</span>
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
                    {t('network.cores', { count: metrics?.cpu?.cores || 1, model: metrics?.cpu?.model || 'Hardware HAL' })}
                  </div>
                </div>

                {/* RAM Gauge */}
                <div className="p-4 bg-soft-meadow rounded-[20px] border border-onyx/5">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-caption font-semibold uppercase text-slate">{t('network.memory')}</span>
                    <HardDrive className="w-4 h-4 text-deep-ink" />
                  </div>
                  <div className="text-heading font-serif text-deep-ink">
                    {t('network.megabytes', { value: metrics?.memory?.used_mb || 0 })}
                  </div>
                  <div className="w-full bg-onyx/10 h-2 rounded-full mt-3 overflow-hidden">
                    <div
                      className="bg-emerald-600 h-full rounded-full transition-all"
                      style={{ width: `${Math.min(100, ((metrics?.memory?.used_mb || 0) / (metrics?.memory?.total_mb || 1)) * 100)}%` }}
                    />
                  </div>
                  <div className="text-[11px] font-mono text-slate mt-2">
                    {t('network.memoryDetail', { total: metrics?.memory?.total_mb || 0, daemon: metrics?.memory?.actond_mb || 0 })}
                  </div>
                </div>

                {/* Storage Gauge */}
                <div className="p-4 bg-soft-meadow rounded-[20px] border border-onyx/5">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-caption font-semibold uppercase text-slate">{t('network.disk')}</span>
                    <Layers className="w-4 h-4 text-deep-ink" />
                  </div>
                  <div className="text-heading font-serif text-deep-ink">
                    {t('network.gigabytes', { value: (metrics?.disk?.used_gb || 0).toFixed(1) })}
                  </div>
                  <div className="w-full bg-onyx/10 h-2 rounded-full mt-3 overflow-hidden">
                    <div
                      className="bg-amber-600 h-full rounded-full transition-all"
                      style={{ width: `${Math.min(100, ((metrics?.disk?.used_gb || 0) / (metrics?.disk?.total_gb || 1)) * 100)}%` }}
                    />
                  </div>
                  <div className="text-[11px] font-mono text-slate mt-2">
                    {t('network.diskDetail', { total: metrics?.disk?.total_gb || 0, data: metrics?.disk?.data_dir_gb || 0 })}
                  </div>
                </div>

                {/* Uptime & Thermal */}
                <div className="p-4 bg-soft-meadow rounded-[20px] border border-onyx/5">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-caption font-semibold uppercase text-slate">{t('network.uptime')}</span>
                    <Clock className="w-4 h-4 text-deep-ink" />
                  </div>
                  <div className="text-heading font-serif text-deep-ink">
                    {Math.floor((metrics?.uptime_seconds || 0) / 60)}m
                  </div>
                  <div className="flex items-center gap-1.5 text-caption font-mono text-emerald-700 mt-3">
                    <CheckCircle2 className="w-3.5 h-3.5" />
                    <span>{t('network.healthy')}</span>
                  </div>
                  <div className="text-[11px] font-mono text-slate mt-2">
                    {t('network.temperature', { value: metrics?.cpu?.temperature_celsius || 42 })}
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
                    <h4 className="font-serif text-heading-sm text-deep-ink">{t('network.tailscaleTitle')}</h4>
                  </div>
                  <Badge variant={tailscale?.connected ? 'active' : 'stopped'}>
                    {tailscale?.connected ? t('network.meshActive') : t('providers.disabled')}
                  </Badge>
                </div>
                <p className="font-sans text-caption text-slate mb-4">
                  {t('network.tailscaleDescription')}
                </p>

                {tailscale?.connected && (
                  <div className="space-y-1.5 font-mono text-caption text-slate bg-soft-meadow p-3 rounded-xl border border-onyx/5">
                    <div>{t('network.nodeIP')} <strong className="text-deep-ink">{tailscale.ip}</strong></div>
                    <div>{t('network.hostname')} <strong className="text-deep-ink">{tailscale.hostname}</strong></div>
                    <div>{t('network.peers')} <strong className="text-deep-ink">{tailscale.peers_count}</strong></div>
                  </div>
                )}
              </Card>

              {/* Wi-Fi Scanner */}
              <Card className="p-6 border border-onyx/10 bg-canvas/90">
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2.5">
                    <Wifi className="w-5 h-5 text-deep-ink" />
                    <h4 className="font-serif text-heading-sm text-deep-ink">{t('network.wifiTitle')}</h4>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={handleScanWifi}
                    disabled={loadingWifi}
                  >
                    {loadingWifi ? t('network.scanning') : t('network.scan')}
                  </Button>
                </div>

                <div className="space-y-2">
                  <select
                    value={selectedSSID}
                    onChange={(e) => setSelectedSSID(e.target.value)}
                    className="w-full bg-soft-meadow text-deep-ink p-2.5 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none"
                  >
                    <option value="">{t('network.selectWifi', { count: wifiNetworks.length })}</option>
                    {wifiNetworks.map((net) => (
                      <option key={net.ssid} value={net.ssid}>
                        {net.ssid} ({net.signal_strength || 80}%)
                      </option>
                    ))}
                  </select>

                  <Input
                    type="password"
                    placeholder={t('network.wifiPassword')}
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
                    {t('network.join')}
                  </Button>
                </div>
              </Card>
            </div>
          </div>
        )}

        {/* TAB 4: Storage & Maintenance */}
        {activeTab === 'maintenance' && (
          <div className="space-y-6">
            {/* About ActonOS Appliance Banner */}
            <Card className="p-6 border border-onyx/10 bg-canvas/90 shadow-xs flex flex-col sm:flex-row sm:items-center justify-between gap-6">
              <div className="flex items-center gap-5">
                <img
                  src={resolvedTheme === 'light' ? "/actonos_logo.png" : "/actonos_logo_light.png"}
                  alt="ActonOS"
                  className="h-10 w-auto object-contain shrink-0"
                />
                <div className="border-l border-onyx/10 pl-5 hidden sm:block">
                  <div className="font-serif font-bold text-body text-deep-ink">{t('maintenance.runtimeTitle')}</div>
                  <p className="font-sans text-caption text-slate">
                    {t('maintenance.runtimeDescription')}
                  </p>
                </div>
              </div>

              <div className="flex items-center gap-3 shrink-0">
                <Badge variant="active" className="font-mono text-caption">
                  v{daemonVersion}
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
                  <span>{t('maintenance.storageTitle')}</span>
                </h4>
                <p className="font-sans text-caption text-slate">
                  {t('maintenance.storageDescription')}
                </p>

                {storageInfo && (
                  <div className="space-y-2 font-mono text-caption text-slate bg-soft-meadow p-3 rounded-xl border border-onyx/5">
                    <div>{t('maintenance.database')} <strong className="text-deep-ink">{t('maintenance.megabytes', { value: (storageInfo.storage_bytes / (1024 * 1024)).toFixed(2) })}</strong></div>
                    <div>{t('maintenance.vectors')} <strong className="text-deep-ink">{t('maintenance.megabytes', { value: (storageInfo.vectors_bytes / (1024 * 1024)).toFixed(2) })}</strong></div>
                    <div>{t('maintenance.workspace')} <strong className="text-deep-ink">{t('maintenance.megabytes', { value: (storageInfo.workspace_bytes / (1024 * 1024)).toFixed(2) })}</strong></div>
                    <div>{t('maintenance.total')} <strong className="text-deep-ink">{t('maintenance.megabytes', { value: (storageInfo.total_bytes / (1024 * 1024)).toFixed(2) })}</strong></div>
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
                    } catch (err) {
                      error('Backup Failed', getErrorMessage(err));
                    }
                  }}
                  className="w-full justify-center"
                >
                  {t('maintenance.download')}
                </Button>
              </Card>

              {/* OTA Update */}
              <Card className="p-6 border border-onyx/10 bg-canvas/90 space-y-4">
                <div className="flex items-center justify-between">
                  <h4 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
                    <DownloadCloud className="w-5 h-5 text-deep-ink" />
                    <span>{t('maintenance.otaTitle')}</span>
                  </h4>
                  <Badge variant="neutral">v{daemonVersion}</Badge>
                </div>
                <p className="font-sans text-caption text-slate">
                  {t('maintenance.otaDescription')}
                </p>

                {otaStatus && (
                  <div className="space-y-1 font-mono text-caption text-slate bg-soft-meadow p-3 rounded-xl border border-onyx/5">
                    <div>{t('maintenance.current')} <strong className="text-deep-ink">v{otaStatus.current_version}</strong></div>
                    <div>{t('maintenance.latest')} <strong className="text-deep-ink">v{otaStatus.latest_version}</strong></div>
                    <div>{t('maintenance.updateAvailable')} <span className={otaStatus.update_available ? 'text-hi-yellow font-bold' : 'text-emerald-700'}>
                      {otaStatus.update_available ? t('maintenance.yes') : t('maintenance.no')}
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
                  {checkingOTA ? t('maintenance.checking') : t('maintenance.check')}
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
        title={t('restart.title')}
        description={t('restart.description')}
        confirmLabel={t('restart.confirm')}
        variant="warning"
      />
    </div>
  );
}
