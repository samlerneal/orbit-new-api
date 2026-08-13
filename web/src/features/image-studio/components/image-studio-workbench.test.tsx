import assert from 'node:assert/strict'
import { after, afterEach, before, beforeEach, describe, test } from 'node:test'

import { GlobalRegistrator } from '@happy-dom/global-registrator'
import { createElement } from 'react'
import { flushSync } from 'react-dom'
import { createRoot, type Root } from 'react-dom/client'

import i18n from '@/i18n/config'

import type { ImageGenerationRequest } from '../hooks/use-image-generation'
import { ImageStudioWorkbench } from './image-studio-workbench'

let container: HTMLDivElement
let root: Root
let requests: Array<{ prompt: string; signal: AbortSignal }> = []

const pendingRequest: ImageGenerationRequest = (prompt, signal) => {
  requests.push({ prompt, signal })
  return new Promise(() => undefined)
}

function renderWorkbench() {
  flushSync(() =>
    root.render(
      createElement(ImageStudioWorkbench, { requestImage: pendingRequest })
    )
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
})

afterEach(async () => {
  flushSync(() => root.unmount())
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
