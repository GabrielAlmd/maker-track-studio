// components/layout/Layout.jsx
import { useEffect, useState } from 'react';
import { Outlet, NavLink } from 'react-router-dom';
import {
  LayoutDashboard,
  Package,
  Printer,
  ShoppingCart,
  Boxes,
  Settings,
  FileArchive,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useFeatures } from '../../contexts/featureState';
import { navigationContributions } from '../../modules';
import { getDesktopApp } from '../../platform/runtime';

function Layout() {
  const { t } = useTranslation();
  const { features, loading: featuresLoading } = useFeatures();
  const [version, setVersion] = useState(null);

  useEffect(() => {
    let active = true;
    const getVersion = getDesktopApp()?.GetVersion;
    if (getVersion) {
      getVersion().then((value) => {
        if (active) setVersion(value);
      }).catch(() => {
        // The version is optional if the desktop bridge is unavailable.
      });
    }
    return () => { active = false; };
  }, []);

  const navigation = [
    { name: t('nav.dashboard'), href: '/dashboard', icon: LayoutDashboard },
    { name: t('nav.printJobs'), href: '/print-jobs', icon: Printer },
    { name: t('nav.printers'), href: '/printers', icon: Printer },
    { name: t('nav.models'), href: '/models', icon: Boxes },
    { name: t('nav.filaments'), href: '/filaments', icon: Package },
    { name: t('nav.purchases'), href: '/purchases', icon: ShoppingCart },
    { name: t('nav.reports'), href: '/reports', icon: FileArchive },
    ...(!featuresLoading ? navigationContributions(features, t) : []),
    { name: t('nav.settings'), href: '/settings', icon: Settings },
  ];

  return (
    <div className="app-shell flex h-screen">
      {/* Sidebar Navigation */}
      <aside className="app-sidebar app-sidebar-compact w-64 shrink-0 border-r">
        <div className="app-sidebar-brand border-b">
          <span className="app-compact-title">Maker Track Studio</span>
          <div className="app-sidebar-logo-viewport">
            <img
              src="/MakerTrackerStudio-MiniLogoHorizontal.png"
              alt="Maker Track Studio"
              className="app-sidebar-logo"
            />
          </div>
          <p className="app-sidebar-tagline text-xs tracking-[0.22em] uppercase ui-text-muted">{t('nav.tagline')}</p>
          {version && <p className="app-sidebar-version">v{version}</p>}
        </div>
        <nav className="app-sidebar-nav" aria-label={t('nav.primary')}>
          {navigation.map((item) => (
            <NavLink
              key={item.name}
              to={item.href}
              className={({ isActive }) =>
                `app-nav-item flex h-11 items-center rounded-md px-3 text-sm font-medium transition-colors duration-200 ${
                  isActive
                    ? 'app-nav-active border'
                    : 'app-nav-link ui-text-muted border border-transparent'
                }`
              }
            >
              <item.icon className="mr-3 h-5 w-5 shrink-0" aria-hidden="true" />
              <span className="whitespace-nowrap">{item.name}</span>
            </NavLink>
          ))}
        </nav>
      </aside>

      {/* Main Content Area */}
      <div className="min-w-0 flex-1 flex flex-col overflow-hidden">
        {/* Page Content */}
        <main className="flex-1 overflow-y-auto p-8 lg:p-10">
          <Outlet /> {/* This is where your page components render */}
        </main>
      </div>
    </div>
  );
}

export default Layout;
