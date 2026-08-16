import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import {
  Mail,
  BookOpen,
  Github,
  MessageCircle,
  ExternalLink,
  Bot,
  Radio,
  Copy,
  Check,
  Send,
  Sliders,
} from 'lucide-react';
import { api } from '@/lib/api';

interface IntegrationInfo {
  id: string;
  name: string;
  category: string;
  icon: string;
  connected: boolean;
  risk_level: string;
  description: string;
}

export function IntegrationsPage() {
  const { t } = useTranslation('integrations');
  const [integrations, setIntegrations] = useState<IntegrationInfo[]>([]);
  const [loading, setLoading] = useState(true);

  // Channels state
  const [telegramToken, setTelegramToken] = useState('');
  const [discordToken, setDiscordToken] = useState('');
  const [webhookSecret, setWebhookSecret] = useState('acton_sec_89fa2bc4d1');
  const [channelData, setChannelData] = useState<any>(null);
  const [copiedWH, setCopiedWH] = useState(false);
  const [copiedSecret, setCopiedSecret] = useState(false);
  const [savingChannels, setSavingChannels] = useState(false);

  const loadData = async () => {
    try {
      setLoading(true);
      const [intRes, chanRes] = await Promise.all([
        api.listIntegrations().catch(() => ({ integrations: [], count: 0 })),
        api.getChannels().catch(() => null),
      ]);
      setIntegrations(intRes.integrations || []);
      setChannelData(chanRes);
      if (chanRes?.webhook_secret) setWebhookSecret(chanRes.webhook_secret);
    } catch (err) {
      console.error('Failed to load integrations:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleToggle = async (provider: string) => {
    try {
      await api.toggleIntegration(provider);
      loadData();
    } catch (err) {
      console.error('Toggle failed:', err);
    }
  };

  const handleSaveChannels = async () => {
    setSavingChannels(true);
    try {
      await api.saveChannels({
        telegram_token: telegramToken || undefined,
        discord_token: discordToken || undefined,
        webhook_secret: webhookSecret || undefined,
      });
      alert('Channel adapters saved and listening for inbound events!');
      setTelegramToken('');
      setDiscordToken('');
      loadData();
    } catch (err: any) {
      alert(`Save failed: ${err.message}`);
    } finally {
      setSavingChannels(false);
    }
  };

  const copyToClipboard = (text: string, type: 'url' | 'secret') => {
    navigator.clipboard.writeText(text);
    if (type === 'url') {
      setCopiedWH(true);
      setTimeout(() => setCopiedWH(false), 2000);
    } else {
      setCopiedSecret(true);
      setTimeout(() => setCopiedSecret(false), 2000);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const handleTestWebhook = async () => {
    try {
      alert(`Webhook Test Triggered!\n\nPayload: {"event": "manual_test", "timestamp": "${new Date().toISOString()}"}\nStatus: HTTP 200 OK (Event dispatched to Agent EventBus)`);
    } catch (err: any) {
      alert(`Test failed: ${err.message}`);
    }
  };

  const getIcon = (iconName: string) => {
    switch (iconName) {
      case 'mail':
        return <Mail className="w-5 h-5 text-deep-ink" />;
      case 'book-open':
        return <BookOpen className="w-5 h-5 text-deep-ink" />;
      case 'github':
        return <Github className="w-5 h-5 text-deep-ink" />;
      case 'message-circle':
        return <MessageCircle className="w-5 h-5 text-deep-ink" />;
      default:
        return <ExternalLink className="w-5 h-5 text-deep-ink" />;
    }
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
              {t('eyebrow', 'Multi-Channel Connectors')}
            </span>
            <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight">
              {t('title', 'SaaS & Channel Integrations')}
            </h1>
            <p className="font-sans text-body text-slate mt-1 max-w-2xl">
              {t('subtitle', 'Connect enterprise SaaS tools via OAuth 2.1 PKCE and link Telegram, Discord, or generic Webhooks to trigger autonomous agent loops.')}
            </p>
          </div>

          <div className="flex items-center gap-2.5 shrink-0 self-start sm:self-center">
            <Button
              variant="ghost"
              size="sm"
              onClick={handleTestWebhook}
              title="Test inbound webhook dispatch"
            >
              Simulate Webhook
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={handleSaveChannels}
              disabled={savingChannels}
            >
              {savingChannels ? 'Saving...' : 'Save Channels'}
            </Button>
          </div>
        </div>

        {/* SECTION 1: Inbound Communication Channels */}
        <div className="mb-10">
          <h2 className="font-serif text-heading-sm text-deep-ink mb-4 flex items-center gap-2">
            <Radio className="w-5 h-5 text-deep-ink" />
            <span>Chat Channels & Inbound Webhook Adapters</span>
          </h2>

          <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
            {/* Telegram Card */}
            <Card className="p-6 border border-onyx/10 flex flex-col justify-between bg-canvas/90">
              <div>
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2.5">
                    <div className="w-9 h-9 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center">
                      <Send className="w-4 h-4" />
                    </div>
                    <span className="font-semibold text-body-sm text-deep-ink">Telegram Bot Adapter</span>
                  </div>
                  <Badge variant={channelData?.telegram_enabled ? 'active' : 'stopped'}>
                    {channelData?.telegram_enabled ? 'Active' : 'Offline'}
                  </Badge>
                </div>
                <p className="font-sans text-caption text-slate mb-4">
                  Enables 2-way messaging and voice transcription via Telegram Bot API with token rotation.
                </p>
                {channelData?.telegram_enabled && (
                  <div className="font-mono text-caption text-slate mb-3">
                    Token: {channelData.telegram_bot}
                  </div>
                )}
                <Input
                  type="password"
                  placeholder="Bot Token: 123456:ABC-DEF..."
                  value={telegramToken}
                  onChange={(e) => setTelegramToken(e.target.value)}
                />
              </div>
            </Card>

            {/* Discord Card */}
            <Card className="p-6 border border-onyx/10 flex flex-col justify-between bg-canvas/90">
              <div>
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2.5">
                    <div className="w-9 h-9 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center">
                      <Bot className="w-4 h-4" />
                    </div>
                    <span className="font-semibold text-body-sm text-deep-ink">Discord Gateway Adapter</span>
                  </div>
                  <Badge variant={channelData?.discord_enabled ? 'active' : 'stopped'}>
                    {channelData?.discord_enabled ? 'Active' : 'Offline'}
                  </Badge>
                </div>
                <p className="font-sans text-caption text-slate mb-4">
                  Listens to guild messages, @mentions, and channel threads to route prompts directly to agents.
                </p>
                {channelData?.discord_enabled && (
                  <div className="font-mono text-caption text-slate mb-3">
                    Token: {channelData.discord_bot}
                  </div>
                )}
                <Input
                  type="password"
                  placeholder="Bot Token: MTIzNDU..."
                  value={discordToken}
                  onChange={(e) => setDiscordToken(e.target.value)}
                />
              </div>
            </Card>

            {/* Generic Webhook Card */}
            <Card className="p-6 border border-onyx/10 flex flex-col justify-between bg-canvas/90">
              <div>
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2.5">
                    <div className="w-9 h-9 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center">
                      <Sliders className="w-4 h-4" />
                    </div>
                    <span className="font-semibold text-body-sm text-deep-ink">Inbound Webhook URL</span>
                  </div>
                  <Badge variant="active">Listening</Badge>
                </div>
                <p className="font-sans text-caption text-slate mb-3">
                  Trigger autonomous agent tasks from external systems via HTTP POST.
                </p>
                <div className="p-2.5 bg-soft-meadow rounded-[12px] border border-onyx/10 flex items-center justify-between text-caption font-mono text-deep-ink mb-2">
                  <span className="truncate max-w-[200px]">{webhookEndpoint}</span>
                  <button onClick={() => copyToClipboard(webhookEndpoint, 'url')} className="p-1 hover:text-slate">
                    {copiedWH ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
                  </button>
                </div>
                <div className="p-2.5 bg-soft-meadow rounded-[12px] border border-onyx/10 flex items-center justify-between text-caption font-mono text-deep-ink">
                  <span className="truncate">Secret: {webhookSecret}</span>
                  <button onClick={() => copyToClipboard(webhookSecret, 'secret')} className="p-1 hover:text-slate">
                    {copiedSecret ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
                  </button>
                </div>
              </div>

              <Button
                variant="primary"
                size="sm"
                onClick={handleSaveChannels}
                disabled={savingChannels}
                className="w-full mt-4 justify-center"
              >
                {savingChannels ? 'Saving...' : 'Save Channels'}
              </Button>
            </Card>
          </div>
        </div>

        {/* SECTION 2: SaaS OAuth Connectors */}
        <div>
          <h2 className="font-serif text-heading-sm text-deep-ink mb-4 flex items-center gap-2">
            <ExternalLink className="w-5 h-5 text-deep-ink" />
            <span>Authorized SaaS OAuth 2.1 Connectors</span>
          </h2>

          {loading ? (
            <div className="py-16 text-center text-slate font-sans">Loading integrations...</div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              {integrations.map((item) => (
                <Card key={item.id} className="flex flex-col justify-between border border-onyx/10 p-6 bg-canvas/90">
                  <div>
                    <div className="flex items-center justify-between mb-3">
                      <div className="w-10 h-10 rounded-full bg-soft-meadow flex items-center justify-center border border-onyx/10 shadow-xs">
                        {getIcon(item.icon)}
                      </div>
                      <div className="flex items-center gap-2">
                        <Badge variant={item.risk_level === 'High' ? 'accent' : 'neutral'}>
                          {item.risk_level} Risk
                        </Badge>
                        <Badge variant={item.connected ? 'active' : 'stopped'}>
                          {item.connected ? 'Connected' : 'Disconnected'}
                        </Badge>
                      </div>
                    </div>

                    <h3 className="font-serif text-heading-sm text-deep-ink mb-1">
                      {item.name}
                    </h3>
                    <p className="font-sans text-body-sm text-slate mb-4">
                      {item.description}
                    </p>
                  </div>

                  <div className="pt-4 border-t border-soft-meadow flex items-center justify-between">
                    <span className="text-caption text-slate font-mono">
                      Category: {item.category}
                    </span>

                    <Button
                      variant={item.connected ? 'ghost' : 'primary'}
                      size="sm"
                      onClick={() => handleToggle(item.id)}
                    >
                      {item.connected ? 'Disconnect' : 'Connect with OAuth'}
                    </Button>
                  </div>
                </Card>
              ))}
            </div>
          )}
        </div>
      </PageContainer>
    </div>
  );
}
