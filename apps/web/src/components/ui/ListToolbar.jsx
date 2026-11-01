import { ChevronLeft, ChevronRight, Filter, X } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

export default function ListToolbar({ list, sorts, children, sortLabel, sortValue, onSortChange, nativeSort = false }) {
  const { t } = useTranslation();
  const [filtersOpen, setFiltersOpen] = useState(false);
  const total = list.data?.total || 0;
  const pages = Math.max(1, Math.ceil(total / list.pageSize));
  const accessibleSortLabel = sortLabel || t('list.sortRecords');
  const activeFilterCount = Object.values(list.filters ?? {}).filter((value) => value !== '' && value != null).length;
  return <div className="ui-toolbar relative z-20 flex flex-wrap items-center gap-2 border-b p-4" role="search" aria-label={t('list.controls')}>
    <label className="sr-only" htmlFor="list-search">{t('list.searchRecords')}</label>
    <input id="list-search" type="search" value={list.searchInput} onChange={(e) => list.setSearchInput(e.target.value)} placeholder={t('common.search')} className="min-w-48 rounded-md border ui-border-default px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ui-focus-ring" />
    <label className="sr-only" htmlFor="list-sort">{accessibleSortLabel}</label>
    <select id="list-sort" role={nativeSort ? undefined : 'listbox'} aria-label={nativeSort ? accessibleSortLabel : undefined} value={sortValue ?? list.sort} onChange={(e) => { if (onSortChange) onSortChange(e.target.value); else { list.setSort(e.target.value); list.setPage(1); } }} className="rounded-md border ui-border-default px-3 py-2 text-sm">
      {sorts.map((s) => <option key={s.value} value={s.value}>{s.label}</option>)}
    </select>
    <button type="button" onClick={() => { list.setDirection(list.direction === 'asc' ? 'desc' : 'asc'); list.setPage(1); }} className="rounded-md border ui-border-default px-3 py-2 text-sm" aria-label={t(list.direction === 'asc' ? 'list.sortDescending' : 'list.sortAscending')}>
      {list.direction === 'asc' ? `↑ ${t('list.ascending')}` : `↓ ${t('list.descending')}`}
    </button>
    {children && <details className="relative" onToggle={(event) => setFiltersOpen(event.currentTarget.open)}><summary className={`cursor-pointer list-none rounded-md border px-3 py-2 text-sm ${activeFilterCount ? 'ui-tone-primary ui-border-default' : 'ui-border-default'}`}><Filter className="mr-1 inline h-4 w-4" />{activeFilterCount ? t('common.filtersActive', { count: activeFilterCount }) : t('common.filters')}</summary>{filtersOpen && <div className="ui-popover absolute z-30 mt-2 min-w-56 rounded-md border p-3">{children}</div>}</details>}
    {list.hasFilters && <button type="button" onClick={list.clear} className="rounded-md px-2 py-2 text-sm ui-text-secondary hover:ui-surface-hover"><X className="mr-1 inline h-4 w-4" />{t('common.clearFilters')}</button>}
    <div className="ml-auto flex items-center gap-2 whitespace-nowrap">
      <span className="text-sm ui-text-secondary" aria-live="polite">{t('list.result', { count: total })}</span>
      <label className="text-sm ui-text-secondary">{t('list.rows')} <select role="listbox" value={list.pageSize} onChange={(e) => { list.setPageSize(Number(e.target.value)); list.setPage(1); }} className="ml-1 rounded border ui-border-default p-1">{[25, 50, 100].map((n) => <option key={n}>{n}</option>)}</select></label>
      <button type="button" disabled={list.page <= 1} onClick={() => list.setPage(list.page - 1)} className="rounded p-2 disabled:opacity-40" aria-label={t('list.previous')}><ChevronLeft className="h-4 w-4" /></button>
      <span className="text-sm">{list.page} / {pages}</span>
      <button type="button" disabled={list.page >= pages} onClick={() => list.setPage(list.page + 1)} className="rounded p-2 disabled:opacity-40" aria-label={t('list.next')}><ChevronRight className="h-4 w-4" /></button>
    </div>
  </div>;
}
