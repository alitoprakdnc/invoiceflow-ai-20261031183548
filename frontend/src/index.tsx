import React, { Component, ReactNode } from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import {
  QueryClient,
  QueryClientProvider,
  QueryCache,
  MutationCache,
} from '@tanstack/react-query';
import { ReactQueryDevtools } from '@tanstack/react-query-devtools';

import App from '@/App';
import { ThemeProvider } from '@/theme';
import '@/styles/tailwind.css';

/**
 * Global error boundary that catches rendering errors in the React tree.
 * It logs the error and displays a user‑friendly fallback UI.
 */
class GlobalErrorBoundary extends Component<
  { children: ReactNode },
  { hasError: boolean; error?: Error }
> {
  constructor(props: { children: ReactNode }) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(_: Error) {
    return { hasError: true };
  }

  componentDidCatch(error: Error, errorInfo: React.ErrorInfo) {
    // In production you would send this to an error‑tracking service.
    // eslint-disable-next-line no-console
    console.error('Uncaught error in React tree:', error, errorInfo);
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex min-h-screen items-center justify-center bg-gray-50 dark:bg-gray-900 p-4">
          <div className="max-w-md rounded-lg bg-white dark:bg-gray-800 shadow-lg p-6 text-center">
            <h1 className="mb-4 text-2xl font-semibold text-gray-800 dark:text-gray-100">
              Something went wrong
            </h1>
            <p className="mb-6 text-gray-600 dark:text-gray-300">
              An unexpected error occurred. Please refresh the page or contact
              support if the problem persists.
            </p>
            <button
              onClick={() => window.location.reload()}
              className="rounded bg-primary-600 hover:bg-primary-700 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-600 text-white px-4 py-2 transition-colors"
            >
              Reload
            </button>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}

/**
 * Configure a single QueryClient for the whole application.
 * Default options are tuned for a SaaS product:
 * - No infinite retries (avoid hammering the API)
 * - Stale time of 5 minutes for most GETs
 * - No refetch on window focus (explicit refetch via React‑Query hooks)
 */
const queryClient = new QueryClient({
  queryCache: new QueryCache({
    onError: (error) => {
      // eslint-disable-next-line no-console
      console.error('React‑Query error:', error);
    },
  }),
  mutationCache: new MutationCache({
    onError: (error) => {
      // eslint-disable-next-line no-console
      console.error('React‑Query mutation error:', error);
    },
  }),
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 5 * 60 * 1000, // 5 minutes
      refetchOnWindowFocus: false,
      refetchOnMount: false,
      refetchOnReconnect: false,
    },
    mutations: {
      retry: 0,
    },
  },
});

/**
 * Render the application.
 * The order of providers mirrors the dependency chain:
 * ThemeProvider → BrowserRouter → QueryClientProvider → GlobalErrorBoundary → App
 */
const rootElement = document.getElementById('root');
if (!rootElement) {
  throw new Error('Root element not found');
}

const root = ReactDOM.createRoot(rootElement);
root.render(
  <React.StrictMode>
    <ThemeProvider>
      <BrowserRouter>
        <QueryClientProvider client={queryClient}>
          <GlobalErrorBoundary>
            <App />
          </GlobalErrorBoundary>
          {/* Devtools are hidden in production builds */}
          {process.env.NODE_ENV === 'development' && (
            <ReactQueryDevtools initialIsOpen={false} />
          )}
        </QueryClientProvider>
      </BrowserRouter>
    </ThemeProvider>
  </React.StrictMode>
);