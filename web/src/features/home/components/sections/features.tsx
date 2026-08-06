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
        <div className='grid gap-8 md:grid-cols-2'>
          <div>
            <h2 className='text-base font-semibold'>{t('Available now')}</h2>
            <ul
              data-catalog-source={PUBLIC_HOME_CATALOG.availableSource}
              className='mt-4 flex flex-wrap gap-3'
            >
              {availableModels.map((model) => (
                <li
                  key={model}
                  className='bg-background flex items-center gap-2 rounded-lg border px-3 py-2'
                >
                  {getLobeIcon(logoByModel[model as keyof typeof logoByModel])}
                  {model}
                </li>
              ))}
            </ul>
          </div>
          <div>
            <h2 className='text-base font-semibold'>{t('Coming soon')}</h2>
            <ul
              data-catalog-source={PUBLIC_HOME_CATALOG.plannedSource}
              className='mt-4 flex flex-wrap gap-3'
            >
              {plannedModels.map((model) => (
                <li
                  key={model}
                  className='bg-background flex items-center gap-2 rounded-lg border px-3 py-2'
                >
                  {getLobeIcon(logoByModel[model as keyof typeof logoByModel])}
                  {model}
                </li>
              ))}
            </ul>
          </div>
        </div>
      </div>
    </section>
  )
}
