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
*/
import { readFile } from 'node:fs/promises'
import { resolve } from 'node:path'

const ROOT = resolve(import.meta.dirname, '..')
const MAX_RESPONSE_BYTES = 512 * 1024
const REQUEST_TIMEOUT_MS = 5_000
const LOCALE_FILES = ['zh.json', 'zh-TW.json', 'en.json', 'ru.json']
const PUBLIC_SOURCE_FILES = [
  'src/components/layout/components/public-header.tsx',
  'src/components/layout/components/footer.tsx',
  'src/features/home/index.tsx',
  'src/features/home/constants.ts',
  'src/features/home/components/sections/hero.tsx',
  'src/features/home/components/sections/features.tsx',
  'src/features/home/components/sections/how-it-works.tsx',
  'src/features/home/components/sections/cta.tsx',
  'src/lib/public-link.ts',
]
const REQUIRED_PUBLIC_KEYS = [
  'Contact support',
  'Mydaily API operator',
  'Modified source',
  'Open source license',
  'Public CTA description',
  'Public CTA title',
  'Public catalog description',
  'Public catalog planned',
  'Public catalog title',
  'Public contact',
  'Public footer navigation',
  'Public hero description',
  'Public hero title',
  'Public model pricing',
  'Public navigation',
  'Public open console',
  'Public privacy policy',
  'Public step one',
  'Public step one description',
  'Public step three',
  'Public step three description',
  'Public step two',
  'Public step two description',
  'Public steps description',
  'Public steps title',
  'Public terms of service',
  'Public tutorial',
  'Public usage tutorial',
  'Public view tutorial',
  'Under preparation',
  'Available now',
]
const LOCKED_PUBLIC_LOCALE_VALUES: Record<string, Record<string, string>> = {
  'zh.json': {
    'Public catalog planned': '即将接入',
    'Public privacy policy': '隐私政策',
    'Public terms of service': '服务条款',
    'Public model pricing': '模型价格',
    'Public usage tutorial': '使用教程',
    'Public contact': '联系我们',
  },
  'zh-TW.json': {
    'Public catalog planned': '即將接入',
    'Public privacy policy': '隱私政策',
    'Public terms of service': '服務條款',
    'Public model pricing': '模型價格',
    'Public usage tutorial': '使用教學',
    'Public contact': '聯絡我們',
  },
  'en.json': {
    'Public catalog planned': 'Coming next',
    'Public privacy policy': 'Privacy Policy',
    'Public terms of service': 'Terms of Service',
    'Public model pricing': 'Model Pricing',
    'Public usage tutorial': 'Usage Tutorial',
    'Public contact': 'Contact Us',
  },
  'ru.json': {
    'Public catalog planned': 'Скоро подключим',
    'Public privacy policy': 'Политика конфиденциальности',
    'Public terms of service': 'Условия использования',
    'Public model pricing': 'Цены на модели',
    'Public usage tutorial': 'Руководство по использованию',
    'Public contact': 'Связаться с нами',
  },
}
const REQUIRED_CATALOG_LOGOS = ['OpenAI', 'Anthropic', 'Gemini', 'XAI']
const LEGACY_NARRATIVE =
  /upstream services integrated|model billing support|enterprise-grade security|multi-region deployment|automatic load balancing/i

type Finding = { id: string; path: string; summary: string }

function printFinding(finding: Finding): void {
  process.stderr.write(`${finding.id} ${finding.path} ${finding.summary}\n`)
}

function isAllowedBaseUrl(value: string): boolean {
  try {
    const parsed = new URL(value)
    if (
      parsed.username ||
      parsed.password ||
      parsed.search ||
      parsed.hash ||
      parsed.pathname !== '/'
    ) {
      return false
    }
    const isLocalhost = parsed.hostname === 'localhost'
    const isCanonical =
      parsed.protocol === 'https:' &&
      parsed.hostname === 'api.mydaily.info' &&
      parsed.port === ''
    return (
      (isLocalhost &&
        (parsed.protocol === 'http:' || parsed.protocol === 'https:')) ||
      isCanonical
    )
  } catch {
    return false
  }
}

async function readUtf8(relativePath: string): Promise<string> {
  return readFile(resolve(ROOT, relativePath), 'utf8')
}

function parseConfiguredString(
  source: string,
  name: string
): string | null | undefined {
  const match = source.match(
    new RegExp(`${name}(?::[^=]+)?\\s*=\\s*(null|'([^']*)')`)
  )
  if (!match) return undefined
  return match[1] === 'null' ? null : match[2]
}

function parseConfiguredItems(
  source: string,
  name: string
): string[] | undefined {
  const match = source.match(new RegExp(`${name}:\\s*\\[([^\\]]*)\\]`))
  if (!match) return undefined
  return [...match[1].matchAll(/'([^']+)'/g)].map((item) => item[1])
}

function parseCatalogSource(
  source: string,
  name: string
): string | null | undefined {
  const match = source.match(new RegExp(`${name}:\\s*(null|'([^']*)')`))
  if (!match) return undefined
  return match[1] === 'null' ? null : match[2]
}

function isSafeConfiguredPublicLink(value: string): boolean {
  if (value.startsWith('/')) {
    if (value.startsWith('//') || value.includes('\\') || /%5c/i.test(value)) {
      return false
    }
    try {
      const parsed = new URL(value, 'https://public.invalid')
      return (
        parsed.origin === 'https://public.invalid' &&
        !parsed.search &&
        !parsed.hash &&
        !parsed.pathname.startsWith('//')
      )
    } catch {
      return false
    }
  }
  try {
    const parsed = new URL(value)
    return (
      (parsed.protocol === 'https:' || parsed.protocol === 'mailto:') &&
      !parsed.username &&
      !parsed.password &&
      ![...parsed.searchParams.keys()].some((key) =>
        /(?:token|key|secret|password|auth)/i.test(key)
      )
    )
  } catch {
    return false
  }
}

async function checkStaticContent(release: boolean): Promise<Finding[]> {
  const findings: Finding[] = []
  for (const localeFile of LOCALE_FILES) {
    const path = `src/i18n/locales/${localeFile}`
    try {
      const locale = JSON.parse(await readUtf8(path)) as {
        translation?: Record<string, unknown>
      }
      for (const key of REQUIRED_PUBLIC_KEYS) {
        if (
          typeof locale.translation?.[key] !== 'string' ||
          !locale.translation[key]
        ) {
          findings.push({ id: 'LOCALE_NONEMPTY', path, summary: key })
        }
      }
      for (const [key, expectedValue] of Object.entries(
        LOCKED_PUBLIC_LOCALE_VALUES[localeFile] ?? {}
      )) {
        if (locale.translation?.[key] !== expectedValue) {
          findings.push({ id: 'LOCALE_CONTRACT', path, summary: key })
        }
      }
    } catch {
      findings.push({
        id: 'LOCALE_PARSE',
        path,
        summary: 'invalid locale JSON',
      })
    }
  }

  const source: { path: string; content: string }[] = []
  for (const path of PUBLIC_SOURCE_FILES) {
    try {
      source.push({ path, content: await readUtf8(path) })
    } catch {
      findings.push({
        id: 'SOURCE_MISSING',
        path,
        summary: 'required public source unavailable',
      })
    }
  }
  const publicLinkSource = source.find(
    ({ path }) => path === 'src/lib/public-link.ts'
  )?.content
  const catalogSource = source.find(
    ({ path }) => path === 'src/features/home/constants.ts'
  )?.content
  const contactTarget = publicLinkSource
    ? parseConfiguredString(publicLinkSource, 'PUBLIC_CONTACT_TARGET')
    : undefined
  const contactFallback = publicLinkSource
    ? parseConfiguredString(publicLinkSource, 'PUBLIC_CONTACT_FALLBACK_LABEL')
    : undefined
  const availableSource = catalogSource
    ? parseCatalogSource(catalogSource, 'availableSource')
    : undefined
  const plannedSource = catalogSource
    ? parseCatalogSource(catalogSource, 'plannedSource')
    : undefined
  const availableModels = catalogSource
    ? parseConfiguredItems(catalogSource, 'availableModels')
    : undefined
  const plannedModels = catalogSource
    ? parseConfiguredItems(catalogSource, 'plannedModels')
    : undefined
  if (contactTarget === undefined) {
    findings.push({
      id: 'CONTACT_SSOT',
      path: 'src/lib/public-link.ts',
      summary: 'missing contact source',
    })
  }
  if (
    availableSource === undefined ||
    plannedSource === undefined ||
    availableModels === undefined ||
    plannedModels === undefined
  ) {
    findings.push({
      id: 'CATALOG_SOURCE',
      path: 'src/features/home/constants.ts',
      summary: 'missing catalog sources',
    })
  }
  const headerSource = source.find(
    ({ path }) => path === 'src/components/layout/components/public-header.tsx'
  )?.content
  if (
    !headerSource?.includes("'日课 API'") ||
    headerSource.includes('getPublicHostname') ||
    !headerSource.includes("copyToClipboard('3184917639')")
  ) {
    findings.push({
      id: 'HEADER_BRAND_OR_SUPPORT',
      path: 'src/components/layout/components/public-header.tsx',
      summary: 'brand or support popover contract is invalid',
    })
  }
  const featureSource = source.find(
    ({ path }) => path === 'src/features/home/components/sections/features.tsx'
  )?.content
  if (
    !featureSource ||
    REQUIRED_CATALOG_LOGOS.some((logo) => !featureSource.includes(`'${logo}'`))
  ) {
    findings.push({
      id: 'CATALOG_LOGOS',
      path: 'src/features/home/components/sections/features.tsx',
      summary: 'required provider logos are absent',
    })
  }
  if (
    !featureSource?.includes('justify-center') ||
    !featureSource.includes('cursor-default') ||
    !featureSource.includes('aria-label={model}') ||
    featureSource.includes('<button') ||
    featureSource.includes('<a ')
  ) {
    findings.push({
      id: 'CATALOG_CHIP_INTERACTION',
      path: 'src/features/home/components/sections/features.tsx',
      summary: 'catalog logos must remain centered non-interactive chips',
    })
  }
  const heroSource = source.find(
    ({ path }) => path === 'src/features/home/components/sections/hero.tsx'
  )?.content
  if (
    !heroSource?.includes('status?.docs_link') ||
    !heroSource.includes('resolveSafePublicLink') ||
    heroSource.includes("href='/docs'")
  ) {
    findings.push({
      id: 'HERO_TUTORIAL_TARGET',
      path: 'src/features/home/components/sections/hero.tsx',
      summary: 'tutorial target must use the safe status SSOT',
    })
  }
  const stepsSource = source.find(
    ({ path }) =>
      path === 'src/features/home/components/sections/how-it-works.tsx'
  )?.content
  if (
    !stepsSource?.includes('status?.docs_link') ||
    !stepsSource.includes('resolveSafePublicLink') ||
    stepsSource.includes("href='/docs'")
  ) {
    findings.push({
      id: 'STEPS_TUTORIAL_TARGET',
      path: 'src/features/home/components/sections/how-it-works.tsx',
      summary: 'steps tutorial must use the safe status SSOT',
    })
  }
  if (
    !featureSource?.includes("t('Public catalog planned')") ||
    featureSource.includes("t('Coming soon')")
  ) {
    findings.push({
      id: 'CATALOG_PLANNED_LABEL',
      path: 'src/features/home/components/sections/features.tsx',
      summary: 'planned catalog must use the public semantic key',
    })
  }
  if (release) {
    for (const { path, content } of source) {
      if (content.includes('CONTENT_PENDING')) {
        findings.push({
          id: 'CONTENT_PENDING',
          path,
          summary: 'release content is pending',
        })
      }
      if (LEGACY_NARRATIVE.test(content)) {
        findings.push({
          id: 'LEGACY_NARRATIVE',
          path,
          summary: 'legacy public narrative',
        })
      }
    }
    if (!contactTarget) {
      const footerSource = source.find(
        ({ path }) => path === 'src/components/layout/components/footer.tsx'
      )?.content
      if (
        contactFallback !== '客服 QQ：3184917639' ||
        !headerSource?.includes('PUBLIC_CONTACT_FALLBACK_LABEL') ||
        !headerSource?.includes("copyToClipboard('3184917639')") ||
        !footerSource?.includes('PUBLIC_CONTACT_FALLBACK_LABEL') ||
        !footerSource?.includes("aria-disabled='true'")
      ) {
        findings.push({
          id: 'CONTACT_FALLBACK',
          path: 'src/lib/public-link.ts',
          summary: 'QQ fallback must be exact and non-interactive',
        })
      }
    } else if (!isSafeConfiguredPublicLink(contactTarget)) {
      findings.push({
        id: 'CONTACT_UNSAFE',
        path: 'src/lib/public-link.ts',
        summary: 'contact target is unsafe',
      })
    }
    if (
      !availableSource ||
      !plannedSource ||
      !availableModels?.length ||
      !plannedModels?.length
    ) {
      findings.push({
        id: 'CATALOG_PENDING',
        path: 'src/features/home/constants.ts',
        summary: 'catalog source is absent',
      })
    }
    if (
      availableModels &&
      plannedModels &&
      new Set([...availableModels, ...plannedModels]).size !==
        availableModels.length + plannedModels.length
    ) {
      findings.push({
        id: 'CATALOG_INCONSISTENT',
        path: 'src/features/home/constants.ts',
        summary: 'catalog entries overlap',
      })
    }
    const footerSource = source.find(
      ({ path }) => path === 'src/components/layout/components/footer.tsx'
    )?.content
    const requiredFooterKeys = [
      'Public privacy policy',
      'Public terms of service',
      'Public model pricing',
      'Public usage tutorial',
      'Public contact',
    ]
    if (
      !footerSource ||
      requiredFooterKeys.some((key) => !footerSource.includes(`'${key}'`)) ||
      footerSource.includes('PUBLIC_FILING_TEXT')
    ) {
      findings.push({
        id: 'FOOTER_PLACEHOLDER',
        path: 'src/components/layout/components/footer.tsx',
        summary: 'legal placeholder or filing contract is invalid',
      })
    }
  }
  return findings
}

async function checkLegalPages(baseUrl: string): Promise<Finding[]> {
  const findings: Finding[] = []
  const origin = new URL(baseUrl).origin
  for (const legalPath of ['/privacy-policy', '/user-agreement'] as const) {
    const path = `legal${legalPath}`
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS)
    try {
      const response = await fetch(new URL(legalPath, origin), {
        method: 'GET',
        redirect: 'manual',
        signal: controller.signal,
      })
      const contentType = response.headers.get('content-type') || ''
      if (response.status !== 200) {
        findings.push({
          id: 'LEGAL_STATUS',
          path,
          summary: `status ${response.status}`,
        })
        continue
      }
      if (!/^text\/(html|plain)|application\/xhtml\+xml/i.test(contentType)) {
        findings.push({
          id: 'LEGAL_CONTENT_TYPE',
          path,
          summary: 'unsupported content type',
        })
        continue
      }
      if (response.url !== new URL(legalPath, origin).toString()) {
        findings.push({
          id: 'LEGAL_REDIRECT',
          path,
          summary: 'redirect rejected',
        })
        continue
      }
      const reader = response.body?.getReader()
      if (!reader) {
        findings.push({
          id: 'LEGAL_BODY',
          path,
          summary: 'empty response body',
        })
        continue
      }
      let size = 0
      let nonWhitespace = false
      while (true) {
        const next = await reader.read()
        if (next.done) break
        size += next.value.byteLength
        if (size > MAX_RESPONSE_BYTES) {
          findings.push({
            id: 'LEGAL_SIZE',
            path,
            summary: 'response exceeds limit',
          })
          await reader.cancel()
          break
        }
        nonWhitespace ||= /\S/.test(new TextDecoder().decode(next.value))
      }
      if (size <= MAX_RESPONSE_BYTES && !nonWhitespace) {
        findings.push({
          id: 'LEGAL_BODY',
          path,
          summary: 'empty response body',
        })
      }
    } catch {
      findings.push({
        id: 'LEGAL_REQUEST',
        path,
        summary: 'request failed or timed out',
      })
    } finally {
      clearTimeout(timer)
    }
  }
  return findings
}

function runSelfTest(): Finding[] {
  const findings: Finding[] = []
  if (
    !isAllowedBaseUrl('http://localhost:3000') ||
    !isAllowedBaseUrl('https://api.mydaily.info')
  ) {
    findings.push({
      id: 'SELFTEST_BASE',
      path: 'self-test',
      summary: 'allowed origins rejected',
    })
  }
  for (const invalid of [
    'https://example.test',
    'https://user:pass@api.mydaily.info',
    'https://api.mydaily.info/?token=secret',
    'https://api.mydaily.info/other',
  ]) {
    if (isAllowedBaseUrl(invalid)) {
      findings.push({
        id: 'SELFTEST_BASE',
        path: 'self-test',
        summary: 'unsafe origin accepted',
      })
    }
  }
  for (const unsafeLink of [
    '/%2e%2e/%2e%2e//evil.test',
    'https://example.test/?%74%6f%6b%65%6e=secret',
    'mailto:help@example.test?%61%75%74%68=secret',
  ]) {
    if (isSafeConfiguredPublicLink(unsafeLink)) {
      findings.push({
        id: 'SELFTEST_LINK',
        path: 'self-test',
        summary: 'unsafe public link accepted',
      })
    }
  }
  return findings
}

async function main(): Promise<void> {
  const args = process.argv.slice(2)
  const release = args.includes('--release')
  const baseIndex = args.indexOf('--base-url')
  const baseUrl = baseIndex === -1 ? undefined : args[baseIndex + 1]
  const expectedArgCount =
    baseIndex === -1 ? Number(release) : Number(release) + 2
  if (
    args.length !== expectedArgCount ||
    args.filter((arg) => arg === '--base-url').length > 1
  ) {
    process.exitCode = 2
    printFinding({
      id: 'USAGE',
      path: 'arguments',
      summary: 'unsupported argument',
    })
    return
  }
  if (
    (baseIndex !== -1 && !baseUrl) ||
    (baseIndex === -1 && args.includes('--base-url'))
  ) {
    process.exitCode = 2
    printFinding({
      id: 'USAGE',
      path: 'arguments',
      summary: 'missing base URL',
    })
    return
  }
  if (baseUrl && !isAllowedBaseUrl(baseUrl)) {
    process.exitCode = 2
    printFinding({
      id: 'BASE_URL',
      path: 'arguments',
      summary: 'origin is not allowlisted',
    })
    return
  }

  const findings = [...runSelfTest(), ...(await checkStaticContent(release))]
  if (baseUrl) findings.push(...(await checkLegalPages(baseUrl)))
  for (const finding of findings) printFinding(finding)
  if (findings.length > 0) process.exitCode = 1
}

await main()
