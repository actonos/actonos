import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';

export type UIDensity = 'comfortable' | 'compact';

interface DensityContextValue {
  density: UIDensity;
  setDensity: (density: UIDensity) => void;
  toggleDensity: () => void;
}

const STORAGE_KEY = 'actonos_ui_density';
const DensityContext = createContext<DensityContextValue | null>(null);

export function DensityProvider({ children }: { children: ReactNode }) {
  const [density, setDensityState] = useState<UIDensity>(() => {
    return localStorage.getItem(STORAGE_KEY) === 'compact' ? 'compact' : 'comfortable';
  });

  const setDensity = useCallback((next: UIDensity) => {
    setDensityState(next);
    localStorage.setItem(STORAGE_KEY, next);
  }, []);

  const toggleDensity = useCallback(() => {
    setDensity(density === 'compact' ? 'comfortable' : 'compact');
  }, [density, setDensity]);

  useEffect(() => {
    document.documentElement.dataset.density = density;
  }, [density]);

  const value = useMemo(
    () => ({ density, setDensity, toggleDensity }),
    [density, setDensity, toggleDensity]
  );

  return <DensityContext.Provider value={value}>{children}</DensityContext.Provider>;
}

export function useDensity() {
  const value = useContext(DensityContext);
  if (!value) throw new Error('useDensity must be used inside DensityProvider');
  return value;
}
