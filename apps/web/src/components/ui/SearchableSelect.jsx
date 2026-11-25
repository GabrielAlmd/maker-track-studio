import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react';
import { Check, ChevronDown } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import useOptionPages from './useOptionPages';

const empty = [];
const optionId = option => option.id;
const optionName = option => option.name;
const normalize = text => String(text ?? '').trim().toLocaleLowerCase();

export default function SearchableSelect({
  id, ref, label, value, onChange, onBlur, options = empty, getKey = optionId, getLabel = optionName,
  getSearchText = getLabel, renderOption, placeholder, clearLabel, emptyLabel, toggleLabel,
  clearable = true, required, disabled, invalid, loading, error, className = '',
  createLabel, loadOptions, debounceMs = 250, ...input
}) {
  const { t } = useTranslation();
  const autoId = useId();
  const inputId = id ?? autoId;
  const listId = inputId + '-list';
  const inputRef = useRef(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [active, setActive] = useState(-1);
  const [retained, setRetained] = useState(null);
  const pages = useOptionPages(loadOptions, open && !disabled, query, debounceMs);
  const source = loadOptions ? pages.items : options;
  const selected = source.find(option => getKey(option) === value) ??
    (loadOptions && options.find(option => getKey(option) === value)) ??
    (retained && getKey(retained) === value ? retained : null);
  const selectedLabel = createLabel ? value ?? '' : selected == null ? '' : getLabel(selected);
  const expanded = open && !disabled;
  const choices = useMemo(() => {
    if (!expanded) return empty;
    const search = normalize(query);
    const unique = loadOptions ? Array.from(new Map(source.map(option => [getKey(option), option])).values()) : source;
    const rows = unique.filter(option => loadOptions || normalize(getSearchText(option)).includes(search))
      .map(option => ({ key: getKey(option), label: getLabel(option), option }));
    if (!search && clearable && (clearLabel || value)) rows.unshift({ key: '', label: clearLabel || t('selector.clear') });
    if (createLabel && search && !rows.some(row => normalize(row.label) === search)) {
      rows.push({ key: query.trim(), label: createLabel(query.trim()) });
    }
    return rows;
  }, [expanded, source, query, loadOptions, getSearchText, getKey, getLabel, clearable, clearLabel, value, createLabel, t]);
  const activeId = expanded && choices[active] ? listId + '-' + active : undefined;
  useEffect(() => {
    if (activeId) document.getElementById(activeId)?.scrollIntoView?.({ block: 'nearest' });
  }, [activeId]);

  const attachRef = useCallback(node => {
    inputRef.current = node;
    if (typeof ref === 'function') ref(node);
    else if (ref) ref.current = node;
  }, [ref]);

  function show() {
    if (open || disabled) return;
    setQuery(createLabel ? value ?? '' : '');
    setActive(-1);
    setOpen(true);
  }
  function select(choice) {
    setRetained(choice.option ?? null);
    onChange(choice.key);
    inputRef.current?.focus();
    setOpen(false);
  }
  function keyDown(event) {
    if (event.nativeEvent.isComposing) return;
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      show();
      setActive(index => Math.max(0, Math.min(choices.length - 1, index + (event.key === 'ArrowDown' ? 1 : -1))));
    } else if (expanded && event.key === 'Enter') {
      event.preventDefault();
      if (choices[active]) select(choices[active]);
    } else if (expanded && event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      setOpen(false);
    }
  }
  return <div className={'relative min-w-0 ' + className} onBlur={event => {
    if (!event.currentTarget.contains(event.relatedTarget)) {
      setOpen(false);
      onBlur?.();
    }
  }}>
    {label && <label htmlFor={inputId}>{label}</label>}
    <div className="relative">
      <input {...input} id={inputId} ref={attachRef}
        type="text" role="combobox" autoComplete="off" aria-autocomplete="list" disabled={disabled}
        aria-expanded={expanded} aria-controls={expanded ? listId : undefined} aria-activedescendant={activeId}
        aria-required={required || undefined} aria-invalid={invalid || input['aria-invalid'] || undefined}
        placeholder={placeholder ?? clearLabel ?? t('selector.search')} value={expanded ? query : selectedLabel}
        onFocus={event => event.target.select()} onClick={show} onKeyDown={keyDown} onChange={event => {
          setQuery(event.target.value);
          setActive(0);
          setOpen(true);
          if (createLabel) onChange(event.target.value);
        }}
        className="w-full min-w-0 rounded-md border ui-border-default pl-3 pr-9 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring disabled:opacity-50" />
      <button type="button" disabled={disabled} tabIndex={-1} aria-label={toggleLabel ?? t('selector.show')}
        onMouseDown={event => event.preventDefault()} onClick={() => {
          if (expanded) setOpen(false); else { inputRef.current?.focus(); show(); }
        }} className="absolute inset-y-0 right-0 flex items-center px-2 ui-text-muted">
        <ChevronDown className="h-4 w-4" aria-hidden="true" />
      </button>
    </div>
    {expanded && <div className="absolute z-50 mt-1 max-h-56 w-full overflow-auto rounded-md border ui-border-default ui-surface-card py-1 shadow-lg">
      <div id={listId} role="listbox" aria-label={toggleLabel ?? t('selector.show')}>
        {choices.map((choice, i) => <button type="button" role="option" tabIndex={-1} key={choice.key}
          id={listId + '-' + i} aria-selected={choice.key === value}
          onMouseDown={event => event.preventDefault()} onMouseEnter={() => setActive(i)} onClick={() => select(choice)}
          className={'flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm ' + (i === active ? 'ui-surface-hover' : '')}>
          <span>{renderOption && choice.option != null ? renderOption(choice.option) : choice.label}</span>
          {choice.key === value && <Check className="h-4 w-4 shrink-0" aria-hidden="true" />}
        </button>)}
      </div>
      {(loading || pages.loading || error || pages.error || !choices.some(choice => choice.key !== '')) &&
        <p role="status" className="px-3 py-2 text-sm ui-text-muted">{loading || pages.loading ? t('common.loading')
          : error || pages.error ? t('selector.error') : emptyLabel ?? t('selector.empty')}</p>}
      {loadOptions && pages.next && <button type="button" onMouseDown={event => event.preventDefault()}
        className="page-action page-action-secondary" onClick={() => { inputRef.current?.focus(); pages.next(); }}>{t(pages.error ? 'common.retry' : 'selector.more')}</button>}
    </div>}
  </div>;
}
