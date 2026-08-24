import {
  createContext,
  useContext,
  useState,
  useCallback,
  useRef,
  useEffect,
  type ReactNode,
} from 'react';
import { isApprovalRequired } from '@/lib/types';

export interface ActionStep {
  id: string;
  label: string;
  description?: string;
  status?: 'pending' | 'running' | 'success' | 'error';
}

export interface ActionItem {
  id: string;
  targetId: string;
  title: string;
  subtitle?: string;
  steps: ActionStep[];
  currentStepIndex: number;
  progressPercent?: number;
  status: 'running' | 'waiting_approval' | 'success' | 'error';
  error?: string | null;
  createdAt: number;
  approvalId?: string;
  onSuccess?: (result?: unknown) => void | Promise<void>;
  onError?: (err: unknown) => void;
}

export interface ExecuteActionOptions<T> {
  targetId?: string;
  title: string;
  subtitle?: string;
  steps: { id: string; label: string; description?: string }[];
  action: () => Promise<T>;
  onSuccess?: (result?: T) => void | Promise<void>;
  onError?: (err: unknown) => void;
  autoCloseDelay?: number;
}

interface ActionProgressContextValue {
  actions: ActionItem[];
  activeAction: ActionItem | null;
  activeIndex: number;
  setActiveIndex: (index: number) => void;
  nextAction: () => void;
  prevAction: () => void;
  executeAction: <T>(options: ExecuteActionOptions<T>) => Promise<string>;
  dismissAction: (id: string) => void;
  clearAll: () => void;
  isExecuting: (targetId?: string) => boolean;
}

const ActionProgressContext = createContext<ActionProgressContextValue | null>(null);

export function ActionProgressProvider({ children }: { children: ReactNode }) {
  const [actions, setActions] = useState<ActionItem[]>([]);
  const [activeIndex, setActiveIndex] = useState(0);

  const autoDismissTimersRef = useRef<Map<string, number>>(new Map());

  // Clamped activeIndex whenever actions change
  useEffect(() => {
    if (actions.length === 0) {
      setActiveIndex(0);
    } else if (activeIndex >= actions.length) {
      setActiveIndex(actions.length - 1);
    }
  }, [actions.length, activeIndex]);

  const dismissAction = useCallback((id: string) => {
    const timer = autoDismissTimersRef.current.get(id);
    if (timer) {
      window.clearTimeout(timer);
      autoDismissTimersRef.current.delete(id);
    }
    setActions((prev) => prev.filter((a) => a.id !== id));
  }, []);

  const clearAll = useCallback(() => {
    autoDismissTimersRef.current.forEach((t) => window.clearTimeout(t));
    autoDismissTimersRef.current.clear();
    setActions([]);
    setActiveIndex(0);
  }, []);

  const scheduleAutoDismiss = useCallback(
    (id: string, delay = 4000) => {
      const existing = autoDismissTimersRef.current.get(id);
      if (existing) window.clearTimeout(existing);
      const timer = window.setTimeout(() => {
        dismissAction(id);
      }, delay);
      autoDismissTimersRef.current.set(id, timer);
    },
    [dismissAction]
  );

  const executeAction = useCallback(
    async <T,>(options: ExecuteActionOptions<T>): Promise<string> => {
      const actionId = `act_${Date.now()}_${Math.random().toString(36).slice(2, 7)}`;
      const targetId = options.targetId || options.title;

      const initialSteps: ActionStep[] = options.steps.map((s, idx) => ({
        ...s,
        status: idx === 0 ? 'running' : 'pending',
      }));

      const newAction: ActionItem = {
        id: actionId,
        targetId,
        title: options.title,
        subtitle: options.subtitle,
        steps: initialSteps,
        currentStepIndex: 0,
        progressPercent: 10,
        status: 'running',
        createdAt: Date.now(),
        onSuccess: options.onSuccess ? (r?: unknown) => options.onSuccess?.(r as T) : undefined,
        onError: options.onError,
      };

      setActions((prev) => {
        const next = [...prev, newAction];
        setActiveIndex(next.length - 1);
        return next;
      });

      try {
        const result = await options.action();

        if (isApprovalRequired(result)) {
          const approval = result.approval;
          const approvalId = approval?.id;

          setActions((prev) =>
            prev.map((a) =>
              a.id === actionId
                ? {
                  ...a,
                  status: 'waiting_approval',
                  approvalId,
                  steps: a.steps.map((s, idx) =>
                    idx === 0
                      ? { ...s, status: 'running', description: 'Waiting for operator authorization' }
                      : s
                  ),
                }
                : a
            )
          );

          // Broadcast event to trigger the central ApprovalInterruption modal
          window.dispatchEvent(
            new CustomEvent('actonos:approval-required', { detail: approval })
          );
          return actionId;
        }

        // Successfully executed without approval
        setActions((prev) =>
          prev.map((a) =>
            a.id === actionId
              ? {
                ...a,
                status: 'success',
                currentStepIndex: a.steps.length > 0 ? a.steps.length - 1 : 0,
                progressPercent: 100,
                steps: a.steps.map((s) => ({ ...s, status: 'success' })),
              }
              : a
          )
        );

        window.dispatchEvent(new CustomEvent('actonos:tools-updated'));

        if (options.onSuccess) {
          await options.onSuccess(result);
        }

        scheduleAutoDismiss(actionId, options.autoCloseDelay ?? 4000);
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err);
        setActions((prev) =>
          prev.map((a) =>
            a.id === actionId
              ? {
                ...a,
                status: 'error',
                error: msg,
                steps: a.steps.map((s) => ({ ...s, status: 'error' })),
              }
              : a
          )
        );
        if (options.onError) {
          options.onError(err);
        }
      }

      return actionId;
    },
    [scheduleAutoDismiss]
  );

  // Listen to Backend Realtime Progress Events ('skill.progress', 'plugin.progress', etc.)
  useEffect(() => {
    const handleBackendProgress = (event: Event) => {
      const detail = (event as CustomEvent<{
        type: string;
        agent_id?: string;
        payload: {
          skill_id?: string;
          plugin_id?: string;
          step?: string;
          message?: string;
          progress?: number;
          current_file?: string;
          file_index?: number;
          total_files?: number;
        };
      }>).detail;

      if (!detail?.payload) return;
      const { skill_id, plugin_id, message, progress } = detail.payload;
      const targetEntityId = plugin_id || skill_id;

      setActions((prev) =>
        prev.map((a) => {
          if (
            (targetEntityId && (a.targetId === targetEntityId || a.title.toLowerCase().includes(targetEntityId.toLowerCase()))) &&
            (a.status === 'running' || a.status === 'waiting_approval')
          ) {
            let nextIndex = a.currentStepIndex;
            if (progress !== undefined) {
              if (progress >= 80) nextIndex = Math.min(2, a.steps.length - 1);
              else if (progress >= 20) nextIndex = Math.min(1, a.steps.length - 1);
            }

            return {
              ...a,
              status: progress === 100 ? 'success' : a.status,
              progressPercent: progress ?? a.progressPercent,
              currentStepIndex: nextIndex,
              steps: a.steps.map((s, idx) => {
                if (idx === nextIndex) {
                  return { ...s, status: progress === 100 ? 'success' : 'running', description: message || s.description };
                }
                if (idx < nextIndex) {
                  return { ...s, status: 'success' };
                }
                return s;
              }),
            };
          }
          return a;
        })
      );
    };

    window.addEventListener('actonos:skill-progress', handleBackendProgress);
    window.addEventListener('actonos:plugin-progress', handleBackendProgress);
    return () => {
      window.removeEventListener('actonos:skill-progress', handleBackendProgress);
      window.removeEventListener('actonos:plugin-progress', handleBackendProgress);
    };
  }, []);

  // Listen to Global Approval Decisions from ApprovalInterruption
  useEffect(() => {
    const handleApprovalDecided = async (event: Event) => {
      const detail = (event as CustomEvent<{ id: string; approved: boolean; tool: string }>).detail;
      if (!detail) return;

      setActions((prev) =>
        prev.map((a) => {
          if (
            a.status === 'waiting_approval' &&
            (!a.approvalId || a.approvalId === detail.id || detail.tool.includes(a.targetId))
          ) {
            if (detail.approved) {
              if (a.onSuccess) {
                setTimeout(() => {
                  a.onSuccess?.();
                }, 100);
              }
              scheduleAutoDismiss(a.id, 4000);
              return {
                ...a,
                status: 'success',
                progressPercent: 100,
                currentStepIndex: a.steps.length > 0 ? a.steps.length - 1 : 0,
                steps: a.steps.map((s) => ({ ...s, status: 'success' })),
              };
            } else {
              return {
                ...a,
                status: 'error',
                error: 'Action was rejected by operator.',
                steps: a.steps.map((s) => ({ ...s, status: 'error' })),
              };
            }
          }
          return a;
        })
      );
    };

    window.addEventListener('actonos:approval-decided', handleApprovalDecided);
    return () => {
      window.removeEventListener('actonos:approval-decided', handleApprovalDecided);
    };
  }, [scheduleAutoDismiss]);

  const nextAction = useCallback(() => {
    setActiveIndex((prev) => Math.min(actions.length - 1, prev + 1));
  }, [actions.length]);

  const prevAction = useCallback(() => {
    setActiveIndex((prev) => Math.max(0, prev - 1));
  }, []);

  const isExecuting = useCallback(
    (targetId?: string) => {
      if (targetId) {
        return actions.some(
          (a) =>
            (a.targetId === targetId || a.id === targetId) &&
            (a.status === 'running' || a.status === 'waiting_approval')
        );
      }
      return actions.some((a) => a.status === 'running' || a.status === 'waiting_approval');
    },
    [actions]
  );

  const activeAction = actions[activeIndex] || null;

  return (
    <ActionProgressContext.Provider
      value={{
        actions,
        activeAction,
        activeIndex,
        setActiveIndex,
        nextAction,
        prevAction,
        executeAction,
        dismissAction,
        clearAll,
        isExecuting,
      }}
    >
      {children}
    </ActionProgressContext.Provider>
  );
}

export function useActionProgress() {
  const context = useContext(ActionProgressContext);
  if (!context) {
    throw new Error('useActionProgress must be used within an ActionProgressProvider');
  }
  return context;
}
