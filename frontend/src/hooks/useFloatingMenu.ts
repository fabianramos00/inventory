import { useLayoutEffect, useMemo, useRef, useState, type CSSProperties, type RefObject } from 'react'

export const FLOATING_MENU_ATTRIBUTE = 'data-floating-menu'

/** Above the modal backdrops (z-index 100) so a menu opened inside a dialog stays visible. */
const MENU_Z_INDEX = 200

export interface FloatingTriggerRect {
  top: number
  bottom: number
  left: number
  right: number
  width: number
}

export interface FloatingPlacementInput {
  trigger: FloatingTriggerRect
  viewportWidth: number
  viewportHeight: number
  menuHeight: number
  menuWidth?: number
  gap?: number
  padding?: number
}

export interface FloatingPlacement {
  placement: 'down' | 'up'
  top: number
  left: number
  /** Space the menu may use on the chosen side of the trigger. */
  maxHeight: number
}

/**
 * Pure placement math for a floating menu.
 * Opens downward when the viewport has room below, upward when it does not,
 * and clamps to the roomier side when neither side fits.
 */
export function computeFloatingPlacement({
  trigger,
  viewportWidth,
  viewportHeight,
  menuHeight,
  menuWidth = 0,
  gap = 6,
  padding = 8,
}: FloatingPlacementInput): FloatingPlacement {
  const spaceBelow = viewportHeight - trigger.bottom - gap - padding
  const spaceAbove = trigger.top - gap - padding
  const fitsBelow = spaceBelow >= menuHeight
  const fitsAbove = spaceAbove >= menuHeight
  const placement = fitsBelow
    ? 'down'
    : fitsAbove
      ? 'up'
      : spaceAbove > spaceBelow
        ? 'up'
        : 'down'
  const maxHeight = Math.max(placement === 'down' ? spaceBelow : spaceAbove, 0)
  const height = Math.min(menuHeight, maxHeight)
  const top = placement === 'down'
    ? trigger.bottom + gap
    : Math.max(padding, trigger.top - gap - height)
  const maxLeft = Math.max(padding, viewportWidth - padding - menuWidth)
  const left = Math.min(Math.max(trigger.left, padding), maxLeft)

  return { placement, top, left, maxHeight }
}

/** Anchors of the portaled menus, so a dialog can tell which menu belongs to it. */
const floatingAnchors = new WeakMap<HTMLElement, HTMLElement>()

/**
 * Height the menu is allowed to take, declared by the component in
 * `--floating-menu-cap`. Without it the menu grows with its content.
 */
function readMenuCap(element: HTMLElement): number {
  const raw = getComputedStyle(element).getPropertyValue('--floating-menu-cap')
  const value = Number.parseFloat(raw)
  return Number.isFinite(value) && value > 0 ? value : Number.POSITIVE_INFINITY
}

export interface FloatingMenuOwner {
  panel: HTMLElement
  anchor: HTMLElement | null
}

/** Returns the floating menu that owns `element`, if any. */
export function floatingMenuOwner(element: Element | null): FloatingMenuOwner | null {
  const panel = element?.closest?.(`[${FLOATING_MENU_ATTRIBUTE}]`) as HTMLElement | null
  if (!panel) return null
  return { panel, anchor: floatingAnchors.get(panel) ?? null }
}

export interface UseFloatingMenuOptions {
  open: boolean
  /** Element the menu is anchored to. */
  anchor: RefObject<HTMLElement | null>
  /** Portaled menu element. */
  panel: RefObject<HTMLElement | null>
  /** `trigger` matches the trigger width, `content` lets the menu grow with its content. */
  width?: 'trigger' | 'content'
  gap?: number
  padding?: number
  /** Fixed content inside the menu (a search input) that does not scroll with the list. */
  chromeRef?: RefObject<HTMLElement | null>
}
export interface FloatingMenu {
  panelProps: { style: CSSProperties; 'data-floating-menu': '' }
  placement: 'down' | 'up'
}

interface FloatingState {
  placement: FloatingPlacement
  listSpace: number
  triggerWidth: number
}

/**
 * Anchors a portaled menu to its trigger without growing the document.
 * The menu is rendered in `document.body` with `position: fixed`, flipped when the
 * viewport has no room below, and constrained to the space available on the
 * chosen side through the `--floating-menu-max-height` custom property.
 */
export function useFloatingMenu({
  open,
  anchor,
  panel,
  width = 'content',
  gap = 6,
  padding = 8,
  chromeRef,
}: UseFloatingMenuOptions): FloatingMenu {
  const [state, setState] = useState<FloatingState | null>(null)
  const [triggerWidth, setTriggerWidth] = useState(0)
  const naturalHeightRef = useRef(0)

  useLayoutEffect(() => {
    const element = anchor.current
    if (!element) return
    const update = () => {
      const next = element.getBoundingClientRect().width
      setTriggerWidth(previous => (previous === next ? previous : next))
    }
    update()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(update)
    observer.observe(element)
    return () => observer.disconnect()
  }, [anchor])

  useLayoutEffect(() => {
    if (!open) return
    const trigger = anchor.current
    const panelElement = panel.current
    if (!trigger || !panelElement) return

    floatingAnchors.set(panelElement, trigger)
    // The content height, capped by the design height of the menu.
    const measure = () => Math.min(panelElement.scrollHeight, readMenuCap(panelElement))
    naturalHeightRef.current = measure()

    const reposition = () => {
      const rect = trigger.getBoundingClientRect()
      const chrome = chromeRef?.current?.getBoundingClientRect().height ?? 0
      const placement = computeFloatingPlacement({
        trigger: {
          top: rect.top,
          bottom: rect.bottom,
          left: rect.left,
          right: rect.right,
          width: rect.width,
        },
        viewportWidth: window.innerWidth,
        viewportHeight: window.innerHeight,
        menuHeight: naturalHeightRef.current,
        menuWidth: panelElement.getBoundingClientRect().width,
        gap,
        padding,
      })
      const next: FloatingState = {
        placement,
        listSpace: Math.max(placement.maxHeight - chrome, 0),
        triggerWidth: rect.width,
      }
      setState(previous =>
        previous &&
        previous.placement.placement === next.placement.placement &&
        previous.placement.top === next.placement.top &&
        previous.placement.left === next.placement.left &&
        previous.placement.maxHeight === next.placement.maxHeight &&
        previous.listSpace === next.listSpace &&
        previous.triggerWidth === next.triggerWidth
          ? previous
          : next,
      )
    }

    reposition()

    const onViewportChange = () => reposition()
    window.addEventListener('scroll', onViewportChange, true)
    window.addEventListener('resize', onViewportChange)

    let observer: ResizeObserver | undefined
    if (typeof ResizeObserver !== 'undefined') {
      observer = new ResizeObserver(() => {
        naturalHeightRef.current = measure()
        reposition()
      })
      observer.observe(panelElement)
    }

    return () => {
      window.removeEventListener('scroll', onViewportChange, true)
      window.removeEventListener('resize', onViewportChange)
      observer?.disconnect()
    }
  }, [open, anchor, panel, chromeRef, gap, padding])

  const panelStyle = useMemo(() => {
    const size = width === 'trigger'
      ? { width: triggerWidth, minWidth: triggerWidth }
      : { minWidth: triggerWidth, width: 'max-content' }
    const style: Record<string, string | number> = {
      position: 'fixed',
      right: 'auto',
      zIndex: MENU_Z_INDEX,
      ...size,
    }
    if (state) {
      style.top = state.placement.top
      style.left = state.placement.left
      style['--floating-menu-max-height'] = `${state.listSpace}px`
    } else {
      style.top = 0
      style.left = 0
    }
    return style as CSSProperties
  }, [state, triggerWidth, width])

  return {
    panelProps: { style: panelStyle, 'data-floating-menu': '' },
    placement: state?.placement.placement ?? 'down',
  }
}
