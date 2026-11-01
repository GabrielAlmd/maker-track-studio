import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { FirstRunLanguageChoice, LocaleProvider } from './index';

describe('first-run locale choice', () => {
  it('prompts on a clean portable launch and persists the choice', async () => {
    const setLocale = vi.fn().mockResolvedValue(undefined);
    window.go = { main: { App: {
      GetLocalePreference: vi.fn().mockResolvedValue({ locale: 'pt-PT', is_set: false }),
      SetLocalePreference: setLocale,
    } } };
    render(<LocaleProvider><FirstRunLanguageChoice><p>Business screen</p></FirstRunLanguageChoice></LocaleProvider>);
    expect(await screen.findByRole('heading', { name: 'Escolha o seu idioma' })).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole('button', { name: 'Continuar' }));
    expect(setLocale).toHaveBeenCalledWith('pt-PT');
    expect(await screen.findByText('Business screen')).toBeInTheDocument();
  });

  it('does not replace an existing preference during an upgrade launch', async () => {
    const setLocale = vi.fn();
    window.go = { main: { App: {
      GetLocalePreference: vi.fn().mockResolvedValue({ locale: 'en', is_set: true }),
      SetLocalePreference: setLocale,
    } } };
    render(<LocaleProvider><FirstRunLanguageChoice><p>Business screen</p></FirstRunLanguageChoice></LocaleProvider>);
    expect(await screen.findByText('Business screen')).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Choose your language' })).not.toBeInTheDocument();
    expect(setLocale).not.toHaveBeenCalled();
  });
});
