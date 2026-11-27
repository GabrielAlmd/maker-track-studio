import * as yup from 'yup';

export function safePurchaseURL(value) {
  const input = typeof value === 'string' ? value.trim() : '';
  if (!/^https?:\/\/[^/]/i.test(input) || /[\s\p{Cc}\\]/u.test(input) || [...input].length > 2048) return null;
  const authority = input.match(/^https?:\/\/([^/?#]+)/i)?.[1];
  if (!authority || /[@%]/.test(authority)) return null;
  try {
    const parsed = new URL(input);
    return parsed.hostname && !parsed.username && !parsed.password ? parsed.href : null;
  } catch {
    return null;
  }
}

export const purchaseURLSchema = (t) => yup.string().trim().test(
  'safe-purchase-url', t('purchaseDetails.invalidURL'), (value) => !value || !!safePurchaseURL(value),
);

const supplierSchema = (t) => yup.string().trim().test(
  'supplier-text', t('purchaseDetails.invalidText'), (value) => !value || ([...value].length <= 200 && !/\p{Cc}/u.test(value)),
);

export const purchaseDetailsSchema = (t) => ({
  supplier: supplierSchema(t),
  supplier_reference: supplierSchema(t),
  purchase_url: purchaseURLSchema(t),
});
