import { it, expect } from 'vitest';
import { stockWarning } from './inventoryService';
it('warns from verified physical balances and keeps unknown separate', () => {
 const item = { threshold_mg:100, balance:{ verified:true, available:100 } };
 expect(stockWarning(item)).toBe('Low stock');
 expect(stockWarning(item,101)).toBe('Insufficient material');
 expect(stockWarning({ ...item, balance:{verified:true,available:101} })).toBe('');
 expect(stockWarning({ ...item, balance:{verified:false,available:999} })).toMatch(/Unknown/);
 expect(stockWarning({ ...item, balance:{verified:true,available:-1} })).toMatch(/Negative/);
});
