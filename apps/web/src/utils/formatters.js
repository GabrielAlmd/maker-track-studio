import i18n from '../i18n';

export const VALUE_FALLBACKS = Object.freeze({
  unknown: '—',
  get notRecorded() { return i18n.t('common.notRecorded'); },
  get notCalculated() { return i18n.t('common.notCalculated'); },
  get noPurchaseData() { return i18n.t('common.noPurchaseData'); },
  get dataUnavailable() { return i18n.t('common.dataUnavailable'); },
});

export function activeIntlLocale() {
  return i18n.resolvedLanguage === 'pt-PT' ? 'pt-PT' : 'en-IE';
}

// Deliberately accepts numeric API strings but never coerces an absent or blank
// value to zero. Display values must be finite before they are formatted.
export function toFiniteNumber(value) {
  if (typeof value === 'number') return Number.isFinite(value) ? value : null;
  if (typeof value !== 'string' || value.trim() === '') return null;

  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

export function isFiniteNumber(value) {
  return toFiniteNumber(value) !== null;
}

function fallbackFor(value, fallback) {
  return toFiniteNumber(value) === null ? fallback : null;
}

export function formatNumber(value, { maximumFractionDigits = 0, minimumFractionDigits, fallback = VALUE_FALLBACKS.dataUnavailable } = {}) {
  const missing = fallbackFor(value, fallback);
  if (missing) return missing;

  return new Intl.NumberFormat(activeIntlLocale(), { maximumFractionDigits, minimumFractionDigits }).format(toFiniteNumber(value));
}

export function formatCurrency(value, { fractionDigits = 2, fallback = VALUE_FALLBACKS.dataUnavailable } = {}) {
  const missing = fallbackFor(value, fallback);
  if (missing) return missing;

  return new Intl.NumberFormat(activeIntlLocale(), {
    style: 'currency', currency: 'EUR', minimumFractionDigits: fractionDigits, maximumFractionDigits: fractionDigits,
  }).format(toFiniteNumber(value));
}

export function formatCurrencyFromCents(value, { fractionDigits = 2, fallback = VALUE_FALLBACKS.dataUnavailable } = {}) {
  const cents = toFiniteNumber(value);
  if (cents === null) return fallback;

  return formatCurrency(cents / 100, { fractionDigits, fallback });
}

export function formatWeight(value, { fallback = VALUE_FALLBACKS.dataUnavailable } = {}) {
  const grams = toFiniteNumber(value);
  if (grams === null) return fallback;

  if (Math.abs(grams) >= 1000) {
    return `${formatNumber(grams / 1000, {
      maximumFractionDigits: 5,
      fallback,
    })} kg`;
  }

  return `${formatNumber(grams, {
    maximumFractionDigits: 2,
    fallback,
  })} g`;
}

export function formatPercentage(value, { fractionDigits = 1, fallback = VALUE_FALLBACKS.dataUnavailable } = {}) {
  const missing = fallbackFor(value, fallback);
  if (missing) return missing;

  return `${formatNumber(value, { minimumFractionDigits: fractionDigits, maximumFractionDigits: fractionDigits, fallback })}%`;
}

export function formatDuration(value, { fallback = VALUE_FALLBACKS.notRecorded } = {}) {
  const minutes = toFiniteNumber(value);
  if (minutes === null || minutes < 0) return fallback;

  const wholeMinutes = Math.round(minutes);
  if (wholeMinutes < 60) return `${formatNumber(wholeMinutes, { fallback })} min`;
  return `${Math.floor(wholeMinutes / 60)}h${wholeMinutes % 60 ? ` ${wholeMinutes % 60}m` : ''}`;
}

export function formatDate(value, { fallback = VALUE_FALLBACKS.dataUnavailable } = {}) {
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return fallback;
  return new Intl.DateTimeFormat(activeIntlLocale(), { year: 'numeric', month: 'short', day: 'numeric' }).format(date);
}
