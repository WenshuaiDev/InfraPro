export type Theme = 'light' | 'dark' | 'system';
export const themeStorageKey = 'infrapro:theme';

export function readTheme(): Theme {
  try {
    const value = localStorage.getItem(themeStorageKey);
    if (value === 'light' || value === 'dark') return value;
  } catch { /* Storage can be disabled by the browser. */ }
  return 'system';
}

export function resolveTheme(theme: Theme): 'light' | 'dark' {
  return theme === 'system' ? (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light') : theme;
}

export function applyTheme(theme: Theme) {
  const resolved = resolveTheme(theme);
  document.documentElement.classList.toggle('dark', resolved === 'dark');
  document.documentElement.style.colorScheme = resolved;
  return resolved;
}

export function initializeTheme() { applyTheme(readTheme()); }
