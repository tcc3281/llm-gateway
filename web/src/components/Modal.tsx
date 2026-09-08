import { ReactNode } from "react";

export default function Modal({
  title,
  onClose,
  children,
  footer,
  maxWidth,
  style,
}: {
  title: ReactNode;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  maxWidth?: string | number;
  style?: React.CSSProperties;
}) {
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div
        className="modal"
        style={{ ...(maxWidth ? { maxWidth } : {}), ...style }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal__head">
          <span className="modal__title">{title}</span>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            ×
          </button>
        </div>
        <div className="modal__body">{children}</div>
        {footer && <div className="modal__foot">{footer}</div>}
      </div>
    </div>
  );
}
