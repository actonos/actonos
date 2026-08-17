import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import {
  Send,
  Phone,
  Bot,
  MessageSquare,
  Sparkles,
  Sliders,
  Check,
  Plus,
  ExternalLink,
  ShieldCheck,
} from 'lucide-react';
import type { ChannelAccount, ChannelDefinition } from '@/lib/types';

interface ChannelCardProps {
  channel: ChannelDefinition;
  accounts: ChannelAccount[];
  onOpenManage: (channelId: string) => void;
  onQuickToggle: (channelId: string, accountId: string) => void;
  onOpenAdd: (channelId: string) => void;
}

export function ChannelCard({
  channel,
  accounts,
  onOpenManage,
  onQuickToggle,
  onOpenAdd,
}: ChannelCardProps) {
  const { t } = useTranslation('channels');
  const activeAccounts = accounts.filter((a) => a.enabled);
  const isConfigured = accounts.length > 0;

  const getChannelIcon = (id: string) => {
    switch (id) {
      case 'telegram':
        return <Send className="w-4 h-4" />;
      case 'whatsapp':
        return <Phone className="w-4 h-4" />;
      case 'discord':
        return <Bot className="w-4 h-4" />;
      case 'slack':
        return <MessageSquare className="w-4 h-4" />;
      case 'webhook':
        return <Sliders className="w-4 h-4" />;
      default:
        return <Sparkles className="w-4 h-4" />;
    }
  };

  return (
    <Card
      className={`flex flex-col justify-between p-6 border transition-all duration-200 ${
        channel.isComingSoon
          ? 'border-onyx/5 bg-canvas/40 opacity-75'
          : activeAccounts.length > 0
          ? 'border-emerald-500/30 bg-canvas/95 shadow-xs'
          : 'border-onyx/10 bg-canvas/85 hover:border-onyx/20'
      }`}
    >
      <div>
        {/* Top bar: Icon, Channel Name, Badges */}
        <div className="flex items-start justify-between gap-3 mb-3">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center shadow-xs shrink-0">
              {getChannelIcon(channel.id)}
            </div>
            <div>
              <h3 className="font-semibold text-body-sm text-deep-ink flex items-center gap-2">
                <span>{t(channel.nameKey, channel.id)}</span>
              </h3>
              <span className="text-[11px] font-mono text-slate capitalize">
                {channel.category}
              </span>
            </div>
          </div>

          <div className="flex items-center gap-1.5 shrink-0">
            {channel.isComingSoon ? (
              <Badge variant="neutral" className="text-[10px]">
                {t('status.comingSoon', 'Coming Soon')}
              </Badge>
            ) : activeAccounts.length > 0 ? (
              <Badge variant="active" className="text-[10px]">
                {activeAccounts.length} {t('status.active', 'Active')}
              </Badge>
            ) : isConfigured ? (
              <Badge variant="stopped" className="text-[10px]">
                {t('status.paused', 'Paused')}
              </Badge>
            ) : (
              <Badge variant="neutral" className="text-[10px]">
                {t('status.notConfigured', 'Not Configured')}
              </Badge>
            )}
          </div>
        </div>

        {/* Description */}
        <p className="font-sans text-body-sm text-slate mb-4 line-clamp-2 leading-relaxed">
          {t(channel.descKey)}
        </p>

        {/* Capabilities Pill List */}
        {channel.capabilities && channel.capabilities.length > 0 && (
          <div className="flex flex-wrap gap-1.5 mb-4">
            {channel.capabilities.map((cap) => (
              <span
                key={cap}
                className="text-[10px] font-mono bg-soft-meadow text-deep-ink/80 px-2.5 py-0.5 rounded-full border border-onyx/5"
              >
                {cap}
              </span>
            ))}
          </div>
        )}

        {/* Connected Accounts Snapshot */}
        {accounts.length > 0 && (
          <div className="space-y-1.5 mb-4">
            <div className="flex items-center justify-between text-[11px] text-slate font-medium px-1">
              <span>{t('accounts.title', 'Accounts')} ({accounts.length})</span>
              <span className="text-emerald-700 font-mono text-[10px]">
                {activeAccounts.length} enabled
              </span>
            </div>

            <div className="space-y-1.5 max-h-36 overflow-y-auto pr-1">
              {accounts.map((acc) => (
                <div
                  key={acc.id}
                  className={`p-2 rounded-xl border text-caption flex items-center justify-between transition-colors ${
                    acc.enabled
                      ? 'bg-soft-meadow/70 border-onyx/10 text-deep-ink'
                      : 'bg-canvas/50 border-onyx/5 text-slate opacity-60'
                  }`}
                >
                  <div className="flex items-center gap-2 min-w-0">
                    <button
                      type="button"
                      onClick={() => onQuickToggle(channel.id, acc.id)}
                      title={acc.enabled ? 'Disable' : 'Enable'}
                      className={`w-4 h-4 rounded-full border flex items-center justify-center shrink-0 transition-colors cursor-pointer ${
                        acc.enabled
                          ? 'bg-emerald-500 border-emerald-500 text-white'
                          : 'bg-canvas border-onyx/20 hover:border-onyx/40'
                      }`}
                    >
                      {acc.enabled && <Check className="w-2.5 h-2.5" />}
                    </button>
                    <span className="font-medium truncate max-w-[140px]">
                      {acc.label}
                    </span>
                  </div>

                  <span className="font-mono text-[10px] text-slate truncate max-w-[100px]">
                    {acc.phone_id ? `ID: ${acc.phone_id}` : acc.token ? '••••••' : 'No token'}
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* Footer / Actions */}
      <div className="pt-4 border-t border-onyx/10 flex items-center justify-between gap-2 mt-2">
        {channel.docsUrl ? (
          <a
            href={channel.docsUrl}
            target="_blank"
            rel="noreferrer"
            className="text-[11px] font-sans text-slate hover:text-deep-ink inline-flex items-center gap-1 transition-colors"
          >
            <span>{t('actions.guide', 'Setup Guide')}</span>
            <ExternalLink className="w-3 h-3" />
          </a>
        ) : (
          <span className="text-[11px] text-slate flex items-center gap-1 font-mono">
            <ShieldCheck className="w-3 h-3 text-emerald-600" />
            <span>Encrypted Vault</span>
          </span>
        )}

        <div className="flex items-center gap-1.5">
          {channel.isComingSoon ? (
            <Button variant="ghost" size="sm" disabled className="text-[11px]">
              Coming Soon
            </Button>
          ) : (
            <>
              {isConfigured && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => onOpenManage(channel.id)}
                  className="text-[11px]"
                >
                  {t('actions.manage', 'Manage')}
                </Button>
              )}
              <Button
                variant={isConfigured ? 'ghost' : 'primary'}
                size="sm"
                icon={<Plus className="w-3 h-3" />}
                onClick={() => onOpenAdd(channel.id)}
                className="text-[11px]"
              >
                {t('addAccount', 'Add Account')}
              </Button>
            </>
          )}
        </div>
      </div>
    </Card>
  );
}
