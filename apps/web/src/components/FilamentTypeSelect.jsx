import { useTranslation } from 'react-i18next';
import { useController } from 'react-hook-form';
import SearchableSelect from './ui/SearchableSelect';

export default function FilamentTypeSelect({ control, name, types, ...props }) {
  const { t } = useTranslation();
  const { field, fieldState } = useController({ control, name });
  const options = types ?? [];
  const historical = field.value && !options.includes(field.value);
  return <SearchableSelect {...field} {...props} invalid={fieldState.invalid} required
    options={historical ? [field.value, ...options] : options} getKey={type => type}
    getLabel={type => historical && type === field.value ? type + ' (' + t('filamentTypes.current') + ')' : type}
    clearLabel={t('filamentTypes.select')} />;
}
