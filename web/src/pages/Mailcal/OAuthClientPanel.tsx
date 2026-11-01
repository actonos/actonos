import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { mailcalApi } from '@/lib/api/mailcal';
import { needsHttpsForSignIn } from '@/lib/secureOrigin';

const KNOWN_HELP = ['google', 'microsoft'];

/** The user's own OAuth client (bring-your-own). The secret is write-only: it is never shown again. */
export function OAuthClientPanel({ provider }: { provider: string }) {
  const { t } = useTranslation('mailcal');
  const [configured, setConfigured] = useState<boolean | null>(null);
  const [replacing, setReplacing] = useState(false);
  const [clientId, setClientId] = useState('');
  const [clientSecret, setClientSecret] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    let alive = true;
    mailcalApi
      .getOAuthClient(provider)
      .then((r) => alive && setConfigured(r.configured))
      .catch(() => alive && setConfigured(false));
    return () => {
      alive = false;
    };
  }, [provider]);

  const save = async () => {
    setSaving(true);
    setError('');
    try {
      await mailcalApi.putOAuthClient(provider, clientId.trim(), clientSecret);
      setConfigured(true);
      setReplacing(false);
      setClientId('');
      setClientSecret('');
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  const help = KNOWN_HELP.includes(provider) ? provider : 'generic';
  const redirect = `${window.location.origin}/api/mailcal/oauth/callback`;
  const showForm = configured === false || replacing;
  const insecure = needsHttpsForSignIn(window.location.protocol, window.location.hostname);

  return (
    <fieldset className="flex flex-col gap-3 rounded-2xl border border-onyx/15 p-3">
      <legend className="px-2 text-caption font-medium uppercase tracking-wider text-slate">{t('oauth.client.title')}</legend>
      <p className="text-caption text-slate">{t(`oauth.help.${help}`)}</p>
      <p className="text-caption text-slate">
        {t('oauth.redirectHint')} <code className="break-all">{redirect}</code>
      </p>
      {insecure && (
        <p role="alert" className="text-body-sm text-status-danger">{t('oauth.httpsRequired')}</p>
      )}
      {configured && !replacing && (
        <div className="flex items-center gap-2">
          <Badge variant="success">{t('oauth.client.configured')}</Badge>
          <Button type="button" variant="ghost" size="sm" onClick={() => setReplacing(true)}>{t('oauth.client.replace')}</Button>
        </div>
      )}
      {showForm && (
        <>
          <Input id="oauth-client-id" label={t('oauth.client.id')} value={clientId} onChange={(e) => setClientId(e.target.value)} autoComplete="off" />
          <Input id="oauth-client-secret" label={t('oauth.client.secret')} type="password" value={clientSecret} onChange={(e) => setClientSecret(e.target.value)} autoComplete="new-password" />
          {error && <p role="alert" className="text-body-sm text-status-danger">{error}</p>}
          <div>
            <Button type="button" size="sm" disabled={saving || clientId.trim() === ''} onClick={() => void save()}>{t('oauth.client.save')}</Button>
          </div>
        </>
      )}
    </fieldset>
  );
}
