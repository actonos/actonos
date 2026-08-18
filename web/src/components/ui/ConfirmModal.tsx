import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { AlertTriangle, Info, Trash2 } from 'lucide-react';

export interface ConfirmModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: () => void | Promise<void>;
  title: string;
  description: string;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: 'danger' | 'primary' | 'warning';
  loading?: boolean;
}

export function ConfirmModal({
  isOpen,
  onClose,
  onConfirm,
  title,
  description,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  variant = 'danger',
  loading = false,
}: ConfirmModalProps) {
  const [submitting, setSubmitting] = useState(false);
  const busy = loading || submitting;

  const handleConfirm = async () => {
    setSubmitting(true);
    try {
      await onConfirm();
      onClose();
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={title}>
      <div className="space-y-4">
        <div className="flex items-start gap-3.5 bg-soft-meadow p-4 rounded-[16px] border border-onyx/5">
          <div className="shrink-0 mt-0.5">
            {variant === 'danger' ? (
              <div className="w-8 h-8 rounded-full bg-red-500/15 flex items-center justify-center text-red-600">
                <Trash2 className="w-4 h-4" />
              </div>
            ) : variant === 'warning' ? (
              <div className="w-8 h-8 rounded-full bg-amber-500/15 flex items-center justify-center text-amber-600">
                <AlertTriangle className="w-4 h-4" />
              </div>
            ) : (
              <div className="w-8 h-8 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center">
                <Info className="w-4 h-4" />
              </div>
            )}
          </div>

          <p className="text-body-sm text-slate font-sans leading-relaxed flex-1">
            {description}
          </p>
        </div>

        <div className="flex items-center justify-end gap-2.5 pt-2">
          <Button variant="ghost" size="sm" onClick={onClose} disabled={busy}>
            {cancelLabel}
          </Button>
          <Button
            variant={variant === 'danger' ? 'danger' : 'primary'}
            size="sm"
            onClick={handleConfirm}
            disabled={busy}
          >
            {confirmLabel}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
import { useState } from 'react';
