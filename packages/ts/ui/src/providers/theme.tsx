import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import { applyTheme, readTheme, resolveTheme, themeStorageKey, type Theme } from './theme-state';

type ThemeContextValue = { theme: Theme; resolvedTheme: 'light' | 'dark'; setTheme: (value: Theme) => void };
const ThemeContext = createContext<ThemeContextValue | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setValue] = useState<Theme>(readTheme);
  const [resolvedTheme, setResolved] = useState(() => resolveTheme(readTheme()));
  useEffect(() => {
    applyTheme(theme);
    const update = () => setResolved(applyTheme(theme));
    const storage = (event: StorageEvent) => {
      if (event.key === themeStorageKey || event.key === null) { const value = readTheme(); setValue(value); setResolved(applyTheme(value)); }
    };
    const media = matchMedia('(prefers-color-scheme: dark)');
    media.addEventListener('change', update);
    window.addEventListener('storage', storage);
    return () => { media.removeEventListener('change', update); window.removeEventListener('storage', storage); };
  }, [theme]);
  function setTheme(value: Theme) {
    try { localStorage.setItem(themeStorageKey, value); } catch { /* In-memory theme still works. */ }
    setValue(value);
    setResolved(applyTheme(value));
  }
  return <ThemeContext value={{ theme, resolvedTheme, setTheme }}>{children}</ThemeContext>;
}

// The hook is the public interface to this provider's context.
// eslint-disable-next-line react-refresh/only-export-components
export function useTheme() {
  const value = useContext(ThemeContext);
  if (!value) throw new Error('ThemeProvider is required');
  return value;
}
