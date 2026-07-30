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
export interface PublicBrandInput {
  locale: string
  publicBrandZhCN?: string | null
  systemName?: string | null
  defaultSystemName: string
}

export const CANONICAL_PUBLIC_BRAND_NAME = 'Mydaily API'

export const CC_SWITCH_DEFAULT_PROVIDER_BY_APP = {
  claude: CANONICAL_PUBLIC_BRAND_NAME,
  codex: CANONICAL_PUBLIC_BRAND_NAME,
  gemini: CANONICAL_PUBLIC_BRAND_NAME,
} as const

export function getPublicBrandName({
  locale,
  publicBrandZhCN,
  systemName,
  defaultSystemName,
}: PublicBrandInput): string {
  const canonicalName = systemName?.trim() || defaultSystemName
  const chineseBrandName = publicBrandZhCN?.trim()

  if (locale === 'zhCN' && chineseBrandName) return chineseBrandName

  return canonicalName
}
