import { useState, type FormEvent } from 'react';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { useToast } from '@/components/ui/Toast';
import { api } from '@/lib/api';
import { Lock, Sparkles, Key, Eye, EyeOff, ShieldAlert } from 'lucide-react';

export interface LoginPageProps {
  userName?: string;
  onAuthenticated: () => void;
}

export function LoginPage({ userName, onAuthenticated }: LoginPageProps) {
  const { success, error } = useToast();

  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loginError, setLoginError] = useState<string | null>(null);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!password.trim()) return;

    try {
      setLoading(true);
      setLoginError(null);
      const res = await api.login(password);
      if (res.token) {
        localStorage.setItem('actonos_token', res.token);
      }
      success('Kernel Unlocked', `Welcome back, ${userName || 'Operator'}.`);
      onAuthenticated();
    } catch (err: any) {
      setLoginError(err.message || 'Incorrect administrator password');
      error('Access Denied', err.message || 'Invalid administrator password.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen bg-canvas flex flex-col items-center justify-center p-4 relative overflow-hidden">
      <BlobBackdrop />

      <div className="w-full max-w-md z-10">
        {/* Lock Screen Emblem & Logo */}
        <div className="text-center mb-8">
          <img
            src="/actonos_logo.png"
            alt="ActonOS"
            className="h-12 w-auto mx-auto mb-5 object-contain"
          />
          <h1 className="font-serif text-heading text-deep-ink mb-1 tracking-tight flex items-center justify-center gap-2">
            <Lock className="w-5 h-5 text-deep-ink" />
            <span>Appliance Locked</span>
          </h1>
          <p className="font-sans text-body-sm text-slate">
            {userName ? `Operator Session: ${userName}` : 'Hardware Node & REST API Secured'}
          </p>
        </div>

        <Card className="p-8 border border-onyx/15 shadow-sm bg-soft-meadow">
          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="text-caption uppercase text-slate font-semibold block mb-1.5 flex items-center gap-1.5">
                <Key className="w-3.5 h-3.5" /> Administrator Password
              </label>
              <div className="relative">
                <input
                  type={showPassword ? 'text' : 'password'}
                  required
                  autoFocus
                  value={password}
                  onChange={(e) => {
                    setPassword(e.target.value);
                    if (loginError) setLoginError(null);
                  }}
                  placeholder="Enter administrator password..."
                  className={`w-full bg-canvas text-deep-ink px-4 py-3 pr-11 rounded-full border font-sans text-body-sm focus:outline-none focus:ring-2 ${
                    loginError
                      ? 'border-red-500 focus:ring-red-400'
                      : 'border-onyx/15 focus:ring-deep-ink'
                  }`}
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-3.5 top-1/2 -translate-y-1/2 text-slate hover:text-deep-ink cursor-pointer"
                >
                  {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                </button>
              </div>
            </div>

            {loginError && (
              <div className="flex items-center gap-2 p-3 bg-red-50 border border-red-200 rounded-2xl text-[12px] text-red-700 font-sans">
                <ShieldAlert className="w-4 h-4 shrink-0" />
                <span>{loginError}</span>
              </div>
            )}

            <div className="pt-3">
              <Button
                type="submit"
                variant="primary"
                size="md"
                disabled={!password.trim() || loading}
                icon={<Sparkles className="w-4 h-4" />}
                className="w-full justify-center py-3 font-semibold text-body-sm shadow-xs"
              >
                {loading ? 'Authenticating...' : 'Unlock ActonOS Kernel'}
              </Button>
            </div>
          </form>
        </Card>

        {/* Footer Note */}
        <p className="text-center text-caption text-slate mt-6 font-mono">
          Hardware Security Vault • AES-256-GCM + SHA-256 Session Token
        </p>
      </div>
    </div>
  );
}
