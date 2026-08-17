import { useState, type FormEvent } from 'react';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { useToast } from '@/components/ui/Toast';
import { api } from '@/lib/api';
import {
  Sparkles,
  ShieldCheck,
  ArrowRight,
  ArrowLeft,
  Key,
  Globe,
  User,
  Clock,
  Eye,
  EyeOff,
  CheckCircle2,
} from 'lucide-react';

export interface SetupWizardPageProps {
  onCompleted: () => void;
}

export function SetupWizardPage({ onCompleted }: SetupWizardPageProps) {
  const { success, error } = useToast();

  const [step, setStep] = useState<1 | 2 | 3>(1);
  const [loading, setLoading] = useState(false);

  // Form State
  const [userName, setUserName] = useState('Operator');
  const [userRole, setUserRole] = useState('System Architect & Lead Developer');
  const [language, setLanguage] = useState<'en' | 'vi'>('en');
  const [timezone, setTimezone] = useState('Asia/Ho_Chi_Minh');
  const [customInstructions, setCustomInstructions] = useState(
    'Provide intelligent, natural, and empathetic responses. Act as a trusted senior engineering partner. Proactively solve problems and avoid robotic or stiff clichés.'
  );

  // Password State
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);

  const handleNextFromStep1 = (e: FormEvent) => {
    e.preventDefault();
    if (!userName.trim()) {
      error('Validation Error', 'Please enter your name.');
      return;
    }
    setStep(2);
  };

  const handleNextFromStep2 = (e: FormEvent) => {
    e.preventDefault();
    if (password.length < 4) {
      error('Password Too Short', 'Master password must be at least 4 characters long.');
      return;
    }
    if (password !== confirmPassword) {
      error('Passwords Do Not Match', 'Please ensure both passwords match.');
      return;
    }
    setStep(3);
  };

  const handleFinishSetup = async () => {
    try {
      setLoading(true);
      const res = await api.setupInitialAdmin({
        password,
        user_name: userName,
        user_role: userRole,
        language,
        timezone,
        custom_instructions: customInstructions,
      });

      if (res.token) {
        localStorage.setItem('actonos_token', res.token);
      }
      success('ActonOS Initialized', 'Welcome to your autonomous AI operating system.');
      onCompleted();
    } catch (err: any) {
      error('Initialization Failed', err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen bg-canvas flex flex-col items-center justify-center p-4 relative overflow-hidden">
      <BlobBackdrop />

      <div className="w-full max-w-xl z-10">
        {/* Top Branding Header */}
        <div className="text-center mb-8">
          <img
            src="/actonos_logo.png"
            alt="ActonOS"
            className="h-12 w-auto mx-auto mb-5 object-contain"
          />
          <h1 className="font-serif text-heading text-deep-ink mb-1 tracking-tight">
            Kernel Appliance Initialization
          </h1>
          <p className="font-sans text-body-sm text-slate">
            Hardware-Bound AI Agent Operating System Kernel Setup
          </p>

          {/* Stepper Dots */}
          <div className="flex items-center justify-center gap-2.5 mt-6">
            {[1, 2, 3].map((s) => (
              <div
                key={s}
                className={`h-2 rounded-full transition-all duration-300 ${
                  step === s
                    ? 'w-8 bg-deep-ink'
                    : step > s
                      ? 'w-4 bg-hi-yellow border border-deep-ink'
                      : 'w-2 bg-onyx/20'
                }`}
              />
            ))}
          </div>
        </div>

        {/* Step 1: Owner Identity */}
        {step === 1 && (
          <Card className="p-8 border border-onyx/15 shadow-sm bg-soft-meadow">
            <div className="flex items-center gap-2 border-b border-onyx/10 pb-4 mb-6">
              <User className="w-5 h-5 text-deep-ink" />
              <h2 className="font-serif text-subheading font-semibold text-deep-ink">
                Step 1: Operator Identity & Regional Settings
              </h2>
            </div>

            <form onSubmit={handleNextFromStep1} className="space-y-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <label className="text-caption uppercase text-slate font-semibold block mb-1.5">
                    Your Name / Handle
                  </label>
                  <input
                    type="text"
                    required
                    value={userName}
                    onChange={(e) => setUserName(e.target.value)}
                    placeholder="e.g. Alex, Operator"
                    className="w-full bg-canvas text-deep-ink px-4 py-2.5 rounded-full border border-onyx/15 font-sans text-body-sm focus:outline-none focus:ring-2 focus:ring-deep-ink"
                  />
                </div>

                <div>
                  <label className="text-caption uppercase text-slate font-semibold block mb-1.5">
                    Role & Title
                  </label>
                  <input
                    type="text"
                    value={userRole}
                    onChange={(e) => setUserRole(e.target.value)}
                    placeholder="e.g. System Architect"
                    className="w-full bg-canvas text-deep-ink px-4 py-2.5 rounded-full border border-onyx/15 font-sans text-body-sm focus:outline-none focus:ring-2 focus:ring-deep-ink"
                  />
                </div>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-1">
                <div>
                  <label className="text-caption uppercase text-slate font-semibold block mb-1.5 flex items-center gap-1.5">
                    <Globe className="w-3.5 h-3.5" /> Language
                  </label>
                  <select
                    value={language}
                    onChange={(e) => setLanguage(e.target.value as 'en' | 'vi')}
                    className="w-full bg-canvas text-deep-ink px-4 py-2.5 rounded-full border border-onyx/15 font-sans text-body-sm focus:outline-none focus:ring-2 focus:ring-deep-ink"
                  >
                    <option value="en">English (US)</option>
                    <option value="vi">Tiếng Việt (Vietnam)</option>
                  </select>
                </div>

                <div>
                  <label className="text-caption uppercase text-slate font-semibold block mb-1.5 flex items-center gap-1.5">
                    <Clock className="w-3.5 h-3.5" /> Timezone
                  </label>
                  <select
                    value={timezone}
                    onChange={(e) => setTimezone(e.target.value)}
                    className="w-full bg-canvas text-deep-ink px-4 py-2.5 rounded-full border border-onyx/15 font-sans text-body-sm focus:outline-none focus:ring-2 focus:ring-deep-ink"
                  >
                    <option value="Asia/Ho_Chi_Minh">Asia/Ho_Chi_Minh (UTC+7)</option>
                    <option value="UTC">UTC (Universal)</option>
                    <option value="America/New_York">America/New_York (EST)</option>
                    <option value="America/Los_Angeles">America/Los_Angeles (PST)</option>
                    <option value="Europe/London">Europe/London (GMT)</option>
                    <option value="Asia/Tokyo">Asia/Tokyo (JST)</option>
                    <option value="Asia/Singapore">Asia/Singapore (SGT)</option>
                  </select>
                </div>
              </div>

              <div className="pt-2">
                <label className="text-caption uppercase text-slate font-semibold block mb-1.5">
                  Universal Operator Custom Directives
                </label>
                <textarea
                  rows={3}
                  value={customInstructions}
                  onChange={(e) => setCustomInstructions(e.target.value)}
                  className="w-full bg-canvas text-deep-ink p-3 rounded-2xl border border-onyx/15 font-sans text-body-sm focus:outline-none focus:ring-2 focus:ring-deep-ink resize-none leading-relaxed"
                />
              </div>

              <div className="pt-4 flex justify-end">
                <Button
                  type="submit"
                  variant="primary"
                  size="md"
                  icon={<ArrowRight className="w-4 h-4" />}
                  className="px-6 font-semibold"
                >
                  Continue to Security
                </Button>
              </div>
            </form>
          </Card>
        )}

        {/* Step 2: Master Admin Password */}
        {step === 2 && (
          <Card className="p-8 border border-onyx/15 shadow-sm bg-soft-meadow">
            <div className="flex items-center gap-2 border-b border-onyx/10 pb-4 mb-6">
              <Key className="w-5 h-5 text-deep-ink" />
              <h2 className="font-serif text-subheading font-semibold text-deep-ink">
                Step 2: Master Administrator Password
              </h2>
            </div>

            <p className="font-sans text-body-sm text-slate mb-6">
              Set a master password to secure your local hardware node, REST API endpoints, and web administration console.
            </p>

            <form onSubmit={handleNextFromStep2} className="space-y-4">
              <div>
                <label className="text-caption uppercase text-slate font-semibold block mb-1.5">
                  Master Password
                </label>
                <div className="relative">
                  <input
                    type={showPassword ? 'text' : 'password'}
                    required
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder="Enter strong admin password..."
                    className="w-full bg-canvas text-deep-ink px-4 py-2.5 pr-11 rounded-full border border-onyx/15 font-sans text-body-sm focus:outline-none focus:ring-2 focus:ring-deep-ink"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-3.5 top-1/2 -translate-y-1/2 text-slate hover:text-deep-ink"
                  >
                    {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
              </div>

              <div>
                <label className="text-caption uppercase text-slate font-semibold block mb-1.5">
                  Confirm Master Password
                </label>
                <input
                  type={showPassword ? 'text' : 'password'}
                  required
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  placeholder="Re-type password..."
                  className="w-full bg-canvas text-deep-ink px-4 py-2.5 rounded-full border border-onyx/15 font-sans text-body-sm focus:outline-none focus:ring-2 focus:ring-deep-ink"
                />
              </div>

              <div className="pt-6 flex items-center justify-between">
                <Button
                  type="button"
                  variant="ghost"
                  size="md"
                  onClick={() => setStep(1)}
                  icon={<ArrowLeft className="w-4 h-4" />}
                >
                  Back
                </Button>
                <Button
                  type="submit"
                  variant="primary"
                  size="md"
                  icon={<ArrowRight className="w-4 h-4" />}
                  className="px-6 font-semibold"
                >
                  Review & Boot
                </Button>
              </div>
            </form>
          </Card>
        )}

        {/* Step 3: Review & Finish */}
        {step === 3 && (
          <Card className="p-8 border border-onyx/15 shadow-sm bg-soft-meadow">
            <div className="flex items-center gap-2 border-b border-onyx/10 pb-4 mb-6">
              <ShieldCheck className="w-5 h-5 text-emerald-600" />
              <h2 className="font-serif text-subheading font-semibold text-deep-ink">
                Step 3: Review & Initialize Appliance
              </h2>
            </div>

            <div className="space-y-4 mb-8 bg-canvas p-5 rounded-2xl border border-onyx/10 text-body-sm">
              <div className="flex items-center justify-between border-b border-onyx/5 pb-2.5">
                <span className="text-slate">Operator Name:</span>
                <span className="font-semibold text-deep-ink">{userName}</span>
              </div>
              <div className="flex items-center justify-between border-b border-onyx/5 pb-2.5">
                <span className="text-slate">System Role:</span>
                <span className="font-medium text-deep-ink">{userRole}</span>
              </div>
              <div className="flex items-center justify-between border-b border-onyx/5 pb-2.5">
                <span className="text-slate">Interface Language:</span>
                <span className="font-medium text-deep-ink">{language === 'en' ? 'English (US)' : 'Tiếng Việt'}</span>
              </div>
              <div className="flex items-center justify-between border-b border-onyx/5 pb-2.5">
                <span className="text-slate">Timezone:</span>
                <span className="font-mono text-[12px] text-deep-ink">{timezone}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate">Admin Password:</span>
                <span className="flex items-center gap-1 text-emerald-700 font-semibold text-caption uppercase">
                  <CheckCircle2 className="w-3.5 h-3.5" /> Configured & Protected
                </span>
              </div>
            </div>

            <div className="flex items-center justify-between pt-2">
              <Button
                type="button"
                variant="ghost"
                size="md"
                onClick={() => setStep(2)}
                disabled={loading}
                icon={<ArrowLeft className="w-4 h-4" />}
              >
                Back
              </Button>
              <Button
                type="button"
                variant="primary"
                size="lg"
                onClick={handleFinishSetup}
                disabled={loading}
                icon={<Sparkles className="w-4 h-4" />}
                className="px-8 font-semibold"
              >
                {loading ? 'Initializing Kernel...' : 'Initialize & Launch ActonOS'}
              </Button>
            </div>
          </Card>
        )}
      </div>
    </div>
  );
}
