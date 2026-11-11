import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';

type LayoutState = {
  sidebarOpen: boolean;
  setSidebarOpen: (open: boolean) => void;
  reset: () => void;
};

export const useLayoutStore = create<LayoutState>()(persist(
  (set) => ({ sidebarOpen: true, setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }), reset: () => set({ sidebarOpen: true }) }),
  {
    name: 'infrapro:layout', version: 1,
    // Plain account pages do not read the workspace sidebar preference.
    skipHydration: true,
    storage: createJSONStorage(() => localStorage),
    partialize: ({ sidebarOpen }) => ({ sidebarOpen }),
    merge: (persisted, current) => ({ ...current, sidebarOpen: typeof persisted === 'object' && persisted !== null && 'sidebarOpen' in persisted && typeof persisted.sidebarOpen === 'boolean' ? persisted.sidebarOpen : true }),
  },
));
