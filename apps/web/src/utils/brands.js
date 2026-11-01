export const normalizeBrand = (brand = '') => brand.trim().toLocaleLowerCase();

export const uniqueBrands = (brands = []) => {
  const seen = new Set();
  return brands
    .map((brand) => brand?.trim())
    .filter((brand) => {
      if (!brand) return false;
      const key = normalizeBrand(brand);
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    })
    .sort((left, right) => left.localeCompare(right, undefined, { sensitivity: 'base' }));
};
