function roundedMoney(value) {
  return Math.round((value + Number.EPSILON) * 100) / 100;
}

export function unitCostFromPrintJobs(jobs = []) {
  const job = jobs.find((candidate) => {
    const totalCost = Number(candidate?.cost?.total_cost);
    const unitsPrinted = Number(candidate?.units_printed);
    return candidate?.cost?.state === 'calculated' && Number.isFinite(totalCost) && totalCost >= 0 && Number.isSafeInteger(unitsPrinted) && unitsPrinted > 0;
  });
  return job ? roundedMoney(Number(job.cost.total_cost) / Number(job.units_printed)) : null;
}

export function suggestedUnitPrice(unitCost, targetProfitPercent) {
  const cost = Number(unitCost);
  const percentage = Number(targetProfitPercent);
  if (unitCost === null || unitCost === '' || targetProfitPercent === '' || targetProfitPercent === null ||
      !Number.isFinite(cost) || cost < 0 || !Number.isFinite(percentage) || percentage < 0) return null;
  const suggestion = cost * (1 + percentage / 100);
  return Number.isFinite(suggestion) ? roundedMoney(suggestion) : null;
}
