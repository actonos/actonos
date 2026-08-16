import { useTranslation } from 'react-i18next';
import { LanguageSwitcher } from '@/components/ui/LanguageSwitcher';
import { Button } from '@/components/ui/Button';
import { Plus, Radio } from 'lucide-react';

export type NavTab = 'agents' | 'tools' | 'workspace' | 'integrations' | 'chat' | 'settings';

export interface NavbarProps {
  activeTab: NavTab;
  onSelectTab: (tab: NavTab) => void;
  onCreateAgent?: () => void;
}

export function Navbar({ activeTab, onSelectTab, onCreateAgent }: NavbarProps) {
  const { t } = useTranslation('nav');

  const navItems: { id: NavTab; label: string }[] = [
    { id: 'agents', label: t('links.agents') },
    { id: 'tools', label: t('links.toolHub') },
    { id: 'workspace', label: t('links.workspace') },
    { id: 'integrations', label: t('links.integrations') },
    { id: 'chat', label: t('links.chat') },
    { id: 'settings', label: t('links.settings') },
  ];

  return (
    <header className="w-full bg-soft-meadow border-b border-soft-meadow sticky top-0 z-40">
      <div className="max-w-[1200px] mx-auto px-4 md:px-8 h-18 flex items-center justify-between">
        {/* Logo lockup */}
        <div className="flex items-center cursor-pointer select-none" onClick={() => onSelectTab('agents')}>
          <img
            src="/actonos_logo.png"
            alt="ActonOS"
            className="h-9 md:h-10 w-auto object-contain"
          />
        </div>

        {/* Center navigation capsule */}
        <nav className="hidden lg:flex items-center bg-canvas px-2 py-1.5 rounded-full border border-onyx/10">
          {navItems.map((item) => (
            <button
              key={item.id}
              onClick={() => onSelectTab(item.id)}
              className={`px-4 py-1.5 rounded-full text-body-sm font-sans font-medium transition-all cursor-pointer ${
                activeTab === item.id
                  ? 'bg-deep-ink text-white font-semibold'
                  : 'text-deep-ink hover:text-slate'
              }`}
            >
              {item.label}
            </button>
          ))}
        </nav>

        {/* Right CTA / Language controls */}
        <div className="flex items-center gap-3">
          <div className="hidden xl:flex items-center gap-1.5 text-caption uppercase text-slate font-medium px-3 py-1 bg-canvas rounded-full border border-onyx/10">
            <Radio className="w-3.5 h-3.5 text-emerald-600 animate-pulse" />
            <span>{t('cta.statusOnline')}</span>
          </div>

          <LanguageSwitcher />

          {onCreateAgent && (
            <Button
              variant="primary"
              size="sm"
              icon={<Plus className="w-4 h-4" />}
              onClick={onCreateAgent}
            >
              {t('cta.createAgent')}
            </Button>
          )}
        </div>
      </div>
    </header>
  );
}
