import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import {
  Sliders,
  Copy,
  Check,
  Eye,
  EyeOff,
  CheckCircle2,
  Terminal,
} from 'lucide-react';

interface WebhookCardProps {
  webhookSecret: string;
  webhookEndpoint: string;
  onCopySuccess: (msg: string) => void;
}

export function WebhookCard({
  webhookSecret,
  webhookEndpoint,
  onCopySuccess,
}: WebhookCardProps) {
  const { t } = useTranslation('channels');
  const [copiedURL, setCopiedURL] = useState(false);
  const [copiedSecret, setCopiedSecret] = useState(false);
  const [showSecret, setShowSecret] = useState(false);
  const [showCurlSnippet, setShowCurlSnippet] = useState(false);

  const handleCopy = (text: string, type: 'url' | 'secret') => {
    navigator.clipboard.writeText(text);
    if (type === 'url') {
      setCopiedURL(true);
      setTimeout(() => setCopiedURL(false), 2000);
      onCopySuccess('Webhook endpoint URL copied to clipboard.');
    } else {
      setCopiedSecret(true);
      setTimeout(() => setCopiedSecret(false), 2000);
      onCopySuccess('Webhook secret key copied to clipboard.');
    }
  };

  const sampleCurl = `curl -X POST "${webhookEndpoint}" \\
  -H "Content-Type: application/json" \\
  -H "X-Acton-Secret: ${webhookSecret}" \\
  -d '{"sender_id":"sys_alert","channel":"webhook","text":"Hello ActonOS Agent!"}'`;

  return (
    <Card className="p-6 border border-onyx/10 bg-canvas/95">
      <div className="flex items-start justify-between gap-3 mb-4">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center shadow-xs shrink-0">
            <Sliders className="w-4 h-4" />
          </div>
          <div>
            <h3 className="font-semibold text-body-sm text-deep-ink">
              {t('webhook.name', 'Generic Inbound Webhook Gateway')}
            </h3>
            <span className="text-[11px] font-mono text-slate">
              {t('ui.webhookApi')}
            </span>
          </div>
        </div>

        <Badge variant="active" className="text-[10px]">
          <CheckCircle2 className="w-3 h-3 mr-1" />
          <span>{t('status.listening', 'Listening')}</span>
        </Badge>
      </div>

      <p className="font-sans text-body-sm text-slate mb-4 leading-relaxed">
        {t(
          'webhook.desc',
          'Receive structured HTTP POST payloads from custom backends, scripts, CI/CD pipelines, or external SaaS webhooks.'
        )}
      </p>

      <div className="space-y-2.5">
        {/* Webhook Endpoint */}
        <div>
          <label className="text-[11px] font-semibold text-deep-ink block mb-1">
            {t('ui.endpointUrl')}
          </label>
          <div className="p-2.5 bg-soft-meadow rounded-xl border border-onyx/10 flex items-center justify-between gap-2 text-caption font-mono text-deep-ink">
            <span className="truncate">{webhookEndpoint}</span>
            <button
              type="button"
              onClick={() => handleCopy(webhookEndpoint, 'url')}
              className="p-1.5 rounded-lg hover:bg-canvas text-slate hover:text-deep-ink transition-colors cursor-pointer shrink-0"
              title={t('ui.copyEndpoint')}
            >
              {copiedURL ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
            </button>
          </div>
        </div>

        {/* Webhook Secret */}
        <div>
          <label className="text-[11px] font-semibold text-deep-ink block mb-1">
            {t('ui.secretKey')}
          </label>
          <div className="p-2.5 bg-soft-meadow rounded-xl border border-onyx/10 flex items-center justify-between gap-2 text-caption font-mono text-deep-ink">
            <span className="truncate">
              {showSecret ? webhookSecret : '••••••••••••••••••••••••'}
            </span>
            <div className="flex items-center gap-1 shrink-0">
              <button
                type="button"
                onClick={() => setShowSecret(!showSecret)}
                className="p-1.5 rounded-lg hover:bg-canvas text-slate hover:text-deep-ink transition-colors cursor-pointer"
                title={showSecret ? 'Hide secret' : 'Show secret'}
              >
                {showSecret ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
              </button>
              <button
                type="button"
                onClick={() => handleCopy(webhookSecret, 'secret')}
                className="p-1.5 rounded-lg hover:bg-canvas text-slate hover:text-deep-ink transition-colors cursor-pointer"
                title={t('ui.copySecret')}
              >
                {copiedSecret ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* cURL Snippet toggle */}
      <div className="mt-4 pt-3 border-t border-onyx/10">
        <button
          type="button"
          onClick={() => setShowCurlSnippet(!showCurlSnippet)}
          className="text-[11px] font-semibold uppercase tracking-wider text-slate hover:text-deep-ink flex items-center gap-1.5 transition-colors cursor-pointer"
        >
          <Terminal className="w-3.5 h-3.5" />
          <span>{showCurlSnippet ? 'Hide cURL Example' : 'View cURL Payload Example'}</span>
        </button>

        {showCurlSnippet && (
          <div className="mt-2.5 p-3 rounded-xl bg-deep-ink text-white font-mono text-[11px] overflow-x-auto relative">
            <pre className="whitespace-pre">{sampleCurl}</pre>
          </div>
        )}
      </div>
    </Card>
  );
}
