import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Sparkles, Wifi, Key, Shield, CheckCircle } from 'lucide-react';

import { getAuthHeaders } from '@/lib/api';

export interface SetupWizardPageProps {
  onComplete: () => void;
}

export function SetupWizardPage({ onComplete }: SetupWizardPageProps) {
  const { t } = useTranslation('setup');

  const [wifiSSID, setWifiSSID] = useState('');
  const [wifiPassword, setWifiPassword] = useState('');
  const [anthropicKey, setAnthropicKey] = useState('');
  const [geminiKey, setGeminiKey] = useState('');
  const [openaiKey, setOpenAIKey] = useState('');
  const [tailscaleKey, setTailscaleKey] = useState('');
  const [adminPIN, setAdminPIN] = useState('');
  const [loading, setLoading] = useState(false);
  const [success, setSuccess] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setLoading(true);

    try {
      await fetch('/api/setup/wizard', {
        method: 'POST',
        headers: getAuthHeaders({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({
          wifi_ssid: wifiSSID,
          wifi_password: wifiPassword,
          anthropic_key: anthropicKey,
          gemini_key: geminiKey,
          openai_key: openaiKey,
          tailscale_key: tailscaleKey,
          admin_pin: adminPIN,
        }),
      });

      setSuccess(true);
      setTimeout(() => {
        onComplete();
      }, 1500);
    } catch (err) {
      console.error('Setup wizard failed:', err);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="relative min-h-screen flex items-center justify-center p-4">
      <BlobBackdrop />

      <PageContainer className="max-w-2xl py-6">
        <div className="text-center mb-8">
          <div className="w-14 h-14 rounded-full bg-deep-ink flex items-center justify-center text-hi-yellow mx-auto mb-3 shadow-md">
            <Sparkles className="w-7 h-7" />
          </div>
          <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight">
            {t('title')}
          </h1>
          <p className="font-sans text-body text-slate mt-2">
            {t('subtitle')}
          </p>
        </div>

        {success ? (
          <Card className="text-center py-12 border border-emerald-300 bg-emerald-50">
            <CheckCircle className="w-12 h-12 text-emerald-600 mx-auto mb-3" />
            <h3 className="font-serif text-heading-sm text-emerald-900 mb-2">Setup Completed!</h3>
            <p className="font-sans text-body-sm text-emerald-700">Connecting to network and booting kernel...</p>
          </Card>
        ) : (
          <form onSubmit={handleSubmit} className="flex flex-col gap-6">
            {/* Step 1: Wi-Fi */}
            <Card className="border border-onyx/10">
              <div className="flex items-center gap-2 mb-4">
                <Wifi className="w-5 h-5 text-deep-ink" />
                <h3 className="font-serif text-subheading text-deep-ink font-semibold">{t('wifi.title')}</h3>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <Input
                  label={t('wifi.ssid')}
                  placeholder="Home-WiFi-5G"
                  value={wifiSSID}
                  onChange={(e) => setWifiSSID(e.target.value)}
                />
                <Input
                  type="password"
                  label={t('wifi.password')}
                  placeholder="••••••••"
                  value={wifiPassword}
                  onChange={(e) => setWifiPassword(e.target.value)}
                />
              </div>
            </Card>

            {/* Step 2: LLM Keys */}
            <Card className="border border-onyx/10">
              <div className="flex items-center gap-2 mb-4">
                <Key className="w-5 h-5 text-deep-ink" />
                <h3 className="font-serif text-subheading text-deep-ink font-semibold">{t('llm.title')}</h3>
              </div>
              <div className="flex flex-col gap-3">
                <Input
                  type="password"
                  label={t('llm.anthropic')}
                  placeholder="sk-ant-api03-..."
                  value={anthropicKey}
                  onChange={(e) => setAnthropicKey(e.target.value)}
                />
                <Input
                  type="password"
                  label={t('llm.gemini')}
                  placeholder="AIzaSy..."
                  value={geminiKey}
                  onChange={(e) => setGeminiKey(e.target.value)}
                />
                <Input
                  type="password"
                  label={t('llm.openai')}
                  placeholder="sk-proj-..."
                  value={openaiKey}
                  onChange={(e) => setOpenAIKey(e.target.value)}
                />
              </div>
            </Card>

            {/* Step 3: Remote Access & PIN */}
            <Card className="border border-onyx/10">
              <div className="flex items-center gap-2 mb-4">
                <Shield className="w-5 h-5 text-deep-ink" />
                <h3 className="font-serif text-subheading text-deep-ink font-semibold">{t('remote.title')}</h3>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <Input
                  type="password"
                  label={t('remote.tailscale')}
                  placeholder="tskey-auth-..."
                  value={tailscaleKey}
                  onChange={(e) => setTailscaleKey(e.target.value)}
                />
                <Input
                  type="password"
                  label={t('remote.adminPin')}
                  placeholder="e.g. 1234"
                  value={adminPIN}
                  onChange={(e) => setAdminPIN(e.target.value)}
                />
              </div>
            </Card>

            <Button type="submit" variant="primary" size="lg" disabled={loading} className="w-full">
              {loading ? '...' : t('finish')}
            </Button>
          </form>
        )}
      </PageContainer>
    </div>
  );
}
