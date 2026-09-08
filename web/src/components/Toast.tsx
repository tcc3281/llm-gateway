import React, { createContext, useCallback, useContext, useMemo, useState } from "react";

export type ToastType = "info" | "warning" | "error" | "success";

export interface ToastItem {
  id: string;
  type: ToastType;
  message: string;
  codeTitle?: string;
}

interface ToastContextValue {
  show: (type: ToastType, message: string, codeTitle?: string) => void;
  info: (message: string, codeTitle?: string) => void;
  warning: (message: string, codeTitle?: string) => void;
  error: (message: string, codeTitle?: string) => void;
  success: (message: string, codeTitle?: string) => void;
  dismiss: (id: string) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

const DEFAULT_CODE_TITLES: Record<ToastType, string> = {
  info: "[INFO]",
  warning: "[WARNING]",
  error: "[ERROR]",
  success: "[200 OK]",
};

const ICONS: Record<ToastType, string> = {
  info: "info",
  warning: "warning",
  error: "error",
  success: "check_circle",
};

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  const dismiss = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const show = useCallback(
    (type: ToastType, message: string, codeTitle?: string) => {
      const id = `${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;
      const title = codeTitle || DEFAULT_CODE_TITLES[type];
      const newToast: ToastItem = { id, type, message, codeTitle: title };

      setToasts((prev) => [...prev.slice(-4), newToast]); // keep max 5 active

      setTimeout(() => {
        dismiss(id);
      }, 4500);
    },
    [dismiss]
  );

  const info = useCallback((msg: string, title?: string) => show("info", msg, title), [show]);
  const warning = useCallback((msg: string, title?: string) => show("warning", msg, title), [show]);
  const error = useCallback((msg: string, title?: string) => show("error", msg, title), [show]);
  const success = useCallback((msg: string, title?: string) => show("success", msg, title), [show]);

  const value = useMemo(
    () => ({ show, info, warning, error, success, dismiss }),
    [show, info, warning, error, success, dismiss]
  );

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="toast-stack" role="region" aria-live="polite" aria-label="Notifications">
        {toasts.map((t) => (
          <div key={t.id} className={`toast-item toast-item--${t.type}`}>
            <span className="material-symbols-outlined toast-item__icon">
              {ICONS[t.type]}
            </span>
            <div className="toast-item__content">
              {t.codeTitle && <div className="toast-item__code">{t.codeTitle}</div>}
              <div className="toast-item__msg">{t.message}</div>
            </div>
            <button
              type="button"
              className="toast-item__close"
              onClick={() => dismiss(t.id)}
              aria-label="Close notification"
            >
              <span className="material-symbols-outlined" style={{ fontSize: "16px" }}>
                close
              </span>
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    throw new Error("useToast must be used within a ToastProvider");
  }
  return ctx;
}
