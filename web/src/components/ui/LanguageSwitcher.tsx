import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Globe, ChevronDown } from 'lucide-react';
import { LanguageSelectModal, SUPPORTED_LANGUAGES } from './LanguageSelectModal';

export interface LanguageSwitcherProps {
  onClick?: () => void;
  className?: string;
  compact?: boolean;
}

export function LanguageSwitcher({ onClick, className = '', compact = false }: LanguageSwitcherProps) {
  const { i18n, t } = useTranslation('common');
  const [isModalOpen, setIsModalOpen] = useState(false);

  const currentCode = (i18n.language || 'en').split('-')[0].toLowerCase();
  const currentLang = SUPPORTED_LANGUAGES.find((l) => l.code === currentCode) || SUPPORTED_LANGUAGES[0];

  const handleClick = () => {
    if (onClick) {
      onClick();
    } else {
      setIsModalOpen(true);
    }
  };

  return (
    <>
      <button
        onClick={handleClick}
        className={
          compact
            ? `w-9 h-9 p-0 rounded-full bg-soft-meadow text-deep-ink hover:bg-canvas transition-all flex items-center justify-center cursor-pointer border border-onyx/10 hover:border-onyx/20 shadow-2xs text-sm shrink-0 ${className}`
            : `inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-soft-meadow text-deep-ink hover:bg-canvas transition-all text-body-sm font-sans font-medium cursor-pointer border border-onyx/10 hover:border-onyx/20 shadow-2xs shrink-0 ${className}`
        }
        title={`${t('language.toggle', 'Change System Language')}: ${currentLang.nativeName}`}
      >
        {compact ? (
          <span className="text-caption font-semibold select-none leading-none" aria-label={currentLang.name}>
            {currentLang.shortCode}
          </span>
        ) : (
          <>
            <Globe className="w-3.5 h-3.5 text-slate" />
            <span className="text-caption font-semibold uppercase">{currentCode}</span>
            <ChevronDown className="w-3 h-3 text-slate opacity-60 ml-0.5" />
          </>
        )}
      </button>

      {!onClick && (
        <LanguageSelectModal
          isOpen={isModalOpen}
          onClose={() => setIsModalOpen(false)}
        />
      )}
    </>
  );
}
