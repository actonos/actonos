import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { LanguageSwitcher } from '@/components/ui/LanguageSwitcher';
import {
  Bot,
  MessageSquare,
  Wrench,
  Folder,
  Layers,
  Sliders,
  Menu,
  X,
} from 'lucide-react';

export type NavTab = 'agents' | 'chat' | 'tools' | 'workspace' | 'integrations' | 'settings';

export interface NavbarProps {
  activeTab: NavTab;
  onSelectTab: (tab: NavTab) => void;
}

export function Navbar({ activeTab, onSelectTab }: NavbarProps) {
  const { t } = useTranslation('nav');
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  const navItems: { id: NavTab; label: string; icon: React.ElementType }[] = [
    { id: 'agents', label: t('links.agents', 'Agents'), icon: Bot },
    { id: 'chat', label: t('links.chat', 'Chat'), icon: MessageSquare },
    { id: 'tools', label: t('links.toolHub', 'Tool Hub'), icon: Wrench },
    { id: 'workspace', label: t('links.workspace', 'Workspace'), icon: Folder },
    { id: 'integrations', label: t('links.integrations', 'Integrations'), icon: Layers },
    { id: 'settings', label: t('links.settings', 'Settings'), icon: Sliders },
  ];

  const handleSelect = (tab: NavTab) => {
    onSelectTab(tab);
    setMobileMenuOpen(false);
  };

  return (
    <header className="w-full bg-soft-meadow/90 backdrop-blur-md border-b border-onyx/10 sticky top-0 z-50">
      <div className="max-w-[1280px] mx-auto px-4 sm:px-6 lg:px-8 h-16 flex items-center justify-between">
        {/* Left: Logo */}
        <div
          className="flex items-center cursor-pointer select-none shrink-0"
          onClick={() => handleSelect('agents')}
        >
          <img
            src="/actonos_logo.png"
            alt="ActonOS"
            className="h-8 sm:h-9 w-auto object-contain"
          />
        </div>

        {/* Center: Desktop Navigation Capsule */}
        <nav className="hidden lg:flex items-center bg-canvas/80 p-1 rounded-full border border-onyx/10 shadow-xs">
          {navItems.map((item) => {
            const Icon = item.icon;
            const isActive = activeTab === item.id;
            return (
              <button
                key={item.id}
                onClick={() => handleSelect(item.id)}
                className={`flex items-center gap-1.5 px-3.5 py-1.5 rounded-full text-caption font-sans font-medium transition-all duration-150 cursor-pointer ${
                  isActive
                    ? 'bg-deep-ink text-white font-semibold shadow-xs'
                    : 'text-slate hover:text-deep-ink hover:bg-black/5'
                }`}
              >
                <Icon className={`w-3.5 h-3.5 ${isActive ? 'text-hi-yellow' : 'opacity-70'}`} />
                <span>{item.label}</span>
              </button>
            );
          })}
        </nav>

        {/* Right: Status Indicator, Language Switcher & Mobile Trigger */}
        <div className="flex items-center gap-2 sm:gap-3">
          {/* Status Indicator */}
          <div className="hidden sm:flex items-center gap-1.5 text-caption font-mono uppercase text-slate font-medium px-2.5 py-1 bg-canvas rounded-full border border-onyx/10">
            <span className="relative flex h-2 w-2">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
              <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
            </span>
            <span className="hidden xl:inline">{t('cta.statusOnline', 'Online')}</span>
          </div>

          <LanguageSwitcher />

          {/* Mobile Hamburger Button */}
          <button
            onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
            className="lg:hidden p-2 rounded-full bg-canvas border border-onyx/10 text-deep-ink hover:bg-white transition-colors"
            aria-label="Toggle navigation menu"
          >
            {mobileMenuOpen ? <X className="w-5 h-5" /> : <Menu className="w-5 h-5" />}
          </button>
        </div>
      </div>

      {/* Mobile Drawer Menu */}
      {mobileMenuOpen && (
        <div className="lg:hidden bg-soft-meadow border-b border-onyx/10 px-4 pt-2 pb-6 space-y-3 shadow-lg animate-in slide-in-from-top-2 duration-200">
          <div className="grid grid-cols-2 gap-2">
            {navItems.map((item) => {
              const Icon = item.icon;
              const isActive = activeTab === item.id;
              return (
                <button
                  key={item.id}
                  onClick={() => handleSelect(item.id)}
                  className={`flex items-center gap-2.5 p-3 rounded-[16px] text-body-sm font-medium transition-all ${
                    isActive
                      ? 'bg-deep-ink text-white font-semibold shadow-xs'
                      : 'bg-canvas text-deep-ink hover:bg-white border border-onyx/5'
                  }`}
                >
                  <Icon className={`w-4 h-4 ${isActive ? 'text-hi-yellow' : 'text-slate'}`} />
                  <span>{item.label}</span>
                </button>
              );
            })}
          </div>
        </div>
      )}
    </header>
  );
}
