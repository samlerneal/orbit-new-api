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
type StatusWithServerAddress = {
  server_address?: unknown
  data?: {
    server_address?: unknown
  } | null
}

function getConfiguredServerAddress(status: StatusWithServerAddress): string {
  if (typeof status.server_address === 'string') return status.server_address
  if (typeof status.data?.server_address === 'string') {
    return status.data.server_address
  }

  return ''
}

export function resolvePublicServerAddress(
  status: StatusWithServerAddress | null | undefined,
  origin: string
): string {
  const configuredAddress = status ? getConfiguredServerAddress(status) : ''

  try {
    const parsedAddress = new URL(configuredAddress.trim())
    const isHttpOrigin =
      parsedAddress.protocol === 'http:' || parsedAddress.protocol === 'https:'
    const hasNoCredentials =
      parsedAddress.username === '' && parsedAddress.password === ''
    const hasNoPathOrSuffix =
      parsedAddress.pathname === '/' &&
      parsedAddress.search === '' &&
      parsedAddress.hash === ''

    if (isHttpOrigin && hasNoCredentials && hasNoPathOrSuffix) {
      return parsedAddress.origin
    }
  } catch {
    /* empty */
  }

  return origin
}

export function getPublicServerAddress(): string {
  const origin = window.location.origin

  try {
    const rawStatus = localStorage.getItem('status')
    if (rawStatus) {
      return resolvePublicServerAddress(JSON.parse(rawStatus), origin)
    }
  } catch {
    /* empty */
  }

  return resolvePublicServerAddress(undefined, origin)
}
