import { useTranslation } from 'react-i18next';
import { Globe } from 'lucide-react';

export function LanguageSwitcher() {
  const { i18n, t } = useTranslation('common');

  const toggleLanguage = () => {
    const nextLang = i18n.language.startsWith('vi') ? 'en' : 'vi';
    i18n.changeLanguage(nextLang);
  };

  return (
    <button
      onClick={toggleLanguage}
      className="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-full bg-soft-meadow text-deep-ink hover:bg-canvas transition-colors text-body-sm font-sans font-medium cursor-pointer border border-transparent hover:border-soft-meadow"
      title={t('language.toggle')}
    >
      <Globe className="w-4 h-4 text-slate" />
      <span className="uppercase font-semibold text-caption">{i18n.language.slice(0, 2)}</span>
    </button>
  );
}
