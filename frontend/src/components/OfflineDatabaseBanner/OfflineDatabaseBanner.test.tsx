import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import OfflineDatabaseBanner from './OfflineDatabaseBanner'
import { fetchHealth } from '@/lib/api/health'

vi.mock('@/lib/api/health', () => ({ fetchHealth: vi.fn() }))

const BANNER_HEIGHT = 72

class MockResizeObserver {
  static instances: MockResizeObserver[] = []

  readonly observe = vi.fn()
  readonly unobserve = vi.fn()
  readonly disconnect = vi.fn()

  constructor(readonly callback: ResizeObserverCallback) {
    MockResizeObserver.instances.push(this)
  }
}

function bannerHeightVariable() {
  return document.documentElement.style.getPropertyValue('--offline-banner-height')
}

describe('OfflineDatabaseBanner offset contract', () => {
  beforeEach(() => {
    vi.mocked(fetchHealth).mockReset()
    MockResizeObserver.instances = []
    vi.stubGlobal('ResizeObserver', MockResizeObserver)
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      height: BANNER_HEIGHT,
    } as unknown as DOMRect)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    document.documentElement.style.removeProperty('--offline-banner-height')
  })

  it('publishes its measured height and clears it on unmount', async () => {
    vi.mocked(fetchHealth).mockResolvedValue({ status: 'ok', database: 'sqlite' })
    const { unmount } = render(<OfflineDatabaseBanner />)

    expect(await screen.findByRole('status')).toBeTruthy()
    await waitFor(() => expect(bannerHeightVariable()).toBe(`${BANNER_HEIGHT}px`))
    expect(MockResizeObserver.instances).toHaveLength(1)
    expect(MockResizeObserver.instances[0].observe).toHaveBeenCalledWith(
      screen.getByRole('status'),
    )

    unmount()

    expect(MockResizeObserver.instances[0].disconnect).toHaveBeenCalledTimes(1)
    expect(bannerHeightVariable()).toBe('')
  })

  it('leaves the offset variable unset outside sqlite mode', async () => {
    vi.mocked(fetchHealth).mockResolvedValue({ status: 'ok', database: 'postgres' })
    render(<OfflineDatabaseBanner />)

    await waitFor(() => expect(fetchHealth).toHaveBeenCalled())

    expect(screen.queryByRole('status')).toBeNull()
    expect(bannerHeightVariable()).toBe('')
    expect(MockResizeObserver.instances).toHaveLength(0)
  })
})
