import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from './Modal';
import { Check, Globe, Search } from 'lucide-react';

export interface LanguageOption {
  code: string;
  name: string;
  nativeName: string;
  region: string;
  shortCode: string;
  coverage: '100%' | 'Core';
  isDefault?: boolean;
}

export const SUPPORTED_LANGUAGES: LanguageOption[] = [
  {
    code: 'en',
    name: 'English',
    nativeName: 'English (US)',
    region: 'International',
    shortCode: 'EN',
    coverage: '100%',
    isDefault: true,
  },
  {
    code: 'vi',
    name: 'Vietnamese',
    nativeName: 'Tiếng Việt',
    region: 'Vietnam',
    shortCode: 'VI',
    coverage: '100%',
  },
];

export interface LanguageSelectModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export function LanguageSelectModal({ isOpen, onClose }: LanguageSelectModalProps) {
  const { i18n, t } = useTranslation('common');
  const [search, setSearch] = useState('');

  const currentLangCode = (i18n.language || 'en').split('-')[0].toLowerCase();

  const handleSelectLanguage = (code: string) => {
    i18n.changeLanguage(code);
    localStorage.setItem('i18nextLng', code);
    onClose();
  };

  const filtered = SUPPORTED_LANGUAGES.filter(
    (lang) =>
      lang.name.toLowerCase().includes(search.toLowerCase()) ||
      lang.nativeName.toLowerCase().includes(search.toLowerCase()) ||
      lang.region.toLowerCase().includes(search.toLowerCase()) ||
      lang.code.toLowerCase().includes(search.toLowerCase())
  );

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('language.selectTitle')}
      maxWidth="max-w-xl"
    >
      <div className="space-y-4">
        {/* Search Input */}
        <div className="relative">
          <Search className="w-4 h-4 text-slate absolute left-3.5 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            placeholder={t('language.searchPlaceholder')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full bg-soft-meadow text-deep-ink pl-10 pr-4 py-2.5 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink/20"
            autoFocus
          />
        </div>

        {/* Language Grid / List */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-2.5 max-h-[380px] overflow-y-auto pr-1">
          {filtered.map((lang) => {
            const isSelected = currentLangCode === lang.code;
            return (
              <button
                key={lang.code}
                onClick={() => handleSelectLanguage(lang.code)}
                className={`flex items-center justify-between p-3.5 rounded-2xl border text-left transition-all cursor-pointer select-none ${
                  isSelected
                    ? 'bg-deep-ink text-white border-deep-ink shadow-xs'
                    : 'bg-soft-meadow hover:bg-canvas hover:border-onyx/20 border-onyx/5 text-deep-ink'
                }`}
              >
                <div className="flex items-center gap-3 min-w-0">
                  <span className="text-xl shrink-0" role="img" aria-label={lang.name}>
                    {lang.shortCode}
                  </span>
                  <div className="min-w-0">
                    <div className="font-sans font-semibold text-body-sm truncate flex items-center gap-1.5">
                      <span>{lang.nativeName}</span>
                    </div>
                    <div
                      className={`text-caption font-sans truncate ${
                        isSelected ? 'text-white/70' : 'text-slate'
                      }`}
                    >
                      {lang.name} • {lang.region}
                    </div>
                  </div>
                </div>

                <div className="flex items-center gap-2 shrink-0 ml-2">
                  {lang.coverage === '100%' && (
                    <span
                      className={`text-[10px] font-mono px-2 py-0.5 rounded-full font-semibold ${
                        isSelected
                          ? 'bg-hi-yellow text-deep-ink'
                          : 'bg-emerald-100 text-emerald-800'
                      }`}
                    >
                      {t('language.fullUI')}
                    </span>
                  )}
                  {isSelected && <Check className="w-4 h-4 text-hi-yellow" />}
                </div>
              </button>
            );
          })}
        </div>

        {filtered.length === 0 && (
          <div className="py-8 text-center text-slate font-sans text-body-sm">
            {t('language.noMatches', { search })}
          </div>
        )}

        {/* Footer info */}
        <div className="pt-3 border-t border-onyx/10 flex items-center justify-between text-caption font-mono text-slate">
          <span className="flex items-center gap-1.5">
            <Globe className="w-3.5 h-3.5 text-deep-ink" />
            <span>{t('language.active', { language: SUPPORTED_LANGUAGES.find((l) => l.code === currentLangCode)?.nativeName || currentLangCode })}</span>
          </span>
          <span className="text-[11px] text-slate/70">{t('language.engine')}</span>
        </div>
      </div>
    </Modal>
  );
}
