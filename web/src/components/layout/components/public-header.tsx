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
import { Link, useNavigate, useRouterState } from '@tanstack/react-router'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { LanguageSwitcher } from '@/components/language-switcher'
import { NotificationPopover } from '@/components/notification-popover'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { ThemeSwitch } from '@/components/theme-switch'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Skeleton } from '@/components/ui/skeleton'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { useNotifications } from '@/hooks/use-notifications'
import { useSystemConfig } from '@/hooks/use-system-config'
import { useTopNavLinks } from '@/hooks/use-top-nav-links'
import { PUBLIC_CONTACT_FALLBACK_LABEL } from '@/lib/public-link'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import type { TopNavLink } from '../types'
import { HeaderLogo } from './header-logo'

const PUBLIC_LANGUAGE_CODES = ['zhCN', 'zhTW', 'en', 'ru'] as const

type AuthPromptTarget = Pick<TopNavLink, 'title' | 'href'>

export interface PublicHeaderProps {
  navLinks?: TopNavLink[]
  showThemeSwitch?: boolean
  showLanguageSwitcher?: boolean
  logo?: React.ReactNode
  siteName?: string
  homeUrl?: string
  showAuthButtons?: boolean
  showNotifications?: boolean
  className?: string
}

function PublicNavItem(props: {
  link: TopNavLink
  onClick: (
    event: React.MouseEvent<HTMLAnchorElement>,
    link: TopNavLink
  ) => void
  mobile?: boolean
}) {
  const { t } = useTranslation()
  const className = cn(
    'shrink-0 rounded-lg px-3 py-1.5 text-[13px] font-medium text-muted-foreground transition-colors hover:text-foreground',
    props.mobile && 'px-3 text-sm',
    props.link.disabled && 'pointer-events-none opacity-50'
  )

  if (props.link.disabled) {
    return (
      <span aria-disabled='true' className={className}>
        {t(props.link.title)}
      </span>
    )
  }

  if (props.link.external) {
    return (
      <a
        href={props.link.href}
        target='_blank'
        rel='noopener noreferrer'
        onClick={(event) => props.onClick(event, props.link)}
        className={className}
      >
        {t(props.link.title)}
      </a>
    )
  }

  return (
    <Link
      to={props.link.href}
      disabled={props.link.disabled}
      onClick={(event) => props.onClick(event, props.link)}
      className={className}
    >
      {t(props.link.title)}
    </Link>
  )
}

function PublicSupportPopover(props: { mobile?: boolean }) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const triggerClassName = cn(
    'shrink-0 rounded-lg px-3 py-1.5 text-[13px] font-medium text-muted-foreground transition-colors hover:text-foreground',
    props.mobile && 'px-3 text-sm'
  )

  return (
    <Popover>
      <PopoverTrigger
        render={
          <button type='button' className={triggerClassName}>
            {t('Contact support')}
          </button>
        }
      />
      <PopoverContent className='w-60' align='end'>
        <p className='font-medium'>{PUBLIC_CONTACT_FALLBACK_LABEL}</p>
        <Button
          size='sm'
          variant='outline'
          className='mt-2 w-full'
          onClick={() => void copyToClipboard('3184917639')}
        >
          {copiedText === '3184917639' ? t('Copied') : t('Copy')}
        </Button>
      </PopoverContent>
    </Popover>
  )
}

export function PublicHeader(props: PublicHeaderProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { auth } = useAuthStore()
  const { logo: systemLogo, loading, logoLoaded } = useSystemConfig()
  const notifications = useNotifications()
  const pathname = useRouterState().location.pathname
  const dynamicLinks = useTopNavLinks({ scope: 'public' })
  const [authPromptTarget, setAuthPromptTarget] =
    useState<AuthPromptTarget | null>(null)
  const isAuthenticated = Boolean(auth.user)
  const publicLinks: TopNavLink[] = dynamicLinks.length
    ? dynamicLinks
    : (props.navLinks ?? [])

  useEffect(() => {
    if (!authPromptTarget) return
    const timeoutId = window.setTimeout(
      () =>
        navigate({
          to: '/sign-in',
          search: { redirect: authPromptTarget.href },
        }),
      5000
    )
    return () => window.clearTimeout(timeoutId)
  }, [authPromptTarget, navigate])

  const handleNavLinkClick = useCallback(
    (event: React.MouseEvent<HTMLAnchorElement>, link: TopNavLink) => {
      if (link.disabled) {
        event.preventDefault()
        return
      }
      if (link.requiresAuth) {
        event.preventDefault()
        setAuthPromptTarget(link)
      }
    },
    []
  )

  return (
    <>
      <header
        className={cn(
          'bg-background/90 fixed inset-x-0 top-0 z-50 border-b backdrop-blur',
          props.className
        )}
      >
        <div className='mx-auto max-w-7xl px-4 md:px-6'>
          <div className='flex h-16 items-center justify-between gap-3'>
            <Link
              to={props.homeUrl || '/'}
              className='group flex min-w-0 shrink-0 items-center gap-2.5'
            >
              <div className='flex size-7 shrink-0 items-center justify-center'>
                {loading ? (
                  <Skeleton className='size-full rounded-lg' />
                ) : (
                  props.logo || (
                    <HeaderLogo
                      src={systemLogo}
                      loading={loading}
                      logoLoaded={logoLoaded}
                      className='size-full rounded-lg object-contain'
                    />
                  )
                )}
              </div>
              <span className='truncate text-sm font-semibold tracking-tight'>
                {loading ? <Skeleton className='h-4 w-16' /> : '日课 API'}
              </span>
            </Link>
            <nav
              aria-label={t('Public navigation')}
              className='hidden items-center gap-0.5 md:flex'
            >
              {publicLinks.map((link) => (
                <PublicNavItem
                  key={link.title}
                  link={link}
                  onClick={handleNavLinkClick}
                />
              ))}
              <PublicSupportPopover />
            </nav>
            <div className='flex shrink-0 items-center gap-1'>
              {props.showNotifications !== false && (
                <NotificationPopover
                  open={notifications.popoverOpen}
                  onOpenChange={notifications.setPopoverOpen}
                  unreadCount={notifications.unreadCount}
                  activeTab={notifications.activeTab}
                  onTabChange={notifications.setActiveTab}
                  notice={notifications.notice}
                  announcements={notifications.announcements}
                  loading={notifications.loading}
                />
              )}
              {props.showThemeSwitch !== false && <ThemeSwitch />}
              {props.showLanguageSwitcher !== false && (
                <LanguageSwitcher
                  visibleLanguageCodes={PUBLIC_LANGUAGE_CODES}
                />
              )}
              {props.showAuthButtons !== false &&
                (isAuthenticated ? (
                  <ProfileDropdown />
                ) : (
                  <Button
                    size='sm'
                    className='h-8 rounded-lg px-3.5 text-xs'
                    render={<Link to='/sign-in' />}
                  >
                    {t('Sign in')}
                  </Button>
                ))}
            </div>
          </div>
          <nav
            aria-label={t('Public navigation')}
            className='-mx-4 flex h-12 overflow-x-auto border-t px-4 md:hidden'
          >
            {publicLinks.map((link) => (
              <PublicNavItem
                key={link.title}
                link={link}
                onClick={handleNavLinkClick}
                mobile
              />
            ))}
            <PublicSupportPopover mobile />
          </nav>
        </div>
      </header>
      <Dialog
        open={Boolean(authPromptTarget)}
        onOpenChange={(open) => !open && setAuthPromptTarget(null)}
        title={t('Sign in required')}
        description={t('Please sign in to view {{module}}.', {
          module: authPromptTarget?.title || '',
        })}
        footer={
          <Button
            onClick={() =>
              navigate({
                to: '/sign-in',
                search: { redirect: authPromptTarget?.href || pathname },
              })
            }
          >
            {t('Sign in now')}
          </Button>
        }
      >
        {null}
      </Dialog>
    </>
  )
}
