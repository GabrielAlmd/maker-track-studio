import i18n from '../i18n';

export function modelName(job) {
  return job.model?.name ?? i18n.t('printJobCard.modelNotAssigned');
}

export function filamentName(row) {
  return row.filament?.name ?? i18n.t('printJobCard.unavailableFilament');
}
