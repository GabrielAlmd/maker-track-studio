import { useEffect, useId, useMemo, useRef, useState } from 'react';
import { Check, ChevronDown } from 'lucide-react';
import { useTranslation } from 'react-i18next';

const filamentOptionLabel = (filament) =>
  [filament?.brand, filament?.type].filter(Boolean).join(' ') +
  (filament?.color ? ` — ${filament.color}` : '');

const sortFilaments = (filaments = []) =>
  [...(filaments || [])].sort((left, right) => {
    const labelComparison = filamentOptionLabel(left).localeCompare(
      filamentOptionLabel(right),
      undefined,
      { sensitivity: 'base', numeric: true },
    );
    return labelComparison || String(left.id).localeCompare(String(right.id));
  });

export default function FilamentSelect({
  id,
  value = '',
  onChange,
  filaments = [],
  'aria-label': ariaLabel,
  'aria-describedby': ariaDescribedBy,
  invalid = false,
  disabled = false,
  className = '',
}) {
  const { t } = useTranslation();
  const generatedId = useId();
  const inputId = id || `filament-select-${generatedId}`;
  const listboxId = `${inputId}-options`;
  const containerRef = useRef(null);
  const inputRef = useRef(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [activeIndex, setActiveIndex] = useState(-1);

  const sortedFilaments = useMemo(() => sortFilaments(filaments), [filaments]);
  const selected = sortedFilaments.find((filament) => filament.id === value);
  const selectedLabel = selected ? filamentOptionLabel(selected) : '';
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const filteredFilaments = useMemo(
    () => sortedFilaments.filter((filament) =>
      filamentOptionLabel(filament).toLocaleLowerCase().includes(normalizedQuery)),
    [normalizedQuery, sortedFilaments],
  );

  useEffect(() => {
    setActiveIndex((current) => Math.min(current, filteredFilaments.length - 1));
  }, [filteredFilaments.length]);

  const showOptions = () => {
    if (disabled) return;
    setQuery('');
    setOpen(true);
    setActiveIndex(-1);
  };

  const closeOptions = () => {
    setOpen(false);
    setQuery('');
    setActiveIndex(-1);
  };

  const selectFilament = (filament) => {
    onChange(filament.id);
    closeOptions();
    inputRef.current?.focus();
  };

  const handleKeyDown = (event) => {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      if (!open) showOptions();
      setActiveIndex((current) => Math.min(current + 1, filteredFilaments.length - 1));
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      if (!open) showOptions();
      setActiveIndex((current) => Math.max(current - 1, 0));
    } else if (event.key === 'Enter' && open && activeIndex >= 0) {
      event.preventDefault();
      selectFilament(filteredFilaments[activeIndex]);
    } else if (event.key === 'Escape' && open) {
      event.preventDefault();
      closeOptions();
    }
  };

  return (
    <div
      ref={containerRef}
      className={`relative ${className}`}
      onBlur={(event) => {
        if (!containerRef.current?.contains(event.relatedTarget)) closeOptions();
      }}
    >
      <div className="relative">
        <input
          ref={inputRef}
          id={inputId}
          role="combobox"
          type="text"
          autoComplete="off"
          aria-label={ariaLabel}
          aria-autocomplete="list"
          aria-expanded={open}
          aria-controls={listboxId}
          aria-activedescendant={open && activeIndex >= 0 ? `${listboxId}-${activeIndex}` : undefined}
          aria-describedby={ariaDescribedBy}
          aria-invalid={invalid || undefined}
          value={open ? query : selectedLabel}
          placeholder={t('purchases.selectFilament')}
          disabled={disabled}
          onFocus={() => { if (!open) showOptions(); }}
          onChange={(event) => {
            setQuery(event.target.value);
            setOpen(true);
            setActiveIndex(0);
          }}
          onKeyDown={handleKeyDown}
          className="w-full border ui-border-default rounded-md pl-3 pr-9 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring disabled:opacity-50"
        />
        <button
          type="button"
          tabIndex={-1}
          aria-label={t('filamentSelect.showOptions')}
          disabled={disabled}
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => {
            if (open) closeOptions();
            else {
              showOptions();
              inputRef.current?.focus();
            }
          }}
          className="absolute inset-y-0 right-0 flex items-center px-2 ui-text-muted disabled:opacity-50"
        >
          <ChevronDown className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>
      {open && (
        <div
          id={listboxId}
          role="listbox"
          className="absolute z-50 mt-1 max-h-56 w-full overflow-auto rounded-md border ui-border-default ui-surface-card py-1 shadow-lg"
        >
          {filteredFilaments.length === 0 ? (
            <p className="px-3 py-2 text-sm ui-text-muted">{t('filamentSelect.noResults')}</p>
          ) : filteredFilaments.map((filament, index) => {
            const label = filamentOptionLabel(filament);
            return (
              <button
                id={`${listboxId}-${index}`}
                key={filament.id}
                type="button"
                role="option"
                aria-selected={filament.id === value}
                onMouseDown={(event) => event.preventDefault()}
                onMouseEnter={() => setActiveIndex(index)}
                onClick={() => selectFilament(filament)}
                className={`flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm ${activeIndex === index ? 'ui-surface-hover' : ''}`}
              >
                <span>{label}</span>
                {filament.id === value && <Check className="h-4 w-4 shrink-0" aria-hidden="true" />}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
