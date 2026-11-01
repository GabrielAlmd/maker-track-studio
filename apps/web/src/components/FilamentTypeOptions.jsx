import { useTranslation } from 'react-i18next';

export default function FilamentTypeOptions({ types, current }) {
  const { t } = useTranslation();
  return (
    <>
      <option value="">{t('filamentTypes.select')}</option>
      {current && !types.includes(current) && <option value={current}>{current} ({t('filamentTypes.current')})</option>}
      {types.map((type) => <option key={type} value={type}>{type}</option>)}
    </>
  );
}
