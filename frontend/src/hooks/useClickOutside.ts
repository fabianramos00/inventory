import { useEffect, useRef, type RefObject } from 'react';

export function useClickOutside(
  ref: RefObject<HTMLElement | null>,
  handler: () => void,
  enabled = true,
  /** Extra node that counts as inside, e.g. a portaled menu. */
  insideRef?: RefObject<HTMLElement | null>,
) {
  const handlerRef = useRef(handler);

  useEffect(() => {
    handlerRef.current = handler;
  }, [handler]);

  useEffect(() => {
    if (!enabled) return;

    const handleClick = (event: MouseEvent) => {
      const target = event.target as Node;
      const inside =
        ref.current?.contains(target) || insideRef?.current?.contains(target);
      if (!inside) {
        handlerRef.current();
      }
    };

    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [ref, insideRef, enabled]);
}
