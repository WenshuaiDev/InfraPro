import type { CSSProperties } from 'react';
import { Toaster as Sonner, type ToasterProps } from 'sonner';
import { useTheme } from '../providers/theme';

export function Toaster(props: ToasterProps) {
  const { theme } = useTheme();
  return <Sonner theme={theme} className="toaster group [&_div[data-content]]:w-full" closeButton containerAriaLabel="操作提示" toastOptions={{ closeButtonAriaLabel: '关闭提示' }} style={{ '--normal-bg': 'var(--popover)', '--normal-text': 'var(--popover-foreground)', '--normal-border': 'var(--border)' } as CSSProperties} {...props} />;
}
