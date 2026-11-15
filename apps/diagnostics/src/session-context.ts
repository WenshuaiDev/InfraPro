import { createContext } from 'react';
import type { DiagnosticsClient, Session } from '@infrapro/diagnostics-client';
export type SessionState = { client: DiagnosticsClient; session?: Session; loading: boolean; unavailable: boolean; login: (username: string, password: string) => Promise<void>; logout: () => Promise<void> };
export const SessionContext = createContext<SessionState | undefined>(undefined);
