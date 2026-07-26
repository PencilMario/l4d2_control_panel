import '@testing-library/jest-dom/vitest'
import {cleanup} from '@testing-library/react'
import {afterEach} from 'vitest'
import 'fake-indexeddb/auto'
afterEach(async () => {
  cleanup()
  await new Promise<void>((resolve) => {
    const request = indexedDB.deleteDatabase('l4d2-panel-vpk-uploads')
    request.onsuccess = request.onerror = request.onblocked = () => resolve()
  })
})
