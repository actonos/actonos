import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Mail, BookOpen, Github, MessageCircle, ExternalLink } from 'lucide-react';

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

  const loadIntegrations = async () => {
    try {
      setLoading(true);
      const res = await fetch('/api/integrations');
      const data = await res.json();
      setIntegrations(data.data?.integrations || []);
    } catch (err) {
      console.error('Failed to load integrations:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleConnect = async (provider: string) => {
    try {
      const res = await fetch(`/api/integrations/${provider}/auth-url`, { method: 'POST' });
      const data = await res.json();
      if (data.data?.auth_url) {
        window.open(data.data.auth_url, '_blank', 'width=600,height=700');
      }
    } catch (err) {
      console.error('Failed to start OAuth flow:', err);
    }
  };

  useEffect(() => {
    loadIntegrations();
  }, []);

  const getIcon = (iconName: string) => {
    switch (iconName) {
      case 'mail':
        return <Mail className="w-6 h-6 text-deep-ink" />;
      case 'book-open':
        return <BookOpen className="w-6 h-6 text-deep-ink" />;
      case 'github':
        return <Github className="w-6 h-6 text-deep-ink" />;
      case 'message-circle':
        return <MessageCircle className="w-6 h-6 text-deep-ink" />;
      default:
        return <ExternalLink className="w-6 h-6 text-deep-ink" />;
    }
  };

  return (
    <div className="relative min-h-[calc(100vh-72px)]">
      <BlobBackdrop />

      <PageContainer>
        {/* Header */}
        <div className="mb-8">
          <span className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
            {t('eyebrow')}
          </span>
          <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight">
            {t('title')}
          </h1>
          <p className="font-sans text-body text-slate mt-2 max-w-2xl">
            {t('subtitle')}
          </p>
        </div>

        {loading ? (
          <div className="py-20 text-center text-slate font-sans">Loading integrations...</div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            {integrations.map((item) => (
              <Card key={item.id} className="flex flex-col justify-between border border-onyx/10">
                <div>
                  <div className="flex items-center justify-between mb-4">
                    <div className="w-12 h-12 rounded-full bg-canvas flex items-center justify-center border border-onyx shadow-xs">
                      {getIcon(item.icon)}
                    </div>
                    <div className="flex items-center gap-2">
                      <Badge variant={item.risk_level === 'High' ? 'accent' : 'neutral'}>
                        {item.risk_level}
                      </Badge>
                      <Badge variant={item.connected ? 'active' : 'stopped'}>
                        {item.connected ? 'Connected' : 'Available'}
                      </Badge>
                    </div>
                  </div>

                  <h3 className="font-serif text-heading-sm text-deep-ink mb-1">
                    {item.name}
                  </h3>
                  <span className="text-caption uppercase text-slate font-medium block mb-3">
                    {item.category}
                  </span>

                  <p className="font-sans text-body-sm text-slate mb-6">
                    {item.description}
                  </p>
                </div>

                <div className="pt-4 border-t border-canvas flex items-center justify-between">
                  <span className="text-caption font-mono text-slate">OAuth 2.1 PKCE (S256)</span>
                  <Button
                    variant={item.connected ? 'ghost' : 'primary'}
                    size="sm"
                    icon={<ExternalLink className="w-3.5 h-3.5" />}
                    onClick={() => handleConnect(item.id)}
                  >
                    {item.connected ? t('actions.connected') : t('actions.connect')}
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
