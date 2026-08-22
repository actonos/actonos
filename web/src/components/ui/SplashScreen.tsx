import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useTheme } from '@/components/providers/ThemeProvider';

export interface SplashScreenProps {
  /** Triggered when the boot animation and loading sequence has gracefully finished */
  onComplete?: () => void;
  /** Whether the underlying system initialization (auth, config, models) is ready */
  isReady?: boolean;
}

export function SplashScreen({ onComplete, isReady = false }: SplashScreenProps) {
  const { t } = useTranslation('common');
  const { resolvedTheme } = useTheme();

  const [progress, setProgress] = useState(15);
  const [stageIndex, setStageIndex] = useState(0);
  const [fadeOut, setFadeOut] = useState(false);

  const stages = [
    t('splash.stages.core', 'Initializing Kernel Core...'),
    t('splash.stages.vault', 'Mounting Hardware-Bound Encrypted Vault...'),
    t('splash.stages.realtime', 'Connecting Realtime WebSocket Event Bus...'),
    t('splash.stages.models', 'Syncing Universal AI Model Catalog...'),
    t('splash.stages.ready', 'Kernel Ready'),
  ];

  useEffect(() => {
    // Step progress forward gradually
    const interval = setInterval(() => {
      setProgress((prev) => {
        if (prev < 90) {
          const next = prev + Math.floor(Math.random() * 18) + 8;
          return Math.min(next, 90);
        }
        return prev;
      });
    }, 180);

    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    // Sync stage index with progress percentage
    if (progress < 30) setStageIndex(0);
    else if (progress < 55) setStageIndex(1);
    else if (progress < 80) setStageIndex(2);
    else if (progress < 95) setStageIndex(3);
    else setStageIndex(4);
  }, [progress]);

  useEffect(() => {
    if (isReady) {
      setProgress(100);
      setStageIndex(4);
      const timer = setTimeout(() => {
        setFadeOut(true);
        const exitTimer = setTimeout(() => {
          onComplete?.();
        }, 450);
        return () => clearTimeout(exitTimer);
      }, 350);
      return () => clearTimeout(timer);
    }
  }, [isReady, onComplete]);

  return (
    <div
      data-testid="actonos-splash-screen"
      className={`fixed inset-0 z-50 flex flex-col items-center justify-center bg-canvas transition-all duration-500 ease-out select-none ${
        fadeOut ? 'opacity-0 scale-98 pointer-events-none' : 'opacity-100 scale-100'
      }`}
    >
      {/* Ambient Halo Glow */}
      <div className="absolute w-96 h-96 rounded-full bg-gradient-to-tr from-hi-yellow/20 via-fuchsia/15 to-moss-green/20 blur-3xl -z-10 animate-pulse" />

      <div className="flex flex-col items-center max-w-sm w-full px-6 text-center">
        {/* Breathing Logo Icon */}
        <div className="relative mb-6 flex items-center justify-center">
          <div className="w-20 h-20 rounded-3xl bg-soft-meadow border border-onyx/10 shadow-lg flex items-center justify-center p-3.5 transition-transform duration-700 ease-in-out hover:scale-105">
            <img
              src={resolvedTheme === 'dark' ? '/actonos_logo_light.png' : '/actonos_logo.png'}
              alt="ActonOS"
              className="w-full h-auto object-contain drop-shadow-xs"
            />
          </div>
          {/* Subtle spinning outer orbit dot */}
          <div className="absolute -inset-2.5 rounded-full border border-dashed border-deep-ink/20 animate-spin" style={{ animationDuration: '12s' }} />
        </div>

        {/* Brand Title & Tagline */}
        <h1 className="font-serif text-heading-sm font-bold text-deep-ink tracking-tight mb-1">
          {t('splash.bootTitle', 'ActonOS Kernel')}
        </h1>
        <p className="font-sans text-caption text-slate mb-8">
          {t('splash.bootSubtitle', 'Autonomous AI Agent Operating System')}
        </p>

        {/* Glowing Progress Bar */}
        <div className="w-full bg-soft-meadow/80 rounded-full h-1.5 p-0.5 border border-onyx/10 mb-3 overflow-hidden shadow-inner">
          <div
            className="h-full rounded-full bg-gradient-to-r from-hi-yellow via-fuchsia to-moss-green transition-all duration-300 ease-out"
            style={{ width: `${progress}%` }}
          />
        </div>

        {/* Live Stage Readout */}
        <div className="h-5 flex items-center justify-center">
          <span className="font-mono text-[11px] text-slate tracking-wide animate-fade-in">
            {stages[stageIndex]}
          </span>
        </div>
      </div>
    </div>
  );
}
