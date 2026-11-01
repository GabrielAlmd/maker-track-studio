import { createContext, useContext } from 'react';

const disabledFeatures = Object.freeze({});

export const FeatureContext = createContext({
  features: disabledFeatures,
  loading: false,
  saving: false,
  error: null,
  refetch: async () => disabledFeatures,
  updateFeature: async () => disabledFeatures,
});

export function useFeatures() {
  return useContext(FeatureContext);
}
