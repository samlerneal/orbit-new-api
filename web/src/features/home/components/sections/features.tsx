import { useTranslation } from 'react-i18next'

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
import { getLobeIcon } from '@/lib/lobe-icon'

import {
  hasCompletePublicHomeCatalog,
  PUBLIC_HOME_CATALOG,
} from '../../constants'

export function Features() {
  const { t } = useTranslation()
  const hasCatalog = hasCompletePublicHomeCatalog(PUBLIC_HOME_CATALOG)
  const availableModels = hasCatalog ? PUBLIC_HOME_CATALOG.availableModels : []
  const plannedModels = hasCatalog ? PUBLIC_HOME_CATALOG.plannedModels : []
  const logoByModel = {
    GPT: 'OpenAI',
    Claude: 'Anthropic',
    Gemini: 'Gemini',
    xAI: 'XAI',
  } as const

  if (!hasCatalog) return null

  return (
    <section className='border-border/40 border-y px-6 py-20'>
      <div className='mx-auto max-w-4xl'>
        <div className='mb-10 text-center'>
          <h2 className='text-2xl font-semibold'>
            {t('Public catalog title')}
          </h2>
          <p className='text-muted-foreground mt-3'>
            {t('Public catalog description')}
          </p>
        </div>
        <div className='overflow-x-auto pb-2'>
          <div
            data-catalog-layout='horizontal'
            className='mx-auto flex w-max min-w-full items-center justify-center gap-8 px-2'
          >
            <div className='flex shrink-0 items-center gap-3'>
              <h3 className='text-base font-semibold whitespace-nowrap'>
                {t('Available now')}
              </h3>
              <ul
                data-catalog-source={PUBLIC_HOME_CATALOG.availableSource}
                className='flex flex-nowrap items-center justify-center gap-3'
              >
                {availableModels.map((model) => (
                  <li
                    key={model}
                    aria-label={model}
                    title={model}
                    className='bg-background flex cursor-default items-center gap-2 rounded-lg border px-3 py-2 shadow-sm transition duration-200 hover:-translate-y-0.5 hover:shadow-md'
                  >
                    {getLobeIcon(
                      logoByModel[model as keyof typeof logoByModel]
                    )}
                  </li>
                ))}
              </ul>
            </div>
            <div className='flex shrink-0 items-center gap-3'>
              <h3 className='text-base font-semibold whitespace-nowrap'>
                {t('Public catalog planned')}
              </h3>
              <ul
                data-catalog-source={PUBLIC_HOME_CATALOG.plannedSource}
                className='flex flex-nowrap items-center justify-center gap-3'
              >
                {plannedModels.map((model) => (
                  <li
                    key={model}
                    aria-label={model}
                    title={model}
                    className='bg-background flex cursor-default items-center gap-2 rounded-lg border px-3 py-2 shadow-sm transition duration-200 hover:-translate-y-0.5 hover:shadow-md'
                  >
                    {getLobeIcon(
                      logoByModel[model as keyof typeof logoByModel]
                    )}
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
