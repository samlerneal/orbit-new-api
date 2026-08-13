import assert from 'node:assert/strict'
import { after, afterEach, before, beforeEach, describe, test } from 'node:test'

import { GlobalRegistrator } from '@happy-dom/global-registrator'
import { act, createElement } from 'react'
import { flushSync } from 'react-dom'
import { createRoot, type Root } from 'react-dom/client'

import i18n from '@/i18n/config'
import { useAuthStore } from '@/stores/auth-store'

import type { ImageGenerationRequest } from '../hooks/use-image-generation'
import type { ImageHistoryItem } from '../lib/image-history'
import {
  ImageHistoryPanel,
  ImageHistoryUrlScope,
  ImageStudioWorkbench,
} from './image-studio-workbench'

let container: HTMLDivElement
let root: Root
let requests: Array<{ prompt: string; signal: AbortSignal }> = []

const pendingRequest: ImageGenerationRequest = (prompt, signal) => {
  requests.push({ prompt, signal })
  return new Promise(() => undefined)
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve
  })
  return { promise, resolve }
}

function renderWorkbench(requestImage = pendingRequest) {
  flushSync(() =>
    root.render(createElement(ImageStudioWorkbench, { requestImage }))
  )
}

function createHistoryItem(id: string): ImageHistoryItem {
  return {
    blob: new Blob([new Uint8Array([1])], { type: 'image/png' }),
    createdAt: 1,
    generationId: `generation-${id}`,
    id,
    model: 'gpt-image-2',
    ownerId: 47,
    prompt: `Saved prompt ${id}`,
    size: '1024×1024 PNG',
  }
}

function HistoryUrlHarness(props: { history: ImageHistoryItem[] }) {
  return (
    <ImageHistoryUrlScope history={props.history}>
      {(urls: Record<string, string>) =>
        createElement(
          'div',
          null,
          props.history.map((item) =>
            createElement('img', {
              alt: item.id,
              key: item.id,
              src: urls[item.id],
            })
          )
        )
      }
    </ImageHistoryUrlScope>
  )
}

function generateButton() {
  const button = [...container.querySelectorAll('button')].find(
    (candidate) => candidate.textContent === 'Generate now'
  )
  assert.ok(button)
  return button as HTMLButtonElement
}

function setTextareaValue(value: string) {
  const textarea = container.querySelector(
    '#image-prompt'
  ) as HTMLTextAreaElement
  assert.ok(textarea)
  const setter = Object.getOwnPropertyDescriptor(
    HTMLTextAreaElement.prototype,
    'value'
  )?.set
  assert.ok(setter)
  setter.call(textarea, value)
  textarea.dispatchEvent(new Event('input', { bubbles: true }))
}

before(async () => {
  GlobalRegistrator.register({ url: 'http://localhost/image-studio' })
  ;(
    globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
  ).IS_REACT_ACT_ENVIRONMENT = true
  await i18n.changeLanguage('en')
})

beforeEach(() => {
  container = document.createElement('div')
  document.body.replaceChildren(container)
  root = createRoot(container)
  requests = []
  useAuthStore
    .getState()
    .auth.setUser({ id: 47, role: 1, username: 'image-history-test' })
  useAuthStore.getState().auth.setBootstrapState('complete')
})

afterEach(async () => {
  flushSync(() => root.unmount())
  useAuthStore.getState().auth.reset('idle')
  container.remove()
})

after(() => {
  GlobalRegistrator.unregister()
})

describe('image studio workbench', () => {
  test('sends only prompt and replaces generation with a disabled stop action', async () => {
    renderWorkbench()
    flushSync(() => setTextareaValue('画一只猫'))
    flushSync(() => generateButton().click())
    assert.deepEqual(
      requests.map(({ prompt }) => prompt),
      ['画一只猫']
    )

    assert.equal(
      container.querySelector('button[data-slot="button"]')?.textContent,
      'Stop waiting'
    )
    assert.equal(
      [...container.querySelectorAll('button')].some(
        (candidate) => candidate.textContent === 'Generate now'
      ),
      false
    )
    assert.equal(requests.length, 1)
  })

  test('enforces the 4,000 Unicode code point boundary', async () => {
    renderWorkbench()
    flushSync(() => setTextareaValue('😀'.repeat(4000)))
    assert.equal(generateButton().disabled, false)
    assert.match(container.textContent ?? '', /4000\/4000/)
    flushSync(() => setTextareaValue('😀'.repeat(4001)))
    assert.equal(generateButton().disabled, true)
    assert.match(container.textContent ?? '', /4001\/4000/)
  })

  test('keeps the successful image frame separate from the accessible download action', async () => {
    renderWorkbench(async () => ({
      data: [{ b64_json: 'iVBORw0KGgo=' }],
    }))
    flushSync(() => setTextareaValue('A fixed local test image'))
    flushSync(() => generateButton().click())
    await act(async () => {
      await Promise.resolve()
    })

    const stage = container.querySelector('[data-image-studio-stage]')
    const image = stage?.querySelector('img[alt="Generated image"]')
    const download = stage?.querySelector('a[download="image-studio.png"]')
    assert.ok(stage)
    assert.ok(image)
    assert.ok(download)
    assert.equal(download.textContent, 'Download image')
    assert.match(download.className, /min-h-11/)

    const imageFrame = image.parentElement
    assert.ok(imageFrame)
    assert.equal(imageFrame.classList.contains('aspect-square'), true)
    assert.equal(imageFrame.contains(download), false)
    assert.equal(stage.contains(download), true)
  })

  test('removes the current image from the DOM after the authenticated owner changes', async () => {
    const revoked: string[] = []
    const originalCreateObjectUrl = URL.createObjectURL
    const originalRevokeObjectUrl = URL.revokeObjectURL
    URL.createObjectURL = () => 'blob:current-owner'
    URL.revokeObjectURL = (url) => revoked.push(url)
    try {
      renderWorkbench(async () => ({
        data: [{ b64_json: 'iVBORw0KGgo=' }],
      }))
      flushSync(() => setTextareaValue('A private local fixture'))
      flushSync(() => generateButton().click())
      await act(async () => {
        await Promise.resolve()
      })
      assert.ok(container.querySelector('img[alt="Generated image"]'))

      flushSync(() => {
        useAuthStore.getState().auth.setUser({
          id: 48,
          role: 1,
          username: 'next-owner',
        })
      })
      await act(async () => {
        await Promise.resolve()
      })
      assert.equal(container.querySelector('img[alt="Generated image"]'), null)
      assert.deepEqual(revoked, ['blob:current-owner'])
    } finally {
      URL.createObjectURL = originalCreateObjectUrl
      URL.revokeObjectURL = originalRevokeObjectUrl
    }
  })

  test('fails closed when a deferred success resolves after changing owners or logging out', async () => {
    const revoked: string[] = []
    const originalCreateObjectUrl = URL.createObjectURL
    const originalRevokeObjectUrl = URL.revokeObjectURL
    let createdUrls = 0
    URL.createObjectURL = () => `blob:late-${++createdUrls}`
    URL.revokeObjectURL = (url) => revoked.push(url)
    try {
      for (const nextAuth of [
        { id: 48, bootstrapState: 'complete' as const },
        { id: null, bootstrapState: 'complete' as const },
      ]) {
        requests = []
        const response = deferred<{
          data: Array<{ b64_json: string }>
        }>()
        renderWorkbench((prompt, signal) => {
          requests.push({ prompt, signal })
          return response.promise
        })
        flushSync(() => setTextareaValue('A deferred local fixture'))
        flushSync(() => generateButton().click())
        assert.equal(requests.length, 1)
        assert.equal(requests[0]?.prompt, 'A deferred local fixture')
        assert.equal(requests[0]?.signal.aborted, false)

        flushSync(() => {
          useAuthStore
            .getState()
            .auth.setUser(
              nextAuth.id === null
                ? null
                : { id: nextAuth.id, role: 1, username: 'next-owner' }
            )
          useAuthStore
            .getState()
            .auth.setBootstrapState(nextAuth.bootstrapState)
        })
        response.resolve({ data: [{ b64_json: 'iVBORw0KGgo=' }] })
        await act(async () => {
          await Promise.resolve()
        })

        assert.equal(requests.length, 1)
        assert.equal(
          container.querySelector('img[alt="Generated image"]'),
          null
        )
        assert.equal(
          container.querySelectorAll('[data-image-history-list] img').length,
          0
        )
        useAuthStore
          .getState()
          .auth.setUser({ id: 47, role: 1, username: 'image-history-test' })
      }
      assert.deepEqual(revoked, ['blob:late-1', 'blob:late-2'])
    } finally {
      URL.createObjectURL = originalCreateObjectUrl
      URL.revokeObjectURL = originalRevokeObjectUrl
    }
  })

  test('selects, deletes, and confirms clearing saved image history through real controls', () => {
    const selected: string[] = []
    const removed: string[] = []
    let clearCalls = 0
    const item = createHistoryItem('one')
    flushSync(() =>
      root.render(
        createElement(ImageHistoryPanel, {
          history: [item],
          historyUrls: { one: 'blob:saved' },
          onClear: () => {
            clearCalls += 1
          },
          onRemove: (id) => removed.push(id),
          onSelect: (id) => selected.push(id),
          saveWarning: false,
          selectedHistoryId: null,
        })
      )
    )

    const selection = container.querySelector(
      'button[aria-label^="View saved image"]'
    ) as HTMLButtonElement
    assert.ok(selection)
    const historyList = container.querySelector('[data-image-history-list]')
    assert.ok(historyList)
    assert.match(historyList.className, /overflow-x-auto/)
    assert.match(historyList.className, /min-w-0/)
    selection.click()
    assert.deepEqual(selected, ['one'])
    const deleteButton = [...container.querySelectorAll('button')].find(
      (button) => button.textContent === 'Delete'
    )
    assert.ok(deleteButton)
    deleteButton.click()
    assert.deepEqual(removed, ['one'])

    const originalConfirm = window.confirm
    window.confirm = () => false
    try {
      const clearButton = [...container.querySelectorAll('button')].find(
        (button) => button.textContent === 'Clear all'
      )
      assert.ok(clearButton)
      clearButton.click()
      assert.equal(clearCalls, 0)
      window.confirm = () => true
      clearButton.click()
      assert.equal(clearCalls, 1)
    } finally {
      window.confirm = originalConfirm
    }
  })

  test('reclaims history URLs when the selected list is replaced and unmounted', () => {
    const revoked: string[] = []
    const originalCreateObjectUrl = URL.createObjectURL
    const originalRevokeObjectUrl = URL.revokeObjectURL
    let counter = 0
    URL.createObjectURL = () => `blob:history-${++counter}`
    URL.revokeObjectURL = (url) => revoked.push(url)
    try {
      flushSync(() =>
        root.render(
          createElement(HistoryUrlHarness, {
            history: [createHistoryItem('one')],
          })
        )
      )
      flushSync(() =>
        root.render(
          createElement(HistoryUrlHarness, {
            history: [createHistoryItem('two')],
          })
        )
      )
      flushSync(() => root.unmount())
      assert.deepEqual(revoked, ['blob:history-1', 'blob:history-2'])
      root = createRoot(container)
    } finally {
      URL.createObjectURL = originalCreateObjectUrl
      URL.revokeObjectURL = originalRevokeObjectUrl
    }
  })

  test('renders the fixed recipe with one accessible model option and no key or size input', async () => {
    renderWorkbench()
    assert.match(container.textContent ?? '', /Turn ideas into images/)
    assert.match(container.textContent ?? '', /1 image/)
    assert.match(container.textContent ?? '', /Text to image/)
    assert.match(container.textContent ?? '', /Image to image/)
    assert.match(container.textContent ?? '', /Billing notice:/)
    assert.equal(
      container.querySelector('strong')?.textContent,
      'Billing notice:'
    )
    const modelControl = container.querySelector('#image-model')
    assert.ok(modelControl instanceof HTMLSelectElement)
    assert.equal(modelControl.options.length, 1)
    assert.equal(modelControl.value, 'gpt-image-2')
    assert.equal(container.querySelector('input[type="file"]'), null)
    assert.equal(container.querySelector('input[name*="key" i]'), null)
    assert.equal(container.querySelector('input[name*="model" i]'), null)
    assert.equal(container.querySelector('input[name*="size" i]'), null)
  })

  test('localizes the hierarchy, action, and billing notice', async () => {
    await i18n.changeLanguage('zh')
    renderWorkbench()
    assert.match(container.textContent ?? '', /把想法变成图片/)
    assert.match(container.textContent ?? '', /1 张/)
    assert.match(container.textContent ?? '', /立即生成/)
    assert.match(container.textContent ?? '', /计费说明：当前 Beta/)
    await i18n.changeLanguage('en')
  })

  test('keeps a single prompt control, data-driven settings, and a separate result stage', async () => {
    renderWorkbench()
    const layout = container.querySelector('[data-image-studio-layout]')
    const config = container.querySelector('[data-image-studio-config]')
    const stage = container.querySelector('[data-image-studio-stage]')
    assert.ok(layout)
    assert.ok(config)
    assert.ok(stage)
    assert.equal(layout.children.length, 2)
    assert.equal(container.querySelectorAll('#image-prompt').length, 1)
    assert.equal(stage.getAttribute('aria-live'), 'polite')
    const settings = container.querySelector(
      '[data-image-studio-fixed-settings]'
    )
    assert.ok(settings)
    const disabledControls = settings.querySelectorAll('button[disabled]')
    assert.equal(disabledControls.length, 8)
    const firstDisabledControl = disabledControls[0]
    if (!(firstDisabledControl instanceof HTMLButtonElement)) {
      throw new Error('Expected the first disabled control to be a button.')
    }
    firstDisabledControl.click()
    assert.equal(requests.length, 0)
    assert.equal(container.querySelectorAll('button[disabled]').length, 9)
    assert.equal(container.querySelectorAll('[role="tooltip"]').length, 9)
    assert.match(stage.textContent ?? '', /AI image studio/)
    assert.match(stage.textContent ?? '', /Generation result/)
    assert.match(stage.textContent ?? '', /Ready/)
  })
})
