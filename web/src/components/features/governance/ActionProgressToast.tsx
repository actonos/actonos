import { useTranslation } from 'react-i18next';
import {
  CheckCircle2,
  AlertTriangle,
  Loader2,
  XCircle,
  X,
  ChevronLeft,
  ChevronRight,
} from 'lucide-react';
import { useActionProgress } from '@/lib/useActionProgress';

export function ActionProgressToast() {
  const { t } = useTranslation('common');
  const {
    actions,
    activeAction,
    activeIndex,
    nextAction,
    prevAction,
    dismissAction,
  } = useActionProgress();

  if (!activeAction || actions.length === 0) return null;

  const isSuccess = activeAction.status === 'success';
  const isError = activeAction.status === 'error';
  const isWaitingApproval = activeAction.status === 'waiting_approval';
  const currentStep = activeAction.steps[activeAction.currentStepIndex] || activeAction.steps[0];
  const progressPercent = activeAction.progressPercent ?? 0;

  return (
    <div className="fixed top-[76px] right-6 z-40 max-w-sm w-full pointer-events-auto animate-in slide-in-from-top-2 duration-300">
      <div
        className={`p-4 rounded-[22px] border shadow-lg bg-canvas/95 backdrop-blur-md transition-all ${
          isSuccess
            ? 'border-emerald-500/40 bg-emerald-50/85 shadow-emerald-500/10'
            : isError
            ? 'border-red-500/40 bg-red-50/85 shadow-red-500/10'
            : isWaitingApproval
            ? 'border-amber-500/40 bg-amber-50/85 shadow-amber-500/10'
            : 'border-onyx/15 bg-canvas/95 shadow-onyx/5'
        }`}
      >
        {/* Header: Icon + Title + Multi-Action Carousel (< 1/3 >) + Close */}
        <div className="flex items-center justify-between gap-2 mb-2">
          <div className="flex items-center gap-2.5 min-w-0 flex-1">
            {isSuccess ? (
              <div className="w-7 h-7 rounded-full bg-emerald-100 flex items-center justify-center shrink-0">
                <CheckCircle2 className="w-4 h-4 text-emerald-600" />
              </div>
            ) : isError ? (
              <div className="w-7 h-7 rounded-full bg-red-100 flex items-center justify-center shrink-0">
                <XCircle className="w-4 h-4 text-red-600" />
              </div>
            ) : isWaitingApproval ? (
              <div className="w-7 h-7 rounded-full bg-amber-100 flex items-center justify-center shrink-0 animate-pulse">
                <AlertTriangle className="w-4 h-4 text-amber-600" />
              </div>
            ) : (
              <div className="w-7 h-7 rounded-full bg-soft-meadow flex items-center justify-center shrink-0 border border-onyx/10">
                <Loader2 className="w-4 h-4 text-deep-ink animate-spin" />
              </div>
            )}
            <div className="min-w-0 flex-1">
              <h4 className="font-serif text-body-sm font-bold text-deep-ink truncate">
                {activeAction.title}
              </h4>
            </div>
          </div>

          {/* Carousel Navigation Controls (Visible when multiple actions exist) */}
          <div className="flex items-center gap-1 shrink-0">
            {actions.length > 1 && (
              <div className="flex items-center gap-0.5 bg-onyx/5 px-1.5 py-0.5 rounded-full border border-onyx/10 mr-1">
                <button
                  type="button"
                  onClick={prevAction}
                  disabled={activeIndex === 0}
                  className="text-slate hover:text-deep-ink disabled:opacity-30 p-0.5 rounded-full transition-colors"
                  title="Previous action"
                >
                  <ChevronLeft className="w-3.5 h-3.5" />
                </button>
                <span className="font-mono text-[10px] font-semibold text-deep-ink px-1 select-none">
                  {activeIndex + 1}/{actions.length}
                </span>
                <button
                  type="button"
                  onClick={nextAction}
                  disabled={activeIndex === actions.length - 1}
                  className="text-slate hover:text-deep-ink disabled:opacity-30 p-0.5 rounded-full transition-colors"
                  title="Next action"
                >
                  <ChevronRight className="w-3.5 h-3.5" />
                </button>
              </div>
            )}

            <button
              type="button"
              onClick={() => dismissAction(activeAction.id)}
              className="text-slate hover:text-deep-ink p-1 rounded-full hover:bg-onyx/5 transition-colors"
              title="Dismiss"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>

        {/* Action Body & Real-Time Progress Indicator */}
        <div className="pl-9.5 pr-1 space-y-2">
          {isSuccess ? (
            <p className="text-caption text-emerald-800 font-medium">
              {t('actionProgress.completed', 'Completed successfully. State synchronized.')}
            </p>
          ) : isError ? (
            <p className="text-caption text-red-700 font-mono text-[11px] whitespace-pre-wrap">
              {activeAction.error || t('actionProgress.failed', 'Action execution failed')}
            </p>
          ) : isWaitingApproval ? (
            <div className="space-y-1">
              <p className="text-caption text-amber-900 font-semibold flex items-center gap-1.5">
                <AlertTriangle className="w-3.5 h-3.5 text-amber-600" />
                {t('approval.queuedTitle', 'Waiting for operator approval...')}
              </p>
              <p className="text-[11px] text-slate">
                {t('approval.queuedDescription', 'Please review and confirm in the approval dialog.')}
              </p>
            </div>
          ) : (
            <div className="space-y-1.5">
              <div className="flex items-center justify-between text-[11px] text-slate font-medium">
                <span className="truncate pr-2">
                  {currentStep?.description || currentStep?.label || 'Processing...'}
                </span>
                <span className="font-mono text-deep-ink shrink-0">
                  {progressPercent > 0 ? `${progressPercent}%` : `${activeAction.currentStepIndex + 1}/${activeAction.steps.length}`}
                </span>
              </div>

              {/* Real-time Continuous Progress Bar */}
              <div className="w-full bg-onyx/10 h-1.5 rounded-full overflow-hidden">
                <div
                  className="h-full bg-deep-ink rounded-full transition-all duration-300 ease-out"
                  style={{
                    width: `${Math.max(10, Math.min(100, progressPercent > 0 ? progressPercent : ((activeAction.currentStepIndex + 1) / activeAction.steps.length) * 100))}%`,
                  }}
                />
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
