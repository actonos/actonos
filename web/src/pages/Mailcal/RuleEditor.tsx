import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Modal } from '@/components/ui/Modal';
import {
  mailcalApi,
  type MailcalAccount,
  type MailcalDecisionResult,
  type MailcalRuleAction,
  type MailcalRuleInput,
} from '@/lib/api/mailcal';

const ACTIONS: MailcalRuleAction[] = ['reply', 'send_new', 'calendar_accept', 'calendar_create', 'calendar_update'];

const isMail = (a: MailcalRuleAction) => a === 'reply' || a === 'send_new';
const lines = (s: string) => s.split('\n').map((x) => x.trim()).filter(Boolean);
const selectClass = 'rounded-full border border-onyx/15 bg-canvas px-4 py-2.5 text-body-sm text-deep-ink';

interface RuleEditorProps {
  isOpen: boolean;
  accounts: MailcalAccount[];
  saving: boolean;
  error: string;
  onClose: () => void;
  onSubmit: (rule: MailcalRuleInput) => void | Promise<void>;
}

export function RuleEditor({ isOpen, accounts, saving, error, onClose, onSubmit }: RuleEditorProps) {
  const { t } = useTranslation('mailcal');
  const [accountId, setAccountId] = useState('');
  const [action, setAction] = useState<MailcalRuleAction>('reply');
  const [effect, setEffect] = useState<'allow' | 'deny'>('allow');
  const [addresses, setAddresses] = useState('');
  const [domains, setDomains] = useState('');
  const [mode, setMode] = useState<'template' | 'free'>('template');
  const [template, setTemplate] = useState('');
  const [allowLinks, setAllowLinks] = useState(false);
  const [freeAck, setFreeAck] = useState(false);
  const [perHour, setPerHour] = useState('3');
  const [perDay, setPerDay] = useState('10');
  const [hours, setHours] = useState('09:00-18:00');
  const [maxMinutes, setMaxMinutes] = useState('60');
  const [attendeeDomains, setAttendeeDomains] = useState('');
  const [draftId, setDraftId] = useState('');
  const [dry, setDry] = useState<MailcalDecisionResult | null>(null);
  const [dryError, setDryError] = useState('');
  const [localError, setLocalError] = useState('');

  useEffect(() => {
    if (!isOpen) return;
    setAccountId(accounts[0]?.id ?? '');
    setAction('reply');
    setEffect('allow');
    setAddresses('');
    setDomains('');
    setMode('template');
    setTemplate('');
    setAllowLinks(false);
    setFreeAck(false);
    setPerHour('3');
    setPerDay('10');
    setHours('09:00-18:00');
    setMaxMinutes('60');
    setAttendeeDomains('');
    setDraftId('');
    setDry(null);
    setDryError('');
    setLocalError('');
  }, [isOpen, accounts]);

  const mail = isMail(action);
  const allow = effect === 'allow';

  const build = (): MailcalRuleInput | string => {
    if (!accountId) return t('rules.editor.errors.account');
    const rule: MailcalRuleInput = {
      enabled: true,
      account_id: accountId,
      action,
      effect,
      match: { recipients: { addresses: lines(addresses), domains: lines(domains) }, body: {} },
      limits: {},
    };
    if (mail) {
      rule.match.reply_only = action === 'reply';
      rule.match.body = { mode, allow_links: allowLinks };
      if (allow && mode === 'template') {
        if (!template.trim()) return t('rules.editor.errors.template');
        rule.match.body.template = template;
      }
      if (allow && mode === 'free' && !freeAck) return t('rules.editor.errors.ack');
      if (allow && (rule.match.recipients.addresses?.length ?? 0) + (rule.match.recipients.domains?.length ?? 0) === 0) {
        return t('rules.editor.errors.recipients');
      }
    } else {
      rule.match.calendar = { attendee_domains: lines(attendeeDomains) };
      if (action !== 'calendar_accept') {
        rule.match.calendar.working_hours = hours.trim();
        rule.match.calendar.max_minutes = Number(maxMinutes);
      }
    }
    if (allow) {
      const h = Number(perHour);
      const d = Number(perDay);
      if (!Number.isInteger(h) || h < 1 || !Number.isInteger(d) || d < 1) return t('rules.editor.errors.limits');
      rule.limits = { per_hour: h, per_day: d };
    }
    return rule;
  };

  const submit = () => {
    const built = build();
    if (typeof built === 'string') {
      setLocalError(built);
      return;
    }
    setLocalError('');
    void onSubmit(built);
  };

  const runDry = async () => {
    const built = build();
    if (typeof built === 'string') {
      setDryError(built);
      return;
    }
    setDryError('');
    setDry(null);
    try {
      setDry(await mailcalApi.dryRun(built, draftId.trim()));
    } catch (err) {
      setDryError(err instanceof Error ? err.message : String(err));
    }
  };

  const shownError = localError || error;

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={t('rules.editor.title')} maxWidth="lg">
      <form
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
      >
        <div className="grid gap-4 sm:grid-cols-3">
          <div className="flex flex-col gap-1.5">
            <label htmlFor="rule-account" className="text-caption font-medium uppercase tracking-wider text-slate">{t('rules.editor.account')}</label>
            <select id="rule-account" value={accountId} onChange={(e) => setAccountId(e.target.value)} className={selectClass}>
              {accounts.map((a) => (
                <option key={a.id} value={a.id}>{a.label}</option>
              ))}
            </select>
          </div>
          <div className="flex flex-col gap-1.5">
            <label htmlFor="rule-action" className="text-caption font-medium uppercase tracking-wider text-slate">{t('rules.editor.action')}</label>
            <select id="rule-action" value={action} onChange={(e) => setAction(e.target.value as MailcalRuleAction)} className={selectClass}>
              {ACTIONS.map((a) => (
                <option key={a} value={a}>{t(`rules.action.${a}`)}</option>
              ))}
            </select>
          </div>
          <div className="flex flex-col gap-1.5">
            <label htmlFor="rule-effect" className="text-caption font-medium uppercase tracking-wider text-slate">{t('rules.editor.effect')}</label>
            <select id="rule-effect" value={effect} onChange={(e) => setEffect(e.target.value as 'allow' | 'deny')} className={selectClass}>
              <option value="allow">{t('rules.effect.allow')}</option>
              <option value="deny">{t('rules.effect.deny')}</option>
            </select>
          </div>
        </div>

        {mail ? (
          <>
            <div className="flex flex-col gap-1.5">
              <label htmlFor="rule-addresses" className="text-caption font-medium uppercase tracking-wider text-slate">{t('rules.editor.addresses')}</label>
              <textarea id="rule-addresses" value={addresses} onChange={(e) => setAddresses(e.target.value)} rows={3} className="rounded-2xl border border-onyx/25 bg-canvas px-4 py-2 text-body-sm text-deep-ink" />
            </div>
            <div className="flex flex-col gap-1.5">
              <label htmlFor="rule-domains" className="text-caption font-medium uppercase tracking-wider text-slate">{t('rules.editor.domains')}</label>
              <textarea id="rule-domains" value={domains} onChange={(e) => setDomains(e.target.value)} rows={2} className="rounded-2xl border border-onyx/25 bg-canvas px-4 py-2 text-body-sm text-deep-ink" />
            </div>
            <p className="text-caption text-slate">{t('rules.editor.noWildcards')}</p>
            {allow && (
              <>
                <div className="flex flex-col gap-1.5">
                  <label htmlFor="rule-mode" className="text-caption font-medium uppercase tracking-wider text-slate">{t('rules.editor.bodyMode')}</label>
                  <select id="rule-mode" value={mode} onChange={(e) => setMode(e.target.value as 'template' | 'free')} className={selectClass}>
                    <option value="template">{t('rules.bodyMode.template')}</option>
                    <option value="free">{t('rules.bodyMode.free')}</option>
                  </select>
                </div>
                {mode === 'template' ? (
                  <div className="flex flex-col gap-1.5">
                    <label htmlFor="rule-template" className="text-caption font-medium uppercase tracking-wider text-slate">{t('rules.editor.template')}</label>
                    <textarea id="rule-template" value={template} onChange={(e) => setTemplate(e.target.value)} rows={4} className="rounded-2xl border border-onyx/25 bg-canvas px-4 py-2 text-body-sm text-deep-ink" />
                  </div>
                ) : (
                  <label className="flex items-start gap-2 text-body-sm text-status-warning">
                    <input type="checkbox" checked={freeAck} onChange={(e) => setFreeAck(e.target.checked)} className="mt-1" />
                    <span>{t('rules.editor.freeWarning')}</span>
                  </label>
                )}
                <label className="flex items-center gap-2 text-body-sm text-deep-ink">
                  <input type="checkbox" checked={allowLinks} onChange={(e) => setAllowLinks(e.target.checked)} />
                  {t('rules.editor.allowLinks')}
                </label>
              </>
            )}
          </>
        ) : (
          <>
            <div className="flex flex-col gap-1.5">
              <label htmlFor="rule-att" className="text-caption font-medium uppercase tracking-wider text-slate">{t('rules.editor.attendeeDomains')}</label>
              <textarea id="rule-att" value={attendeeDomains} onChange={(e) => setAttendeeDomains(e.target.value)} rows={2} className="rounded-2xl border border-onyx/25 bg-canvas px-4 py-2 text-body-sm text-deep-ink" />
            </div>
            {action !== 'calendar_accept' && (
              <div className="grid gap-4 sm:grid-cols-2">
                <Input id="rule-hours" label={t('rules.editor.workingHours')} value={hours} onChange={(e) => setHours(e.target.value)} />
                <Input id="rule-max" label={t('rules.editor.maxMinutes')} inputMode="numeric" value={maxMinutes} onChange={(e) => setMaxMinutes(e.target.value)} />
              </div>
            )}
          </>
        )}

        {allow && (
          <div className="grid gap-4 sm:grid-cols-2">
            <Input id="rule-hour" label={t('rules.editor.perHour')} inputMode="numeric" value={perHour} onChange={(e) => setPerHour(e.target.value)} />
            <Input id="rule-day" label={t('rules.editor.perDay')} inputMode="numeric" value={perDay} onChange={(e) => setPerDay(e.target.value)} />
          </div>
        )}

        {mail && (
          <fieldset className="flex flex-col gap-2 rounded-2xl border border-onyx/15 p-3">
            <legend className="px-2 text-caption font-medium uppercase tracking-wider text-slate">{t('rules.editor.dryRun')}</legend>
            <p className="text-caption text-slate">{t('rules.editor.dryRunHint')}</p>
            <div className="flex gap-2">
              <div className="flex-1">
                <Input id="rule-draft" aria-label={t('rules.editor.draftId')} placeholder={t('rules.editor.draftId')} value={draftId} onChange={(e) => setDraftId(e.target.value)} />
              </div>
              <Button type="button" variant="secondary" disabled={!draftId.trim()} onClick={() => void runDry()}>
                {t('rules.editor.runDry')}
              </Button>
            </div>
            {dryError && <p role="alert" className="text-body-sm text-status-danger">{dryError}</p>}
            {dry && (
              <div aria-live="polite" className="text-body-sm text-deep-ink">
                <p className="font-semibold">{t(`verdict.${dry.verdict}`)}</p>
                <ul className="list-disc pl-5">
                  {dry.reasons.map((r) => (
                    <li key={r}>{r}</li>
                  ))}
                </ul>
              </div>
            )}
          </fieldset>
        )}

        {shownError && <p role="alert" className="text-body-sm text-status-danger">{shownError}</p>}

        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>{t('actions.cancel')}</Button>
          <Button type="submit" disabled={saving}>{t('rules.editor.save')}</Button>
        </div>
      </form>
    </Modal>
  );
}
