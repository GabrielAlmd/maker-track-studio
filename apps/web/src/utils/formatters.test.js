import { describe, expect, it } from 'vitest';
import i18n from '../i18n';
import { VALUE_FALLBACKS, formatCurrency, formatCurrencyFromCents, formatDate, formatDuration, formatNumber, formatPercentage, formatWeight, isFiniteNumber } from './formatters';

describe('numeric display formatters', () => {
  it.each([[0, true], [null, false], [undefined, false], ['', false], [NaN, false], [Infinity, false], ['12.5', true]])(
    'identifies %p as %p', (value, expected) => expect(isFiniteNumber(value)).toBe(expected),
  );

  it('formats confirmed zero and numeric strings without changing absent data into zero', () => {
    expect(formatCurrency(0)).toBe('€0.00');
    expect(formatNumber('12.5', { maximumFractionDigits: 1 })).toBe('12.5');
    expect(formatCurrency(null, { fallback: VALUE_FALLBACKS.notCalculated })).toBe('Not calculated');
    expect(formatNumber('', { fallback: VALUE_FALLBACKS.notRecorded })).toBe('Not recorded');
    expect(formatNumber(NaN)).toBe('Data unavailable');
    expect(formatNumber(Infinity)).toBe('Data unavailable');
  });

  it('formats integer cents without converting unavailable Sales costs to zero', () => {
    expect(formatCurrencyFromCents(1234)).toBe('€12.34');
    expect(formatCurrencyFromCents(null, { fallback: VALUE_FALLBACKS.notCalculated })).toBe('Not calculated');
  });

  it('formats weights in grams and kilograms', () => {
    expect(formatWeight(0.01)).toBe('0.01 g');
    expect(formatWeight(0.1)).toBe('0.1 g');
    expect(formatWeight(1)).toBe('1 g');
    expect(formatWeight(12.34)).toBe('12.34 g');
    expect(formatWeight(999.99)).toBe('999.99 g');
    expect(formatWeight(1250)).toBe('1.25 kg');
    expect(formatWeight(1000.01)).toBe('1.00001 kg');
  });

  it('formats recorded durations and retains their semantic fallback', () => {
    expect(formatDuration(90)).toBe('1h 30m');
    expect(formatDuration(undefined)).toBe('Not recorded');
  });

  it('formats percentages without appending a unit to invalid data', () => {
    expect(formatPercentage('12.5')).toBe('12.5%');
    expect(formatPercentage(undefined, { fallback: VALUE_FALLBACKS.notCalculated })).toBe('Not calculated');
  });

  it('uses Portuguese decimal, currency, fallback, and date presentation without changing values', async () => {
    await i18n.changeLanguage('pt-PT');
    expect(formatNumber(12.5, { maximumFractionDigits: 1 })).toBe('12,5');
    expect(formatCurrency(12.5)).toMatch(/^12,50\s€$/);
    expect(formatCurrency(null)).toBe('Dados indisponíveis');
    expect(formatDate('2026-09-18T12:00:00Z')).toMatch(/^18.*2026$/);
  });
});
