import assert from 'node:assert/strict'
import { after, afterEach, before, beforeEach, describe, test } from 'node:test'
import { deflateSync } from 'node:zlib'

import { GlobalRegistrator } from '@happy-dom/global-registrator'
import type { Root } from 'react-dom/client'

import type { ImageGenerationRequest } from '../hooks/use-image-generation'
import type { ImageHistoryItem } from '../lib/image-history'

function crc32(bytes: Uint8Array) {
  let crc = 0xffffffff
  for (const byte of bytes) {
    crc ^= byte
    for (let bit = 0; bit < 8; bit += 1) {
      crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0)
    }
  }
  return (crc ^ 0xffffffff) >>> 0
}
function pngBase64(width: number, height: number) {
  const header = new Uint8Array(13)
  const view = new DataView(header.buffer)
  view.setUint32(0, width)
  view.setUint32(4, height)
  header[8] = 8
  header[9] = 6
  const raw = new Uint8Array((width * 4 + 1) * height)
  const chunk = (type: string, data: Uint8Array) => {
    const bytes = new Uint8Array(data.length + 12)
    new DataView(bytes.buffer).setUint32(0, data.length)
    bytes.set(
      [...type].map((character) => character.charCodeAt(0)),
      4
    )
    bytes.set(data, 8)
    new DataView(bytes.buffer).setUint32(
      8 + data.length,
      crc32(bytes.slice(4, 8 + data.length))
    )
    return Buffer.from(bytes)
  }
  return Buffer.concat([
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    chunk('IHDR', header),
    chunk('IDAT', deflateSync(raw)),
    chunk('IEND', new Uint8Array()),
  ]).toString('base64')
}
const squarePngBase64 = pngBase64(1024, 1024)
const landscapePngBase64 = pngBase64(1536, 864)

let act: typeof import('react').act
let createElement: typeof import('react').createElement
let flushSync: typeof import('react-dom').flushSync
let createRoot: typeof import('react-dom/client').createRoot
let i18n: typeof import('@/i18n/config').default
let useAuthStore: typeof import('@/stores/auth-store').useAuthStore
let ImageHistoryPanel: typeof import('./image-studio-workbench').ImageHistoryPanel
let ImageHistoryUrlScope: typeof import('./image-studio-workbench').ImageHistoryUrlScope
let ImageStudioWorkbench: typeof import('./image-studio-workbench').ImageStudioWorkbench

let container: HTMLDivElement
let root: Root
let requests: Array<{ aspect: string; prompt: string; signal: AbortSignal }> =
  []
const nodeEnvironmentKey = ['NODE', 'ENV'].join('_')
const originalNodeEnvironment = process.env[nodeEnvironmentKey]

const pendingRequest: ImageGenerationRequest = (prompt, aspect, signal) => {
  requests.push({ prompt, aspect, signal })
  return new Promise(() => undefined)
}

function aspectButton(label: string, ratio: string) {
  const button = [...container.querySelectorAll('button')].find((candidate) => {
    const lines =
      candidate.querySelector('span > span')?.parentElement?.children
    return (
      lines?.length === 2 &&
      lines[0]?.textContent === label &&
      lines[1]?.textContent === ratio
    )
  })
  assert.ok(button)
  assert.equal(button.textContent, `${label}${ratio}`)
  return button as HTMLButtonElement
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
    aspect: 'square',
    blob: new Blob([new Uint8Array([1])], { type: 'image/png' }),
    createdAt: 1,
    generationId: `generation-${id}`,
    height: 1024,
    id,
    model: 'gpt-image-2',
    ownerId: 47,
    prompt: `Saved prompt ${id}`,
    size: '1024×1024 PNG',
    width: 1024,
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

function assertFixedQuantity(label: string) {
  const quantityLabel = [...container.querySelectorAll('p')].find(
    (element) => element.textContent === label
  )
  assert.ok(quantityLabel)
  const quantityControls = quantityLabel.nextElementSibling?.children
  assert.ok(quantityControls)
  assert.equal(quantityControls[0]?.textContent, '1')
  const unavailableQuantityButtons =
    quantityLabel.nextElementSibling?.querySelectorAll('button')
  assert.equal(unavailableQuantityButtons?.length, 2)
  assert.equal(unavailableQuantityButtons?.[0]?.textContent, '2')
  assert.equal(unavailableQuantityButtons?.[1]?.textContent, '4')
  assert.equal(unavailableQuantityButtons?.[0]?.getAttribute('disabled'), '')
  assert.equal(unavailableQuantityButtons?.[1]?.getAttribute('disabled'), '')
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
  process.env[nodeEnvironmentKey] = 'development'
  ;(
    globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
  ).IS_REACT_ACT_ENVIRONMENT = true
  ;({ act, createElement } = await import('react'))
  ;({ flushSync } = await import('react-dom'))
  ;({ createRoot } = await import('react-dom/client'))
  ;({ default: i18n } = await import('@/i18n/config'))
  ;({ useAuthStore } = await import('@/stores/auth-store'))
  ;({ ImageHistoryPanel, ImageHistoryUrlScope, ImageStudioWorkbench } =
    await import('./image-studio-workbench'))
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
  await act(async () => {
    root.unmount()
    useAuthStore.getState().auth.reset('idle')
  })
  container.remove()
})

after(() => {
  GlobalRegistrator.unregister()
  if (originalNodeEnvironment === undefined) {
    delete process.env[nodeEnvironmentKey]
  } else {
    process.env[nodeEnvironmentKey] = originalNodeEnvironment
  }
})

describe('image studio workbench', () => {
  test('exposes all four aspect controls as two-line buttons', () => {
    renderWorkbench()
    for (const [label, ratio] of [
      ['Square image', '1:1'],
      ['Xiaohongshu', '3:4'],
      ['Landscape', '16:9'],
      ['Douyin', '9:16'],
    ]) {
      assert.ok(aspectButton(label, ratio))
    }
  })

  test('does not loop when the real history URL scope updates after history initialization', async () => {
    const originalConsoleError = console.error
    const consoleErrors: string[] = []
    console.error = (...arguments_) => {
      consoleErrors.push(arguments_.join(' '))
    }
    try {
      await act(async () => {
        useAuthStore.getState().auth.setBootstrapState('checking')
        renderWorkbench()
      })
      assert.equal(
        consoleErrors.some((message) =>
          message.includes('Maximum update depth exceeded')
        ),
        false
      )
    } finally {
      console.error = originalConsoleError
    }
  })

  test('keeps generation available when local image history cannot open', async () => {
    const originalIndexedDb = globalThis.indexedDB
    globalThis.indexedDB = undefined as unknown as IDBFactory
    try {
      renderWorkbench()
      await act(async () => {
        await Promise.resolve()
      })
      assert.match(container.textContent ?? '', /not saved to history/)
      flushSync(() => setTextareaValue('A local failure fixture'))
      flushSync(() => generateButton().click())
      assert.equal(requests.length, 1)
    } finally {
      globalThis.indexedDB = originalIndexedDb
    }
  })

  test('sends only prompt and replaces generation with a disabled stop action', async () => {
    renderWorkbench()
    flushSync(() => setTextareaValue('画一只猫'))
    flushSync(() => generateButton().click())
    assert.deepEqual(
      requests.map(({ prompt, aspect }) => ({ prompt, aspect })),
      [{ prompt: '画一只猫', aspect: 'square' }]
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

  test('snapshots the selected aspect for a pending request and its successful result', async () => {
    const response = deferred<{ data: Array<{ b64_json: string }> }>()
    renderWorkbench((prompt, aspect, signal) => {
      requests.push({ prompt, aspect, signal })
      return response.promise
    })
    flushSync(() => aspectButton('Landscape', '16:9').click())
    flushSync(() => setTextareaValue('A wide local fixture'))
    flushSync(() => generateButton().click())
    flushSync(() => aspectButton('Douyin', '9:16').click())
    assert.deepEqual(
      requests.map(({ aspect }) => aspect),
      ['landscape']
    )
    assert.match(
      container.querySelector('[data-image-studio-stage]')?.textContent ?? '',
      /1536×864/
    )

    response.resolve({ data: [{ b64_json: landscapePngBase64 }] })
    await act(async () => {
      await Promise.resolve()
    })
    const stage = container.querySelector('[data-image-studio-stage]')
    assert.ok(stage)
    assert.match(stage.textContent ?? '', /1536×864/)
    assert.match(stage.querySelector('img')?.className ?? '', /object-contain/)
    assert.match(
      stage.querySelector('img')?.parentElement?.className ?? '',
      /aspect-video/
    )
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
      data: [{ b64_json: squarePngBase64 }],
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
        data: [{ b64_json: squarePngBase64 }],
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
        renderWorkbench((prompt, aspect, signal) => {
          requests.push({ prompt, aspect, signal })
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
        response.resolve({ data: [{ b64_json: squarePngBase64 }] })
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
    assertFixedQuantity('Quantity')
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
    await i18n.changeLanguage('zhCN')
    renderWorkbench()
    assert.match(container.textContent ?? '', /把想法变成图片/)
    assertFixedQuantity('数量')
    assert.match(container.textContent ?? '', /立即生成/)
    assert.match(container.textContent ?? '', /计费说明：当前 Beta/)
    await i18n.changeLanguage('en')
  })

  test('writes the currently localized prompt example into the input', async () => {
    await act(async () => {
      await i18n.changeLanguage('zhCN')
      renderWorkbench()
    })
    const chineseExample = [...container.querySelectorAll('button')].find(
      (button) => button.textContent === '阳光照亮的绿植阅读角'
    )
    assert.ok(chineseExample)
    await act(async () => chineseExample.click())
    assert.equal(
      (container.querySelector('#image-prompt') as HTMLTextAreaElement).value,
      '阳光照亮的绿植阅读角'
    )

    await act(async () => {
      await i18n.changeLanguage('en')
      renderWorkbench()
    })
    const englishExample = [...container.querySelectorAll('button')].find(
      (button) => button.textContent === 'A sunlit reading corner with plants'
    )
    assert.ok(englishExample)
    await act(async () => englishExample.click())
    assert.equal(
      (container.querySelector('#image-prompt') as HTMLTextAreaElement).value,
      'A sunlit reading corner with plants'
    )
  })

  test('owns its named container and queries only that container for the grid', () => {
    renderWorkbench()
    const page = container.querySelector('[data-image-studio-page]')
    const layout = container.querySelector('[data-image-studio-layout]')
    assert.ok(page)
    assert.ok(layout)
    assert.match(page.className, /@container\/image-studio/)
    assert.match(
      layout.className,
      /@\[768px\]\/image-studio:grid-cols-\[340px_minmax\(0,1fr\)\]/
    )
    assert.doesNotMatch(layout.className, /\/content:/)
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
    assert.equal(disabledControls.length, 6)
    const firstDisabledControl = disabledControls[0]
    if (!(firstDisabledControl instanceof HTMLButtonElement)) {
      throw new Error('Expected the first disabled control to be a button.')
    }
    firstDisabledControl.click()
    assert.equal(requests.length, 0)
    assert.equal(container.querySelectorAll('button[disabled]').length, 8)
    assert.equal(container.querySelectorAll('[role="tooltip"]').length, 7)
    assert.match(stage.textContent ?? '', /AI image studio/)
    assert.match(stage.textContent ?? '', /Generation result/)
    assert.match(stage.textContent ?? '', /Ready/)
  })
})
