import { useEffect, useId, useMemo, useRef, useState } from 'react';
import { Check, ChevronDown, Plus } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { normalizeBrand, uniqueBrands } from '../utils/brands';

export default function BrandSelect({
  id,
  value = '',
  onChange,
  onBlur,
  brands = [],
  invalid = false,
  disabled = false,
  className = '',
}) {
  const { t } = useTranslation();
  const generatedId = useId();
  const inputId = id || `brand-select-${generatedId}`;
  const listboxId = `${inputId}-options`;
  const containerRef = useRef(null);
  const inputRef = useRef(null);
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);
  const options = useMemo(() => uniqueBrands(brands), [brands]);
  const query = value || '';
  const normalizedQuery = normalizeBrand(query);
  const filteredBrands = useMemo(
    () => options.filter((brand) => normalizeBrand(brand).includes(normalizedQuery)),
    [normalizedQuery, options],
  );
  const hasExactMatch = options.some((brand) => normalizeBrand(brand) === normalizedQuery);
  const canCreate = normalizedQuery.length > 0 && !hasExactMatch;
  const optionCount = filteredBrands.length + (canCreate ? 1 : 0);

  useEffect(() => {
    setActiveIndex((current) => Math.min(current, optionCount - 1));
  }, [optionCount]);

  const closeOptions = () => {
    setOpen(false);
    setActiveIndex(-1);
  };

  const selectBrand = (brand) => {
    onChange(brand);
    closeOptions();
    inputRef.current?.focus();
  };

  const selectActiveOption = () => {
    if (activeIndex < filteredBrands.length) selectBrand(filteredBrands[activeIndex]);
    else if (canCreate) selectBrand(query.trim());
  };

  const handleKeyDown = (event) => {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      setOpen(true);
      setActiveIndex((current) => Math.min(current + 1, optionCount - 1));
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      setOpen(true);
      setActiveIndex((current) => Math.max(current - 1, 0));
    } else if (event.key === 'Enter' && open && activeIndex >= 0) {
      event.preventDefault();
      selectActiveOption();
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
        if (!containerRef.current?.contains(event.relatedTarget)) {
          closeOptions();
          onBlur?.();
        }
      }}
    >
      <div className="relative">
        <input
          ref={inputRef}
          id={inputId}
          role="combobox"
          type="text"
          autoComplete="off"
          aria-autocomplete="list"
          aria-expanded={open}
          aria-controls={listboxId}
          aria-activedescendant={open && activeIndex >= 0 ? `${listboxId}-${activeIndex}` : undefined}
          aria-invalid={invalid || undefined}
          value={query}
          placeholder={t('brandSelect.placeholder')}
          disabled={disabled}
          onFocus={() => setOpen(true)}
          onChange={(event) => {
            onChange(event.target.value);
            setOpen(true);
            setActiveIndex(0);
          }}
          onKeyDown={handleKeyDown}
          className="w-full border ui-border-default rounded-md pl-3 pr-9 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring disabled:opacity-50"
        />
        <button
          type="button"
          tabIndex={-1}
          aria-label={t('brandSelect.showOptions')}
          disabled={disabled}
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => {
            setOpen((current) => !current);
            inputRef.current?.focus();
          }}
          className="absolute inset-y-0 right-0 flex items-center px-2 ui-text-muted disabled:opacity-50"
        >
          <ChevronDown className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>
      {open && (
        <div id={listboxId} role="listbox" className="absolute z-50 mt-1 max-h-56 w-full overflow-auto rounded-md border ui-border-default ui-surface-card py-1 shadow-lg">
          {filteredBrands.map((brand, index) => (
            <button
              id={`${listboxId}-${index}`}
              key={brand}
              type="button"
              role="option"
              aria-selected={normalizeBrand(brand) === normalizeBrand(query)}
              onMouseDown={(event) => event.preventDefault()}
              onMouseEnter={() => setActiveIndex(index)}
              onClick={() => selectBrand(brand)}
              className={`flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm ${activeIndex === index ? 'ui-surface-hover' : ''}`}
            >
              <span>{brand}</span>
              {normalizeBrand(brand) === normalizeBrand(query) && <Check className="h-4 w-4 shrink-0" aria-hidden="true" />}
            </button>
          ))}
          {canCreate && (
            <button
              id={`${listboxId}-${filteredBrands.length}`}
              type="button"
              role="option"
              aria-selected="false"
              onMouseDown={(event) => event.preventDefault()}
              onMouseEnter={() => setActiveIndex(filteredBrands.length)}
              onClick={() => selectBrand(query.trim())}
              className={`flex w-full items-center gap-2 px-3 py-2 text-left text-sm ${activeIndex === filteredBrands.length ? 'ui-surface-hover' : ''}`}
            >
              <Plus className="h-4 w-4 shrink-0" aria-hidden="true" />
              <span>{t('brandSelect.create', { brand: query.trim() })}</span>
            </button>
          )}
          {optionCount === 0 && <p className="px-3 py-2 text-sm ui-text-muted">{t('brandSelect.noBrands')}</p>}
        </div>
      )}
    </div>
  );
}
