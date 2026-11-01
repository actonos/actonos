import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@/components/ui/Badge';
import { mailcalApi, type MailcalDecisionResult, type MailcalDraft } from '@/lib/api/mailcal';

const MAIL_SEND_TOOL = 'native_mail_send';

/** The draft id of an approval for a mail send, or '' for any other approval. */
export function mailDraftIdOf(approval: { tool_name: string; input: Record<string, unknown> }): string {
  const id = approval.input?.draft_id;
  return approval.tool_name === MAIL_SEND_TOOL && typeof id === 'string' ? id : '';
}

/**
 * Shows exactly what will be sent, and why no rule covered it. The text is rendered as plain text,
 * never as HTML: it was written by an agent from untrusted mail.
 */
export function MailApprovalPreview({ draftId }: { draftId: string }) {
  const { t } = useTranslation('mailcal');
  const [draft, setDraft] = useState<MailcalDraft | null>(null);
  const [decision, setDecision] = useState<MailcalDecisionResult | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let alive = true;
    Promise.all([mailcalApi.getDraft(draftId), mailcalApi.evaluateDraft(draftId)])
      .then(([d, e]) => {
        if (!alive) return;
        setDraft(d);
        setDecision(e);
      })
      .catch(() => alive && setFailed(true));
    return () => {
      alive = false;
    };
  }, [draftId]);

  if (failed) return <p role="alert" className="text-body-sm text-status-danger">{t('approval.loadFailed')}</p>;
  if (!draft) return <p className="text-body-sm text-slate">{t('approval.loading')}</p>;

  const known = new Set(draft.reply?.participants ?? []);
  const everyone = [...draft.payload.to, ...(draft.payload.cc ?? []), ...(draft.payload.bcc ?? [])];
  const strangers = draft.reply?.verified ? everyone.filter((a) => !known.has(a.email)) : everyone;
  const list = (items?: { email: string }[]) => items?.map((a) => a.email).join(', ');

  return (
    <section aria-label={t('approval.previewLabel')} className="flex flex-col gap-2 text-body-sm">
      {strangers.length > 0 && (
        <p role="alert" className="font-semibold text-status-danger">
          {draft.reply?.verified ? t('approval.notInThread') : t('approval.notAReply')}
        </p>
      )}
      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
        <dt className="text-slate">{t('approval.to')}</dt>
        <dd className="break-all">{list(draft.payload.to)}</dd>
        {(draft.payload.cc?.length ?? 0) > 0 && (
          <>
            <dt className="text-slate">{t('approval.cc')}</dt>
            <dd className="break-all">{list(draft.payload.cc)}</dd>
          </>
        )}
        {(draft.payload.bcc?.length ?? 0) > 0 && (
          <>
            <dt className="text-slate">{t('approval.bcc')}</dt>
            <dd className="break-all">{list(draft.payload.bcc)}</dd>
          </>
        )}
        <dt className="text-slate">{t('approval.subject')}</dt>
        <dd>{draft.payload.subject}</dd>
      </dl>
      <pre className="max-h-64 overflow-auto whitespace-pre-wrap rounded-2xl border border-onyx/15 bg-soft-meadow p-3">{draft.payload.body_text}</pre>
      {decision && decision.reasons.length > 0 && (
        <div>
          <Badge variant="warning">{t(`verdict.${decision.verdict}`)}</Badge>
          <ul className="mt-1 list-disc pl-5 text-slate">
            {decision.reasons.map((r) => (
              <li key={r}>{r}</li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}
