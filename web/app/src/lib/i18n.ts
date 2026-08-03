import i18n from "i18next"
import { initReactI18next } from "react-i18next"

import en from "@/locales/en"
import zhCN from "@/locales/zh-CN"

const STORAGE_KEY = "carpool-locale"

export const supportedLanguages = [
  { code: "zh-CN", label: "中文" },
  { code: "en", label: "EN" },
] as const

function initialLanguage(): string {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored === "zh-CN" || stored === "en") {
      return stored
    }
  } catch {
    // localStorage unavailable — fall through to default.
  }
  return "zh-CN"
}

void i18n.use(initReactI18next).init({
  resources: {
    "zh-CN": zhCN,
    en,
  },
  lng: initialLanguage(),
  fallbackLng: "zh-CN",
  interpolation: {
    // React already escapes rendered strings.
    escapeValue: false,
  },
  returnObjects: true,
})

i18n.on("languageChanged", (language) => {
  try {
    localStorage.setItem(STORAGE_KEY, language)
  } catch {
    // Ignore storage failures (private mode etc.).
  }
  document.documentElement.lang = language
})

export default i18n
