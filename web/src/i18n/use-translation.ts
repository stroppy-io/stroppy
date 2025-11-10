import { useTranslation as useI18nTranslation } from 'react-i18next'

export const useTranslation = (namespace?: string) => {
  const { t, i18n } = useI18nTranslation(namespace)

  const changeLanguage = (language: string) => {
    i18n.changeLanguage(language)
  }

  const getCurrentLanguage = () => i18n.language

  const getAvailableLanguages = () => [
    { code: 'ru', name: 'Русский', nativeName: 'Русский' },
    { code: 'en', name: 'English', nativeName: 'English' },
  ]

  return {
    t,
    changeLanguage,
    getCurrentLanguage,
    getAvailableLanguages,
    isReady: i18n.isInitialized,
  }
}
