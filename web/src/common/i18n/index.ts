import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

// Import translation files
import commonRu from '../locales/ru/common.json'
import commonEn from '../locales/en/common.json'
import runsRu from '../locales/ru/runs.json'
import runsEn from '../locales/en/runs.json'
import dashboardRu from '../locales/ru/dashboard.json'
import dashboardEn from '../locales/en/dashboard.json'
import configuratorRu from '../locales/ru/configurator.json'
import configuratorEn from '../locales/en/configurator.json'
import landingRu from '../locales/ru/landing.json'
import landingEn from '../locales/en/landing.json'

const resources = {
  ru: {
    common: commonRu,
    runs: runsRu,
    dashboard: dashboardRu,
    configurator: configuratorRu,
    landing: landingRu,
  },
  en: {
    common: commonEn,
    runs: runsEn,
    dashboard: dashboardEn,
    configurator: configuratorEn,
    landing: landingEn,
  },
}

i18n
  .use(initReactI18next)
  .init({
    resources,
    lng: 'ru', // default language
    fallbackLng: 'en',
    debug: import.meta.env.DEV,
    
    interpolation: {
      escapeValue: false, // React already does escaping
    },
    
    // Namespace configuration
    defaultNS: 'common',
    ns: ['common', 'runs', 'dashboard', 'configurator', 'landing'],
    
    // Key separator
    keySeparator: '.',
    nsSeparator: ':',
  })

export default i18n
