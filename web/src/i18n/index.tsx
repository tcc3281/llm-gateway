import { createContext, useContext, useState, useEffect, ReactNode } from "react";
import { Locale, translations } from "./translations";

interface LanguageContextType {
  lang: Locale;
  setLang: (lang: Locale) => void;
  toggleLang: () => void;
  t: (key: string, fallback?: string) => string;
}

const LanguageContext = createContext<LanguageContextType | null>(null);

const STORAGE_KEY = "llm_gateway_lang";

export function LanguageProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Locale>(() => {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved === "en" || saved === "vi") {
      return saved;
    }
    return "vi"; // Default to Vietnamese for the user
  });

  const setLang = (nextLang: Locale) => {
    setLangState(nextLang);
    localStorage.setItem(STORAGE_KEY, nextLang);
  };

  const toggleLang = () => {
    const next = lang === "vi" ? "en" : "vi";
    setLang(next);
  };

  const t = (key: string, fallback?: string): string => {
    const dict = translations[lang] || translations.vi;
    if (dict && dict[key]) {
      return dict[key];
    }
    const fallbackDict = translations.en;
    if (fallbackDict && fallbackDict[key]) {
      return fallbackDict[key];
    }
    return fallback || key;
  };

  useEffect(() => {
    document.documentElement.lang = lang;
  }, [lang]);

  return (
    <LanguageContext.Provider value={{ lang, setLang, toggleLang, t }}>
      {children}
    </LanguageContext.Provider>
  );
}

export function useLanguage() {
  const ctx = useContext(LanguageContext);
  if (!ctx) {
    throw new Error("useLanguage must be used within a LanguageProvider");
  }
  return ctx;
}
