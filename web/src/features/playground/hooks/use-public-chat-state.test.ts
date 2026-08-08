/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  canMutatePublicChatSession,
  getPublicChatIdentityState,
  getPublicChatPersistDecision,
  shouldReloadPublicChatStorage,
} from './use-public-chat-state'

describe('public chat identity and persistence decisions', () => {
  test('fails closed until auth and the loaded owner agree', () => {
    assert.deepEqual(getPublicChatIdentityState(1, 1, false), {
      identityReady: false,
    })
    assert.deepEqual(getPublicChatIdentityState(1, 2, true), {
      identityReady: false,
    })
    assert.deepEqual(getPublicChatIdentityState(undefined, undefined, true), {
      identityReady: false,
    })
    assert.deepEqual(getPublicChatIdentityState(1, 1, true), {
      identityReady: true,
    })
  })

  test('reloads only when the requested identity changes', () => {
    assert.equal(shouldReloadPublicChatStorage(1, 1), false)
    assert.equal(shouldReloadPublicChatStorage(1, 2), true)
    assert.equal(shouldReloadPublicChatStorage(1, undefined), true)
  })

  test('keeps memory after persistence failure and replaces only trimmed state', () => {
    assert.deepEqual(getPublicChatPersistDecision(false, false), {
      notice: 'unavailable',
      replaceState: false,
    })
    assert.deepEqual(getPublicChatPersistDecision(true, true), {
      notice: 'trimmed',
      replaceState: true,
    })
    assert.deepEqual(getPublicChatPersistDecision(true, false), {
      notice: null,
      replaceState: false,
    })
  })

  test('locks session mutation while generating', () => {
    assert.equal(canMutatePublicChatSession(true), false)
    assert.equal(canMutatePublicChatSession(false), true)
  })
})
