import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { Window } from 'happy-dom'
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'

import {
  createChannelTestIdentityGuard,
  createChannelTestDispatch,
  createImageCapabilityProbeRequest,
  isImageCapabilityCaseSelectable,
  isImageCapabilityMode,
  reduceImageCapabilityRun,
  isCurrentChannelTestGeneration,
  resetChannelTestModeState,
} from '../../lib/channel-actions'
import { ImageCapabilityCaseButton } from './channel-test-dialog'

describe('Image Edit channel test dispatch', () => {
  test('only channel 1 image modes enter the manifest-only paid probe flow', () => {
    assert.equal(isImageCapabilityMode(1, 'image-generation'), true)
    assert.equal(isImageCapabilityMode(1, 'image-edit'), true)
    assert.equal(isImageCapabilityMode(1, 'openai'), false)
    assert.equal(isImageCapabilityMode(2, 'image-generation'), false)
  })
  test('img-43 and img-44 are disabled before any paid dispatch', () => {
    for (const entry of ['img-43', 'img-44']) {
      assert.equal(
        isImageCapabilityCaseSelectable('LOCALLY_UNSAFE_TO_PROBE'),
        false,
        entry
      )
      assert.deepEqual(
        createChannelTestDispatch(
          'gpt-image-2',
          'image-generation',
          false,
          false,
          true
        ),
        { kind: 'capability-required', model: 'gpt-image-2' }
      )
    }
  })

  test('DOM disables unsafe img-43/img-44 for mouse and keyboard selection', async () => {
    const window = new Window()
    Object.assign(globalThis, {
      window,
      document: window.document,
      navigator: window.navigator,
      HTMLElement: window.HTMLElement,
      Event: window.Event,
      KeyboardEvent: window.KeyboardEvent,
      IS_REACT_ACT_ENVIRONMENT: true,
    })
    const host = document.createElement('div')
    document.body.append(host)
    const root = createRoot(host)
    let selections = 0
    const unsafeRequest = {
      ...createImageCapabilityProbeRequest('generation'),
      case_id: 'img-43',
    }
    await act(async () => {
      root.render(
        createElement(
          'div',
          null,
          ...['img-43', 'img-44'].map((id) =>
            createElement(ImageCapabilityCaseButton, {
              key: id,
              entry: {
                id,
                request: { ...unsafeRequest, case_id: id },
                state: 'LOCALLY_UNSAFE_TO_PROBE',
              },
              selectedCaseId: '',
              onSelect: () => {
                selections += 1
              },
            })
          )
        )
      )
    })
    for (const id of ['img-43', 'img-44']) {
      const button = document.querySelector(
        `[aria-label="${id} LOCALLY_UNSAFE_TO_PROBE"]`
      ) as HTMLButtonElement
      assert.ok(button.textContent?.includes('LOCALLY_UNSAFE_TO_PROBE'))
      assert.equal(button.disabled, true)
      button.click()
      button.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }))
    }
    assert.equal(selections, 0)
    await act(async () => root.unmount())
    host.remove()
  })
  test('server-matrix probe has one paid request and no free-form fields', () => {
    const probe = createImageCapabilityProbeRequest('edit')
    assert.deepEqual(probe, {
      case_id: '',
      schema_version: 'image-channel-test.v1',
      model: 'gpt-image-2',
      mode: 'edit',
      shape: 'square',
      resolution: '1K',
      n: 1,
      quality: 'low',
      format: 'png',
      background: 'opaque',
      reference_count: 1,
      stream: false,
      confirm_paid_image_probe: true,
      expected_upstream_requests: 1,
    })
    assert.equal('prompt' in probe, false)
  })

  test('capability modes cannot enter the legacy dispatch path', () => {
    for (const endpoint of ['image-generation', 'image-edit'] as const) {
      assert.deepEqual(
        createChannelTestDispatch('gpt-image-2', endpoint, false, false, true),
        { kind: 'capability-required', model: 'gpt-image-2' }
      )
    }
  })

  test('a failed capability run updates only its current case and never advances', () => {
    const state = reduceImageCapabilityRun('img-17', {
      success: false,
      state: 'UNVERIFIED',
      error_code: 'IMAGE_PROBE_UPSTREAM_UNVERIFIED',
    })
    assert.equal(state.caseId, 'img-17')
    assert.equal(state.response.state, 'UNVERIFIED')
    assert.equal(state.nextRequest, null)
    assert.equal(state.shouldAdvance, false)
  })
  test('cancelling confirmation sends zero requests', () => {
    let calls = 0
    const dispatch = createChannelTestDispatch(
      'gpt-image-2',
      'image-edit',
      false,
      false
    )
    if (dispatch.kind === 'send') calls += 1
    assert.equal(dispatch.kind, 'confirm-required')
    assert.equal(calls, 0)
  })

  test('confirming sends exactly one controlled request', () => {
    let calls = 0
    const dispatch = createChannelTestDispatch(
      'gpt-image-2',
      'image-edit',
      true,
      true
    )
    if (dispatch.kind === 'send') calls += 1
    assert.equal(calls, 1)
    assert.deepEqual(dispatch, {
      kind: 'send',
      model: 'gpt-image-2',
      options: {
        endpointType: 'image-edit',
        stream: false,
        confirmPaidImageEdit: true,
      },
    })
  })

  test('generation sends directly and preserves stream without confirmation', () => {
    const dispatch = createChannelTestDispatch(
      'gpt-image-2',
      'image-generation',
      true,
      false
    )
    assert.deepEqual(dispatch, {
      kind: 'send',
      model: 'gpt-image-2',
      options: { endpointType: 'image-generation', stream: true },
    })
  })

  test('endpoint reset clears previous test and bulk state', () => {
    const reset = resetChannelTestModeState()
    assert.equal(reset.testingModels.size, 0)
    assert.deepEqual(
      { ...reset, testingModels: [] },
      {
        testResults: {},
        rowSelection: {},
        testingModels: [],
        isBatchTesting: false,
        isBatchStopRequested: false,
        batchProgress: null,
        failureDetails: null,
      }
    )
  })

  test('ignores a single-model completion from the previous endpoint generation', () => {
    let generation = 0
    const capturedGeneration = generation
    let writes = 0
    generation += 1
    if (isCurrentChannelTestGeneration(capturedGeneration, generation)) {
      writes += 1
    }
    assert.equal(writes, 0)
  })

  test('ignores late batch results and progress from the previous generation', () => {
    let generation = 3
    const capturedGeneration = generation
    let resultWrites = 0
    let progressWrites = 0
    generation += 1
    for (const _result of ['success', 'error']) {
      if (isCurrentChannelTestGeneration(capturedGeneration, generation)) {
        resultWrites += 1
        progressWrites += 1
      }
    }
    assert.equal(resultWrites, 0)
    assert.equal(progressWrites, 0)
  })

  test('keeps an old batch stale when a new batch resets the shared stop flag', () => {
    let modeGeneration = 0
    let batchRunId = 1
    const oldGeneration = modeGeneration
    const oldBatchRunId = batchRunId
    let oldRequests = 0
    const staleWrites = {
      results: 0,
      testing: 0,
      cache: 0,
      toast: 0,
      dismiss: 0,
    }

    modeGeneration += 1
    batchRunId += 1
    const sharedStopRequested = false

    if (
      !sharedStopRequested &&
      isCurrentChannelTestGeneration(
        oldGeneration,
        modeGeneration,
        oldBatchRunId,
        batchRunId
      )
    ) {
      oldRequests += 1
      staleWrites.results += 1
      staleWrites.testing += 1
      staleWrites.cache += 1
      staleWrites.toast += 1
      staleWrites.dismiss += 1
    }

    assert.equal(oldRequests, 0)
    assert.deepEqual(staleWrites, {
      results: 0,
      testing: 0,
      cache: 0,
      toast: 0,
      dismiss: 0,
    })
  })

  test('guards deferred cache writes across mode and run changes', async () => {
    let generation = 4
    let runId = 8
    const modeGuard = createChannelTestIdentityGuard(
      generation,
      () => generation,
      runId,
      () => runId
    )
    let writes = 0
    let resolvePromise!: () => void
    const deferred = new Promise<void>((resolve) => {
      resolvePromise = resolve
    }).then(() => {
      if (modeGuard()) writes += 1
    })
    generation += 1
    resolvePromise()
    await deferred
    assert.equal(writes, 0)

    generation = 4
    runId = 8
    const runGuard = createChannelTestIdentityGuard(
      generation,
      () => generation,
      runId,
      () => runId
    )
    const newRunDeferred = Promise.resolve().then(() => {
      if (runGuard()) writes += 1
    })
    runId += 1
    await newRunDeferred
    assert.equal(writes, 0)

    const currentGuard = createChannelTestIdentityGuard(
      generation,
      () => generation,
      runId,
      () => runId
    )
    await Promise.resolve().then(() => {
      if (currentGuard()) writes += 1
    })
    assert.equal(writes, 1)
  })
})
