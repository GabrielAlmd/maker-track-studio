// Read decimal text directly so no floating-point multiplication can change
// a cent. Both decimal separators are supported without grouping separators.
export function parseHundredths(value) {
  const match = /^(\d{1,14})(?:[.,](\d{1,2}))?$/.exec(String(value).trim());
  if (!match) return NaN;
  const cents = Number(match[1] + (match[2] ?? '').padEnd(2, '0'));
  return Number.isSafeInteger(cents) ? cents : NaN;
}

export function decimalFromHundredths(cents) {
  const digits = String(cents).padStart(3, '0');
  return digits.slice(0, -2) + '.' + digits.slice(-2);
}
