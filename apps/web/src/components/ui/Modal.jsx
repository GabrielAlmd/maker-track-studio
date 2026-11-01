import { useEffect, useId, useRef } from 'react';
import { createPortal } from 'react-dom';
import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';

/**
 * Modal — renders via React Portal directly on document.body.
 *
 * This ensures the backdrop and panel always sit above every stacking context
 * in the app (sidebars, scroll containers, flex layouts), regardless of where
 * in the component tree the modal is declared.
 *
 * Props:
 *   title    — string shown in the header
 *   onClose  — called when backdrop or × button is clicked
 *   children — form content; rendered inside the scrollable panel body
 */
function Modal({ title, onClose, children, size = 'default' }) {
  const { t } = useTranslation();
  const titleId = useId();
  const onCloseRef = useRef(onClose);
  const dialogRef = useRef(null);

  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  // Lock body scroll while open so the page doesn't scroll behind the backdrop.
  useEffect(() => {
    const previous = document.body.style.overflow;
    const previousFocus = document.activeElement;
    document.body.style.overflow = 'hidden';
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') {
        event.stopPropagation();
        onCloseRef.current();
      }
      if (event.key !== 'Tab') return;
      const focusable = [...dialogRef.current.querySelectorAll('a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])')]
        .filter((element) => !element.hidden && getComputedStyle(element).display !== 'none' && getComputedStyle(element).visibility !== 'hidden');
      if (!focusable.length) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && (document.activeElement === first || !dialogRef.current.contains(document.activeElement))) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && (document.activeElement === last || !dialogRef.current.contains(document.activeElement))) {
        event.preventDefault();
        first.focus();
      }
    };
    dialogRef.current?.querySelector('.ui-modal-body input:not([disabled]), .ui-modal-body select:not([disabled]), .ui-modal-body textarea:not([disabled]), .ui-modal-body button:not([disabled])')?.focus();
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.body.style.overflow = previous;
      document.removeEventListener('keydown', handleKeyDown);
      previousFocus?.focus?.();
    };
  }, []);

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto p-4 sm:p-8">
      {/* Semi-transparent backdrop — clicking it closes the modal */}
      <div
        className="ui-modal-backdrop fixed inset-0"
        aria-hidden="true"
        onClick={onClose}
      />

      {/* Panel — sits above the backdrop via relative positioning + same z-index layer */}
      <div ref={dialogRef} className={`ui-modal relative z-10 my-auto flex max-h-[calc(100dvh-2rem)] w-full flex-col ${size === 'wide' ? 'max-w-xl' : 'max-w-md'}`} role="dialog" aria-modal="true" aria-labelledby={titleId}>
        {/* Header */}
        <div className="ui-modal-header flex items-center justify-between border-b px-6 py-4">
          <h2 id={titleId} className="text-lg font-semibold">{title}</h2>
          <button
            type="button"
            onClick={onClose}
            className="ui-icon-button p-1 transition-colors"
            aria-label={t('common.close')}
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Body — form content goes here */}
        <div className="ui-modal-body min-h-0 overflow-y-auto px-6 py-5">{children}</div>
      </div>
    </div>,
    document.body,
  );
}

export default Modal;
