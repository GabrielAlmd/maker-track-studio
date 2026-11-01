import '@testing-library/jest-dom';
import { afterEach } from 'vitest';
import i18n from '../i18n';

afterEach(() => {
  i18n.changeLanguage('en');
});
