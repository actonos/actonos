import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Input } from '@/components/ui/Input';
import { SegmentedControl } from '@/components/ui/SegmentedControl';

export function AgentIdentitySection({
  name,
  identifier,
  description,
  status,
  identifierLocked,
  onNameChange,
  onIdentifierChange,
  onDescriptionChange,
  onStatusChange,
}: {
  name: string;
  identifier: string;
  description: string;
  avatarIcon?: string;
  status: 'active' | 'stopped';
  identifierLocked: boolean;
  onNameChange: (value: string) => void;
  onIdentifierChange: (value: string) => void;
  onDescriptionChange: (value: string) => void;
  onStatusChange: (value: 'active' | 'stopped') => void;
}) {
  const { t } = useTranslation('agents');
  return (
    <Card className="mb-8 space-y-4 border border-onyx/15 bg-soft-meadow p-6 shadow-xs">
      <div className="grid grid-cols-1 gap-6 md:grid-cols-3">
        <div className="space-y-4 md:col-span-2">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <label className="text-caption font-semibold text-deep-ink">
              <span className="mb-1 block">{t('studio.name')}</span>
              <Input value={name} onChange={(event) => onNameChange(event.target.value)} placeholder={t('studio.namePlaceholder')} />
            </label>
            <label className="text-caption font-semibold text-deep-ink">
              <span className="mb-1 block">{t('studio.identifier')}</span>
              <Input value={identifier} onChange={(event) => onIdentifierChange(event.target.value)} placeholder={t('studio.identifierPlaceholder')} disabled={identifierLocked} />
            </label>
          </div>
          <label className="text-caption font-semibold text-deep-ink">
            <span className="mb-1 block">{t('studio.description')}</span>
            <Input value={description} onChange={(event) => onDescriptionChange(event.target.value)} placeholder={t('studio.descriptionPlaceholder')} />
          </label>
        </div>
        <div className="flex flex-col justify-center rounded-[20px] border border-onyx/10 bg-canvas p-4 shadow-xs">
          <p className="mb-2.5 text-caption font-semibold text-deep-ink">{t('studio.lifecycle')}</p>
          <SegmentedControl
            value={status}
            onChange={onStatusChange}
            label={t('studio.lifecycle')}
            options={[
              { value: 'active', label: t('studio.active') },
              { value: 'stopped', label: t('studio.stopped') },
            ]}
          />
        </div>
      </div>
    </Card>
  );
}
