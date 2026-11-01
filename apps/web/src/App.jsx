import { HashRouter, Routes, Route, Navigate } from 'react-router-dom';
import { Toaster } from 'react-hot-toast';
import Layout from './components/layout/Layout';
import Dashboard from './pages/Dashboard';
import Models from './pages/Models';
import Filaments from './pages/Filaments';
import PrintJobs from './pages/PrintJobs';
import Printers from './pages/Printers';
import Purchases from './pages/Purchases';
import Settings from './pages/Settings';
import Reports from './pages/Reports';
import { routeContributions } from './modules';
import { ThemeProvider } from './contexts/ThemeContext';
import { FeatureProvider } from './contexts/FeatureContext';

function AppContent() {
  const defaultRoute = '/dashboard';

  return (
    <HashRouter>
      <Routes>
        {/* Root route redirects to dashboard */}
        <Route
          path="/"
          element={<Navigate to={defaultRoute} replace />}
        />

        {/* All dashboard routes use the Layout wrapper */}
        <Route path="/" element={<Layout />}>
          <Route path="dashboard" element={<Dashboard />} />
          <Route path="models" element={<Models />} />
          <Route path="filaments" element={<Filaments />} />
          <Route path="print-jobs" element={<PrintJobs />} />
          <Route path="printers" element={<Printers />} />
          <Route path="purchases" element={<Purchases />} />
          <Route path="reports" element={<Reports />} />
          {routeContributions().map((contribution) => <Route key={`${contribution.moduleId}:${contribution.path}`} path={contribution.path} element={<contribution.Component />} />)}
          <Route path="settings" element={<Settings />} />
        </Route>

        {/* 404 catch-all route */}
        <Route
          path="*"
          element={<Navigate to={defaultRoute} replace />}
        />
      </Routes>

      {/* Toast notifications */}
      <Toaster
        position="top-right"
        toastOptions={{
          duration: 4000,
          style: {
            background: 'var(--surface-raised)',
            color: 'var(--text-primary)',
            border: '1px solid var(--border-default)',
          },
          success: {
            iconTheme: {
              primary: 'var(--color-success)',
              secondary: 'var(--text-on-primary)',
            },
          },
          error: {
            iconTheme: {
              primary: 'var(--color-danger)',
              secondary: 'var(--text-on-primary)',
            },
          },
        }}
      />
    </HashRouter>
  );
}

function App() {
  return (
    <ThemeProvider>
      <FeatureProvider><AppContent /></FeatureProvider>
    </ThemeProvider>
  );
}

export default App;
