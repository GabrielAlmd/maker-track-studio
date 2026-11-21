import * as yup from 'yup';

export const textField = (max, required = false) => {
  const field = yup.string().trim().max(max);
  return required ? field.required() : field.default('');
};

export const numberField = (min, max, whole = true) => {
  const field = yup.number().min(min).max(max).required();
  return whole ? field.integer() : field;
};
