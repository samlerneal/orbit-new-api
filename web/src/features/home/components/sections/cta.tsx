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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

export function CTA(_props: { isAuthenticated?: boolean }) {
  const { t } = useTranslation()
  return (
    <section className='px-6 py-24'>
      <div className='mx-auto max-w-3xl text-center'>
        <h2 className='text-3xl font-semibold'>{t('Public CTA title')}</h2>
        <p className='text-muted-foreground mt-4 text-lg'>
          {t('Public CTA description')}
        </p>
        <Button className='mt-8' size='lg' render={<Link to='/dashboard' />}>
          {t('Public open console')}
        </Button>
      </div>
    </section>
  )
}
