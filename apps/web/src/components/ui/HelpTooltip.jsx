import { useEffect, useId, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { CircleHelp } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export default function HelpTooltip({ label, children }) {
  const { t } = useTranslation();
  const id = useId();
  const trigger = useRef(null);
  const tooltip = useRef(null);
  const [position, setPosition] = useState(null);

  const show = () => {
    const rect = trigger.current.getBoundingClientRect();
    const width = Math.min(320, window.innerWidth - 24);
    setPosition({
      left: Math.max(12, Math.min(rect.left, window.innerWidth - width - 12)),
      width,
      ...(rect.bottom < window.innerHeight / 2
        ? { top: rect.bottom }
        : { bottom: window.innerHeight - rect.top }),
    });
  };

  useEffect(() => {
    if (!position) return;
    const dismiss = () => setPosition(null);
    const onKeyDown = (event) => { if (event.key === 'Escape') dismiss(); };
    const onPointerDown = (event) => {
      if (!trigger.current?.contains(event.target) && !tooltip.current?.contains(event.target)) dismiss();
    };
    window.addEventListener('keydown', onKeyDown);
    window.addEventListener('pointerdown', onPointerDown);
    window.addEventListener('resize', dismiss);
    window.addEventListener('scroll', dismiss, true);
    return () => {
      window.removeEventListener('keydown', onKeyDown);
      window.removeEventListener('pointerdown', onPointerDown);
      window.removeEventListener('resize', dismiss);
      window.removeEventListener('scroll', dismiss, true);
    };
  }, [position]);

  const leave = (event) => {
    const enteringTooltip = event.relatedTarget instanceof Node && tooltip.current?.contains(event.relatedTarget);
    if (!enteringTooltip && document.activeElement !== trigger.current) setPosition(null);
  };

  return <>
    <button
      ref={trigger}
      type="button"
      aria-label={t('common.helpFor', { label })}
      aria-describedby={position ? id : undefined}
      onMouseEnter={show}
      onMouseLeave={leave}
      onFocus={show}
      onBlur={() => setPosition(null)}
      onClick={show}
      className="ui-icon-button ml-2 inline-flex align-middle rounded-full p-1"
    >
      <CircleHelp className="h-4 w-4" aria-hidden="true" />
    </button>
    {position && createPortal(
      <div ref={tooltip} style={position} className="fixed z-50 py-2" onMouseLeave={leave}>
        <div id={id} role="tooltip" className="ui-tooltip px-3 py-2 text-sm font-normal">
          {children}
        </div>
      </div>, document.body,
    )}
  </>;
}
