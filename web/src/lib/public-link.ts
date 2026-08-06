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
export const PUBLIC_FALLBACK_HOSTNAME = 'api.mydaily.info'

export const PUBLIC_CONTACT_TARGET: string | null = null
export const PUBLIC_CONTACT_FALLBACK_LABEL = '客服 QQ：3184917639'
export const PUBLIC_FILING_TEXT: string | null = null

export function resolveSafePublicLink(
  value: string | null | undefined
): string | null {
  if (!value) return null
  if (value.startsWith('/')) {
    if (value.startsWith('//') || value.includes('\\') || /%5c/i.test(value)) {
      return null
    }
    try {
      const parsed = new URL(value, 'https://public.invalid')
      if (
        parsed.search ||
        parsed.hash ||
        parsed.origin !== 'https://public.invalid' ||
        parsed.pathname.startsWith('//')
      ) {
        return null
      }
      return parsed.pathname
    } catch {
      return null
    }
  }

  try {
    const parsed = new URL(value)
    const isAllowedProtocol =
      parsed.protocol === 'https:' || parsed.protocol === 'mailto:'
    const hasCredentials = parsed.username !== '' || parsed.password !== ''
    const hasSensitiveQuery = [...parsed.searchParams.keys()].some((key) =>
      /(?:token|key|secret|password|auth)/i.test(key)
    )

    return isAllowedProtocol && !hasCredentials && !hasSensitiveQuery
      ? value
      : null
  } catch {
    return null
  }
}

export function getPublicHostname(serverAddress: unknown): string {
  if (typeof serverAddress !== 'string') return PUBLIC_FALLBACK_HOSTNAME

  try {
    const parsed = new URL(serverAddress)
    if (
      (parsed.protocol === 'http:' || parsed.protocol === 'https:') &&
      parsed.username === '' &&
      parsed.password === '' &&
      parsed.pathname === '/' &&
      parsed.search === '' &&
      parsed.hash === ''
    ) {
      return parsed.hostname
    }
  } catch {
    // Invalid status values must not become public markup.
  }

  return PUBLIC_FALLBACK_HOSTNAME
}
