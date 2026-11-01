import axios from 'axios';
import toast from 'react-hot-toast';
import { reportFrontendFailure } from './diagnostics';
import { isDesktopRuntime } from '../platform/runtime';

export const apiBaseURL = isDesktopRuntime()
  ? (import.meta.env.VITE_API_URL || 'http://127.0.0.1:8080')
  : '/api';

const api = axios.create({
  baseURL: apiBaseURL,
  headers: { 'Content-Type': 'application/json' },
});

// Response interceptor for error handling
api.interceptors.response.use(
  (response) => {
    return response;
  },
  (error) => {
    // Handle different error types
    if (error.response) {
      // Server responded with error status
      const { status, data } = error.response;
      const candidate = data?.error || data?.message;
      const errorMessage = typeof candidate === 'string' && status < 500 ? candidate : `Error ${status}`;

      switch (status) {
        case 400:
          toast.error(`Bad Request: ${errorMessage}`);
          break;
        case 403:
          toast.error(`Forbidden: ${errorMessage}`);
          break;
        case 404:
          toast.error('Resource not found');
          break;
        default:
          toast.error(data?.code === 'APP_STATE_UNSAFE'
            ? 'Restart the application before continuing. Backup recovery remains available.'
            : `Error: ${errorMessage}`);
      }
    } else {
      if (axios.isCancel(error)) return Promise.reject(error);
      reportFrontendFailure('network');
      toast.error('Network error. Please try again later.');
    }

    return Promise.reject(error);
  }
);

export default api;
