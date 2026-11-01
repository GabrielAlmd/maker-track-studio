import { useId, useMemo, useRef, useState } from 'react';
import { Check, ChevronDown } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export default function ModelSelect({ value = '', onChange, models = [], emptyLabel, 'aria-label': ariaLabel }) {
  const { t } = useTranslation();
  const id = useId();
  const listboxId = `model-options-${id}`;
  const containerRef = useRef(null);
  const inputRef = useRef(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [activeIndex, setActiveIndex] = useState(-1);
  const sortedModels = useMemo(() => [...models].sort((a, b) =>
    a.name.localeCompare(b.name, undefined, { sensitivity: 'base', numeric: true }) ||
    String(a.id).localeCompare(String(b.id))), [models]);
  const selected = sortedModels.find((model) => model.id === value);
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const options = normalizedQuery
    ? sortedModels.filter((model) => model.name.toLocaleLowerCase().includes(normalizedQuery))
    : [{ id: '', name: emptyLabel ?? t('printJobs.noModel') }, ...sortedModels];

  const close = () => {
    setOpen(false);
    setQuery('');
    setActiveIndex(-1);
  };

  const select = (model) => {
    onChange(model.id);
    close();
    inputRef.current?.focus();
  };

  return (
    <div ref={containerRef} className="relative" onBlur={(event) => {
      if (!containerRef.current?.contains(event.relatedTarget)) close();
    }}>
      <div className="relative">
        <input
          ref={inputRef}
          role="combobox"
          type="text"
          autoComplete="off"
          aria-label={ariaLabel}
          aria-autocomplete="list"
          aria-expanded={open}
          aria-controls={listboxId}
          aria-activedescendant={open && activeIndex >= 0 && options[activeIndex] ? `${listboxId}-${activeIndex}` : undefined}
          value={open ? query : selected?.name ?? ''}
          placeholder={t('modelSelect.placeholder')}
          onFocus={() => { if (!open) { setOpen(true); setQuery(''); setActiveIndex(-1); } }}
          onClick={() => { if (!open) { setOpen(true); setQuery(''); setActiveIndex(-1); } }}
          onChange={(event) => { setQuery(event.target.value); setOpen(true); setActiveIndex(0); }}
          onKeyDown={(event) => {
            if (event.key === 'ArrowDown') {
              event.preventDefault();
              setOpen(true);
              setActiveIndex((index) => Math.min(index + 1, options.length - 1));
            } else if (event.key === 'ArrowUp') {
              event.preventDefault();
              setOpen(true);
              setActiveIndex((index) => Math.max(index - 1, 0));
            } else if (event.key === 'Enter' && open && options[activeIndex]) {
              event.preventDefault();
              select(options[activeIndex]);
            } else if (event.key === 'Escape' && open) {
              event.preventDefault();
              close();
            }
          }}
          className="w-full border ui-border-default rounded-md pl-3 pr-9 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring"
        />
        <button type="button" tabIndex={-1} aria-label={t('modelSelect.showOptions')}
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => {
            if (open) close();
            else { setOpen(true); setQuery(''); setActiveIndex(-1); inputRef.current?.focus(); }
          }}
          className="absolute inset-y-0 right-0 flex items-center px-2 ui-text-muted">
          <ChevronDown className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>
      {open && <div id={listboxId} role="listbox"
        className="absolute z-50 mt-1 max-h-56 w-full overflow-auto rounded-md border ui-border-default ui-surface-card py-1 shadow-lg">
        {options.length === 0
          ? <p className="px-3 py-2 text-sm ui-text-muted">{t('modelSelect.noResults')}</p>
          : options.map((model, index) => <button
            id={`${listboxId}-${index}`} key={model.id || 'none'} type="button" role="option"
            aria-selected={model.id === value}
            onMouseDown={(event) => event.preventDefault()}
            onMouseEnter={() => setActiveIndex(index)}
            onClick={() => select(model)}
            className={`flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm ${activeIndex === index ? 'ui-surface-hover' : ''}`}>
            <span>{model.name}</span>
            {model.id === value && <Check className="h-4 w-4 shrink-0" aria-hidden="true" />}
          </button>)}
      </div>}
    </div>
  );
}
