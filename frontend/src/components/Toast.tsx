import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  ReactNode,
} from 'react';
import {
  CheckCircleIcon,
  XCircleIcon,
  ExclamationCircleIcon,
  InformationCircleIcon,
} from '@heroicons/react/24/outline';

type ToastType = 'success' | 'error' | 'info' | 'warning';

interface ToastOptions {
  /** Optional title displayed above the message */
  title?: string;
  /** Main message body */
  message: string;
  /** Visual style of the toast */
  type?: ToastType;
  /** Auto‑dismiss after this many milliseconds (0 = never). Defaults to 5000. */
  duration?: number;
}

interface Toast extends ToastOptions {
  id: string;
  /** Resolved duration (always defined) */
  duration: number;
}

/** Context exposing a single `addToast` function */
interface ToastContextValue {
  addToast: (options: ToastOptions) => void;
}

const ToastContext = createContext<ToastContextValue | undefined>(undefined);

/**
 * Provider that holds the toast queue and renders the toast UI.
 *
 * Wrap your application (or a subtree) with this component:
 *
 * ```tsx
 * <ToastProvider>
 *   <App />
 * </ToastProvider>
 * ```
 */
export const ToastProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const addToast = useCallback((options: ToastOptions) => {
    const id =
      typeof crypto !== 'undefined' && crypto.randomUUID
        ? crypto.randomUUID()
        : `${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;

    const toast: Toast = {
      id,
      title: options.title,
      message: options.message,
      type: options.type ?? 'info',
      duration: options.duration ?? 5000,
    };

    setToasts((prev) => [...prev, toast]);
  }, []);

  const removeToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  // Auto‑dismiss handling
  useEffect(() => {
    toasts.forEach((toast) => {
      if (toast.duration === 0) return; // persistent toast
      const timer = setTimeout(() => {
        removeToast(toast.id);
      }, toast.duration);
      return () => clearTimeout(timer);
    });
  }, [toasts, removeToast]);

  return (
    <ToastContext.Provider value={{ addToast }}>
      {children}
      {/* Toast container – fixed to bottom‑right, respects dark mode */}
      <div className="fixed inset-0 flex items-end justify-end p-4 pointer-events-none z-50">
        <div className="flex flex-col space-y-2 w-full max-w-sm">
          {toasts.map((toast) => (
            <ToastItem key={toast.id} toast={toast} onClose={() => removeToast(toast.id)} />
          ))}
        </div>
      </div>
    </ToastContext.Provider>
  );
};

/**
 * Hook to enqueue a toast from any component.
 *
 * ```tsx
 * const addToast = useToast();
 * addToast({ type: 'success', message: 'Invoice saved!' });
 * ```
 */
export const useToast = (): ((options: ToastOptions) => void) => {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    throw new Error('useToast must be used within a ToastProvider');
  }
  return ctx.addToast;
};

/* -------------------------------------------------------------------------- */
/*                               Toast Item UI                               */
/* -------------------------------------------------------------------------- */

interface ToastItemProps {
  toast: Toast;
  onClose: () => void;
}

const iconMap: Record<ToastType, JSX.Element> = {
  success: <CheckCircleIcon className="h-5 w-5 text-green-500" aria-hidden="true" />,
  error: <XCircleIcon className="h-5 w-5 text-red-500" aria-hidden="true" />,
  info: <InformationCircleIcon className="h-5 w-5 text-blue-500" aria-hidden="true" />,
  warning: <ExclamationCircleIcon className="h-5 w-5 text-amber-500" aria-hidden="true" />,
};

const bgClassMap: Record<ToastType, string> = {
  success: 'bg-green-50 border-green-400',
  error: 'bg-red-50 border-red-400',
  info: 'bg-blue-50 border-blue-400',
  warning: 'bg-amber-50 border-amber-400',
};

const textClassMap: Record<ToastType, string> = {
  success: 'text-green-800',
  error: 'text-red-800',
  info: 'text-blue-800',
  warning: 'text-amber-800',
};

const ToastItem: React.FC<ToastItemProps> = ({ toast, onClose }) => {
  const { type, title, message } = toast;
  const bgClass = bgClassMap[type];
  const textClass = textClassMap[type];

  return (
    <div
      role="alert"
      aria-live="assertive"
      className={`flex items-start w-full max-w-sm p-4 rounded-md shadow-md border-l-4 ${bgClass} pointer-events-auto transform transition-all duration-300 ease-in-out`}
    >
      <div className="flex-shrink-0 mr-3">{iconMap[type]}</div>
      <div className="flex-1 min-w-0">
        {title && (
          <p className={`font-medium ${textClass} dark:${textClass}`}>{title}</p>
        )}
        <p className={`mt-1 text-sm ${textClass} dark:${textClass}`}>{message}</p>
      </div>
      <button
        onClick={onClose}
        className="ml-4 flex-shrink-0 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 rounded"
        aria-label="Close toast"
      >
        <XCircleIcon className="h-5 w-5" aria-hidden="true" />
      </button>
    </div>
  );
};