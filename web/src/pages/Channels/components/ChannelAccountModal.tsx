import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Badge } from '@/components/ui/Badge';
import {
  Trash2,
  Check,
  Plus,
  Key,
  Phone,
  Info,
  ExternalLink,
  Sparkles,
} from 'lucide-react';
import type { ChannelAccount, ChannelDefinition } from '@/lib/types';

interface ChannelAccountModalProps {
  isOpen: boolean;
  onClose: () => void;
  channel: ChannelDefinition | null;
  accounts: ChannelAccount[];
  onAddAccount: (channelId: string, account: Omit<ChannelAccount, 'id'>) => void;
  onToggleAccount: (channelId: string, accountId: string) => void;
  onRemoveAccount: (channelId: string, accountId: string) => void;
  initialMode?: 'manage' | 'add';
}

const CHANNEL_SETUP_GUIDES: Record<string, { title: string; hint: string; url: string }> = {
  telegram: {
    title: 'Telegram Bot Token',
    hint: 'Open @BotFather on Telegram, create a bot via /newbot and paste the API token here.',
    url: 'https://t.me/BotFather',
  },
  whatsapp: {
    title: 'WhatsApp Cloud API Access',
    hint: 'Obtain your Permanent System User Token and Phone Number ID from Meta for Developers.',
    url: 'https://developers.facebook.com/docs/whatsapp/cloud-api',
  },
  discord: {
    title: 'Discord Bot Token',
    hint: 'Create an Application in Discord Developer Portal, add a Bot and copy the Bot Token.',
    url: 'https://discord.com/developers/applications',
  },
  slack: {
    title: 'Slack Bot User Token',
    hint: 'Create a Slack App with Bot Token Scopes (chat:write, channels:read) and install to workspace.',
    url: 'https://api.slack.com/apps',
  },
};

export function ChannelAccountModal({
  isOpen,
  onClose,
  channel,
  accounts,
  onAddAccount,
  onToggleAccount,
  onRemoveAccount,
  initialMode = 'manage',
}: ChannelAccountModalProps) {
  const { t } = useTranslation('channels');
  const [mode, setMode] = useState<'manage' | 'add'>(initialMode);
  const [label, setLabel] = useState('');
  const [token, setToken] = useState('');
  const [phoneId, setPhoneId] = useState('');
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null);

  useEffect(() => {
    if (isOpen) {
      setMode(initialMode);
      setLabel('');
      setToken('');
      setPhoneId('');
      setDeleteConfirmId(null);
    }
  }, [isOpen, initialMode]);

  if (!channel) return null;

  const guide = CHANNEL_SETUP_GUIDES[channel.id];

  const handleSaveNew = (e: React.FormEvent) => {
    e.preventDefault();
    if (!token.trim()) return;

    onAddAccount(channel.id, {
      label: label.trim() || `${channel.nameKey || channel.id} Bot`,
      token: token.trim(),
      phone_id: phoneId.trim() || undefined,
      enabled: true,
    });

    setLabel('');
    setToken('');
    setPhoneId('');
    setMode('manage');
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={`${t(channel.nameKey, channel.id)} — ${
        mode === 'add' ? t('addAccount', 'Add Account') : t('accounts.title', 'Manage Accounts')
      }`}
    >
      <div className="space-y-5">
        {/* Sub-navigation tabs: Manage vs Add */}
        <div className="flex items-center gap-1.5 bg-soft-meadow p-1 rounded-full border border-onyx/10">
          <button
            type="button"
            onClick={() => setMode('manage')}
            className={`flex-1 py-1.5 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer ${
              mode === 'manage'
                ? 'bg-deep-ink text-white shadow-xs'
                : 'text-deep-ink hover:text-slate'
            }`}
          >
            {t('accounts.title', 'Accounts')} ({accounts.length})
          </button>
          <button
            type="button"
            onClick={() => setMode('add')}
            className={`flex-1 py-1.5 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer flex items-center justify-center gap-1 ${
              mode === 'add'
                ? 'bg-deep-ink text-white shadow-xs'
                : 'text-deep-ink hover:text-slate'
            }`}
          >
            <Plus className="w-3.5 h-3.5" />
            <span>{t('addAccount', 'Add New Account')}</span>
          </button>
        </div>

        {/* MODE 1: Manage Existing Accounts */}
        {mode === 'manage' && (
          <div className="space-y-3">
            {accounts.length === 0 ? (
              <div className="py-8 text-center bg-soft-meadow/50 rounded-2xl border border-onyx/5">
                <Sparkles className="w-8 h-8 text-slate mx-auto mb-2 opacity-50" />
                <p className="text-body-sm text-deep-ink font-semibold mb-1">
                  {t('accounts.empty', 'No accounts added yet')}
                </p>
                <p className="text-caption text-slate mb-4 max-w-xs mx-auto">
                  {t('accounts.emptyHint', 'Connect your first bot token to start receiving and processing messages.')}
                </p>
                <Button
                  variant="primary"
                  size="sm"
                  icon={<Plus className="w-3.5 h-3.5" />}
                  onClick={() => setMode('add')}
                >
                  {t('addAccount', 'Add Account')}
                </Button>
              </div>
            ) : (
              <div className="space-y-2 max-h-72 overflow-y-auto pr-1">
                {accounts.map((acc) => (
                  <div
                    key={acc.id}
                    className={`p-3.5 rounded-2xl border transition-all flex items-center justify-between gap-3 ${
                      acc.enabled
                        ? 'bg-canvas border-emerald-500/30 shadow-xs'
                        : 'bg-canvas/50 border-onyx/10 opacity-70'
                    }`}
                  >
                    <div className="flex items-center gap-3 min-w-0">
                      <button
                        type="button"
                        onClick={() => onToggleAccount(channel.id, acc.id)}
                        title={acc.enabled ? 'Active (Click to disable)' : 'Inactive (Click to enable)'}
                        className={`w-6 h-6 rounded-full border flex items-center justify-center shrink-0 transition-colors cursor-pointer ${
                          acc.enabled
                            ? 'bg-emerald-500 border-emerald-500 text-white'
                            : 'bg-canvas border-onyx/20 hover:border-onyx/40'
                        }`}
                      >
                        {acc.enabled && <Check className="w-3.5 h-3.5" />}
                      </button>

                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold text-body-sm text-deep-ink truncate">
                            {acc.label}
                          </span>
                          <Badge variant={acc.enabled ? 'active' : 'stopped'} className="text-[10px]">
                            {acc.enabled ? t('status.active', 'Active') : t('status.offline', 'Disabled')}
                          </Badge>
                        </div>
                        <div className="text-caption font-mono text-slate truncate flex items-center gap-2 mt-0.5">
                          <span>Token: {acc.token ? '••••••••' + acc.token.slice(-4) : '••••••'}</span>
                          {acc.phone_id && <span>• Phone ID: {acc.phone_id}</span>}
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-1 shrink-0">
                      {deleteConfirmId === acc.id ? (
                        <div className="flex items-center gap-1 bg-red-50 p-1 rounded-xl border border-red-200">
                          <span className="text-[10px] text-red-700 font-semibold px-1">Delete?</span>
                          <button
                            type="button"
                            onClick={() => {
                              onRemoveAccount(channel.id, acc.id);
                              setDeleteConfirmId(null);
                            }}
                            className="px-2 py-1 text-[11px] bg-red-600 text-white rounded-lg font-medium hover:bg-red-700 cursor-pointer"
                          >
                            Yes
                          </button>
                          <button
                            type="button"
                            onClick={() => setDeleteConfirmId(null)}
                            className="px-2 py-1 text-[11px] bg-slate-200 text-slate-700 rounded-lg hover:bg-slate-300 cursor-pointer"
                          >
                            No
                          </button>
                        </div>
                      ) : (
                        <button
                          type="button"
                          onClick={() => setDeleteConfirmId(acc.id)}
                          className="p-2 rounded-xl text-slate hover:text-red-600 hover:bg-red-50 transition-colors cursor-pointer"
                          title={t('accounts.remove', 'Remove account')}
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {/* MODE 2: Add New Account Form */}
        {mode === 'add' && (
          <form onSubmit={handleSaveNew} className="space-y-4">
            {guide && (
              <div className="p-3.5 rounded-2xl bg-soft-meadow border border-onyx/10 flex items-start gap-2.5 text-caption text-slate">
                <Info className="w-4 h-4 text-deep-ink shrink-0 mt-0.5" />
                <div>
                  <span className="font-semibold text-deep-ink block">{guide.title}</span>
                  <p className="mt-0.5 leading-relaxed">{guide.hint}</p>
                  {guide.url && (
                    <a
                      href={guide.url}
                      target="_blank"
                      rel="noreferrer"
                      className="text-deep-ink font-semibold underline hover:text-slate inline-flex items-center gap-1 mt-1.5"
                    >
                      <span>Open Setup Portal</span>
                      <ExternalLink className="w-3 h-3" />
                    </a>
                  )}
                </div>
              </div>
            )}

            <div>
              <label className="text-caption font-semibold text-deep-ink block mb-1">
                {t('accounts.label', 'Account Label')}
              </label>
              <Input
                placeholder="e.g. Primary Support Bot, Community Monitor"
                value={label}
                onChange={(e) => setLabel(e.target.value)}
              />
            </div>

            <div>
              <label className="text-caption font-semibold text-deep-ink block mb-1">
                {t('accounts.token', 'Bot Token / Secret')} <span className="text-red-500">*</span>
              </label>
              <Input
                type="password"
                placeholder="Paste token or API key..."
                value={token}
                onChange={(e) => setToken(e.target.value)}
                required
              />
            </div>

            {channel.hasPhoneId && (
              <div>
                <label className="text-caption font-semibold text-deep-ink block mb-1 flex items-center gap-1">
                  <Phone className="w-3.5 h-3.5 text-slate" />
                  <span>{t('accounts.phoneId', 'Phone Number ID')}</span>
                </label>
                <Input
                  placeholder="e.g. 104928374928192"
                  value={phoneId}
                  onChange={(e) => setPhoneId(e.target.value)}
                />
              </div>
            )}

            <div className="pt-2 flex items-center gap-2">
              <Button
                type="submit"
                variant="primary"
                size="md"
                disabled={!token.trim()}
                icon={<Key className="w-4 h-4" />}
                className="flex-1 justify-center"
              >
                {t('addAccount', 'Save Account')}
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="md"
                onClick={() => setMode('manage')}
              >
                Cancel
              </Button>
            </div>
          </form>
        )}
      </div>
    </Modal>
  );
}
