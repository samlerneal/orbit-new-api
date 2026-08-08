import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  createChannelTestIdentityGuard,
  createChannelTestDispatch,
  isCurrentChannelTestGeneration,
  resetChannelTestModeState,
} from '../../lib/channel-actions'

describe('Image Edit channel test dispatch', () => {
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
