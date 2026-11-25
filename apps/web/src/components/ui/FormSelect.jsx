import { useController } from 'react-hook-form';
import SearchableSelect from './SearchableSelect';

export default function FormSelect({ control, name, onSelect, ...props }) {
  const { field, fieldState } = useController({ control, name });
  return <SearchableSelect {...field} invalid={fieldState.invalid} {...props} onChange={value => {
    field.onChange(value);
    onSelect?.(value);
  }} />;
}
