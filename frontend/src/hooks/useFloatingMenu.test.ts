import { describe, expect, it } from 'vitest'
import {
  computeFloatingPlacement,
  type FloatingPlacementInput,
  type FloatingTriggerRect,
} from './useFloatingMenu'

const trigger: FloatingTriggerRect = { top: 100, bottom: 136, left: 40, right: 240, width: 200 }

interface Case {
  name: string
  input: FloatingPlacementInput
  placement: 'down' | 'up'
  top: number
  left: number
  maxHeight: number
}

const cases: Case[] = [
  {
    name: 'opens downward when the viewport has room below',
    input: { trigger, viewportWidth: 1280, viewportHeight: 720, menuHeight: 120 },
    placement: 'down',
    top: 142,
    left: 40,
    maxHeight: 570,
  },
  {
    name: 'opens upward when there is no room below but there is room above',
    input: {
      trigger: { top: 600, bottom: 636, left: 40, right: 240, width: 200 },
      viewportWidth: 1280,
      viewportHeight: 720,
      menuHeight: 120,
    },
    placement: 'up',
    top: 474,
    left: 40,
    maxHeight: 586,
  },
  {
    name: 'clamps to the roomier side above when neither side fits',
    input: {
      trigger: { top: 140, bottom: 176, left: 40, right: 240, width: 200 },
      viewportWidth: 1280,
      viewportHeight: 300,
      menuHeight: 200,
    },
    placement: 'up',
    top: 8,
    left: 40,
    maxHeight: 126,
  },
  {
    name: 'clamps to the roomier side below when the space below is larger',
    input: {
      trigger: { top: 100, bottom: 136, left: 40, right: 240, width: 200 },
      viewportWidth: 1280,
      viewportHeight: 300,
      menuHeight: 200,
    },
    placement: 'down',
    top: 142,
    left: 40,
    maxHeight: 150,
  },
  {
    name: 'keeps the menu inside the viewport horizontally',
    input: {
      trigger: { top: 100, bottom: 136, left: 1200, right: 1260, width: 60 },
      viewportWidth: 1280,
      viewportHeight: 720,
      menuHeight: 120,
      menuWidth: 200,
    },
    placement: 'down',
    top: 142,
    left: 1072,
    maxHeight: 570,
  },
  {
    name: 'falls back to a safe placement when the rects are empty',
    input: {
      trigger: { top: 0, bottom: 0, left: 0, right: 0, width: 0 },
      viewportWidth: 0,
      viewportHeight: 0,
      menuHeight: 0,
    },
    placement: 'down',
    top: 6,
    left: 8,
    maxHeight: 0,
  },
  {
    name: 'falls back to a safe placement when the rects are empty in a real viewport',
    input: {
      trigger: { top: 0, bottom: 0, left: 0, right: 0, width: 0 },
      viewportWidth: 1280,
      viewportHeight: 720,
      menuHeight: 120,
    },
    placement: 'down',
    top: 6,
    left: 8,
    maxHeight: 706,
  },
]

describe('computeFloatingPlacement', () => {
  it.each(cases)('$name', ({ input, placement, top, left, maxHeight }) => {
    const result = computeFloatingPlacement(input)

    expect(result.placement).toBe(placement)
    expect(result.top).toBe(top)
    expect(result.left).toBe(left)
    expect(result.maxHeight).toBe(maxHeight)
  })
})
