import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import Layout from '../components/layout/Layout';
import i18n from '.';
import en from './resources/en';
import ptPT from './resources/pt-PT';

function keys(value, prefix = '') {
  return Object.entries(value).flatMap(([key, child]) => {
    const path = prefix ? `${prefix}.${key}` : key;
    return child && typeof child === 'object' ? keys(child, path) : [path];
  }).sort();
}

describe('translation resources', () => {
  it('keeps Portuguese and English translation keys in sync', () => {
    expect(keys(ptPT)).toEqual(keys(en));
  });

  it.each([
    ['en', 'Dashboard', 'Settings'],
    ['pt-PT', 'Painel', 'Definições'],
  ])('renders representative navigation in %s', async (locale, dashboard, settings) => {
    await i18n.changeLanguage(locale);
    render(<MemoryRouter initialEntries={['/dashboard']}><Layout /></MemoryRouter>);
    expect(screen.getByRole('link', { name: dashboard })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: settings })).toBeInTheDocument();
  });
  it.each([
    ['en', 'Sales & Orders', 'New order', 'Units allocated', 'No sales channel', 'Add Sales Channel'],
    ['pt-PT', 'Vendas e encomendas', 'Nova encomenda', 'Unidades atribuídas', 'Sem canal de venda', 'Adicionar canal de venda'],
  ])('provides representative Sales workflow strings in %s', async (locale, title, newOrder, unitsAllocated, noChannel, addChannel) => {
    await i18n.changeLanguage(locale);
    expect(i18n.t('sales.title')).toBe(title);
    expect(i18n.t('sales.newOrder')).toBe(newOrder);
    expect(i18n.t('printJobCard.unitsAllocated')).toBe(unitsAllocated);
    expect(i18n.t('sales.noChannel')).toBe(noChannel);
    expect(i18n.t('sales.addChannel')).toBe(addChannel);
  });

  it.each([
    ['en', 'Features', 'Business', 'Recent Orders'],
    ['pt-PT', 'Funcionalidades', 'Negócio', 'Encomendas recentes'],
  ])('provides feature and Business Dashboard strings in %s', async (locale, features, business, recentOrders) => {
    await i18n.changeLanguage(locale);
    expect(i18n.t('settings.features')).toBe(features);
    expect(i18n.t('dashboard.business')).toBe(business);
    expect(i18n.t('dashboard.recentOrders')).toBe(recentOrders);
  });

  it.each([
    ['en', 'Expenses', 'Spare parts'],
    ['pt-PT', 'Despesas', 'Peças sobresselentes'],
  ])('provides Expenses strings in %s', async (locale, expenses, spareParts) => {
    await i18n.changeLanguage(locale);
    expect(i18n.t('dashboard.expenses')).toBe(expenses);
    expect(i18n.t('dashboard.spareParts')).toBe(spareParts);
  });

  it.each([
    ['en', 'Data Migration', 'Import spreadsheet data'],
    ['pt-PT', 'Migração de dados', 'Importar dados de folhas de cálculo'],
  ])('provides migration workflow strings in %s', async (locale, section, title) => {
    await i18n.changeLanguage(locale);
    expect(i18n.t('settings.migration')).toBe(section);
    expect(i18n.t('migration.title')).toBe(title);
  });

});
