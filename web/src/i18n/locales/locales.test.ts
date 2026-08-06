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
import { resolve } from 'node:path'
import { describe, test } from 'node:test'

const LOCALE_DIR = import.meta.dirname

function loadLocale(filename: string): Record<string, unknown> {
  const raw = readFileSync(resolve(LOCALE_DIR, filename), 'utf-8')
  return JSON.parse(raw)
}

const O006_REQUIRED_KEYS = [
  'Limited four-package bonus: 30%',
  'Each account can receive the campaign bonus once per eligible package.',
  'Limited bonus +30%',
  'Cumulative bonus up to {{amount}}',
]

const O005_MIGRATED_KEYS = [
  'Limited {{total}} accounts, {{remaining}} spots remaining',
  'Bonus balance valid {{days}} days',
  'Selling points',
  'Footer note',
  'Visual style',
  'Once per package per account',
  'Sort order',
  'Maximum participants (0 = unlimited)',
  'Campaign participant stats',
  'Admitted accounts',
  'Reserved accounts',
  'Remaining participant slots',
  'Awarded claims',
  'Reserved claims',
  'Campaign bonus and expiry is managed in campaign settings',
  'Add selling point',
  'd',
]

const O018_CAMPAIGN_SOURCE_FILES = [
  '../../features/system-settings/billing/campaign-management-section.tsx',
  '../../features/system-settings/integrations/topup-rules-editor.tsx',
]
const O018_LOCALE_SAME_TEXT_KEYS = new Set(['QQ'])
const O021_PUBLIC_KEYS = [
  'Available now',
  'Contact support',
  'Mydaily API operator',
  'Modified source',
  'Open source license',
  'Public CTA description',
  'Public CTA title',
  'Public catalog ready',
  'Public catalog description',
  'Public catalog title',
  'Public footer navigation',
  'Public hero description',
  'Public hero title',
  'Public navigation',
  'Public open console',
  'Public step one',
  'Public step three',
  'Public step two',
  'Public steps description',
  'Public steps title',
  'Public tutorial',
  'Public view tutorial',
  'Under preparation',
]

function loadO018CampaignKeys(): string[] {
  const keys = new Set<string>()
  for (const relativePath of O018_CAMPAIGN_SOURCE_FILES) {
    const source = readFileSync(resolve(LOCALE_DIR, relativePath), 'utf-8')
    for (const match of source.matchAll(/\bt\(\s*['`]([^'`]+)['`]/g)) {
      keys.add(match[1])
    }
  }
  return [...keys]
}

describe('locale file structure', () => {
  test('zh.json has only translation as root key', () => {
    const zh = loadLocale('zh.json')
    const rootKeys = Object.keys(zh)
    assert.deepEqual(rootKeys, ['translation'])
  })

  test('en.json has only translation as root key', () => {
    const en = loadLocale('en.json')
    const rootKeys = Object.keys(en)
    assert.deepEqual(rootKeys, ['translation'])
  })
})

describe('O-006 campaign keys in translation', () => {
  for (const filename of ['zh.json', 'en.json']) {
    test(`${filename} contains all O-006 required keys in translation`, () => {
      const locale = loadLocale(filename)
      const translation = locale.translation as Record<string, string>
      for (const key of O006_REQUIRED_KEYS) {
        assert.ok(
          key in translation,
          `${filename}: missing required key "${key}" in translation`
        )
      }
    })
  }
})

describe('O-005 migrated keys are inside translation', () => {
  for (const filename of ['zh.json', 'en.json']) {
    test(`${filename} has all O-005 migrated keys in translation`, () => {
      const locale = loadLocale(filename)
      const translation = locale.translation as Record<string, string>
      for (const key of O005_MIGRATED_KEYS) {
        assert.ok(
          key in translation,
          `${filename}: O-005 key "${key}" should be in translation`
        )
      }
    })
  }
})

describe('O-018 campaign management locale completeness', () => {
  const campaignKeys = loadO018CampaignKeys()

  for (const filename of ['zh.json', 'en.json']) {
    test(`${filename} contains every literal campaign-management t key`, () => {
      const locale = loadLocale(filename)
      const translation = locale.translation as Record<string, string>
      for (const key of campaignKeys) {
        assert.equal(
          typeof translation[key],
          'string',
          `${filename}: missing campaign-management key "${key}"`
        )
      }
    })
  }

  test('zh.json campaign-management values do not fall back to source English', () => {
    const zh = loadLocale('zh.json').translation as Record<string, string>
    const en = loadLocale('en.json').translation as Record<string, string>
    for (const key of campaignKeys) {
      if (O018_LOCALE_SAME_TEXT_KEYS.has(key)) continue
      assert.notEqual(
        zh[key],
        en[key],
        `zh.json: campaign-management key "${key}" still uses English text`
      )
    }
  })
})

describe('no business keys outside translation', () => {
  test('zh.json root has zero business keys outside translation', () => {
    const zh = loadLocale('zh.json')
    const rootKeys = Object.keys(zh).filter((k) => k !== 'translation')
    assert.equal(rootKeys.length, 0)
  })

  test('en.json root has zero business keys outside translation', () => {
    const en = loadLocale('en.json')
    const rootKeys = Object.keys(en).filter((k) => k !== 'translation')
    assert.equal(rootKeys.length, 0)
  })
})

describe('O-021 public page locale completeness', () => {
  for (const filename of ['zh.json', 'zh-TW.json', 'en.json', 'ru.json']) {
    test(`${filename} contains all public-page keys`, () => {
      const translation = loadLocale(filename).translation as Record<
        string,
        string
      >
      for (const key of O021_PUBLIC_KEYS) {
        assert.equal(
          typeof translation[key],
          'string',
          `${filename}: missing ${key}`
        )
        assert.notEqual(translation[key], '', `${filename}: empty ${key}`)
      }
    })
  }
})
