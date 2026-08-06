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

export function HowItWorks() {
  const { t } = useTranslation()
  return (
    <section className='px-6 py-20'>
      <div className='mx-auto max-w-4xl text-center'>
        <h2 className='text-2xl font-semibold'>{t('Public steps title')}</h2>
        <p className='text-muted-foreground mt-3'>
          {t('Public steps description')}
        </p>
        <ol className='mt-10 grid gap-4 text-left md:grid-cols-3'>
          {['Public step one', 'Public step two', 'Public step three'].map(
            (step, index) => (
              <li key={step} className='rounded-xl border p-5'>
                <span className='text-muted-foreground text-sm font-medium'>
                  0{index + 1}
                </span>
                <p className='mt-3 font-medium'>{t(step)}</p>
              </li>
            )
          )}
        </ol>
      </div>
    </section>
  )
}
