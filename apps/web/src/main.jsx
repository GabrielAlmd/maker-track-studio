import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.jsx'
import DiagnosticErrorBoundary from './components/DiagnosticErrorBoundary.jsx'
import DesktopRuntimeGuard from './components/DesktopRuntimeGuard.jsx'
import { installGlobalErrorReporting, reportFrontendFailure } from './services/diagnostics.js'
import { FirstRunLanguageChoice, LocaleProvider } from './i18n/index.jsx'


const removeErrorReporting = installGlobalErrorReporting()
if (import.meta.hot) import.meta.hot.dispose(removeErrorReporting)

createRoot(document.getElementById('root'), {
  // Error boundaries report a fixed event; React must not print raw error data.
  onCaughtError: () => {},
  onUncaughtError: () => reportFrontendFailure('script'),
  onRecoverableError: () => reportFrontendFailure('script'),
}).render(
  <StrictMode>
    <LocaleProvider>
      <DiagnosticErrorBoundary>
        <DesktopRuntimeGuard><FirstRunLanguageChoice><App /></FirstRunLanguageChoice></DesktopRuntimeGuard>
      </DiagnosticErrorBoundary>
    </LocaleProvider>
  </StrictMode>,
)
