import { useState, useRef, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useRealtime } from '@/components/providers/RealtimeProvider';
import { Activity, RefreshCw, Cpu, HardDrive, Wifi, WifiOff } from 'lucide-react';

export interface ConnectionStatusIndicatorProps {
  compact?: boolean;
  placement?: 'bottom-right' | 'top-right' | 'sidebar';
}

export function ConnectionStatusIndicator({
  compact = false,
  placement = 'sidebar',
}: ConnectionStatusIndicatorProps) {
  const { t } = useTranslation(['common', 'nav']);
  const { connection, snapshot } = useRealtime();
  const [isOpen, setIsOpen] = useState(false);
  const popoverRef = useRef<HTMLDivElement>(null);

  const metrics = snapshot?.metrics;
  const isOnline = connection === 'online';
  const isConnecting = connection === 'connecting';

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) {
        setIsOpen(false);
      }
    };
    if (isOpen) {
      document.addEventListener('mousedown', handleClickOutside);
    }
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [isOpen]);

  const handleReconnect = () => {
    window.location.reload();
  };

  const statusLabel = isOnline
    ? t('nav:status.connected', 'System Online')
    : isConnecting
      ? t('common:telemetry.connecting', 'Connecting...')
      : t('common:telemetry.offline', 'System Offline');

  return (
    <div className="relative w-full" ref={popoverRef}>
      {compact ? (
        /* Collapsed Sidebar Icon Mode */
        <button
          type="button"
          onClick={() => setIsOpen(!isOpen)}
          className="w-full flex justify-center py-1.5 cursor-pointer rounded-xl hover:bg-black/5 dark:hover:bg-white/5 transition-colors"
          title={statusLabel}
          aria-label={statusLabel}
        >
          <span className="relative flex h-2.5 w-2.5">
            {isOnline && (
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
            )}
            {isConnecting && (
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75" />
            )}
            <span
              className={`relative inline-flex rounded-full h-2.5 w-2.5 ${
                isOnline ? 'bg-emerald-500' : isConnecting ? 'bg-amber-500' : 'bg-red-500'
              }`}
            />
          </span>
        </button>
      ) : (
        /* Full Sidebar Pill Mode */
        <button
          type="button"
          onClick={() => setIsOpen(!isOpen)}
          className={`w-full flex items-center justify-between px-3 py-2 rounded-2xl border transition-all cursor-pointer text-left ${
            isOnline
              ? 'bg-canvas/60 hover:bg-canvas/90 border-onyx/5'
              : isConnecting
                ? 'bg-amber-500/10 hover:bg-amber-500/15 border-amber-500/20'
                : 'bg-red-500/10 hover:bg-red-500/15 border-red-500/20'
          }`}
          title={statusLabel}
        >
          <div className="flex items-center gap-2 text-caption">
            <span className="relative flex h-2 w-2">
              {isOnline && (
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
              )}
              {isConnecting && (
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75" />
              )}
              <span
                className={`relative inline-flex rounded-full h-2 w-2 ${
                  isOnline ? 'bg-emerald-500' : isConnecting ? 'bg-amber-500' : 'bg-red-500'
                }`}
              />
            </span>
            <span className="font-mono font-medium text-deep-ink text-[12px]">{statusLabel}</span>
          </div>

          <span
            className={`text-[10px] font-mono uppercase px-1.5 py-0.5 rounded-md ${
              isOnline
                ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400 font-semibold'
                : isConnecting
                  ? 'bg-amber-500/15 text-amber-700 dark:text-amber-400 font-semibold'
                  : 'bg-red-500/15 text-red-700 font-semibold'
            }`}
          >
            {isOnline ? 'WS' : connection}
          </span>
        </button>
      )}

      {/* Telemetry Popover */}
      {isOpen && (
        <div
          className={`absolute p-4 bg-canvas/95 backdrop-blur-md border border-onyx/15 rounded-2xl shadow-xl z-50 animate-fade-in text-deep-ink font-sans space-y-3 ${
            placement === 'sidebar'
              ? compact
                ? 'left-full bottom-0 ml-3 w-72'
                : 'bottom-full left-0 mb-2 w-full min-w-[260px]'
              : 'right-0 top-full mt-2 w-72'
          }`}
        >
          {/* Header */}
          <div className="flex items-center justify-between border-b border-onyx/10 pb-2.5">
            <div className="flex items-center gap-2 font-serif font-semibold text-body-sm">
              <Activity className="w-4 h-4 text-deep-ink" />
              <span>{t('common:telemetry.systemLoad', 'System Telemetry')}</span>
            </div>
            <span
              className={`text-[10px] font-mono uppercase px-2 py-0.5 rounded-full ${
                isOnline
                  ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400 font-semibold'
                  : 'bg-red-500/15 text-red-700 font-semibold'
              }`}
            >
              {isOnline
                ? t('common:telemetry.wsConnected', 'Connected')
                : t('common:telemetry.wsDisconnected', 'Offline')}
            </span>
          </div>

          {/* Metrics Grid */}
          <div className="space-y-2 text-caption">
            {/* CPU */}
            <div className="flex items-center justify-between">
              <span className="text-slate flex items-center gap-1.5">
                <Cpu className="w-3.5 h-3.5" />
                CPU
              </span>
              <span className="font-mono font-medium">
                {metrics?.cpu?.usage_percent ? `${metrics.cpu.usage_percent.toFixed(1)}%` : '—'}
              </span>
            </div>

            {/* RAM */}
            <div className="flex items-center justify-between">
              <span className="text-slate flex items-center gap-1.5">
                <HardDrive className="w-3.5 h-3.5" />
                RAM
              </span>
              <span className="font-mono font-medium">
                {metrics?.memory?.used_mb ? `${metrics.memory.used_mb} MB` : '—'}
              </span>
            </div>

            {/* Realtime Stream */}
            <div className="flex items-center justify-between">
              <span className="text-slate flex items-center gap-1.5">
                {isOnline ? (
                  <Wifi className="w-3.5 h-3.5 text-emerald-600" />
                ) : (
                  <WifiOff className="w-3.5 h-3.5 text-red-500" />
                )}
                Realtime Stream
              </span>
              <span className="font-mono font-medium">{isOnline ? '120 FPS / WS' : 'Disconnected'}</span>
            </div>
          </div>

          {/* Reconnect Action if Offline */}
          {!isOnline && (
            <div className="pt-2 border-t border-onyx/10">
              <button
                type="button"
                onClick={handleReconnect}
                className="w-full py-1.5 rounded-xl bg-deep-ink text-white text-caption font-semibold flex items-center justify-center gap-1.5 hover:opacity-90 transition-opacity cursor-pointer shadow-xs"
              >
                <RefreshCw className="w-3.5 h-3.5" />
                <span>{t('common:telemetry.reconnect', 'Reconnect Now')}</span>
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
