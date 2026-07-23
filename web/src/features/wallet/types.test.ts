/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  canSubmitPackagePayment,
  createPackagePaymentRequest,
  REFUND_NOTICE_VERSION,
} from './types.ts'

describe('package payment refund notice gate', () => {
  test('keeps payment disabled until the notice is accepted', () => {
    assert.equal(canSubmitPackagePayment(false, false), false)
    assert.equal(canSubmitPackagePayment(true, false), true)
    assert.equal(canSubmitPackagePayment(true, true), false)
  })

  test('adds the accepted refund notice fields to the package request', () => {
    const request = createPackagePaymentRequest('package-1', {
      refund_notice_accepted: true,
      refund_notice_version: REFUND_NOTICE_VERSION,
      refund_notice_language: 'zhCN',
    })

    assert.deepEqual(request, {
      package_id: 'package-1',
      payment_method: 'wxpay',
      refund_notice_accepted: true,
      refund_notice_version: 'refund-notice-v1',
      refund_notice_language: 'zhCN',
    })
  })
})
