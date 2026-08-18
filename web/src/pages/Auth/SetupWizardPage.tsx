import { useState, type FormEvent } from 'react';
import { getErrorMessage } from '@/lib/errors';
import { useTranslation } from 'react-i18next';
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
  const { t } = useTranslation('setup');
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
      error(t('wizard.validationTitle'), t('wizard.nameRequired'));
      return;
    }
    setStep(2);
  };

  const handleNextFromStep2 = (e: FormEvent) => {
    e.preventDefault();
    if (password.length < 4) {
      error(t('wizard.passwordShortTitle'), t('wizard.passwordShort'));
      return;
    }
    if (password !== confirmPassword) {
      error(t('wizard.passwordMismatchTitle'), t('wizard.passwordMismatch'));
      return;
    }
    setStep(3);
  };

  const handleFinishSetup = async () => {
    try {
      setLoading(true);
      await api.setupInitialAdmin({
        password,
        user_name: userName,
        user_role: userRole,
        language,
        timezone,
        custom_instructions: customInstructions,
      });

      success(t('wizard.successTitle'), t('wizard.successDescription'));
      onCompleted();
    } catch (err) {
      error(t('wizard.failureTitle'), getErrorMessage(err));
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
            {t('wizard.title')}
          </h1>
          <p className="font-sans text-body-sm text-slate">
            {t('wizard.subtitle')}
          </p>

          {/* Stepper Dots */}
          <div className="flex items-center justify-center gap-2.5 mt-6" role="progressbar" aria-valuemin={1} aria-valuemax={3} aria-valuenow={step} aria-label={t('wizard.progress')}>
            {[1, 2, 3].map((s) => (
              <div
                key={s}
                aria-current={step === s ? 'step' : undefined}
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
                {t('wizard.identityTitle')}
              </h2>
            </div>

            <form onSubmit={handleNextFromStep1} className="space-y-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div>
                  <label className="text-caption uppercase text-slate font-semibold block mb-1.5">
                    {t('wizard.nameLabel')}
                  </label>
                  <input
                    type="text"
                    required
                    value={userName}
                    onChange={(e) => setUserName(e.target.value)}
                    placeholder={t('wizard.namePlaceholder')}
                    className="w-full bg-canvas text-deep-ink px-4 py-2.5 rounded-full border border-onyx/15 font-sans text-body-sm focus:outline-none focus:ring-2 focus:ring-deep-ink"
                  />
                </div>

                <div>
                  <label className="text-caption uppercase text-slate font-semibold block mb-1.5">
                    {t('wizard.roleLabel')}
                  </label>
                  <input
                    type="text"
                    value={userRole}
                    onChange={(e) => setUserRole(e.target.value)}
                    placeholder={t('wizard.rolePlaceholder')}
                    className="w-full bg-canvas text-deep-ink px-4 py-2.5 rounded-full border border-onyx/15 font-sans text-body-sm focus:outline-none focus:ring-2 focus:ring-deep-ink"
                  />
                </div>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-1">
                <div>
                  <label className="text-caption uppercase text-slate font-semibold block mb-1.5 flex items-center gap-1.5">
                    <Globe className="w-3.5 h-3.5" /> {t('wizard.languageLabel')}
                  </label>
                  <select
                    value={language}
                    onChange={(e) => setLanguage(e.target.value as 'en' | 'vi')}
                    className="w-full bg-canvas text-deep-ink px-4 py-2.5 rounded-full border border-onyx/15 font-sans text-body-sm focus:outline-none focus:ring-2 focus:ring-deep-ink"
                  >
                    <option value="en">{t('wizard.english')}</option>
                    <option value="vi">{t('wizard.vietnamese')}</option>
                  </select>
                </div>

                <div>
                  <label className="text-caption uppercase text-slate font-semibold block mb-1.5 flex items-center gap-1.5">
                    <Clock className="w-3.5 h-3.5" /> {t('wizard.timezoneLabel')}
                  </label>
                  <select
                    value={timezone}
                    onChange={(e) => setTimezone(e.target.value)}
                    className="w-full bg-canvas text-deep-ink px-4 py-2.5 rounded-full border border-onyx/15 font-sans text-body-sm focus:outline-none focus:ring-2 focus:ring-deep-ink"
                  >
                    <option value="Asia/Ho_Chi_Minh">{t('wizard.timezones.hoChiMinh')}</option>
                    <option value="UTC">{t('wizard.timezones.utc')}</option>
                    <option value="America/New_York">{t('wizard.timezones.newYork')}</option>
                    <option value="America/Los_Angeles">{t('wizard.timezones.losAngeles')}</option>
                    <option value="Europe/London">{t('wizard.timezones.london')}</option>
                    <option value="Asia/Tokyo">{t('wizard.timezones.tokyo')}</option>
                    <option value="Asia/Singapore">{t('wizard.timezones.singapore')}</option>
                  </select>
                </div>
              </div>

              <div className="pt-2">
                <label className="text-caption uppercase text-slate font-semibold block mb-1.5">
                  {t('wizard.directivesLabel')}
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
                  {t('wizard.continue')}
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
                {t('wizard.securityTitle')}
              </h2>
            </div>

            <p className="font-sans text-body-sm text-slate mb-6">
              {t('wizard.securityDescription')}
            </p>

            <form onSubmit={handleNextFromStep2} className="space-y-4">
              <div>
                <label className="text-caption uppercase text-slate font-semibold block mb-1.5">
                  {t('wizard.passwordLabel')}
                </label>
                <div className="relative">
                  <input
                    type={showPassword ? 'text' : 'password'}
                    required
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder={t('wizard.passwordPlaceholder')}
                    className="w-full bg-canvas text-deep-ink px-4 py-2.5 pr-11 rounded-full border border-onyx/15 font-sans text-body-sm focus:outline-none focus:ring-2 focus:ring-deep-ink"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    aria-label={showPassword ? t('login.hidePassword') : t('login.showPassword')}
                    className="absolute right-3.5 top-1/2 -translate-y-1/2 text-slate hover:text-deep-ink"
                  >
                    {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
              </div>

              <div>
                <label className="text-caption uppercase text-slate font-semibold block mb-1.5">
                  {t('wizard.confirmPasswordLabel')}
                </label>
                <input
                  type={showPassword ? 'text' : 'password'}
                  required
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  placeholder={t('wizard.confirmPasswordPlaceholder')}
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
                  {t('wizard.back')}
                </Button>
                <Button
                  type="submit"
                  variant="primary"
                  size="md"
                  icon={<ArrowRight className="w-4 h-4" />}
                  className="px-6 font-semibold"
                >
                  {t('wizard.review')}
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
                {t('wizard.reviewTitle')}
              </h2>
            </div>

            <div className="space-y-4 mb-8 bg-canvas p-5 rounded-2xl border border-onyx/10 text-body-sm">
              <div className="flex items-center justify-between border-b border-onyx/5 pb-2.5">
                <span className="text-slate">{t('wizard.reviewName')}</span>
                <span className="font-semibold text-deep-ink">{userName}</span>
              </div>
              <div className="flex items-center justify-between border-b border-onyx/5 pb-2.5">
                <span className="text-slate">{t('wizard.reviewRole')}</span>
                <span className="font-medium text-deep-ink">{userRole}</span>
              </div>
              <div className="flex items-center justify-between border-b border-onyx/5 pb-2.5">
                <span className="text-slate">{t('wizard.reviewLanguage')}</span>
                <span className="font-medium text-deep-ink">{language === 'en' ? t('wizard.english') : t('wizard.vietnamese')}</span>
              </div>
              <div className="flex items-center justify-between border-b border-onyx/5 pb-2.5">
                <span className="text-slate">{t('wizard.reviewTimezone')}</span>
                <span className="font-mono text-[12px] text-deep-ink">{timezone}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate">{t('wizard.reviewPassword')}</span>
                <span className="flex items-center gap-1 text-emerald-700 font-semibold text-caption uppercase">
                  <CheckCircle2 className="w-3.5 h-3.5" /> {t('wizard.protected')}
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
                {t('wizard.back')}
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
                {loading ? t('wizard.initializing') : t('wizard.launch')}
              </Button>
            </div>
          </Card>
        )}
      </div>
    </div>
  );
}
