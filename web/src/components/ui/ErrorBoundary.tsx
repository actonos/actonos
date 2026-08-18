import { Component, ErrorInfo, ReactNode } from 'react';
import { AlertTriangle, RefreshCw, Home } from 'lucide-react';
import { Button } from './Button';
import i18n from '@/lib/i18n';

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
  errorInfo: ErrorInfo | null;
}

export class ErrorBoundary extends Component<Props, State> {
  public state: State = {
    hasError: false,
    error: null,
    errorInfo: null,
  };

  public static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error, errorInfo: null };
  }

  public componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('Uncaught error caught by ErrorBoundary:', error, errorInfo);
    this.setState({ errorInfo });
  }

  private handleReset = () => {
    this.setState({ hasError: false, error: null, errorInfo: null });
    window.location.reload();
  };

  private handleHome = () => {
    this.setState({ hasError: false, error: null, errorInfo: null });
    window.location.href = '/';
  };

  public render() {
    if (this.state.hasError) {
      if (this.props.fallback) {
        return this.props.fallback;
      }

      return (
        <div className="min-h-screen flex items-center justify-center p-6 bg-slate-900 text-slate-100">
          <div className="max-w-md w-full bg-slate-800/80 border border-emerald-500/20 rounded-3xl p-8 backdrop-blur-xl shadow-2xl text-center">
            <div className="w-16 h-16 bg-emerald-500/10 border border-emerald-500/30 rounded-2xl flex items-center justify-center mx-auto mb-6 text-emerald-400">
              <AlertTriangle className="w-8 h-8" />
            </div>

            <h1 className="text-2xl font-bold tracking-tight text-white mb-2">
              {i18n.t('common:errorBoundary.title')}
            </h1>
            <p className="text-sm text-slate-400 mb-6">
              {i18n.t('common:errorBoundary.description')}
            </p>

            {this.state.error && (
              <div className="bg-slate-950/60 border border-slate-700/50 rounded-xl p-3 text-left mb-6 overflow-x-auto">
                <code className="text-xs text-rose-300 font-mono">
                  {this.state.error.toString()}
                </code>
              </div>
            )}

            <div className="flex gap-3 justify-center">
              <Button
                variant="secondary"
                onClick={this.handleHome}
                className="flex items-center gap-2"
              >
                <Home className="w-4 h-4" />
                {i18n.t('common:errorBoundary.home')}
              </Button>
              <Button
                variant="primary"
                onClick={this.handleReset}
                className="flex items-center gap-2"
              >
                <RefreshCw className="w-4 h-4" />
                {i18n.t('common:errorBoundary.reload')}
              </Button>
            </div>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
