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
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useStatus } from '@/hooks/use-status'
import { resolveSafePublicLink } from '@/lib/public-link'

export function HowItWorks() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const tutorialTarget = resolveSafePublicLink(
    status?.docs_link as string | undefined
  )
  return (
    <section className='px-6 py-20'>
      <div className='mx-auto max-w-4xl text-center'>
        <h2 className='text-2xl font-semibold'>{t('Public steps title')}</h2>
        <p className='text-muted-foreground mt-3'>
          {t('Public steps description')}
        </p>
        <ol className='mt-10 grid gap-4 text-left md:grid-cols-3'>
          {[
            ['Public step one', 'Public step one description'],
            ['Public step two', 'Public step two description'],
            ['Public step three', 'Public step three description'],
          ].map(([step, description], index) => (
            <li key={step} className='rounded-xl border p-5'>
              <span className='text-muted-foreground text-sm font-medium'>
                0{index + 1}
              </span>
              <p className='mt-3 font-medium'>{t(step)}</p>
              <p className='text-muted-foreground mt-2 text-sm leading-6'>
                {t(description)}
              </p>
            </li>
          ))}
        </ol>
        <Button
          className='mt-8'
          variant='outline'
          disabled={!tutorialTarget}
          render={
            <a
              href={tutorialTarget ?? undefined}
              target={
                tutorialTarget?.startsWith('https:') ? '_blank' : undefined
              }
              rel={
                tutorialTarget?.startsWith('https:')
                  ? 'noopener noreferrer'
                  : undefined
              }
            />
          }
        >
          {t('Public view tutorial')}
        </Button>
      </div>
    </section>
  )
}
