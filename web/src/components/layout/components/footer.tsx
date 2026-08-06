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

import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'
import {
  PUBLIC_CONTACT_FALLBACK_LABEL,
  PUBLIC_CONTACT_TARGET,
  resolveSafePublicLink,
} from '@/lib/public-link'
import { cn } from '@/lib/utils'

type FooterProps = { className?: string }

const NEW_API_FOOTER_ATTRIBUTION_KEY = [
  'footer',
  'new' + 'api',
  'projectAttributionSuffix',
].join('.')

function PublicFooterLink(props: { label: string; href: string | null }) {
  const { t } = useTranslation()
  if (!props.href) {
    return (
      <span aria-disabled='true' className='text-muted-foreground/50 text-sm'>
        {t(props.label)}
      </span>
    )
  }
  if (props.href.startsWith('https:') || props.href.startsWith('mailto:')) {
    return (
      <a
        href={props.href}
        target='_blank'
        rel='noopener noreferrer'
        className='text-muted-foreground hover:text-foreground text-sm transition-colors'
      >
        {t(props.label)}
      </a>
    )
  }
  return (
    <Link
      to={props.href}
      className='text-muted-foreground hover:text-foreground text-sm transition-colors'
    >
      {t(props.label)}
    </Link>
  )
}

function ProjectAttribution(props: { currentYear: number }) {
  const { t } = useTranslation()
  return (
    <span className='text-muted-foreground/45 text-xs'>
      &copy; {props.currentYear}{' '}
      <a
        href='https://github.com/QuantumNous/new-api'
        target='_blank'
        rel='noopener noreferrer'
        className='text-foreground/70 hover:text-foreground font-medium transition-colors'
      >
        {t('Open source license')}
      </a>
      . {t(NEW_API_FOOTER_ATTRIBUTION_KEY)} <span aria-hidden='true'>·</span>{' '}
      <a
        href='https://github.com/samlerneal/orbit-new-api'
        target='_blank'
        rel='noopener noreferrer'
        className='text-foreground/70 hover:text-foreground font-medium transition-colors'
      >
        {t('Modified source')}
      </a>
    </span>
  )
}

export function Footer(props: FooterProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const { systemName, logo } = useSystemConfig()
  const contactTarget = resolveSafePublicLink(PUBLIC_CONTACT_TARGET)
  const tutorialTarget =
    resolveSafePublicLink(status?.docs_link as string | undefined) || '/docs'
  const currentYear = new Date().getFullYear()

  return (
    <footer
      className={cn('border-border/40 relative z-10 border-t', props.className)}
    >
      <div className='mx-auto max-w-6xl px-6 py-12 md:py-16'>
        <div className='flex flex-col justify-between gap-8 md:flex-row'>
          <Link to='/' className='flex items-center gap-2.5'>
            <img
              src={logo || '/logo.png'}
              alt={systemName}
              className='size-7 rounded-lg object-contain'
            />
            <span className='text-sm font-semibold tracking-tight'>
              {systemName}
            </span>
          </Link>
          <nav
            aria-label={t('Public footer navigation')}
            className='flex flex-wrap gap-x-5 gap-y-3'
          >
            <span
              aria-disabled='true'
              className='text-muted-foreground/50 text-sm'
            >
              {t('Public privacy policy')}
            </span>
            <span
              aria-disabled='true'
              className='text-muted-foreground/50 text-sm'
            >
              {t('Public terms of service')}
            </span>
            <PublicFooterLink label='Public model pricing' href='/pricing' />
            <PublicFooterLink
              label='Public usage tutorial'
              href={tutorialTarget}
            />
            {contactTarget ? (
              <PublicFooterLink label='Public contact' href={contactTarget} />
            ) : (
              <span
                aria-disabled='true'
                className='text-muted-foreground/50 text-sm'
              >
                {t('Public contact')}: {PUBLIC_CONTACT_FALLBACK_LABEL}
              </span>
            )}
          </nav>
        </div>
        <div className='border-border/30 mt-10 flex flex-col gap-3 border-t pt-6 sm:flex-row sm:items-center sm:justify-between'>
          <span className='text-muted-foreground/40 text-xs'>
            &copy; {currentYear} {t('Mydaily API operator')}
          </span>
          <ProjectAttribution currentYear={currentYear} />
        </div>
      </div>
    </footer>
  )
}
