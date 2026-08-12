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

For commercial licensing, please contact support@quantumnous.com
*/
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { useStatus } from '@/hooks/use-status'
import { parseHeaderNavModulesFromStatus } from '@/lib/nav-modules'
import { resolveSafePublicLink } from '@/lib/public-link'
import { useAuthStore } from '@/stores/auth-store'

export type TopNavLink = {
  title: string
  href: string
  disabled?: boolean
  requiresAuth?: boolean
  external?: boolean
}

type TopNavLinkScope = 'default' | 'public'

type UseTopNavLinksOptions = {
  scope?: TopNavLinkScope
}

export function useTopNavLinks(
  props: UseTopNavLinksOptions = {}
): TopNavLink[] {
  const { t } = useTranslation()
  const { status } = useStatus()
  const { auth } = useAuthStore()

  const docsLink: string | undefined = status?.docs_link as string | undefined
  const safeDocsLink = resolveSafePublicLink(docsLink)
  const isAuthed = !!auth?.user

  const modules = useMemo(() => {
    return parseHeaderNavModulesFromStatus(
      status as Record<string, unknown> | null
    )
  }, [status])

  return useMemo(() => {
    if (props.scope === 'public') {
      return [
        { title: t('Console'), href: '/dashboard', requiresAuth: !isAuthed },
        { title: t('Models'), href: '/pricing' },
        {
          title: t('Public tutorial'),
          href: safeDocsLink || '/docs',
          external: Boolean(
            safeDocsLink?.startsWith('https:') ||
            safeDocsLink?.startsWith('mailto:')
          ),
        },
        { title: t('Chat'), href: '/chat', requiresAuth: !isAuthed },
        {
          title: t('Image studio'),
          href: '/image-studio',
          requiresAuth: !isAuthed,
        },
      ]
    }

    const links: TopNavLink[] = []
    if (modules?.home !== false) links.push({ title: t('Home'), href: '/' })
    if (modules?.console !== false) {
      links.push({ title: t('Console'), href: '/dashboard' })
    }
    const pricing = modules?.pricing
    if (pricing && typeof pricing === 'object' && pricing.enabled) {
      links.push({
        title: t('Model Square'),
        href: '/pricing',
        requiresAuth: pricing.requireAuth && !isAuthed,
      })
    }
    const rankings = modules?.rankings
    if (rankings && typeof rankings === 'object' && rankings.enabled) {
      links.push({
        title: t('Rankings'),
        href: '/rankings',
        requiresAuth: rankings.requireAuth && !isAuthed,
      })
    }
    if (modules?.docs !== false) {
      links.push({
        title: t('Usage guide'),
        href: safeDocsLink || '/docs',
        external: Boolean(
          safeDocsLink?.startsWith('https:') ||
          safeDocsLink?.startsWith('mailto:')
        ),
      })
    }
    if (modules?.about !== false) {
      links.push({ title: t('About'), href: '/about' })
    }
    return links
  }, [isAuthed, modules, props.scope, safeDocsLink, t])
}
