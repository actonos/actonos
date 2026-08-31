import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Download, X, Share2, PlusSquare } from 'lucide-react';
import { Button } from '@/components/ui/Button';

interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed'; platform: string }>;
}

export function PWAInstallBanner() {
  const { t } = useTranslation('common');
  const [installPrompt, setInstallPrompt] = useState<BeforeInstallPromptEvent | null>(null);
  const [showIOSPrompt, setShowIOSPrompt] = useState(false);
  const [dismissed, setDismissed] = useState<boolean>(() => {
    return localStorage.getItem('actonos_pwa_banner_dismissed') === 'true';
  });

  useEffect(() => {
    // Check if already in standalone PWA mode
    const isStandalone =
      window.matchMedia('(display-mode: standalone)').matches ||
      (window.navigator as unknown as { standalone?: boolean }).standalone === true;

    if (isStandalone || dismissed) {
      return;
    }

    // Chromium beforeinstallprompt event
    const handleBeforeInstallPrompt = (e: Event) => {
      e.preventDefault();
      setInstallPrompt(e as BeforeInstallPromptEvent);
    };

    window.addEventListener('beforeinstallprompt', handleBeforeInstallPrompt);

    // iOS Safari detection
    const isIOS = /iPad|iPhone|iPod/.test(navigator.userAgent) && !(window as unknown as { MSStream?: unknown }).MSStream;
    if (isIOS && !isStandalone && !dismissed) {
      // Show iOS help once on mobile safari
      setShowIOSPrompt(true);
    }

    return () => {
      window.removeEventListener('beforeinstallprompt', handleBeforeInstallPrompt);
    };
  }, [dismissed]);

  const handleInstallClick = async () => {
    if (!installPrompt) return;
    try {
      await installPrompt.prompt();
      const choice = await installPrompt.userChoice;
      if (choice.outcome === 'accepted') {
        handleDismiss();
      }
    } catch {
      // User dismissed
    }
  };

  const handleDismiss = () => {
    setDismissed(true);
    setInstallPrompt(null);
    setShowIOSPrompt(false);
    localStorage.setItem('actonos_pwa_banner_dismissed', 'true');
  };

  if (dismissed || (!installPrompt && !showIOSPrompt)) {
    return null;
  }

  return (
    <div className="fixed top-16 left-4 right-4 sm:left-auto sm:right-6 sm:max-w-md z-40 animate-in fade-in slide-in-from-top-4 duration-300">
      <div className="p-3.5 rounded-2xl bg-canvas/95 backdrop-blur-xl border border-onyx/15 shadow-xl flex items-center justify-between gap-3">
        <div className="flex items-center gap-3 min-w-0">
          <img src="/actonos_icon.png" alt="ActonOS" className="w-9 h-9 rounded-xl shadow-xs shrink-0" />
          <div className="min-w-0">
            <h4 className="text-body-sm font-semibold text-deep-ink truncate">
              {t('pwa.title', 'Install ActonOS App')}
            </h4>
            <p className="text-[11px] text-slate line-clamp-1">
              {showIOSPrompt
                ? t('pwa.iosGuide', 'Tap Share icon then "Add to Home Screen"')
                : t('pwa.subtitle', 'Fast 24/7 mobile access & push alerts')}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-1.5 shrink-0">
          {installPrompt && (
            <Button
              variant="primary"
              size="sm"
              onClick={handleInstallClick}
              icon={<Download className="w-3.5 h-3.5" />}
              className="text-xs py-1 px-2.5 h-8"
            >
              {t('pwa.install', 'Install')}
            </Button>
          )}

          {showIOSPrompt && (
            <div className="flex items-center gap-1 text-[11px] text-deep-ink bg-soft-meadow px-2 py-1 rounded-lg border border-onyx/10">
              <Share2 className="w-3 h-3 inline text-deep-ink" />
              <span>+</span>
              <PlusSquare className="w-3 h-3 inline text-deep-ink" />
            </div>
          )}

          <button
            type="button"
            onClick={handleDismiss}
            aria-label="Dismiss"
            className="p-1 rounded-lg hover:bg-soft-meadow text-slate hover:text-deep-ink transition-colors cursor-pointer"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
      </div>
    </div>
  );
}
