import { useState, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Button } from '@/components/ui/Button';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { useToast } from '@/components/ui/Toast';
import {
  RefreshCw,
  Search,
  Radio,
  Users,
  Sliders,
  Send,
  Save,
} from 'lucide-react';
import { api, type ChannelAuthorizationItem } from '@/lib/api';
import type { ChannelAccount, ChannelDefinition } from '@/lib/types';
import { ChannelCard } from './components/ChannelCard';
import { ChannelAccountModal } from './components/ChannelAccountModal';
import { PairingSection } from './components/PairingSection';
import { WebhookCard } from './components/WebhookCard';

// Catalog of available & upcoming chat channels
const CHANNEL_DEFINITIONS: ChannelDefinition[] = [
  {
    id: 'telegram',
    nameKey: 'telegram.name',
    descKey: 'telegram.desc',
    category: 'messaging',
    capabilities: ['Direct Messages', 'Group Chats', 'Voice Notes', 'Slash Commands'],
    docsUrl: 'https://core.telegram.org/bots',
  },
  {
    id: 'whatsapp',
    nameKey: 'whatsapp.name',
    descKey: 'whatsapp.desc',
    category: 'messaging',
    capabilities: ['Direct Messages', 'Media Attachments', 'Template Messaging'],
    hasPhoneId: true,
    docsUrl: 'https://developers.facebook.com/docs/whatsapp/cloud-api',
  },
  {
    id: 'discord',
    nameKey: 'discord.name',
    descKey: 'discord.desc',
    category: 'community',
    capabilities: ['Server Guilds', 'Channel Mentions', 'Thread Replies', 'Slash Commands'],
    docsUrl: 'https://discord.com/developers/docs/intro',
  },
  {
    id: 'slack',
    nameKey: 'slack.name',
    descKey: 'slack.desc',
    category: 'enterprise',
    capabilities: ['Workspaces', 'Channel Mentions', 'Direct Messages'],
    docsUrl: 'https://api.slack.com/bot-users',
    isComingSoon: true,
  },
];

export function ChannelsPage() {
  const { t } = useTranslation('channels');
  const { success, error, info } = useToast();

  // Channel accounts state (multi-account per channel)
  const [channelAccounts, setChannelAccounts] = useState<Record<string, ChannelAccount[]>>({
    telegram: [],
    discord: [],
    whatsapp: [],
    slack: [],
  });
  const [webhookSecret, setWebhookSecret] = useState('acton_sec_89fa2bc4d1');
  const [saving, setSaving] = useState(false);
  const [loading, setLoading] = useState(true);

  // Filter & Search state
  const [activeTab, setActiveTab] = useState<'all' | 'active' | 'pairing' | 'webhook'>('all');
  const [searchQuery, setSearchQuery] = useState('');

  // Account Management Modal state
  const [selectedChannelId, setSelectedChannelId] = useState<string | null>(null);
  const [modalMode, setModalMode] = useState<'manage' | 'add'>('manage');

  // Pairing state
  const [pairingCode, setPairingCode] = useState<string | null>(null);
  const [pairingChannel, setPairingChannel] = useState<'telegram' | 'whatsapp'>('telegram');
  const [authorizations, setAuthorizations] = useState<ChannelAuthorizationItem[]>([]);
  const [generatingCode, setGeneratingCode] = useState(false);
  const [revokingUser, setRevokingUser] = useState<{
    channel_id: string;
    sender_id: string;
    sender_name?: string;
  } | null>(null);

  const loadData = async () => {
    try {
      setLoading(true);
      const [chanRes, authRes] = await Promise.all([
        api.getChannels().catch(() => null),
        api.listAuthorizations().catch(() => ({ users: [], count: 0 })),
      ]);
      if (chanRes) {
        setChannelAccounts({
          telegram: chanRes.telegram || [],
          discord: chanRes.discord || [],
          whatsapp: chanRes.whatsapp || [],
          slack: (chanRes as any).slack || [],
        });
        if (chanRes.webhook_secret) setWebhookSecret(chanRes.webhook_secret);
      }
      setAuthorizations(authRes.users || []);
    } catch (err: any) {
      error('Failed to load channels', err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const handleAddAccount = async (channelId: string, accountData: Omit<ChannelAccount, 'id'>) => {
    const account: ChannelAccount = {
      ...accountData,
      id: `${channelId}_${Date.now()}`,
    };

    const updatedAccounts = {
      ...channelAccounts,
      [channelId]: [...(channelAccounts[channelId] || []), account],
    };
    setChannelAccounts(updatedAccounts);

    setSaving(true);
    try {
      await api.saveChannels({
        telegram_accounts: updatedAccounts.telegram,
        discord_accounts: updatedAccounts.discord,
        whatsapp_accounts: updatedAccounts.whatsapp,
        webhook_secret: webhookSecret,
      });
      success(t('addAccount', 'Account Added'), `${account.label} added and active on ${channelId}.`);
      await loadData();
    } catch (err: any) {
      error('Failed to save channel account', err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleRemoveAccount = async (channelId: string, accountId: string) => {
    const updatedAccounts = {
      ...channelAccounts,
      [channelId]: (channelAccounts[channelId] || []).filter((a) => a.id !== accountId),
    };
    setChannelAccounts(updatedAccounts);

    setSaving(true);
    try {
      await api.saveChannels({
        telegram_accounts: updatedAccounts.telegram,
        discord_accounts: updatedAccounts.discord,
        whatsapp_accounts: updatedAccounts.whatsapp,
        webhook_secret: webhookSecret,
      });
      success('Account Removed', `Account deleted from ${channelId}.`);
      await loadData();
    } catch (err: any) {
      error('Failed to remove channel account', err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleToggleAccount = async (channelId: string, accountId: string) => {
    const updatedAccounts = {
      ...channelAccounts,
      [channelId]: (channelAccounts[channelId] || []).map((a) =>
        a.id === accountId ? { ...a, enabled: !a.enabled } : a
      ),
    };
    setChannelAccounts(updatedAccounts);

    setSaving(true);
    try {
      await api.saveChannels({
        telegram_accounts: updatedAccounts.telegram,
        discord_accounts: updatedAccounts.discord,
        whatsapp_accounts: updatedAccounts.whatsapp,
        webhook_secret: webhookSecret,
      });
      await loadData();
    } catch (err: any) {
      error('Failed to toggle channel account', err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleSaveAll = async () => {
    setSaving(true);
    try {
      await api.saveChannels({
        telegram_accounts: channelAccounts.telegram,
        discord_accounts: channelAccounts.discord,
        whatsapp_accounts: channelAccounts.whatsapp,
        webhook_secret: webhookSecret,
      });
      success(t('saveAll', 'Saved'), 'Channel configuration stored in encrypted vault.');
      await loadData();
    } catch (err: any) {
      error('Failed to save channels', err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleGeneratePairingCode = async (ch: 'telegram' | 'whatsapp') => {
    setGeneratingCode(true);
    setPairingChannel(ch);
    try {
      const res = await api.generatePairingCode(ch);
      setPairingCode(res.code);
      info(t('pairing.activeCode', 'Active PIN'), `PIN: ${res.code} (${Math.round(res.expires_in / 60)}m)`);
    } catch (err: any) {
      error('Failed to generate PIN', err.message);
    } finally {
      setGeneratingCode(false);
    }
  };

  const handleConfirmRevoke = async () => {
    if (!revokingUser) return;
    try {
      await api.revokeAuthorization({
        channel_id: revokingUser.channel_id,
        sender_id: revokingUser.sender_id,
      });
      success(t('pairing.revoke', 'Revoked'), `User ${revokingUser.sender_name || revokingUser.sender_id} removed.`);
      setRevokingUser(null);
      loadData();
    } catch (err: any) {
      error('Failed to revoke authorization', err.message);
    }
  };

  // Metrics calculation
  const totalAccounts = Object.values(channelAccounts).reduce(
    (sum, accs) => sum + accs.length,
    0
  );
  const activeChannelsCount = Object.values(channelAccounts).filter((accs) =>
    accs.some((a) => a.enabled)
  ).length;

  const filteredChannels = useMemo(() => {
    return CHANNEL_DEFINITIONS.filter((ch) => {
      const accs = channelAccounts[ch.id] || [];
      const hasActive = accs.some((a) => a.enabled);

      if (activeTab === 'active' && !hasActive) return false;

      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase();
        const matchesName = ch.id.toLowerCase().includes(q) || t(ch.nameKey).toLowerCase().includes(q);
        const matchesDesc = t(ch.descKey).toLowerCase().includes(q);
        const matchesCap = ch.capabilities.some((c) => c.toLowerCase().includes(q));
        if (!matchesName && !matchesDesc && !matchesCap) return false;
      }

      return true;
    });
  }, [channelAccounts, activeTab, searchQuery, t]);

  const selectedChannelDef = CHANNEL_DEFINITIONS.find((c) => c.id === selectedChannelId) || null;
  const webhookEndpoint = `${window.location.origin}/api/webhooks/inbound`;

  return (
    <div className="relative min-h-[calc(100vh-64px)]">
      <BlobBackdrop />

      <PageContainer>
        {/* Top Header */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
          <div className="flex-1">
            <span className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
              {t('eyebrow', 'Channels & Gateways')}
            </span>
            <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight flex items-center gap-3">
              <span>{t('title', 'Chat Channels')}</span>
            </h1>
            <p className="font-sans text-body text-slate mt-1 max-w-2xl">
              {t(
                'subtitle',
                'Connect Telegram, Discord, WhatsApp, and Webhooks to receive and reply messages automatically.'
              )}
            </p>
          </div>

          <div className="flex items-center gap-2.5 shrink-0 self-start sm:self-center">
            <Button
              variant="ghost"
              size="sm"
              icon={<RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />}
              onClick={loadData}
            >
              {t('actions.refresh', 'Refresh')}
            </Button>
            <Button
              variant="primary"
              size="sm"
              icon={<Save className="w-3.5 h-3.5" />}
              onClick={handleSaveAll}
              disabled={saving}
            >
              {saving ? '...' : t('saveAll', 'Save Changes')}
            </Button>
          </div>
        </div>

        {/* Quick Stats Strip */}
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-8">
          <div className="p-4 rounded-2xl bg-canvas/90 border border-onyx/10 flex items-center gap-3 shadow-xs">
            <div className="w-10 h-10 rounded-full bg-soft-meadow border border-onyx/10 flex items-center justify-center text-deep-ink">
              <Radio className="w-4 h-4 text-emerald-600" />
            </div>
            <div>
              <span className="text-caption text-slate block">{t('stats.activeChannels', 'Active Channels')}</span>
              <span className="text-heading-sm font-serif font-bold text-deep-ink">
                {activeChannelsCount} / {CHANNEL_DEFINITIONS.length}
              </span>
            </div>
          </div>

          <div className="p-4 rounded-2xl bg-canvas/90 border border-onyx/10 flex items-center gap-3 shadow-xs">
            <div className="w-10 h-10 rounded-full bg-soft-meadow border border-onyx/10 flex items-center justify-center text-deep-ink">
              <Send className="w-4 h-4 text-deep-ink" />
            </div>
            <div>
              <span className="text-caption text-slate block">{t('stats.connectedBots', 'Connected Bots')}</span>
              <span className="text-heading-sm font-serif font-bold text-deep-ink">
                {totalAccounts}
              </span>
            </div>
          </div>

          <div className="p-4 rounded-2xl bg-canvas/90 border border-onyx/10 flex items-center gap-3 shadow-xs">
            <div className="w-10 h-10 rounded-full bg-soft-meadow border border-onyx/10 flex items-center justify-center text-deep-ink">
              <Users className="w-4 h-4 text-emerald-600" />
            </div>
            <div>
              <span className="text-caption text-slate block">{t('stats.pairedUsers', 'Paired Users')}</span>
              <span className="text-heading-sm font-serif font-bold text-deep-ink">
                {authorizations.length}
              </span>
            </div>
          </div>

          <div className="p-4 rounded-2xl bg-canvas/90 border border-onyx/10 flex items-center gap-3 shadow-xs">
            <div className="w-10 h-10 rounded-full bg-soft-meadow border border-onyx/10 flex items-center justify-center text-deep-ink">
              <Sliders className="w-4 h-4 text-deep-ink" />
            </div>
            <div>
              <span className="text-caption text-slate block">{t('stats.webhookGateway', 'Webhook Gateway')}</span>
              <div className="flex items-center gap-1.5 mt-0.5">
                <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
                <span className="text-body-sm font-semibold text-deep-ink">Active</span>
              </div>
            </div>
          </div>
        </div>

        {/* View Tabs & Filter Bar */}
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
          <div className="flex items-center gap-1.5 bg-soft-meadow p-1 rounded-full border border-onyx/10 shrink-0">
            <button
              type="button"
              onClick={() => setActiveTab('all')}
              className={`px-4 py-1.5 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer ${
                activeTab === 'all'
                  ? 'bg-deep-ink text-white shadow-xs'
                  : 'text-deep-ink hover:text-slate'
              }`}
            >
              {t('tabs.all', 'All Channels')} ({CHANNEL_DEFINITIONS.length})
            </button>
            <button
              type="button"
              onClick={() => setActiveTab('active')}
              className={`px-4 py-1.5 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer ${
                activeTab === 'active'
                  ? 'bg-deep-ink text-white shadow-xs'
                  : 'text-deep-ink hover:text-slate'
              }`}
            >
              {t('tabs.active', 'Active')} ({activeChannelsCount})
            </button>
            <button
              type="button"
              onClick={() => setActiveTab('pairing')}
              className={`px-4 py-1.5 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer ${
                activeTab === 'pairing'
                  ? 'bg-deep-ink text-white shadow-xs'
                  : 'text-deep-ink hover:text-slate'
              }`}
            >
              {t('tabs.pairing', 'Pairing & Whitelist')} ({authorizations.length})
            </button>
            <button
              type="button"
              onClick={() => setActiveTab('webhook')}
              className={`px-4 py-1.5 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer ${
                activeTab === 'webhook'
                  ? 'bg-deep-ink text-white shadow-xs'
                  : 'text-deep-ink hover:text-slate'
              }`}
            >
              {t('tabs.webhook', 'Webhook Gateway')}
            </button>
          </div>

          {activeTab !== 'pairing' && activeTab !== 'webhook' && (
            <div className="relative w-full md:w-72">
              <Search className="w-3.5 h-3.5 text-slate absolute left-3 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                placeholder={t('actions.searchChannels', 'Search channels or capabilities...')}
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-full pl-9 pr-3 py-1.5 text-caption bg-canvas rounded-full border border-onyx/10 focus:outline-none focus:border-onyx/30 shadow-xs"
              />
            </div>
          )}
        </div>

        {/* TAB CONTENT: Channels Grid */}
        {(activeTab === 'all' || activeTab === 'active') && (
          <div className="space-y-6 mb-12">
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-2 gap-6">
              {filteredChannels.map((ch) => (
                <ChannelCard
                  key={ch.id}
                  channel={ch}
                  accounts={channelAccounts[ch.id] || []}
                  onOpenManage={(id) => {
                    setSelectedChannelId(id);
                    setModalMode('manage');
                  }}
                  onQuickToggle={handleToggleAccount}
                  onOpenAdd={(id) => {
                    setSelectedChannelId(id);
                    setModalMode('add');
                  }}
                />
              ))}
            </div>

            {/* Inbound Webhook Card Banner */}
            <div className="mt-8">
              <WebhookCard
                webhookSecret={webhookSecret}
                webhookEndpoint={webhookEndpoint}
                onCopySuccess={(msg) => success('Copied', msg)}
              />
            </div>
          </div>
        )}

        {/* TAB CONTENT: Pairing & Whitelist Dedicated View */}
        {activeTab === 'pairing' && (
          <div className="mb-12">
            <PairingSection
              pairingCode={pairingCode}
              pairingChannel={pairingChannel}
              authorizations={authorizations}
              generatingCode={generatingCode}
              onGenerateCode={handleGeneratePairingCode}
              onRevokeUser={(item) => setRevokingUser(item)}
              onRefresh={loadData}
            />
          </div>
        )}

        {/* TAB CONTENT: Webhook Dedicated View */}
        {activeTab === 'webhook' && (
          <div className="max-w-3xl mb-12">
            <WebhookCard
              webhookSecret={webhookSecret}
              webhookEndpoint={webhookEndpoint}
              onCopySuccess={(msg) => success('Copied', msg)}
            />
          </div>
        )}

        {/* Channel Account Modal */}
        <ChannelAccountModal
          isOpen={!!selectedChannelId}
          onClose={() => setSelectedChannelId(null)}
          channel={selectedChannelDef}
          accounts={selectedChannelId ? channelAccounts[selectedChannelId] || [] : []}
          onAddAccount={handleAddAccount}
          onToggleAccount={handleToggleAccount}
          onRemoveAccount={handleRemoveAccount}
          initialMode={modalMode}
        />

        {/* Revoke Confirmation Modal */}
        <ConfirmModal
          isOpen={!!revokingUser}
          onClose={() => setRevokingUser(null)}
          onConfirm={handleConfirmRevoke}
          title={t('pairing.revoke', 'Revoke Access')}
          description={`Revoke chat authorization for user ${
            revokingUser?.sender_name || revokingUser?.sender_id
          } on ${revokingUser?.channel_id}?`}
          confirmLabel={t('pairing.revoke', 'Revoke Access')}
          variant="danger"
        />
      </PageContainer>
    </div>
  );
}
