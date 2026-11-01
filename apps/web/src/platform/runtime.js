const configuredMode = import.meta.env.VITE_RUNTIME_MODE;

// Desktop builds set this explicitly. The bridge fallback keeps older release
// assets usable while web builds never infer readiness from a missing bridge.
export function getRuntimeMode() {
  if (configuredMode === 'desktop' || configuredMode === 'web') return configuredMode;
  return typeof window !== 'undefined' && window.go?.main?.App ? 'desktop' : 'web';
}

export const runtimeMode = getRuntimeMode();
export const isDesktopRuntime = () => getRuntimeMode() === 'desktop';

export function getDesktopApp() {
  if (!isDesktopRuntime() || typeof window === 'undefined') return null;
  return window.go?.main?.App || null;
}
