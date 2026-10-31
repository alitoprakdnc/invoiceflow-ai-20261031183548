import React, {
  createContext,
  useContext,
  useEffect,
  useState,
  ReactNode,
  useCallback,
} from "react";

/**
 * Theme mode identifiers.
 */
export type ThemeMode = "light" | "dark";

/**
 * Mapping of CSS custom properties to their values.
 * The keys correspond to the variables used in Tailwind's `theme` extensions
 * and the global `:root` style block.
 */
export type ThemeTokens = Record<string, string>;

const LIGHT_TOKENS: ThemeTokens = {
  "--color-primary": "#0A74DA",
  "--color-secondary": "#14B8A6",
  "--color-accent": "#F59E0B",
  "--color-bg": "#FFFFFF",
  "--color-bg-muted": "#F9FAFB",
  "--color-bg-subtle": "#F3F4F6",
  "--color-text": "#111827",
  "--color-text-muted": "#6B7280",
  "--color-border": "#E5E7EB",
  "--color-card-bg": "#FFFFFF",
  "--color-card-shadow": "rgba(0, 0, 0, 0.05)",
};

const DARK_TOKENS: ThemeTokens = {
  "--color-primary": "#0A74DA",
  "--color-secondary": "#14B8A6",
  "--color-accent": "#F59E0B",
  "--color-bg": "#111827",
  "--color-bg-muted": "#1F2937",
  "--color-bg-subtle": "#27303F",
  "--color-text": "#F9FAFB",
  "--color-text-muted": "#9CA3AF",
  "--color-border": "#374151",
  "--color-card-bg": "#1F2937",
  "--color-card-shadow": "rgba(0, 0, 0, 0.25)",
};

/**
 * Apply a set of CSS custom properties to the document root.
 * This runs in the browser only; any errors are caught and logged.
 */
function applyThemeTokens(tokens: ThemeTokens): void {
  if (typeof document === "undefined") return;
  try {
    const root = document.documentElement;
    Object.entries(tokens).forEach(([key, value]) => {
      root.style.setProperty(key, value);
    });
    // Also expose the current mode as a data attribute for Tailwind's `media` strategy
    const mode = tokens["--color-bg"] === "#FFFFFF" ? "light" : "dark";
    root.setAttribute("data-theme", mode);
  } catch (err) {
    // eslint-disable-next-line no-console
    console.error("Failed to apply theme tokens:", err);
  }
}

/**
 * Retrieve the persisted theme from localStorage, falling back to the
 * OS preference or a safe default.
 */
function getPersistedTheme(): ThemeMode {
  if (typeof window === "undefined") return "light";
  try {
    const stored = window.localStorage.getItem("theme");
    if (stored === "light" || stored === "dark") {
      return stored;
    }
    const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
    return prefersDark ? "dark" : "light";
  } catch {
    // If localStorage is unavailable, default to light
    return "light";
  }
}

/**
 * Persist the chosen theme to localStorage.
 */
function persistTheme(mode: ThemeMode): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem("theme", mode);
  } catch {
    // Silently ignore write errors (e.g., private browsing)
  }
}

/**
 * Context shape exposed to consuming components.
 */
interface ThemeContextValue {
  /** Current theme mode */
  mode: ThemeMode;
  /** Switch between light and dark */
  toggle: () => void;
  /** Explicitly set a theme */
  setMode: (mode: ThemeMode) => void;
}

/**
 * React context used throughout the app.
 */
const ThemeContext = createContext<ThemeContextValue | undefined>(undefined);

/**
 * Provider component that initialises the theme, reacts to changes,
 * and makes the toggle API available.
 */
export const ThemeProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const [mode, setModeState] = useState<ThemeMode>(getPersistedTheme);

  // Apply tokens whenever the mode changes
  useEffect(() => {
    const tokens = mode === "light" ? LIGHT_TOKENS : DARK_TOKENS;
    applyThemeTokens(tokens);
    persistTheme(mode);
  }, [mode]);

  const setMode = useCallback((newMode: ThemeMode) => {
    setModeState(newMode);
  }, []);

  const toggle = useCallback(() => {
    setModeState((prev) => (prev === "light" ? "dark" : "light"));
  }, []);

  const contextValue = {
    mode,
    toggle,
    setMode,
  };

  return <ThemeContext.Provider value={contextValue}>{children}</ThemeContext.Provider>;
};

/**
 * Hook for components to read the current theme and toggle it.
 *
 * @throws if used outside of a ThemeProvider
 */
export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) {
    throw new Error("useTheme must be used within a ThemeProvider");
  }
  return ctx;
}

/**
 * Export the raw token objects for any advanced use‑cases (e.g., server‑side
 * rendering of inline styles). They are deliberately named to avoid
 * accidental mutation.
 */
export const themeTokens = {
  light: LIGHT_TOKENS,
  dark: DARK_TOKENS,
};