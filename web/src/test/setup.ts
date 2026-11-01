import '@testing-library/jest-dom/vitest'
import { act, cleanup } from '@testing-library/react'
import { toast } from 'sonner'

// sonner keeps toasts in a module-wide store: dismiss them so one test's
// toasts never satisfy, or fail, the next test's assertions.
afterEach(() => {
  act(() => {
    toast.dismiss()
  })
  cleanup()
})

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
// Radix Select uses pointer capture and scrollIntoView, which jsdom lacks.
Element.prototype.hasPointerCapture ??= () => false
Element.prototype.releasePointerCapture ??= () => {}
Element.prototype.scrollIntoView ??= () => {}
