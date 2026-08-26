import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { PageHeader } from '@/components/ui/PageHeader';
import { TokenLedgerPanel } from '@/components/modals/TokenLedgerModal';
import { readHashParams, setHashParam } from '@/lib/url-state';

export function CostsPage() {
  const { t } = useTranslation('costs');
  const [view, setView] = useState<'overview' | 'transactions'>(
    () => (readHashParams().get('view') === 'transactions' ? 'transactions' : 'overview'),
  );

  useEffect(() => {
    const sync = () => {
      setView(readHashParams().get('view') === 'transactions' ? 'transactions' : 'overview');
    };
    window.addEventListener('hashchange', sync);
    return () => window.removeEventListener('hashchange', sync);
  }, []);

  return (
    <PageContainer maxWidth="wide">
      <PageHeader
        eyebrow={t('eyebrow')}
        title={t('title')}
        description={t('pageSubtitle')}
      />
      <TokenLedgerPanel
        urlView={view}
        onViewChange={(next) => {
          setView(next);
          setHashParam('view', next === 'overview' ? undefined : next);
        }}
      />
    </PageContainer>
  );
}
