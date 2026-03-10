import { type ReactNode, useEffect, useState } from 'react';

import { detectInitialLocale, I18nContext, LOCALE_STORAGE_KEY, translate, type Locale } from './i18n';

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocale] = useState<Locale>(() => detectInitialLocale());

  useEffect(() => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, locale);
    document.documentElement.lang = locale;
  }, [locale]);

  const value = {
    locale,
    setLocale,
    t: (key: Parameters<typeof translate>[1], params?: Parameters<typeof translate>[2]) => translate(locale, key, params),
  };

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}
