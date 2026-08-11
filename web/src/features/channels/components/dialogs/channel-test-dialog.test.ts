import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { Window } from 'happy-dom'
import { act, createElement, StrictMode, useEffect, useRef } from 'react'
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
import {
  createImageCapabilityProbeLifetime,
  invalidateImageCapabilityProbeLifetime,
  runImageCapabilityProbeStateMachine,
  setupImageCapabilityProbeLifetime,
  shouldStoreImageCapabilityProbeResult,
  type ImageCapabilityResponse,
} from '../../types'
import {
  ImageCapabilityCaseButton,
  ImageCapabilityCaseMatrix,
} from './channel-test-dialog'

function StrictModeProbeLifetimeHarness({
  onSetup,
  onReady,
}: {
  onSetup: () => void
  onReady: (value: {
    lifetime: ReturnType<typeof createImageCapabilityProbeLifetime>
    abortRef: { current: AbortController | null }
    inFlightCases: Set<string>
  }) => void
}) {
  const lifetimeRef = useRef(createImageCapabilityProbeLifetime())
  const abortRef = useRef<AbortController | null>(null)
  const inFlightCasesRef = useRef(new Set<string>())

  useEffect(() => {
    const cleanup = setupImageCapabilityProbeLifetime(
      lifetimeRef.current,
      abortRef,
      inFlightCasesRef.current
    )
    onSetup()
    return cleanup
  }, [onSetup])

  useEffect(() => {
    onReady({
      lifetime: lifetimeRef.current,
      abortRef,
      inFlightCases: inFlightCasesRef.current,
    })
  }, [onReady])

  return null
}

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
      HTMLElement: window.HTMLElement,
      Event: window.Event,
      KeyboardEvent: window.KeyboardEvent,
      IS_REACT_ACT_ENVIRONMENT: true,
    })
    Object.defineProperty(globalThis, 'navigator', {
      configurable: true,
      value: window.navigator,
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
  test('DOM displays the selected server case and sends its exact frozen payload', async () => {
    const window = new Window()
    Object.assign(globalThis, {
      window,
      document: window.document,
      HTMLElement: window.HTMLElement,
      Event: window.Event,
      KeyboardEvent: window.KeyboardEvent,
      IS_REACT_ACT_ENVIRONMENT: true,
    })
    Object.defineProperty(globalThis, 'navigator', {
      configurable: true,
      value: window.navigator,
    })
    const host = document.createElement('div')
    document.body.append(host)
    const root = createRoot(host)
    const img01 = {
      ...createImageCapabilityProbeRequest('generation'),
      case_id: 'img-01',
    }
    const img02 = {
      ...createImageCapabilityProbeRequest('edit'),
      case_id: 'img-02',
      resolution: '2K' as const,
      n: 2 as const,
      quality: 'high' as const,
      format: 'webp' as const,
      background: 'transparent' as const,
      reference_count: 2 as const,
    }
    const img03 = { ...img01, case_id: 'img-03' }
    const img04 = { ...img01, case_id: 'img-04' }
    let selected = img01
    let isAnyRunInFlight = false
    const sent: (typeof img02)[] = []
    const results: Record<string, ImageCapabilityResponse> = {
      'img-03': {
        success: true,
        case_id: 'img-03',
        state: 'OBSERVED_SUPPORTED',
        latency_ms: 1,
        data: {
          mode: 'generation',
          shape: 'square',
          resolution: '1K',
          n: 1,
          quality: 'low',
          format: 'png',
          background: 'opaque',
          reference_count: 0,
          actual_image_count: 1,
          dimensions: ['1024x1024'],
          result_format: 'png',
          request_id_present: true,
          task_id_present: true,
          usage_present: false,
          billable_present: false,
          cost_present: false,
        },
      },
      'img-04': {
        success: false,
        case_id: 'img-04',
        state: 'UNVERIFIED',
        error_code: 'IMAGE_PROBE_UPSTREAM_UNVERIFIED',
        data: {
          mode: 'edit',
          shape: 'square',
          resolution: '2K',
          n: 2,
          quality: 'high',
          format: 'webp',
          background: 'transparent',
          reference_count: 2,
          actual_image_count: 0,
          dimensions: [],
          result_format: 'UNKNOWN',
          request_id_present: true,
          task_id_present: false,
          usage_present: true,
          billable_present: true,
          cost_present: true,
        },
      },
    }
    const render = async () => {
      await act(async () => {
        root.render(
          createElement(ImageCapabilityCaseMatrix, {
            entries: [
              { id: 'img-01', request: img01, state: 'NOT_PROBED' },
              { id: 'img-02', request: img02, state: 'NOT_PROBED' },
              { id: 'img-03', request: img03, state: 'NOT_PROBED' },
              { id: 'img-04', request: img04, state: 'NOT_PROBED' },
            ],
            selected,
            results,
            inFlightCaseId: null,
            isAnyRunInFlight,
            onSelect: (request) => {
              selected = request
            },
            onRequestRun: (request) => {
              sent.push(request as typeof img02)
              isAnyRunInFlight = true
            },
          })
        )
      })
    }
    await render()
    assert.match(host.textContent ?? '', /Selected case: img-01/)
    assert.match(
      host.textContent ?? '',
      /mode=generation; shape=square; resolution=1K; n=1; quality=low; format=png; background=opaque; reference_count=0/
    )
    assert.match(
      host.textContent ?? '',
      /img-03: OBSERVED_SUPPORTED; latency_ms=1; request_id_present=true; task_id_present=true/
    )
    assert.match(
      host.textContent ?? '',
      /img-04: UNVERIFIED; IMAGE_PROBE_UPSTREAM_UNVERIFIED; request_id_present=true; task_id_present=false/
    )
    ;([...host.querySelectorAll('button')] as HTMLButtonElement[])
      .find((button) => button.textContent?.includes('Run current case'))
      ?.click()
    assert.deepEqual(sent, [img01])
    await render()
    ;(
      document.querySelector(
        '[aria-label="img-02 NOT_PROBED"]'
      ) as HTMLButtonElement
    ).click()
    await render()
    const blockedRun = [...host.querySelectorAll('button')].find((button) =>
      button.textContent?.includes('Run current case')
    ) as HTMLButtonElement
    assert.equal(blockedRun.disabled, true)
    blockedRun.click()
    blockedRun.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }))
    assert.deepEqual(sent, [img01])
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
      error_code: 'IMAGE_PROBE_CLIENT_REQUEST_FAILED',
    })
    assert.equal(state.caseId, 'img-17')
    assert.equal(state.response.state, 'UNVERIFIED')
    assert.equal(state.response.error_code, 'IMAGE_PROBE_CLIENT_REQUEST_FAILED')
    assert.equal(state.nextRequest, null)
    assert.equal(state.shouldAdvance, false)
  })
  test('cancelled or late image probes do not store results after generation or identity changes', () => {
    assert.equal(shouldStoreImageCapabilityProbeResult(true, true), true)
    assert.equal(shouldStoreImageCapabilityProbeResult(false, true), false)
    assert.equal(shouldStoreImageCapabilityProbeResult(true, false), false)
  })
  test('probe state machine records a current reject but drops stale settle and finally callbacks', async () => {
    const events: string[] = []
    let requestCalls = 0
    let current = true
    await runImageCapabilityProbeStateMachine(
      () => {
        requestCalls += 1
        return Promise.reject(new Error('sensitive mock body'))
      },
      () => current,
      () => events.push('success'),
      () => events.push('failure'),
      () => events.push('finally')
    )
    assert.equal(requestCalls, 1)
    assert.deepEqual(events, ['failure', 'finally'])

    let resolve!: () => void
    const pending = new Promise<void>((done) => {
      resolve = done
    })
    const staleEvents: string[] = []
    const run = runImageCapabilityProbeStateMachine(
      () => pending,
      () => current,
      () => staleEvents.push('success'),
      () => staleEvents.push('failure'),
      () => staleEvents.push('finally')
    )
    current = false
    resolve()
    await run
    assert.deepEqual(staleEvents, [])
  })
  test('aborting a stale probe neither stores its sensitive failure nor clears a newer in-flight case', async () => {
    const controller = new AbortController()
    let rejectRequest!: (reason: unknown) => void
    let requestCalls = 0
    let generationIsCurrent = true
    let channelIdentityIsCurrent = true
    let inFlightCaseId: string | null = 'img-01'
    const storedErrors: string[] = []
    const pending = runImageCapabilityProbeStateMachine(
      () => {
        requestCalls += 1
        return new Promise<never>((_resolve, reject) => {
          rejectRequest = reject
        })
      },
      () =>
        shouldStoreImageCapabilityProbeResult(
          generationIsCurrent,
          channelIdentityIsCurrent
        ),
      () => assert.fail('aborted request must not succeed'),
      () => storedErrors.push('IMAGE_PROBE_CLIENT_REQUEST_FAILED'),
      () => {
        inFlightCaseId = null
      }
    )

    controller.abort()
    assert.equal(controller.signal.aborted, true)
    generationIsCurrent = false
    inFlightCaseId = 'img-02'
    rejectRequest(new Error('sensitive request body /internal/stack'))
    await pending
    assert.equal(requestCalls, 1)
    assert.deepEqual(storedErrors, [])
    assert.equal(inFlightCaseId, 'img-02')

    generationIsCurrent = true
    channelIdentityIsCurrent = false
    const identityEvents: string[] = []
    await runImageCapabilityProbeStateMachine(
      () => Promise.reject(new Error('sensitive task id')),
      () =>
        shouldStoreImageCapabilityProbeResult(
          generationIsCurrent,
          channelIdentityIsCurrent
        ),
      () => identityEvents.push('success'),
      () => identityEvents.push('failure'),
      () => identityEvents.push('finally')
    )
    assert.deepEqual(identityEvents, [])
  })
  test('component lifetime cleanup aborts a pending probe and makes late settlement a no-op', async () => {
    const lifetime = createImageCapabilityProbeLifetime()
    const controller = new AbortController()
    const oldInFlightCases = new Set(['img-01'])
    const newInFlightCases = new Set(['img-02'])
    const events: string[] = []
    let resolveSuccess!: () => void
    const success = runImageCapabilityProbeStateMachine(
      () =>
        new Promise<void>((resolve) => {
          resolveSuccess = resolve
        }),
      () => lifetime.isCurrent,
      () => events.push('success'),
      () => events.push('failure'),
      () => events.push('finally')
    )

    invalidateImageCapabilityProbeLifetime(
      lifetime,
      controller,
      oldInFlightCases
    )
    assert.equal(controller.signal.aborted, true)
    assert.deepEqual([...oldInFlightCases], [])
    resolveSuccess()
    await success
    assert.deepEqual(events, [])
    assert.deepEqual([...newInFlightCases], ['img-02'])

    const failureLifetime = createImageCapabilityProbeLifetime()
    const failureController = new AbortController()
    const failureInFlightCases = new Set(['img-03'])
    const failureEvents: string[] = []
    let rejectFailure!: (reason: unknown) => void
    const failure = runImageCapabilityProbeStateMachine<void>(
      () =>
        new Promise<void>((_resolve, reject) => {
          rejectFailure = reject
        }),
      () => failureLifetime.isCurrent,
      () => assert.fail('unmounted probe must not succeed'),
      () => failureEvents.push('failure'),
      () => failureEvents.push('finally')
    )
    invalidateImageCapabilityProbeLifetime(
      failureLifetime,
      failureController,
      failureInFlightCases
    )
    rejectFailure(new Error('sensitive response body /internal/stack'))
    await failure
    assert.equal(failureController.signal.aborted, true)
    assert.deepEqual([...failureInFlightCases], [])
    assert.deepEqual(failureEvents, [])
  })
  test('StrictMode replay reactivates the production lifetime and unmount drops late probe settlement', async () => {
    const window = new Window()
    Object.assign(globalThis, {
      window,
      document: window.document,
      HTMLElement: window.HTMLElement,
      Event: window.Event,
      KeyboardEvent: window.KeyboardEvent,
      IS_REACT_ACT_ENVIRONMENT: true,
    })
    Object.defineProperty(globalThis, 'navigator', {
      configurable: true,
      value: window.navigator,
    })
    const host = document.createElement('div')
    document.body.append(host)
    const root = createRoot(host)
    let setupCount = 0
    const harness = {
      probeLifetime: null as ReturnType<
        typeof createImageCapabilityProbeLifetime
      > | null,
      abortRef: null as { current: AbortController | null } | null,
      oldInFlightCases: null as Set<string> | null,
    }
    await act(async () => {
      root.render(
        createElement(
          StrictMode,
          null,
          createElement(StrictModeProbeLifetimeHarness, {
            onSetup: () => {
              setupCount += 1
            },
            onReady: (value) => {
              harness.probeLifetime = value.lifetime
              harness.abortRef = value.abortRef
              harness.oldInFlightCases = value.inFlightCases
            },
          })
        )
      )
    })
    assert.equal(setupCount, 2)
    if (
      !harness.probeLifetime ||
      !harness.abortRef ||
      !harness.oldInFlightCases
    ) {
      assert.fail('StrictMode harness did not expose its production lifetime')
    }
    const currentLifetime = harness.probeLifetime
    const currentAbortRef = harness.abortRef
    const currentOldInFlightCases = harness.oldInFlightCases
    assert.equal(currentLifetime.isCurrent, true)

    let currentRequestCalls = 0
    const currentErrors: string[] = []
    await runImageCapabilityProbeStateMachine(
      () => {
        currentRequestCalls += 1
        return Promise.reject(new Error('sensitive current request'))
      },
      () => currentLifetime.isCurrent,
      () => assert.fail('current rejected probe must not succeed'),
      () => currentErrors.push('IMAGE_PROBE_CLIENT_REQUEST_FAILED'),
      () => assert.equal(currentLifetime.isCurrent, true)
    )
    assert.equal(currentRequestCalls, 1)
    assert.deepEqual(currentErrors, ['IMAGE_PROBE_CLIENT_REQUEST_FAILED'])

    const controller = new AbortController()
    currentAbortRef.current = controller
    currentOldInFlightCases.add('img-01')
    const newInFlightCases = new Set(['img-02'])
    let newInFlightCaseId: string | null = 'img-02'
    const lateEvents: string[] = []
    let resolveLateSuccess!: () => void
    let rejectLateFailure!: (reason: unknown) => void
    const lateSuccess = runImageCapabilityProbeStateMachine(
      () =>
        new Promise<void>((resolve) => {
          resolveLateSuccess = resolve
        }),
      () => currentLifetime.isCurrent,
      () => lateEvents.push('success'),
      () => lateEvents.push('failure'),
      () => {
        lateEvents.push('finally')
        newInFlightCaseId = null
      }
    )
    const lateFailure = runImageCapabilityProbeStateMachine<void>(
      () =>
        new Promise<void>((_resolve, reject) => {
          rejectLateFailure = reject
        }),
      () => currentLifetime.isCurrent,
      () => lateEvents.push('success'),
      () => lateEvents.push('failure'),
      () => {
        lateEvents.push('finally')
        newInFlightCaseId = null
      }
    )
    await act(async () => root.unmount())
    assert.equal(controller.signal.aborted, true)
    assert.equal(currentLifetime.isCurrent, false)
    assert.deepEqual([...currentOldInFlightCases], [])
    resolveLateSuccess()
    rejectLateFailure(new Error('sensitive body /internal/stack'))
    await Promise.all([lateSuccess, lateFailure])
    assert.deepEqual(lateEvents, [])
    assert.equal(newInFlightCaseId, 'img-02')
    assert.deepEqual([...newInFlightCases], ['img-02'])
    host.remove()
  })
  test('a current component lifetime records one rejected request as client failure', async () => {
    const lifetime = createImageCapabilityProbeLifetime()
    let calls = 0
    const errors: string[] = []
    await runImageCapabilityProbeStateMachine(
      () => {
        calls += 1
        return Promise.reject(new Error('sensitive failed request'))
      },
      () => lifetime.isCurrent,
      () => assert.fail('rejected request must not succeed'),
      () => errors.push('IMAGE_PROBE_CLIENT_REQUEST_FAILED'),
      () => assert.equal(lifetime.isCurrent, true)
    )
    assert.equal(calls, 1)
    assert.deepEqual(errors, ['IMAGE_PROBE_CLIENT_REQUEST_FAILED'])
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
