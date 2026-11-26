import { decimalFromHundredths, parseHundredths } from './moneyInput';

it('round-trips exact cents for both decimal separators up to the safe integer limit', () => {
  expect(parseHundredths(' 8,33 ')).toBe(833);
  expect(parseHundredths('8.3')).toBe(830);
  expect(parseHundredths(decimalFromHundredths(Number.MAX_SAFE_INTEGER))).toBe(Number.MAX_SAFE_INTEGER);
});

it.each(['', '0.001', '1e2', '-0.01', '1,000.00', '90071992547409.92'])('rejects unsupported decimal input %j', value => {
  expect(parseHundredths(value)).toBeNaN();
});
