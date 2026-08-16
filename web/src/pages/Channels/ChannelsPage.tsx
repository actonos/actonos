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
  Send,
  Phone,
  Bot,
  Sliders,
  ShieldCheck,
  Key,
  Trash2,
  RefreshCw,
  Copy,
  Check,
  Plus,
  X,
} from 'lucide-react';
import { api, type ChannelAuthorizationItem } from '@/lib/api';
import type { ChannelAccount } from '@/lib/types';

interface ChannelType {
  id: string;
  nameKey: string;
  descKey: string;
  icon: React.ElementType;
  hasPhoneId?: boolean;
}

const CHANNEL_TYPES: ChannelType[] = [
  { id: 'telegram', nameKey: 'telegram.name', descKey: 'telegram.desc', icon: Send },
  { id: 'whatsapp', nameKey: 'whatsapp.name', descKey: 'whatsapp.desc', icon: Phone, hasPhoneId: true },
  { id: 'discord', nameKey: 'discord.name', descKey: 'discord.desc', icon: Bot },
];

export function ChannelsPage() {
  const { t } = useTranslation('channels');
  const { success, error, info } = useToast();

  // Channel accounts state (multi-account per channel)
  const [channelAccounts, setChannelAccounts] = useState<Record<string, ChannelAccount[]>>({
    telegram: [],
    discord: [],
    whatsapp: [],
  });
  const [webhookSecret, setWebhookSecret] = useState('acton_sec_89fa2bc4d1');
  const [copiedWH, setCopiedWH] = useState(false);
  const [copiedSecret, setCopiedSecret] = useState(false);
  const [saving, setSaving] = useState(false);
  const [loading, setLoading] = useState(true);

  // New account form
  const [addingChannel, setAddingChannel] = useState<string | null>(null);
  const [newLabel, setNewLabel] = useState('');
  const [newToken, setNewToken] = useState('');
  const [newPhoneId, setNewPhoneId] = useState('');

  // Pairing state
  const [pairingCode, setPairingCode] = useState<string | null>(null);
  const [pairingChannel, setPairingChannel] = useState<'telegram' | 'whatsapp'>('telegram');
  const [authorizations, setAuthorizations] = useState<ChannelAuthorizationItem[]>([]);
  const [generatingCode, setGeneratingCode] = useState(false);
  const [revokingUser, setRevokingUser] = useState<{ channel_id: string; sender_id: string } | null>(null);

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

  const handleAddAccount = (channelId: string) => {
    if (!newToken.trim()) return;

    const account: ChannelAccount = {
      id: `${channelId}_${Date.now()}`,
      label: newLabel.trim() || 'New Account',
      token: newToken.trim(),
      phone_id: newPhoneId.trim() || undefined,
      enabled: true,
    };

    setChannelAccounts((prev) => ({
      ...prev,
      [channelId]: [...(prev[channelId] || []), account],
    }));

    setAddingChannel(null);
    setNewLabel('');
    setNewToken('');
    setNewPhoneId('');
  };

  const handleRemoveAccount = (channelId: string, accountId: string) => {
    setChannelAccounts((prev) => ({
      ...prev,
      [channelId]: (prev[channelId] || []).filter((a) => a.id !== accountId),
    }));
  };

  const handleToggleAccount = (channelId: string, accountId: string) => {
    setChannelAccounts((prev) => ({
      ...prev,
      [channelId]: (prev[channelId] || []).map((a) =>
        a.id === accountId ? { ...a, enabled: !a.enabled } : a
      ),
    }));
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
      success(t('saveAll', 'Saved'), 'Channel configuration updated.');
      loadData();
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
      await api.revokeAuthorization(revokingUser);
      success(t('pairing.revoke', 'Revoked'), `User ${revokingUser.sender_id} removed.`);
      setRevokingUser(null);
      loadData();
    } catch (err: any) {
      error('Failed to revoke', err.message);
    }
  };

  const copyToClipboard = (text: string, type: 'url' | 'secret' | 'pin') => {
    navigator.clipboard.writeText(text);
    if (type === 'url') {
      setCopiedWH(true);
      setTimeout(() => setCopiedWH(false), 2000);
    } else if (type === 'secret') {
      setCopiedSecret(true);
      setTimeout(() => setCopiedSecret(false), 2000);
    }
    success('Copied', 'Copied to clipboard.');
  };

  const webhookEndpoint = `${window.location.origin}/api/webhooks/inbound`;

  return (
    <div className="relative min-h-[calc(100vh-64px)]">
      <BlobBackdrop />

      <PageContainer>
        {/* Header */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
          <div className="flex-1">
            <span className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
              {t('eyebrow', 'Channels')}
            </span>
            <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight">
              {t('title', 'Chat Channels')}
            </h1>
            <p className="font-sans text-body text-slate mt-1 max-w-2xl">
              {t('subtitle', 'Connect Telegram, Discord, WhatsApp to receive and reply messages automatically.')}
            </p>
          </div>

          <div className="flex items-center gap-2.5 shrink-0 self-start sm:self-center">
            <Button variant="ghost" size="sm" icon={<RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />} onClick={loadData}>
              {t('actions.refresh', 'Refresh')}
            </Button>
            <Button variant="primary" size="sm" onClick={handleSaveAll} disabled={saving}>
              {saving ? '...' : t('saveAll', 'Save')}
            </Button>
          </div>
        </div>

        {/* Channel Cards Grid */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-10">
          {CHANNEL_TYPES.map((ch) => {
            const Icon = ch.icon;
            const accounts = channelAccounts[ch.id] || [];
            const activeCount = accounts.filter((a) => a.enabled).length;

            return (
              <Card key={ch.id} className="p-6 border border-onyx/10 bg-canvas/90">
                {/* Channel Header */}
                <div className="flex items-center justify-between mb-4">
                  <div className="flex items-center gap-2.5">
                    <div className="w-9 h-9 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center">
                      <Icon className="w-4 h-4" />
                    </div>
                    <div>
                      <span className="font-semibold text-body-sm text-deep-ink block">
                        {t(ch.nameKey)}
                      </span>
                      <span className="text-caption text-slate">{t(ch.descKey)}</span>
                    </div>
                  </div>
                  <Badge variant={activeCount > 0 ? 'active' : 'stopped'}>
                    {activeCount > 0 ? `${activeCount} ${t('status.active', 'Active')}` : t('status.offline', 'Offline')}
                  </Badge>
                </div>

                {/* Accounts List */}
                {accounts.length > 0 && (
                  <div className="space-y-2 mb-4">
                    {accounts.map((acc) => (
                      <div
                        key={acc.id}
                        className={`p-3 rounded-2xl border transition-all flex items-center justify-between ${
                          acc.enabled ? 'bg-soft-meadow border-onyx/10' : 'bg-canvas/50 border-onyx/5 opacity-60'
                        }`}
                      >
                        <div className="flex items-center gap-2.5 min-w-0">
                          <button
                            onClick={() => handleToggleAccount(ch.id, acc.id)}
                            className={`w-5 h-5 rounded-full border-2 flex items-center justify-center shrink-0 transition-colors cursor-pointer ${
                              acc.enabled ? 'bg-emerald-500 border-emerald-500 text-white' : 'bg-canvas border-onyx/20'
                            }`}
                          >
                            {acc.enabled && <Check className="w-3 h-3" />}
                          </button>
                          <div className="min-w-0">
                            <span className="text-body-sm font-medium text-deep-ink block truncate">
                              {acc.label}
                            </span>
                            <span className="text-caption font-mono text-slate truncate block">
                              {acc.token || '••••••'}
                              {acc.phone_id ? ` • ${acc.phone_id}` : ''}
                            </span>
                          </div>
                        </div>
                        <button
                          onClick={() => handleRemoveAccount(ch.id, acc.id)}
                          className="p-1.5 rounded-full hover:bg-red-50 text-slate hover:text-red-600 transition-colors shrink-0 cursor-pointer"
                        >
                          <X className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    ))}
                  </div>
                )}

                {/* Add Account Form */}
                {addingChannel === ch.id ? (
                  <div className="space-y-2 p-3 rounded-2xl bg-soft-meadow border border-onyx/10">
                    <Input
                      placeholder={t('accounts.label', 'Account Label')}
                      value={newLabel}
                      onChange={(e) => setNewLabel(e.target.value)}
                    />
                    <Input
                      type="password"
                      placeholder={t('accounts.token', 'Bot Token')}
                      value={newToken}
                      onChange={(e) => setNewToken(e.target.value)}
                    />
                    {ch.hasPhoneId && (
                      <Input
                        placeholder={t('accounts.phoneId', 'Phone Number ID')}
                        value={newPhoneId}
                        onChange={(e) => setNewPhoneId(e.target.value)}
                      />
                    )}
                    <div className="flex items-center gap-2">
                      <Button variant="primary" size="sm" onClick={() => handleAddAccount(ch.id)}>
                        {t('addAccount', 'Add')}
                      </Button>
                      <Button variant="ghost" size="sm" onClick={() => setAddingChannel(null)}>
                        Cancel
                      </Button>
                    </div>
                  </div>
                ) : (
                  <Button
                    variant="ghost"
                    size="sm"
                    icon={<Plus className="w-3.5 h-3.5" />}
                    onClick={() => {
                      setAddingChannel(ch.id);
                      setNewLabel('');
                      setNewToken('');
                      setNewPhoneId('');
                    }}
                    className="w-full justify-center"
                  >
                    {t('addAccount', 'Add Account')}
                  </Button>
                )}
              </Card>
            );
          })}

          {/* Generic Webhook Card */}
          <Card className="p-6 border border-onyx/10 bg-canvas/90">
            <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-2.5">
                <div className="w-9 h-9 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center">
                  <Sliders className="w-4 h-4" />
                </div>
                <div>
                  <span className="font-semibold text-body-sm text-deep-ink block">{t('webhook.name', 'Webhook')}</span>
                  <span className="text-caption text-slate">{t('webhook.desc')}</span>
                </div>
              </div>
              <Badge variant="active">{t('status.listening', 'Listening')}</Badge>
            </div>
            <div className="space-y-2">
              <div className="p-2 bg-soft-meadow rounded-[10px] border border-onyx/10 flex items-center justify-between text-caption font-mono text-deep-ink">
                <span className="truncate max-w-[240px]">{webhookEndpoint}</span>
                <button onClick={() => copyToClipboard(webhookEndpoint, 'url')} className="p-1 hover:text-slate cursor-pointer">
                  {copiedWH ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
                </button>
              </div>
              <div className="p-2 bg-soft-meadow rounded-[10px] border border-onyx/10 flex items-center justify-between text-caption font-mono text-deep-ink">
                <span className="truncate">Secret: {webhookSecret}</span>
                <button onClick={() => copyToClipboard(webhookSecret, 'secret')} className="p-1 hover:text-slate cursor-pointer">
                  {copiedSecret ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
                </button>
              </div>
            </div>
          </Card>
        </div>

        {/* Pairing & Whitelist Section */}
        <div className="mb-10">
          <div className="flex items-center justify-between mb-4">
            <div>
              <h2 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
                <ShieldCheck className="w-5 h-5 text-emerald-600" />
                <span>{t('pairing.title', 'User Pairing')}</span>
              </h2>
              <p className="font-sans text-caption text-slate mt-0.5">
                {t('pairing.subtitle', 'Generate a 6-digit PIN to authenticate new chat users.')}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Button variant="ghost" size="sm" icon={<RefreshCw className="w-3.5 h-3.5" />} onClick={loadData}>
                Refresh
              </Button>
              <Button
                variant="primary"
                size="sm"
                icon={<Key className="w-3.5 h-3.5" />}
                onClick={() => handleGeneratePairingCode('telegram')}
                disabled={generatingCode}
              >
                {t('pairing.generate', 'Generate PIN')}
              </Button>
            </div>
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
            {/* PIN Card */}
            {pairingCode ? (
              <Card className="p-6 border-2 border-emerald-500/40 bg-emerald-500/5 flex flex-col justify-between">
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-caption font-semibold uppercase tracking-wider text-emerald-700">
                      {t('pairing.activeCode', 'Active PIN')} ({pairingChannel})
                    </span>
                    <Badge variant="active">10m</Badge>
                  </div>
                  <div className="my-4 text-center">
                    <span className="font-mono text-heading-lg font-bold tracking-widest text-deep-ink bg-canvas px-6 py-2.5 rounded-xl border border-onyx/10 shadow-xs inline-block">
                      {pairingCode}
                    </span>
                  </div>
                  <p className="font-sans text-caption text-slate text-center">
                    {t('pairing.instructions')}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => copyToClipboard(pairingCode, 'pin')}
                  className="w-full mt-4 justify-center"
                >
                  Copy
                </Button>
              </Card>
            ) : (
              <Card className="p-6 border border-onyx/10 bg-canvas/60 flex flex-col items-center justify-center text-center">
                <Key className="w-8 h-8 text-slate mb-2" />
                <p className="font-sans text-caption text-slate mb-3">
                  {t('pairing.empty', 'No active PIN.')}
                </p>
                <Button variant="ghost" size="sm" onClick={() => handleGeneratePairingCode('telegram')}>
                  {t('pairing.generate', 'Generate PIN')}
                </Button>
              </Card>
            )}

            {/* Authorized Users List */}
            <Card className="lg:col-span-2 p-6 border border-onyx/10 bg-canvas/90">
              <h3 className="font-serif text-body text-deep-ink font-semibold mb-3">
                {t('pairing.whitelist', 'Authorized Users')} ({authorizations.length})
              </h3>
              {authorizations.length === 0 ? (
                <div className="py-8 text-center text-caption text-slate font-sans">
                  {t('pairing.empty', 'No users paired yet.')}
                </div>
              ) : (
                <div className="divide-y divide-onyx/5 max-h-48 overflow-y-auto">
                  {authorizations.map((u) => (
                    <div key={`${u.channel_id}:${u.sender_id}`} className="py-2.5 flex items-center justify-between text-body-sm">
                      <div className="flex items-center gap-3">
                        <Badge variant="active" className="uppercase text-caption font-mono">
                          {u.channel_id}
                        </Badge>
                        <div>
                          <div className="font-semibold text-deep-ink">{u.sender_name || u.sender_id}</div>
                          <div className="text-caption font-mono text-slate">
                            ID: {u.sender_id} • {new Date(u.paired_at).toLocaleDateString()}
                          </div>
                        </div>
                      </div>
                      <Button
                        variant="danger"
                        size="sm"
                        icon={<Trash2 className="w-3.5 h-3.5" />}
                        onClick={() => setRevokingUser({ channel_id: u.channel_id, sender_id: u.sender_id })}
                      >
                        {t('pairing.revoke', 'Revoke')}
                      </Button>
                    </div>
                  ))}
                </div>
              )}
            </Card>
          </div>
        </div>
      </PageContainer>

      {/* Revoke Confirmation Modal */}
      <ConfirmModal
        isOpen={!!revokingUser}
        onClose={() => setRevokingUser(null)}
        onConfirm={handleConfirmRevoke}
        title={t('pairing.revoke', 'Revoke Access')}
        description={`Remove user ${revokingUser?.sender_id} from ${revokingUser?.channel_id}?`}
        confirmLabel={t('pairing.revoke', 'Revoke')}
        variant="danger"
      />
    </div>
  );
}
