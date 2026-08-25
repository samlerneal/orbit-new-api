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
  getTopupPackageVisualStyleClasses,
  isEpayPaymentMethod,
  isTopupPackageVisualStyle,
  REFUND_NOTICE_VERSION,
} from './types.ts'

describe('package payment refund notice gate', () => {
  test('keeps payment disabled until the notice is accepted', () => {
    assert.equal(canSubmitPackagePayment(false, false), false)
    assert.equal(canSubmitPackagePayment(true, false), true)
    assert.equal(canSubmitPackagePayment(true, true), false)
  })

  test('adds the accepted refund notice fields to the package request', () => {
    const request = createPackagePaymentRequest('package-1', 'alipay', {
      refund_notice_accepted: true,
      refund_notice_version: REFUND_NOTICE_VERSION,
      refund_notice_language: 'zhCN',
    })

    assert.deepEqual(request, {
      package_id: 'package-1',
      payment_method: 'alipay',
      refund_notice_accepted: true,
      refund_notice_version: 'refund-notice-v1',
      refund_notice_language: 'zhCN',
    })
  })

  test('allows only the two adapter-backed payment methods', () => {
    assert.equal(isEpayPaymentMethod('wxpay'), true)
    assert.equal(isEpayPaymentMethod('alipay'), true)
    assert.equal(isEpayPaymentMethod('custom1'), false)
    assert.throws(() =>
      createPackagePaymentRequest('package-1', 'custom1', {
        refund_notice_accepted: true,
        refund_notice_version: REFUND_NOTICE_VERSION,
        refund_notice_language: 'zhCN',
      })
    )
  })
})

describe('top-up package visual styles', () => {
  test('accepts only the four controlled presets', () => {
    assert.equal(isTopupPackageVisualStyle('default'), true)
    assert.equal(isTopupPackageVisualStyle('recommended'), true)
    assert.equal(isTopupPackageVisualStyle('popular'), true)
    assert.equal(isTopupPackageVisualStyle('value'), true)
    assert.equal(isTopupPackageVisualStyle('bg-red-500'), false)
  })

  test('maps every preset to a fixed style and falls back safely', () => {
    const recommended = getTopupPackageVisualStyleClasses('recommended')
    const popular = getTopupPackageVisualStyleClasses('popular')
    const value = getTopupPackageVisualStyleClasses('value')

    assert.notEqual(recommended, '')
    assert.notEqual(popular, '')
    assert.notEqual(value, '')
    assert.notEqual(recommended, popular)
    assert.notEqual(popular, value)
    assert.equal(getTopupPackageVisualStyleClasses('bg-red-500'), '')
  })
})
