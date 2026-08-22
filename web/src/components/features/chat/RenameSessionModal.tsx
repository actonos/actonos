import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';

export interface RenameSessionModalProps {
  isOpen: boolean;
  onClose: () => void;
  initialTitle: string;
  onSave: (newTitle: string) => Promise<void> | void;
}

export function RenameSessionModal({
  isOpen,
  onClose,
  initialTitle,
  onSave,
}: RenameSessionModalProps) {
  const { t } = useTranslation('chat');
  const [title, setTitle] = useState(initialTitle);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (isOpen) {
      setTitle(initialTitle);
      setSaving(false);
    }
  }, [isOpen, initialTitle]);

  const handleSubmit = async (e?: React.FormEvent) => {
    if (e) e.preventDefault();
    const trimmed = title.trim();
    if (!trimmed || saving) return;

    setSaving(true);
    try {
      await onSave(trimmed);
      onClose();
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('rename.title')}
      maxWidth="max-w-md"
    >
      <form onSubmit={handleSubmit} className="space-y-4">
        <Input
          label={t('rename.label')}
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder={t('rename.placeholder')}
          autoFocus
        />

        <div className="flex items-center justify-end gap-2 pt-2">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onClose}
            disabled={saving}
          >
            {t('rename.cancel')}
          </Button>
          <Button
            type="submit"
            variant="primary"
            size="sm"
            disabled={!title.trim() || saving}
          >
            {t('rename.save')}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
