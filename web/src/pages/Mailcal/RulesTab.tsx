import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Plus } from 'lucide-react';
import { AsyncState } from '@/components/ui/AsyncState';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { useToast } from '@/components/ui/Toast';
import { mailcalApi, type MailcalAccount, type MailcalRule, type MailcalRuleInput } from '@/lib/api/mailcal';
import { RuleEditor } from './RuleEditor';

export function RulesTab({ accounts }: { accounts: MailcalAccount[] }) {
  const { t } = useTranslation('mailcal');
  const { success, error: toastError } = useToast();
  const [rules, setRules] = useState<MailcalRule[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');
  const [deleting, setDeleting] = useState<MailcalRule | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError('');
    try {
      setRules((await mailcalApi.listRules()).rules);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const accountLabel = (id: string) => accounts.find((a) => a.id === id)?.label ?? id;

  const create = async (rule: MailcalRuleInput) => {
    setSaving(true);
    setSaveError('');
    try {
      await mailcalApi.createRule(rule);
      setEditing(false);
      success(t('rules.toast.created'));
      await load();
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  const toggle = async (rule: MailcalRule) => {
    try {
      await mailcalApi.setRuleEnabled(rule.id, !rule.enabled);
      await load();
    } catch (err) {
      toastError(t('rules.toast.failed'), err instanceof Error ? err.message : String(err));
    }
  };

  const remove = async () => {
    if (!deleting) return;
    try {
      await mailcalApi.deleteRule(deleting.id);
      setDeleting(null);
      await load();
    } catch (err) {
      toastError(t('rules.toast.failed'), err instanceof Error ? err.message : String(err));
    }
  };

  const who = (r: MailcalRule) =>
    [...(r.match.recipients.addresses ?? []), ...(r.match.recipients.domains ?? []).map((d) => `@${d}`), ...(r.match.calendar?.attendee_domains ?? []).map((d) => `@${d}`)].join(', ');

  return (
    <section aria-label={t('tabs.rules')} className="flex flex-col gap-3">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-body-sm text-slate">{t('rules.intro')}</p>
        <Button size="sm" icon={<Plus className="h-4 w-4" />} disabled={accounts.length === 0} onClick={() => { setSaveError(''); setEditing(true); }}>
          {t('rules.add')}
        </Button>
      </div>
      <AsyncState
        loading={loading}
        error={loadError}
        empty={!loading && !loadError && rules.length === 0}
        emptyTitle={t('rules.empty.title')}
        emptyDescription={t('rules.empty.description')}
        errorTitle={t('rules.error')}
        retryLabel={t('actions.refresh')}
        onRetry={() => void load()}
      >
        <ul className="grid gap-3">
          {rules.map((rule) => (
            <li key={rule.id}>
              <Card className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant={rule.effect === 'deny' ? 'danger' : 'info'}>{t(`rules.effect.${rule.effect}`)}</Badge>
                    <span className="font-semibold text-deep-ink">{t(`rules.action.${rule.action}`)}</span>
                    <Badge variant={rule.enabled ? 'success' : 'stopped'}>{rule.enabled ? t('rules.state.enabled') : t('rules.state.paused')}</Badge>
                  </div>
                  <p className="mt-1 truncate text-body-sm text-slate">{accountLabel(rule.account_id)} · {who(rule) || t('rules.everyone')}</p>
                  <p className="text-caption text-slate">
                    {rule.match.body.mode ? t(`rules.bodyMode.${rule.match.body.mode}`) : ''}
                    {rule.limits.per_hour ? ` · ${t('rules.limits', { hour: rule.limits.per_hour, day: rule.limits.per_day })}` : ''}
                  </p>
                  {rule.paused_reason && <p className="text-caption text-status-warning">{rule.paused_reason}</p>}
                </div>
                <div className="flex shrink-0 gap-2">
                  <Button variant="ghost" size="sm" onClick={() => void toggle(rule)}>
                    {rule.enabled ? t('rules.pause') : t('rules.resume')}
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => setDeleting(rule)}>{t('actions.delete')}</Button>
                </div>
              </Card>
            </li>
          ))}
        </ul>
      </AsyncState>
      <RuleEditor isOpen={editing} accounts={accounts} saving={saving} error={saveError} onClose={() => setEditing(false)} onSubmit={create} />
      <ConfirmModal
        isOpen={deleting !== null}
        onClose={() => setDeleting(null)}
        onConfirm={remove}
        title={t('rules.confirmDelete.title')}
        description={t('rules.confirmDelete.description')}
        confirmLabel={t('actions.delete')}
        cancelLabel={t('actions.cancel')}
      />
    </section>
  );
}
