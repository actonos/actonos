import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Mail, Plus, RefreshCw, Trash2, Zap } from 'lucide-react';
import { PageContainer } from '@/components/layout/PageContainer';
import { AsyncState } from '@/components/ui/AsyncState';
import { Badge, type BadgeProps } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { PageHeader } from '@/components/ui/PageHeader';
import { useToast } from '@/components/ui/Toast';
import {
  mailcalApi,
  type MailcalAccount,
  type MailcalAccountInput,
  type MailcalProvider,
  type MailcalStatus,
} from '@/lib/api/mailcal';
import { AccountModal } from './AccountModal';

const STATUS_VARIANT: Record<MailcalStatus, NonNullable<BadgeProps['variant']>> = {
  ok: 'success',
  auth_expired: 'warning',
  paused: 'stopped',
  error: 'danger',
};

export function MailcalPage() {
  const { t } = useTranslation('mailcal');
  const { success, error: toastError } = useToast();
  const [accounts, setAccounts] = useState<MailcalAccount[]>([]);
  const [providers, setProviders] = useState<MailcalProvider[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const [modalOpen, setModalOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [testingId, setTestingId] = useState('');
  const [deleting, setDeleting] = useState<MailcalAccount | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError('');
    try {
      const [accountList, providerList] = await Promise.all([mailcalApi.listAccounts(), mailcalApi.listProviders()]);
      setAccounts(accountList.accounts);
      setProviders(providerList.providers);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const handleCreate = async (input: MailcalAccountInput) => {
    setSaving(true);
    try {
      await mailcalApi.createAccount(input);
      setModalOpen(false);
      success(t('toast.created'));
      await load();
    } catch (err) {
      toastError(t('toast.createFailed'), err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  const handleTest = async (account: MailcalAccount) => {
    setTestingId(account.id);
    try {
      const result = await mailcalApi.testAccount(account.id);
      if (result.ok) success(t('toast.testOk'), (result.checks ?? []).join(' · '));
      else toastError(t('toast.testFailed'), result.detail);
      await load();
    } catch (err) {
      toastError(t('toast.testFailed'), err instanceof Error ? err.message : String(err));
    } finally {
      setTestingId('');
    }
  };

  const handleDelete = async () => {
    if (!deleting) return;
    try {
      await mailcalApi.deleteAccount(deleting.id);
      success(t('toast.deleted'));
      setDeleting(null);
      await load();
    } catch (err) {
      toastError(t('toast.deleteFailed'), err instanceof Error ? err.message : String(err));
    }
  };

  return (
    <PageContainer>
      <PageHeader
        eyebrow={t('eyebrow')}
        title={t('title')}
        description={t('subtitle')}
        actions={
          <>
            <Button variant="ghost" size="sm" icon={<RefreshCw className="h-4 w-4" />} onClick={() => void load()}>
              {t('actions.refresh')}
            </Button>
            <Button size="sm" icon={<Plus className="h-4 w-4" />} onClick={() => setModalOpen(true)} disabled={providers.length === 0}>
              {t('actions.addAccount')}
            </Button>
          </>
        }
      />

      <AsyncState
        loading={loading}
        error={loadError}
        empty={!loading && !loadError && accounts.length === 0}
        emptyTitle={t(providers.length === 0 ? 'empty.noProviderTitle' : 'empty.title')}
        emptyDescription={t(providers.length === 0 ? 'empty.noProviderDescription' : 'empty.description')}
        errorTitle={t('error.title')}
        retryLabel={t('actions.refresh')}
        onRetry={() => void load()}
      >
        <ul className="grid gap-3">
          {accounts.map((account) => (
            <li key={account.id}>
              <Card className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex min-w-0 items-start gap-3">
                  <Mail className="mt-1 h-5 w-5 shrink-0 text-slate" aria-hidden="true" />
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <h2 className="font-serif text-heading-sm font-semibold text-deep-ink">{account.label}</h2>
                      <Badge variant={STATUS_VARIANT[account.status]}>{t(`status.${account.status}`)}</Badge>
                      <Badge variant="neutral">{t(`kind.${account.kind}`)}</Badge>
                    </div>
                    <p className="mt-1 truncate text-body-sm text-slate">
                      {account.email || t('noEmail')} · {account.plugin_id}
                    </p>
                    {account.status_detail && <p className="mt-1 text-caption text-status-warning">{account.status_detail}</p>}
                  </div>
                </div>
                <div className="flex shrink-0 gap-2">
                  <Button
                    variant="ghost"
                    size="sm"
                    icon={<Zap className="h-4 w-4" />}
                    disabled={testingId === account.id}
                    onClick={() => void handleTest(account)}
                  >
                    {t('actions.test')}
                  </Button>
                  <Button variant="ghost" size="sm" icon={<Trash2 className="h-4 w-4" />} onClick={() => setDeleting(account)}>
                    {t('actions.delete')}
                  </Button>
                </div>
              </Card>
            </li>
          ))}
        </ul>
      </AsyncState>

      <AccountModal isOpen={modalOpen} providers={providers} saving={saving} onClose={() => setModalOpen(false)} onSubmit={handleCreate} />
      <ConfirmModal
        isOpen={deleting !== null}
        onClose={() => setDeleting(null)}
        onConfirm={handleDelete}
        title={t('confirmDelete.title')}
        description={t('confirmDelete.description', { label: deleting?.label ?? '' })}
        confirmLabel={t('actions.delete')}
        cancelLabel={t('actions.cancel')}
      />
    </PageContainer>
  );
}
