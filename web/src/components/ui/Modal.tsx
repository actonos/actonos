import { useEffect, useId, useRef, type ReactNode } from 'react';
import { X } from 'lucide-react';

export interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  maxWidth?: string;
  closeLabel?: string;
}

export function Modal({
  isOpen,
  onClose,
  title,
  children,
  maxWidth = 'max-w-2xl',
  closeLabel = 'Close',
}: ModalProps) {
  const titleID = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const previousFocus = useRef<HTMLElement | null>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;
  const wasOpenRef = useRef(false);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isOpen) {
        onCloseRef.current();
        return;
      }
      if (e.key === 'Tab' && isOpen && dialogRef.current) {
        const focusable = dialogRef.current.querySelectorAll<HTMLElement>(
          'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), a[href], [tabindex]:not([tabindex="-1"])'
        );
        if (focusable.length === 0) return;
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    };

    if (isOpen) {
      // Only perform initial focus and body overflow lock when transitioning from closed to open
      if (!wasOpenRef.current) {
        wasOpenRef.current = true;
        previousFocus.current = document.activeElement as HTMLElement | null;
        document.body.style.overflow = 'hidden';

        window.setTimeout(() => {
          if (!dialogRef.current) return;
          // If focus is already inside the dialog (user clicked or started typing), do NOT steal focus!
          if (dialogRef.current.contains(document.activeElement)) {
            return;
          }
          // Prioritize auto-focusing the first input, textarea, select, or [autofocus] element instead of the close button
          const formInput = dialogRef.current.querySelector<HTMLElement>(
            '[autofocus], input:not([disabled]):not([type="hidden"]), textarea:not([disabled]), select:not([disabled])'
          );
          if (formInput) {
            formInput.focus();
            return;
          }
          // Fallback to any focusable element inside the modal
          dialogRef.current.querySelector<HTMLElement>(
            'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), a[href]'
          )?.focus();
        });
      }
    } else if (wasOpenRef.current) {
      wasOpenRef.current = false;
      document.body.style.overflow = '';
      previousFocus.current?.focus();
    }

    window.addEventListener('keydown', handleKeyDown);
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [isOpen]);

  useEffect(() => {
    return () => {
      document.body.style.overflow = '';
    };
  }, []);

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      {/* Backdrop overlay */}
      <div
        className="fixed inset-0 bg-deep-ink/40 backdrop-blur-xs transition-opacity"
        onClick={onClose}
      />

      {/* Modal Dialog */}
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleID}
        className={`relative w-full ${maxWidth} bg-canvas rounded-[24px] border border-soft-meadow p-6 md:p-8 max-h-[90vh] overflow-y-auto z-10`}
      >
        <div className="flex items-center justify-between pb-4 mb-6 border-b border-soft-meadow">
          <h2 id={titleID} className="font-serif text-heading text-deep-ink">{title}</h2>
          <button
            onClick={onClose}
            aria-label={closeLabel}
            className="w-9 h-9 rounded-full bg-soft-meadow flex items-center justify-center text-deep-ink hover:bg-canvas transition-colors cursor-pointer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <div>{children}</div>
      </div>
    </div>
  );
}
