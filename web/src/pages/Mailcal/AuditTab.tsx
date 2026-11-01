import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Download } from 'lucide-react';
import { AsyncState } from '@/components/ui/AsyncState';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { mailcalApi, type MailcalDecision, type MailcalVerdict } from '@/lib/api/mailcal';
import { toCSV } from './AuditExport';

const VARIANT: Record<MailcalVerdict, 'success' | 'warning' | 'danger'> = { allow: 'success', needs_approval: 'warning', deny: 'danger' };
const selectClass = 'rounded-full border border-onyx/15 bg-canvas px-4 py-2 text-body-sm text-deep-ink';

export function AuditTab() {
  const { t } = useTranslation('mailcal');
  const [rows, setRows] = useState<MailcalDecision[]>([]);
  const [verdict, setVerdict] = useState('');
  const [phase, setPhase] = useState('');
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError('');
    try {
      setRows((await mailcalApi.listDecisions({ verdict, phase, limit: 200 })).decisions);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [verdict, phase]);

  useEffect(() => {
    void load();
  }, [load]);

  const exportCsv = () => {
    const blob = new Blob([toCSV(rows)], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'mailcal-audit.csv';
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <section aria-label={t('tabs.audit')} className="flex flex-col gap-3">
      <div className="flex flex-wrap items-end gap-3">
        <div className="flex flex-col gap-1.5">
          <label htmlFor="audit-verdict" className="text-caption font-medium uppercase tracking-wider text-slate">{t('audit.verdict')}</label>
          <select id="audit-verdict" value={verdict} onChange={(e) => setVerdict(e.target.value)} className={selectClass}>
            <option value="">{t('audit.any')}</option>
            <option value="allow">{t('verdict.allow')}</option>
            <option value="needs_approval">{t('verdict.needs_approval')}</option>
            <option value="deny">{t('verdict.deny')}</option>
          </select>
        </div>
        <div className="flex flex-col gap-1.5">
          <label htmlFor="audit-phase" className="text-caption font-medium uppercase tracking-wider text-slate">{t('audit.phase')}</label>
          <select id="audit-phase" value={phase} onChange={(e) => setPhase(e.target.value)} className={selectClass}>
            <option value="">{t('audit.any')}</option>
            <option value="gate">{t('audit.phases.gate')}</option>
            <option value="executed">{t('audit.phases.executed')}</option>
            <option value="bounce">{t('audit.phases.bounce')}</option>
          </select>
        </div>
        <Button variant="ghost" size="sm" icon={<Download className="h-4 w-4" />} disabled={rows.length === 0} onClick={exportCsv}>
          {t('audit.export')}
        </Button>
      </div>
      <AsyncState
        loading={loading}
        error={loadError}
        empty={!loading && !loadError && rows.length === 0}
        emptyTitle={t('audit.empty.title')}
        emptyDescription={t('audit.empty.description')}
        errorTitle={t('audit.error')}
        retryLabel={t('actions.refresh')}
        onRetry={() => void load()}
      >
        <ul className="grid gap-2">
          {rows.map((r) => (
            <li key={r.id}>
              <Card className="flex flex-col gap-1">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant={VARIANT[r.verdict]}>{t(`verdict.${r.verdict}`)}</Badge>
                  <span className="font-mono text-caption text-deep-ink">{r.tool}</span>
                  <span className="text-caption text-slate">{new Date(r.at).toLocaleString()}</span>
                  {r.approved && <Badge variant="info">{t('audit.approvedByPerson')}</Badge>}
                </div>
                {(r.reasons?.length ?? 0) > 0 && <p className="text-body-sm text-slate">{r.reasons?.join('; ')}</p>}
                {r.outcome && <p className="text-caption text-slate">{r.outcome}</p>}
              </Card>
            </li>
          ))}
        </ul>
      </AsyncState>
    </section>
  );
}
