import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'

afterEach(() => cleanup())

// jsdom lacks these browser APIs that sonner and Radix use.
window.matchMedia ??= ((query: string) => ({
  matches: false,
  media: query,
  onchange: null,
  addListener: () => {},
  removeListener: () => {},
  addEventListener: () => {},
  removeEventListener: () => {},
  dispatchEvent: () => false,
})) as typeof window.matchMedia
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
}
