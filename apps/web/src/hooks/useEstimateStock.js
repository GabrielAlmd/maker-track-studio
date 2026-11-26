import { useEffect, useMemo, useState } from 'react';
import { getMaterials } from '../services/inventoryService';

export function useEstimateStock() {
  const [rows, setRows] = useState(null);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    let active = true;
    getMaterials().then(result => { if (active) setRows(result); })
      .catch(() => { if (active) setFailed(true); });
    return () => { active = false; };
  }, []);
  const stock = useMemo(() => new Map(rows?.map(row => [row.id, row])), [rows]);
  return { rows, failed, stock };
}
