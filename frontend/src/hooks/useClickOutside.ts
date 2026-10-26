import { useEffect, useRef, type RefObject } from 'react';

export function useClickOutside(
  refs: RefObject<HTMLElement | null>[],
  handler: () => void,
  enabled = true,
) {
  const handlerRef = useRef(handler);

  useEffect(() => {
    handlerRef.current = handler;
  }, [handler]);

  useEffect(() => {
    if (!enabled) return;

    const handleClick = (event: MouseEvent) => {
      const target = event.target as Node;
      if (refs.every(r => !r.current?.contains(target))) {
        handlerRef.current();
      }
    };

    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
    // refs are stable RefObjects from callers; spreading keeps the effect in sync with the list.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...refs, enabled]);
}
