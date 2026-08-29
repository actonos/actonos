import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import type { StructuredDirective } from '@/lib/types';
import { Clock, ShieldCheck, Target, Zap } from 'lucide-react';

export interface DirectiveModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSave: (directive: StructuredDirective) => void;
  directive?: StructuredDirective | null;
}

export function DirectiveModal({
  isOpen,
  onClose,
  onSave,
  directive,
}: DirectiveModalProps) {
  const { t } = useTranslation('missions');

  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [priority, setPriority] = useState<StructuredDirective['priority']>('p2_normal');
  const [schedule, setSchedule] = useState('0 9 * * *');
  const [expectedOutcome, setExpectedOutcome] = useState('');
  const [verification, setVerification] = useState('');
  const [autoCreateMission, setAutoCreateMission] = useState(true);
  const [maxRuntimeMin, setMaxRuntimeMin] = useState(15);
  const [enabled, setEnabled] = useState(true);

  useEffect(() => {
    if (directive) {
      setTitle(directive.title || '');
      setDescription(directive.description || directive.intent || '');
      setPriority(directive.priority || 'p2_normal');
      setSchedule(directive.schedule || directive.cadence || '0 9 * * *');
      setExpectedOutcome(directive.expected_outcome || '');
      setVerification(directive.verification || directive.verify_rule || '');
      setAutoCreateMission(directive.auto_create_mission ?? true);
      setMaxRuntimeMin(directive.max_runtime_min || 15);
      setEnabled(directive.enabled ?? true);
    } else {
      setTitle('');
      setDescription('');
      setPriority('p2_normal');
      setSchedule('0 9 * * *');
      setExpectedOutcome('');
      setVerification('');
      setAutoCreateMission(true);
      setMaxRuntimeMin(15);
      setEnabled(true);
    }
  }, [directive, isOpen]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim()) return;

    const item: StructuredDirective = {
      id: directive?.id || `dir_${Date.now()}`,
      title: title.trim(),
      description: description.trim(),
      intent: description.trim(),
      priority,
      schedule: schedule.trim() || undefined,
      cadence: schedule.trim() || undefined,
      expected_outcome: expectedOutcome.trim() || undefined,
      verification: verification.trim() || undefined,
      verify_rule: verification.trim() || undefined,
      auto_create_mission: autoCreateMission,
      max_runtime_min: maxRuntimeMin > 0 ? maxRuntimeMin : 15,
      enabled,
    };

    onSave(item);
    onClose();
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={directive ? t('directives.editDirective') : t('directives.newDirective')}
      maxWidth="max-w-2xl"
    >
      <form onSubmit={handleSubmit} className="space-y-4">
        {/* Title */}
        <div>
          <label className="block text-caption font-semibold text-deep-ink mb-1">
            {t('directives.directiveTitle')}
          </label>
          <Input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="e.g. Daily SEO & Sitemap Health Check"
            required
          />
        </div>

        {/* Description / Intent */}
        <div>
          <label className="block text-caption font-semibold text-deep-ink mb-1">
            {t('directives.directiveDescription')}
          </label>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={3}
            placeholder="Describe the operational goals, rules, and actions for this standing directive..."
            className="w-full rounded-[16px] border border-onyx/15 bg-canvas px-3.5 py-2.5 text-body-sm text-deep-ink placeholder:text-slate/60 focus:outline-none focus:ring-1 focus:ring-deep-ink font-sans transition-all"
            required
          />
        </div>

        {/* Priority & Schedule Grid */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3.5">
          <div>
            <label className="block text-caption font-semibold text-deep-ink mb-1">
              {t('modal.priority')}
            </label>
            <select
              value={priority}
              onChange={(e) => setPriority(e.target.value as StructuredDirective['priority'])}
              className="w-full rounded-[16px] border border-onyx/15 bg-canvas px-3.5 py-2 text-body-sm text-deep-ink focus:outline-none focus:ring-1 focus:ring-deep-ink font-sans transition-all h-[42px]"
            >
              <option value="p0_critical">{t('priorities.p0')}</option>
              <option value="p1_high">{t('priorities.p1')}</option>
              <option value="p2_normal">{t('priorities.p2')}</option>
              <option value="p3_low">{t('priorities.p3')}</option>
            </select>
          </div>

          <div>
            <label className="block text-caption font-semibold text-deep-ink mb-1 flex items-center gap-1.5">
              <Clock className="w-3.5 h-3.5 text-slate" />
              {t('directives.scheduleCron')}
            </label>
            <Input
              value={schedule}
              onChange={(e) => setSchedule(e.target.value)}
              placeholder="0 9 * * *"
            />
          </div>
        </div>

        {/* Expected Outcome & Verification Rule */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3.5">
          <div>
            <label className="block text-caption font-semibold text-deep-ink mb-1 flex items-center gap-1.5">
              <Target className="w-3.5 h-3.5 text-slate" />
              {t('directives.expectedOutcome')}
            </label>
            <Input
              value={expectedOutcome}
              onChange={(e) => setExpectedOutcome(e.target.value)}
              placeholder={t('directives.expectedOutcomePlaceholder')}
            />
          </div>

          <div>
            <label className="block text-caption font-semibold text-deep-ink mb-1 flex items-center gap-1.5">
              <ShieldCheck className="w-3.5 h-3.5 text-slate" />
              {t('directives.verificationRule')}
            </label>
            <Input
              value={verification}
              onChange={(e) => setVerification(e.target.value)}
              placeholder={t('directives.verificationRulePlaceholder')}
            />
          </div>
        </div>

        {/* Auto Create Mission & Max Runtime */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3.5 p-3.5 rounded-[18px] bg-soft-meadow border border-onyx/10">
          <div className="flex items-start gap-2.5">
            <input
              id="dir-autocreate"
              type="checkbox"
              checked={autoCreateMission}
              onChange={(e) => setAutoCreateMission(e.target.checked)}
              className="w-4 h-4 rounded accent-deep-ink cursor-pointer mt-0.5"
            />
            <div>
              <label htmlFor="dir-autocreate" className="text-caption font-semibold text-deep-ink block cursor-pointer">
                {t('directives.autoCreateMission')}
              </label>
              <p className="text-[11px] text-slate mt-0.5">
                {t('directives.autoCreateMissionDesc')}
              </p>
            </div>
          </div>

          <div className="flex items-center justify-between sm:justify-end gap-3">
            <label className="text-caption font-semibold text-deep-ink">
              {t('directives.maxRuntimeMin')}
            </label>
            <Input
              type="number"
              min={1}
              max={120}
              value={maxRuntimeMin}
              onChange={(e) => setMaxRuntimeMin(parseInt(e.target.value, 10) || 15)}
              className="w-20 text-center"
            />
          </div>
        </div>

        {/* Enabled Status */}
        <div className="flex items-center justify-between p-3 rounded-[16px] bg-soft-meadow border border-onyx/10">
          <div className="flex items-center gap-2">
            <Zap className="w-4 h-4 text-deep-ink" />
            <label htmlFor="dir-enabled" className="text-body-sm font-semibold text-deep-ink cursor-pointer">
              {t('directives.enabled')}
            </label>
          </div>
          <input
            id="dir-enabled"
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
            className="w-5 h-5 rounded accent-deep-ink cursor-pointer"
          />
        </div>

        {/* Actions */}
        <div className="flex justify-end gap-2 pt-3 border-t border-onyx/10">
          <Button variant="ghost" type="button" onClick={onClose}>
            {t('modal.cancel')}
          </Button>
          <Button variant="primary" type="submit">
            {directive ? t('modal.update') : t('modal.create')}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
