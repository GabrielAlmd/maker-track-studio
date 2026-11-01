import { suggestedUnitPrice, unitCostFromPrintJobs } from '../modules/sales/utils/salesPricing';

describe('Sales unit price suggestions', () => {
  it('uses the latest calculated per-piece Print Job cost', () => {
    expect(unitCostFromPrintJobs([
      { units_printed: 4, cost: { state: 'calculated', total_cost: 10 } },
      { units_printed: 2, cost: { state: 'calculated', total_cost: 20 } },
    ])).toBe(2.5);
  });

  it('adds target profit on cost and supports percentages above 100', () => {
    expect(suggestedUnitPrice(10, 50)).toBe(15);
    expect(suggestedUnitPrice(10, 150)).toBe(25);
  });

  it.each([
    [null, 50], [Number.NaN, 50], [Number.POSITIVE_INFINITY, 50], [-1, 50],
    [10, ''], [10, Number.NaN], [10, Number.POSITIVE_INFINITY], [10, -1],
  ])('does not suggest for invalid cost %p or percentage %p', (cost, percentage) => {
    expect(suggestedUnitPrice(cost, percentage)).toBeNull();
  });

  it('allows a confirmed zero cost or zero percentage', () => {
    expect(suggestedUnitPrice(0, 250)).toBe(0);
    expect(suggestedUnitPrice(10, 0)).toBe(10);
  });
});
