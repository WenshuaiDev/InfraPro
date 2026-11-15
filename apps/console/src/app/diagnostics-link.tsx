import { Activity } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { diagnosticsOrigin } from '../config/diagnostics-link';
const origin = diagnosticsOrigin(import.meta.env.VITE_DIAGNOSTICS_ORIGIN);
export function DiagnosticsLink() {
  return origin ? <Button variant="ghost" size="icon" asChild><a href={origin} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer" aria-label="打开独立运行诊断（新窗口）" title="独立运行诊断"><Activity className="size-4" aria-hidden="true" /></a></Button> : null;
}
