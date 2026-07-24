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
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { describe, test } from 'node:test'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)

function readWalletSource(): string {
  return readFileSync(join(__dirname, 'index.tsx'), 'utf-8')
}

function extractMonthlyTriggerClassNames(source: string): string {
  // Match the TabsTrigger with value='monthly-subscription' and capture its className
  const pattern =
    /value=['"]monthly-subscription['"][\s\S]*?className=['"]([^'"]*)['"]/
  const match = source.match(pattern)
  assert.ok(
    match,
    'Monthly subscription TabsTrigger with className not found in source'
  )
  return match[1]
}

function extractMonthlyTriggerBlock(source: string): string {
  const pattern = /value=['"]monthly-subscription['"][\s\S]*?<\/TabsTrigger>/
  const match = source.match(pattern)
  assert.ok(match, 'Monthly subscription TabsTrigger block not found in source')
  return match[0]
}

describe('wallet monthly tab accessibility invariants', () => {
  test('monthly subscription tab retains disabled attribute', () => {
    const source = readWalletSource()
    const block = extractMonthlyTriggerBlock(source)
    assert.ok(
      block.includes('disabled'),
      'Monthly tab must have disabled attribute'
    )
  })

  test('monthly subscription tab retains internal opacity-55 weak container', () => {
    const source = readWalletSource()
    const block = extractMonthlyTriggerBlock(source)
    assert.ok(
      block.includes('opacity-55'),
      'Monthly tab must wrap content in an opacity-55 weak container'
    )
  })
})

describe('monthly tab visual independence (fix verification)', () => {
  test('monthly tab className has a local background class beyond data-active:bg-background', () => {
    const classNames = extractMonthlyTriggerClassNames(readWalletSource())
    const classes = classNames.split(/\s+/).filter(Boolean)

    // A local background class is a bg-* class that is NOT data-active:bg-background
    const bgClasses = classes.filter(
      (c) => c.startsWith('bg-') && c !== 'data-active:bg-background'
    )

    assert.ok(
      bgClasses.length >= 1,
      `Monthly tab must have at least one local background class, found: [${bgClasses.join(', ')}]`
    )
  })

  test('monthly tab className has a visible border color class', () => {
    const classNames = extractMonthlyTriggerClassNames(readWalletSource())
    const classes = classNames.split(/\s+/).filter(Boolean)

    // A visible border color is a border-{color}/* class (not border-transparent, not bare 'border')
    const borderColorClasses = classes.filter(
      (c) =>
        c.startsWith('border-') && c !== 'border' && c !== 'border-transparent'
    )

    assert.ok(
      borderColorClasses.length >= 1,
      `Monthly tab must have at least one visible border color class, found: [${borderColorClasses.join(', ')}]`
    )
  })

  test('monthly tab inner container has narrow-screen overflow protection classes', () => {
    const source = readWalletSource()
    const block = extractMonthlyTriggerBlock(source)
    assert.ok(
      block.includes('min-w-0'),
      'Monthly tab inner container must have min-w-0 to allow flex shrink on narrow screens'
    )
    assert.ok(
      block.includes('whitespace-normal'),
      'Monthly tab inner container must have whitespace-normal to override inherited whitespace-nowrap on narrow screens'
    )
    assert.ok(
      block.includes('flex-wrap'),
      'Monthly tab inner container must allow child items to wrap on narrow screens'
    )
  })

  test('monthly tab className has a shadow class', () => {
    const classNames = extractMonthlyTriggerClassNames(readWalletSource())
    const classes = classNames.split(/\s+/).filter(Boolean)

    const shadowClasses = classes.filter((c) => c.startsWith('shadow-'))

    assert.ok(
      shadowClasses.length >= 1,
      `Monthly tab must have at least one shadow class, found: [${shadowClasses.join(', ')}]`
    )
  })
})
