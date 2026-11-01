import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AsyncState } from '@/components/ui/AsyncState';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { useToast } from '@/components/ui/Toast';
import { mailcalApi, type MailcalProposal } from '@/lib/api/mailcal';

export function ProposalsTab({ onChanged }: { onChanged?: () => void }) {
  const { t } = useTranslation('mailcal');
  const { success, error: toastError } = useToast();
  const [items, setItems] = useState<MailcalProposal[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const [busy, setBusy] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError('');
    try {
      setItems((await mailcalApi.listProposals('pending')).proposals);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const decide = async (p: MailcalProposal, approve: boolean) => {
    setBusy(p.id);
    try {
      if (approve) {
        await mailcalApi.approveProposal(p.id);
        success(t('proposals.toast.approved'));
      } else {
        await mailcalApi.rejectProposal(p.id);
        success(t('proposals.toast.rejected'));
      }
      await load();
      onChanged?.();
    } catch (err) {
      toastError(t('proposals.toast.failed'), err instanceof Error ? err.message : String(err));
    } finally {
      setBusy('');
    }
  };

  const who = (p: MailcalProposal) =>
    [...(p.rule.match.recipients.addresses ?? []), ...(p.rule.match.recipients.domains ?? []).map((d) => `@${d}`)].join(', ');

  return (
    <section aria-label={t('tabs.proposals')} className="flex flex-col gap-3">
      <p className="text-body-sm text-slate">{t('proposals.intro')}</p>
      <AsyncState
        loading={loading}
        error={loadError}
        empty={!loading && !loadError && items.length === 0}
        emptyTitle={t('proposals.empty.title')}
        emptyDescription={t('proposals.empty.description')}
        errorTitle={t('proposals.error')}
        retryLabel={t('actions.refresh')}
        onRetry={() => void load()}
      >
        <ul className="grid gap-3">
          {items.map((p) => (
            <li key={p.id}>
              <Card className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant={p.source === 'agent' ? 'warning' : 'info'}>{t(`proposals.source.${p.source}`)}</Badge>
                    <span className="font-semibold text-deep-ink">{t(`rules.action.${p.rule.action}`)}</span>
                  </div>
                  <p className="mt-1 truncate text-body-sm text-slate">{who(p)}</p>
                  {p.rule.match.body.template && (
                    <pre className="mt-1 max-h-24 overflow-auto whitespace-pre-wrap rounded border border-onyx/15 p-2 text-caption">{p.rule.match.body.template}</pre>
                  )}
                  {p.rationale && <p className="text-caption text-slate">{p.rationale}</p>}
                </div>
                <div className="flex shrink-0 gap-2">
                  <Button variant="ghost" size="sm" disabled={busy === p.id} onClick={() => void decide(p, false)}>{t('proposals.reject')}</Button>
                  <Button size="sm" disabled={busy === p.id} onClick={() => void decide(p, true)}>{t('proposals.approve')}</Button>
                </div>
              </Card>
            </li>
          ))}
        </ul>
      </AsyncState>
    </section>
  );
}
