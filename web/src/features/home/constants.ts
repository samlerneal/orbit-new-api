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
import type { TFunction } from 'i18next'

export const MAIN_BASE_CLASSES = 'bg-background text-foreground w-full'

// Retained only for the internal GatewayCard import path. Public content must
// remain absent until the Owner-provided catalog is ready.
export function getGatewayFeatures(_t: TFunction): string[] {
  return []
}

export type PublicHomeCatalog = {
  availableModels: readonly string[]
  plannedModels: readonly string[]
  availableSource: string | null
  plannedSource: string | null
}

export const PUBLIC_HOME_CATALOG: PublicHomeCatalog = {
  availableModels: ['GPT'],
  plannedModels: ['Claude', 'Gemini', 'xAI'],
  availableSource: 'Owner production confirmation',
  plannedSource: 'Owner roadmap',
}

export function hasCompletePublicHomeCatalog(
  catalog: PublicHomeCatalog
): boolean {
  const isConsistent =
    new Set([...catalog.availableModels, ...catalog.plannedModels]).size ===
    catalog.availableModels.length + catalog.plannedModels.length

  return (
    catalog.availableModels.length > 0 &&
    catalog.plannedModels.length > 0 &&
    Boolean(catalog.availableSource) &&
    Boolean(catalog.plannedSource) &&
    isConsistent
  )
}
