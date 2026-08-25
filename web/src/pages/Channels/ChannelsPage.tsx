import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { PageHeader } from '@/components/ui/PageHeader';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { EmptyState } from '@/components/ui/EmptyState';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { useToast } from '@/components/ui/Toast';
import { readHashParams, setHashParam } from '@/lib/url-state';
import { api, type ChannelAuthorizationItem, type PairingCodeItem, type PendingPairingSender } from '@/lib/api';
import type { ChannelAccount } from '@/lib/types';
import { livePairingRequired, useInstalledChannels } from '@/lib/installed-channels';
import { getErrorMessage } from '@/lib/errors';
import {
  Copy,
  Check,
  KeyRound,
  Radio,
  RefreshCw,
  ShieldCheck,
  Trash2,
} from 'lucide-react';
import type { NavTab } from '@/components/layout/Sidebar';

function remainingLabel(expiresAt: string, nowMs: number, expiredText: string): string {
  const ms = new Date(expiresAt).getTime() - nowMs;
  if (Number.isNaN(ms) || ms <= 0) return expiredText;
  const total = Math.floor(ms / 1000);
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, '0')}`;
}

export function ChannelsPage({ onNavigateTab }: { onNavigateTab?: (tab: NavTab) => void }) {
  const { t } = useTranslation('channels');
  const { success, error } = useToast();
  const [view, setView] = useState<'accounts' | 'pairing'>(() =>
    readHashParams().get('view') === 'pairing' ? 'pairing' : 'accounts'
  );
  const [accounts, setAccounts] = useState<ChannelAccount[]>([]);
  const [users, setUsers] = useState<ChannelAuthorizationItem[]>([]);
  const [codes, setCodes] = useState<PairingCodeItem[]>([]);
  const [pending, setPending] = useState<PendingPairingSender[]>([]);
  const [policies, setPolicies] = useState<Record<string, boolean>>({});
  const [loading, setLoading] = useState(true);
  const [channelId, setChannelId] = useState('');
  const { channels: pluginChannels, accounts: pluginAccounts, loading: pluginsLoading } = useInstalledChannels();
  const [copied, setCopied] = useState<string | null>(null);
  const [now, setNow] = useState(Date.now());
  const [revokeTarget, setRevokeTarget] = useState<ChannelAuthorizationItem | null>(null);

  const selectView = (next: 'accounts' | 'pairing') => {
    setView(next);
    setHashParam('view', next === 'accounts' ? undefined : next);
  };

  const loadData = useCallback(async () => {
    try {
      const [accs, auths, codeRes, pendingRes, policyRes] = await Promise.all([
        api.listAllChannelAccounts().catch(() => ({ accounts: [], count: 0 })),
        api.listAuthorizations().catch(() => ({ users: [], count: 0 })),
        api.listPairingCodes().catch(() => ({ codes: [], count: 0 })),
        api.listPendingPairing().catch(() => ({ pending: [], count: 0 })),
        api.getPairingPolicies().catch(() => ({ policies: {} })),
      ]);
      setAccounts(accs.accounts || []);
      setUsers(auths.users || []);
      setCodes(codeRes.codes || []);
      setPending(pendingRes.pending || []);
      setPolicies(policyRes.policies || {});
    } catch (err) {
      error(t('errors.loadFailed'), getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  }, [error, t]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  useEffect(() => {
    if (view !== 'pairing') return;
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [view]);

  const pluginIds = useMemo(
    () => new Set(pluginChannels.map((channel) => channel.id)),
    [pluginChannels]
  );
  const channelOptions = useMemo(
    () => pluginChannels.map((channel) => channel.id).sort(),
    [pluginChannels]
  );
  const visibleAccounts = useMemo(() => {
    const byID = new Map<string, ChannelAccount>();
    const overlay = (account: ChannelAccount) => {
      const channel = (account.channel || '').toLowerCase();
      if (!pluginIds.has(channel)) return;
      const pluginDefault = pluginChannels.find((item) => item.id === channel)?.requiresPairing ?? false;
      const prev = byID.get(account.id);
      byID.set(account.id, {
        ...prev,
        ...account,
        channel,
        requires_pairing: livePairingRequired(channel, policies, pluginDefault),
      });
    };
    pluginAccounts.forEach(overlay);
    accounts.forEach(overlay);
    return [...byID.values()];
  }, [pluginAccounts, accounts, pluginIds, policies, pluginChannels]);
  const visibleCodes = useMemo(
    () => codes.filter((item) => pluginIds.has(item.channel_id.toLowerCase())),
    [codes, pluginIds]
  );
  const visiblePending = useMemo(
    () => pending.filter((item) => pluginIds.has(item.channel_id.toLowerCase())),
    [pending, pluginIds]
  );
  const visibleUsers = useMemo(
    () => users.filter((item) => pluginIds.has(item.channel_id.toLowerCase())),
    [users, pluginIds]
  );

  useEffect(() => {
    if (channelOptions.length === 0) {
      if (channelId) setChannelId('');
      return;
    }
    if (!channelId || !channelOptions.includes(channelId)) {
      setChannelId(channelOptions[0]);
    }
  }, [channelOptions, channelId]);

  const channelLabel = (id: string) =>
    pluginChannels.find((channel) => channel.id === id)?.label || id;

  const pairingOn = livePairingRequired(
    channelId,
    policies,
    pluginChannels.find((item) => item.id === channelId)?.requiresPairing ?? false,
  );

  const handleCreateCode = async (forChannel = channelId) => {
    const id = (forChannel || '').toLowerCase();
    if (!id || !pluginIds.has(id)) return;
    try {
      const res = await api.generatePairingCode(id);
      success(t('actions.createCode'), t('toasts.codeCreated'));
      setChannelId(id);
      setCopied(res.code);
      await loadData();
      window.setTimeout(() => setCopied(null), 2000);
    } catch (err) {
      error(t('errors.codeFailed'), getErrorMessage(err));
    }
  };

  const handleCopy = async (code: string) => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(code);
      window.setTimeout(() => setCopied(null), 2000);
    } catch {
      error(t('errors.copyFailed'), t('errors.copyFailed'));
    }
  };

  const handleAllow = async (sender: PendingPairingSender) => {
    try {
      await api.allowPairingSender({
        channel_id: sender.channel_id,
        sender_id: sender.sender_id,
        sender_name: sender.sender_name,
      });
      success(t('actions.allow'), t('toasts.allowed', { name: sender.sender_name || sender.sender_id }));
      await loadData();
    } catch (err) {
      error(t('errors.allowFailed'), getErrorMessage(err));
    }
  };

  const handleRevoke = async () => {
    if (!revokeTarget) return;
    try {
      await api.revokeAuthorization({
        channel_id: revokeTarget.channel_id,
        sender_id: revokeTarget.sender_id,
      });
      success(t('actions.revoke'), t('toasts.revoked'));
      setRevokeTarget(null);
      await loadData();
    } catch (err) {
      error(t('errors.revokeFailed'), getErrorMessage(err));
    }
  };

  const handlePolicy = async (required: boolean) => {
    if (!channelId) return;
    setPolicies((prev) => ({ ...prev, [channelId]: required }));
    try {
      await api.setPairingPolicy(channelId, required);
      success(
        t('pairing.title'),
        required ? t('toasts.policyOn', { channel: channelLabel(channelId) }) : t('toasts.policyOff', { channel: channelLabel(channelId) })
      );
      await loadData();
    } catch (err) {
      await loadData();
      error(t('errors.policyFailed'), getErrorMessage(err));
    }
  };

  const handleAccountPairing = async (account: ChannelAccount, required: boolean) => {
    const channel = (account.channel || '').toLowerCase();
    if (!channel || !pluginIds.has(channel)) return;
    setPolicies((prev) => ({ ...prev, [channel]: required }));
    try {
      await api.setPairingPolicy(channel, required);
      await loadData();
    } catch (err) {
      await loadData();
      error(t('errors.policyFailed'), getErrorMessage(err));
    }
  };

  return (
    <PageContainer>
      <PageHeader
        eyebrow={t('eyebrow')}
        title={t('title')}
        description={t('subtitle')}
        actions={
          <Button variant="ghost" size="sm" icon={<RefreshCw className="h-4 w-4" />} onClick={() => void loadData()}>
            {t('actions.refresh')}
          </Button>
        }
      />

      <div className="mb-6 flex items-center gap-2">
        <button
          type="button"
          onClick={() => selectView('accounts')}
          className={`rounded-full px-4 py-2 text-caption font-semibold ${
            view === 'accounts' ? 'bg-deep-ink text-white' : 'bg-soft-meadow text-deep-ink border border-onyx/10'
          }`}
        >
          {t('views.accounts')}
        </button>
        <button
          type="button"
          onClick={() => selectView('pairing')}
          className={`rounded-full px-4 py-2 text-caption font-semibold ${
            view === 'pairing' ? 'bg-deep-ink text-white' : 'bg-soft-meadow text-deep-ink border border-onyx/10'
          }`}
        >
          {t('views.pairing')}
        </button>
      </div>

      {loading || pluginsLoading ? (
        <p className="text-body-sm text-slate">{t('actions.refresh')}…</p>
      ) : view === 'accounts' ? (
        visibleAccounts.length === 0 ? (
          <EmptyState
            icon={<Radio className="h-8 w-8" />}
            title={t('accounts.emptyTitle')}
            description={t('accounts.emptyDescription')}
            action={
              onNavigateTab ? (
                <Button variant="primary" onClick={() => onNavigateTab('plugins')}>
                  {t('accounts.openPlugins')}
                </Button>
              ) : undefined
            }
          />
        ) : (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            {visibleAccounts.map((account) => (
              <Card key={account.id} className="p-5">
                <div className="mb-3 flex items-start justify-between gap-3">
                  <div>
                    <h3 className="font-serif text-heading-sm text-deep-ink">{account.name || account.id}</h3>
                    <p className="font-mono text-caption text-slate">{channelLabel(account.channel || '')}</p>
                  </div>
                  <Badge variant={account.enabled ? 'active' : 'neutral'}>
                    {account.enabled ? t('accounts.enabled') : t('accounts.disabled')}
                  </Badge>
                </div>
                <p className="mb-4 text-caption text-slate">
                  {account.bound_agent_ids?.includes('*') || !account.bound_agent_ids?.length
                    ? t('accounts.boundAll')
                    : account.bound_agent_ids.join(', ')}
                </p>
                <Button
                  type="button"
                  variant={account.requires_pairing ? 'primary' : 'ghost'}
                  size="sm"
                  onClick={() => void handleAccountPairing(account, !account.requires_pairing)}
                >
                  {account.requires_pairing ? t('accounts.pairingOn') : t('accounts.pairingOff')}
                </Button>
              </Card>
            ))}
          </div>
        )
      ) : pluginChannels.length === 0 ? (
        <EmptyState
          icon={<Radio className="h-8 w-8" />}
          title={t('accounts.noPluginsTitle')}
          description={t('accounts.noPluginsDescription')}
          action={
            onNavigateTab ? (
              <Button variant="primary" onClick={() => onNavigateTab('plugins')}>
                {t('accounts.openPlugins')}
              </Button>
            ) : undefined
          }
        />
      ) : (
        <div className="space-y-8">
          <Card className="p-5">
            <h2 className="mb-1 font-serif text-heading-sm text-deep-ink">{t('pairing.title')}</h2>
            <p className="mb-4 text-body-sm text-slate">{t('pairing.description')}</p>
            <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
              <label className="block min-w-48 flex-1">
                <span className="mb-1.5 block text-caption font-semibold text-deep-ink">{t('pairing.channelLabel')}</span>
                <select
                  value={channelId}
                  onChange={(e) => setChannelId(e.target.value)}
                  className="w-full rounded-full border border-onyx/15 bg-canvas px-4 py-2.5 text-body-sm text-deep-ink"
                >
                  {channelOptions.map((id) => (
                    <option key={id} value={id}>
                      {channelLabel(id)}
                    </option>
                  ))}
                </select>
              </label>
              <Button
                variant="primary"
                icon={<KeyRound className="h-4 w-4" />}
                disabled={!channelId}
                onClick={() => void handleCreateCode()}
              >
                {t('actions.createCode')}
              </Button>
            </div>
            <label className="mt-4 flex items-center gap-2 text-body-sm text-deep-ink">
              <input
                type="checkbox"
                checked={pairingOn}
                onChange={(e) => void handlePolicy(e.target.checked)}
                className="h-4 w-4 rounded text-deep-ink"
              />
              {t('pairing.requireLabel')}
            </label>
            <p className="mt-3 text-caption text-slate">{t('pairing.howTo')}</p>
          </Card>

          <section>
            <h3 className="mb-3 font-serif text-heading-sm text-deep-ink">{t('pairing.activeCodes')}</h3>
            {visibleCodes.length === 0 ? (
              <EmptyState compact icon={<KeyRound className="h-6 w-6" />} title={t('pairing.noCodes')} description={t('pairing.noCodesHint')} />
            ) : (
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                {visibleCodes.map((item) => (
                  <Card key={`${item.channel_id}-${item.code}`} className="p-5">
                    <p className="text-caption font-semibold text-slate">{t('pairing.createdFor', { channel: channelLabel(item.channel_id) })}</p>
                    <p className="my-2 font-mono text-4xl font-bold tracking-[0.35em] text-deep-ink">{item.code}</p>
                    <div className="flex items-center justify-between">
                      <span className="text-caption text-slate">
                        {t('pairing.expiresIn', { time: remainingLabel(item.expires_at, now, t('pairing.expired')) })}
                      </span>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        icon={copied === item.code ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
                        onClick={() => void handleCopy(item.code)}
                      >
                        {copied === item.code ? t('actions.copied') : t('actions.copy')}
                      </Button>
                    </div>
                  </Card>
                ))}
              </div>
            )}
          </section>

          <section>
            <h3 className="mb-3 font-serif text-heading-sm text-deep-ink">{t('pairing.pendingTitle')}</h3>
            {visiblePending.length === 0 ? (
              <EmptyState compact icon={<Radio className="h-6 w-6" />} title={t('pairing.pendingEmpty')} description={t('pairing.pendingEmptyHint')} />
            ) : (
              <div className="space-y-3">
                {visiblePending.map((sender) => (
                  <Card key={`${sender.channel_id}:${sender.sender_id}`} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
                    <div>
                      <p className="font-semibold text-deep-ink">{sender.sender_name || sender.sender_id}</p>
                      <p className="text-caption text-slate">
                        {channelLabel(sender.channel_id)} · {sender.sender_id}
                      </p>
                      {sender.last_content && <p className="mt-1 line-clamp-2 text-caption text-slate">{sender.last_content}</p>}
                    </div>
                    <div className="flex gap-2">
                      <Button variant="ghost" size="sm" onClick={() => void handleCreateCode(sender.channel_id)}>
                        {t('actions.createCode')}
                      </Button>
                      <Button variant="primary" size="sm" icon={<ShieldCheck className="h-3.5 w-3.5" />} onClick={() => void handleAllow(sender)}>
                        {t('actions.allow')}
                      </Button>
                    </div>
                  </Card>
                ))}
              </div>
            )}
          </section>

          <section>
            <h3 className="mb-3 font-serif text-heading-sm text-deep-ink">{t('pairing.usersTitle')}</h3>
            {visibleUsers.length === 0 ? (
              <EmptyState compact icon={<ShieldCheck className="h-6 w-6" />} title={t('pairing.usersEmpty')} />
            ) : (
              <div className="space-y-3">
                {visibleUsers.map((user) => (
                  <Card key={`${user.channel_id}:${user.sender_id}`} className="flex items-center justify-between gap-3 p-4">
                    <div>
                      <p className="font-semibold text-deep-ink">{user.sender_name || user.sender_id}</p>
                      <p className="text-caption text-slate">
                        {channelLabel(user.channel_id)} · {user.sender_id}
                      </p>
                    </div>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      icon={<Trash2 className="h-3.5 w-3.5" />}
                      onClick={() => setRevokeTarget(user)}
                    >
                      {t('actions.revoke')}
                    </Button>
                  </Card>
                ))}
              </div>
            )}
          </section>
        </div>
      )}

      <ConfirmModal
        isOpen={Boolean(revokeTarget)}
        onClose={() => setRevokeTarget(null)}
        onConfirm={handleRevoke}
        variant="danger"
        title={t('confirm.revokeTitle', { name: revokeTarget?.sender_name || revokeTarget?.sender_id || '' })}
        description={t('confirm.revokeDescription')}
        confirmLabel={t('actions.revoke')}
      />
    </PageContainer>
  );
}
