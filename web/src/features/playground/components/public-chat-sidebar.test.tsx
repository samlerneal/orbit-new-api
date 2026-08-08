/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { canChangePublicChatSessions } from './public-chat-sidebar'

describe('public chat sidebar session guard', () => {
  test('locks conversation mutations while generation is active', () => {
    assert.equal(canChangePublicChatSessions(true), false)
    assert.equal(canChangePublicChatSessions(false), true)
  })
})
