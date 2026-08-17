import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import {
  ShieldCheck,
  Key,
  Trash2,
  Copy,
  Check,
  Clock,
  UserCheck,
  Send,
  Phone,
  RefreshCw,
  Search,
} from 'lucide-react';
import type { ChannelAuthorizationItem } from '@/lib/api';

interface PairingSectionProps {
  pairingCode: string | null;
  pairingChannel: 'telegram' | 'whatsapp';
  authorizations: ChannelAuthorizationItem[];
  generatingCode: boolean;
  onGenerateCode: (channel: 'telegram' | 'whatsapp') => void;
  onRevokeUser: (item: { channel_id: string; sender_id: string; sender_name?: string }) => void;
  onRefresh: () => void;
}

export function PairingSection({
  pairingCode,
  pairingChannel,
  authorizations,
  generatingCode,
  onGenerateCode,
  onRevokeUser,
  onRefresh,
}: PairingSectionProps) {
  const { t } = useTranslation('channels');
  const [copiedPIN, setCopiedPIN] = useState(false);
  const [selectedChannel, setSelectedChannel] = useState<'telegram' | 'whatsapp'>(pairingChannel);
  const [filterQuery, setFilterQuery] = useState('');
  const [countdown, setCountdown] = useState<number>(600); // 10 minutes default

  useEffect(() => {
    if (pairingCode) {
      setCountdown(600);
      const timer = setInterval(() => {
        setCountdown((prev) => {
          if (prev <= 1) {
            clearInterval(timer);
            return 0;
          }
          return prev - 1;
        });
      }, 1000);
      return () => clearInterval(timer);
    }
  }, [pairingCode]);

  const handleCopyPIN = () => {
    if (!pairingCode) return;
    navigator.clipboard.writeText(pairingCode);
    setCopiedPIN(true);
    setTimeout(() => setCopiedPIN(false), 2000);
  };

  const formatCountdown = (seconds: number) => {
    const m = Math.floor(seconds / 60);
    const s = seconds % 60;
    return `${m}:${s < 10 ? '0' : ''}${s}`;
  };

  const filteredAuths = authorizations.filter((u) => {
    const q = filterQuery.toLowerCase();
    return (
      u.sender_id.toLowerCase().includes(q) ||
      (u.sender_name && u.sender_name.toLowerCase().includes(q)) ||
      u.channel_id.toLowerCase().includes(q)
    );
  });

  return (
    <div className="space-y-4">
      {/* Section Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div>
          <h2 className="font-serif text-heading-sm text-deep-ink flex items-center gap-2">
            <ShieldCheck className="w-5 h-5 text-emerald-600" />
            <span>{t('pairing.title', 'User Pairing & Whitelist')}</span>
          </h2>
          <p className="font-sans text-caption text-slate mt-0.5">
            {t('pairing.subtitle', 'Generate a secure 6-digit PIN to authenticate new chat users before they can interact.')}
          </p>
        </div>

        <div className="flex items-center gap-2">
          <Button variant="ghost" size="sm" icon={<RefreshCw className="w-3.5 h-3.5" />} onClick={onRefresh}>
            {t('actions.refresh', 'Refresh')}
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left Column: PIN Generator Card */}
        <Card className="p-6 border border-onyx/10 bg-canvas/95 flex flex-col justify-between">
          <div>
            <div className="flex items-center justify-between mb-4">
              <span className="text-caption font-semibold uppercase tracking-wider text-slate">
                {t('pairing.generateCard', 'Security PIN')}
              </span>
              <div className="flex items-center gap-1 bg-soft-meadow p-1 rounded-full border border-onyx/5">
                <button
                  type="button"
                  onClick={() => setSelectedChannel('telegram')}
                  className={`p-1.5 rounded-full transition-colors cursor-pointer ${
                    selectedChannel === 'telegram'
                      ? 'bg-deep-ink text-hi-yellow shadow-xs'
                      : 'text-slate hover:text-deep-ink'
                  }`}
                  title="Telegram"
                >
                  <Send className="w-3.5 h-3.5" />
                </button>
                <button
                  type="button"
                  onClick={() => setSelectedChannel('whatsapp')}
                  className={`p-1.5 rounded-full transition-colors cursor-pointer ${
                    selectedChannel === 'whatsapp'
                      ? 'bg-deep-ink text-hi-yellow shadow-xs'
                      : 'text-slate hover:text-deep-ink'
                  }`}
                  title="WhatsApp"
                >
                  <Phone className="w-3.5 h-3.5" />
                </button>
              </div>
            </div>

            {pairingCode && countdown > 0 ? (
              <div className="my-2 p-5 rounded-2xl bg-emerald-500/5 border-2 border-emerald-500/30 text-center">
                <div className="flex items-center justify-center gap-1.5 text-[11px] font-mono text-emerald-700 font-semibold mb-2">
                  <Clock className="w-3.5 h-3.5 animate-pulse" />
                  <span>Expires in {formatCountdown(countdown)}</span>
                </div>
                <div className="font-mono text-3xl font-bold tracking-widest text-deep-ink bg-canvas py-3 px-6 rounded-xl border border-onyx/10 shadow-xs inline-block">
                  {pairingCode}
                </div>
                <p className="font-sans text-[11px] text-slate mt-3 leading-relaxed">
                  Send <code className="font-mono font-semibold text-deep-ink">/pair {pairingCode}</code> to your {selectedChannel} bot.
                </p>
              </div>
            ) : (
              <div className="my-3 py-6 px-4 rounded-2xl bg-soft-meadow/60 border border-onyx/5 text-center">
                <Key className="w-8 h-8 text-slate mx-auto mb-2 opacity-60" />
                <p className="text-body-sm font-semibold text-deep-ink">
                  {pairingCode ? 'PIN Expired' : 'No Active Pairing PIN'}
                </p>
                <p className="text-caption text-slate mt-1 max-w-[200px] mx-auto">
                  Click generate below to create a single-use 10-minute PIN.
                </p>
              </div>
            )}
          </div>

          <div className="pt-4 border-t border-onyx/10 flex items-center gap-2">
            {pairingCode && countdown > 0 ? (
              <>
                <Button
                  variant="ghost"
                  size="sm"
                  icon={copiedPIN ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
                  onClick={handleCopyPIN}
                  className="flex-1 justify-center"
                >
                  {copiedPIN ? 'Copied' : 'Copy PIN'}
                </Button>
                <Button
                  variant="primary"
                  size="sm"
                  icon={<Key className="w-3.5 h-3.5" />}
                  onClick={() => onGenerateCode(selectedChannel)}
                  disabled={generatingCode}
                >
                  {generatingCode ? '...' : 'New'}
                </Button>
              </>
            ) : (
              <Button
                variant="primary"
                size="md"
                icon={<Key className="w-4 h-4" />}
                onClick={() => onGenerateCode(selectedChannel)}
                disabled={generatingCode}
                className="w-full justify-center"
              >
                {generatingCode ? 'Generating...' : `${t('pairing.generate', 'Generate PIN')} (${selectedChannel})`}
              </Button>
            )}
          </div>
        </Card>

        {/* Right Column: Authorized Users List (Whitelist) */}
        <Card className="lg:col-span-2 p-6 border border-onyx/10 bg-canvas/95 flex flex-col justify-between">
          <div>
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-4">
              <div className="flex items-center gap-2">
                <UserCheck className="w-4 h-4 text-emerald-600" />
                <h3 className="font-semibold text-body-sm text-deep-ink">
                  {t('pairing.whitelist', 'Authorized Users')}
                </h3>
                <Badge variant="active" className="text-[10px] font-mono">
                  {authorizations.length}
                </Badge>
              </div>

              {authorizations.length > 0 && (
                <div className="relative min-w-[200px]">
                  <Search className="w-3.5 h-3.5 text-slate absolute left-3 top-1/2 -translate-y-1/2" />
                  <input
                    type="text"
                    placeholder="Search paired users..."
                    value={filterQuery}
                    onChange={(e) => setFilterQuery(e.target.value)}
                    className="w-full pl-8 pr-3 py-1.5 text-caption bg-soft-meadow rounded-full border border-onyx/10 focus:outline-none focus:border-onyx/30"
                  />
                </div>
              )}
            </div>

            {filteredAuths.length === 0 ? (
              <div className="py-10 text-center bg-soft-meadow/30 rounded-2xl border border-onyx/5">
                <ShieldCheck className="w-8 h-8 text-slate mx-auto mb-2 opacity-40" />
                <p className="text-body-sm font-semibold text-deep-ink">
                  {authorizations.length === 0
                    ? t('pairing.empty', 'No users paired yet.')
                    : 'No matching users found.'}
                </p>
                <p className="text-caption text-slate mt-1 max-w-sm mx-auto">
                  {t('pairing.emptySubtitle', 'Once a user sends a valid PIN via chat bot, their identity is stored here in the encrypted enclave.')}
                </p>
              </div>
            ) : (
              <div className="divide-y divide-onyx/5 max-h-56 overflow-y-auto pr-1">
                {filteredAuths.map((u) => (
                  <div
                    key={`${u.channel_id}:${u.sender_id}`}
                    className="py-3 flex items-center justify-between gap-3 text-body-sm"
                  >
                    <div className="flex items-center gap-3 min-w-0">
                      <div className="w-8 h-8 rounded-full bg-soft-meadow border border-onyx/10 flex items-center justify-center text-deep-ink font-semibold text-caption shrink-0">
                        {(u.sender_name || u.sender_id).charAt(0).toUpperCase()}
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold text-deep-ink truncate">
                            {u.sender_name || u.sender_id}
                          </span>
                          <Badge variant="neutral" className="uppercase text-[9px] font-mono px-1.5 py-0">
                            {u.channel_id}
                          </Badge>
                        </div>
                        <div className="text-[11px] font-mono text-slate truncate flex items-center gap-2 mt-0.5">
                          <span>ID: {u.sender_id}</span>
                          <span>•</span>
                          <span>Paired: {new Date(u.paired_at).toLocaleDateString()}</span>
                        </div>
                      </div>
                    </div>

                    <Button
                      variant="ghost"
                      size="sm"
                      icon={<Trash2 className="w-3.5 h-3.5 text-red-500" />}
                      onClick={() =>
                        onRevokeUser({
                          channel_id: u.channel_id,
                          sender_id: u.sender_id,
                          sender_name: u.sender_name,
                        })
                      }
                      className="text-red-600 hover:bg-red-50 text-[11px] shrink-0"
                    >
                      {t('pairing.revoke', 'Revoke')}
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </Card>
      </div>
    </div>
  );
}
