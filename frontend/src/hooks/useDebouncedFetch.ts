import { useEffect, useRef, useState } from 'react';

export function useDebouncedFetch<T>(
  fetchFn: () => Promise<T[]>,
  deps: unknown[],
  delay = 300,
  enabled = true,
) {
  const [data, setData] = useState<T[]>([]);
  const [loading, setLoading] = useState(false);
  const timeoutRef = useRef<ReturnType<typeof setTimeout>>(null);
  const fetchIdRef = useRef(0);

  useEffect(() => {
    if (!enabled) return;
    if (timeoutRef.current) clearTimeout(timeoutRef.current);

    const fetchId = ++fetchIdRef.current;
    // eslint-disable-next-line react-hooks/set-state-in-effect -- loading starts with the debounce window so the UI reacts immediately
    setLoading(true);

    timeoutRef.current = setTimeout(async () => {
      try {
        const result = await fetchFn();
        if (fetchIdRef.current !== fetchId) return;
        setData(result);
      } catch {
        if (fetchIdRef.current !== fetchId) return;
        setData([]);
      } finally {
        if (fetchIdRef.current === fetchId) setLoading(false);
      }
    }, delay);

    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
    // fetchFn is intentionally excluded: callers pass inline functions and the effect should run on deps changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, delay, enabled]);

  return { data, loading };
}
