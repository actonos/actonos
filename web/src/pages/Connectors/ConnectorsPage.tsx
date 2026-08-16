import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { useToast } from '@/components/ui/Toast';
import {
  Mail,
  BookOpen,
  Github,
  MessageCircle,
  ExternalLink,
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

export function ConnectorsPage() {
  const { t } = useTranslation('connectors');
  const { success, error } = useToast();
  const [integrations, setIntegrations] = useState<IntegrationInfo[]>([]);
  const [loading, setLoading] = useState(true);

  const loadData = async () => {
    try {
      setLoading(true);
      const res = await api.listIntegrations().catch(() => ({ integrations: [], count: 0 }));
      setIntegrations(res.integrations || []);
    } catch (err: any) {
      error('Failed to load connectors', err.message);
    } finally {
      setLoading(false);
    }
  };

  const handleToggle = async (provider: string) => {
    try {
      const res = await api.toggleIntegration(provider);
      success(
        res.connected ? t('status.connected', 'Connected') : t('status.disconnected', 'Disconnected'),
        `${provider} updated.`
      );
      loadData();
    } catch (err: any) {
      error('Toggle failed', err.message);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

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

  return (
    <div className="relative min-h-[calc(100vh-64px)]">
      <BlobBackdrop />

      <PageContainer>
        {/* Header */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
          <div className="flex-1">
            <span className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
              {t('eyebrow', 'Connectors')}
            </span>
            <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight">
              {t('title', 'Service Connectors')}
            </h1>
            <p className="font-sans text-body text-slate mt-1 max-w-2xl">
              {t('subtitle', 'Connect external services like Google, Notion, and GitHub.')}
            </p>
          </div>
        </div>

        {/* Connectors Grid */}
        {loading ? (
          <div className="py-16 text-center text-slate font-sans">Loading...</div>
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
                        {t(`risk.${item.risk_level.toLowerCase()}`, `${item.risk_level} Risk`)}
                      </Badge>
                      <Badge variant={item.connected ? 'active' : 'stopped'}>
                        {item.connected ? t('status.connected', 'Connected') : t('status.disconnected', 'Disconnected')}
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
                    {item.category}
                  </span>
                  <Button
                    variant={item.connected ? 'ghost' : 'primary'}
                    size="sm"
                    onClick={() => handleToggle(item.id)}
                  >
                    {item.connected ? t('actions.disconnect', 'Disconnect') : t('actions.connect', 'Connect')}
                  </Button>
                </div>
              </Card>
            ))}
          </div>
        )}
      </PageContainer>
    </div>
  );
}
