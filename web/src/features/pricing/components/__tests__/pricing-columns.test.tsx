/* Copyright (C) 2023-2026 QuantumNous */
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'

describe('pricing comparison columns', () => {
  test('keeps the six-column public comparison contract in the table', () => {
    const source = readFileSync(
      new URL('../pricing-columns.tsx', import.meta.url),
      'utf8'
    )
    for (const key of [
      'Model',
      'Input price',
      'Output price',
      'Cache create',
      'Cache read',
      'Savings',
    ]) {
      assert.match(source, new RegExp(`'${key}'`))
    }
  })
})
