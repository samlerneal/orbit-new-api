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
import { MessageSquareIcon, PlusIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import type { PublicChatSession } from '../lib/public-chat-sessions'

type PublicChatSidebarProps = {
  activeSessionId: string
  className?: string
  disabled?: boolean
  onCreateSession: () => void
  onSelectSession: (sessionId: string) => void
  sessions: PublicChatSession[]
}

export function PublicChatSidebar({
  activeSessionId,
  className,
  disabled = false,
  onCreateSession,
  onSelectSession,
  sessions,
}: PublicChatSidebarProps) {
  const { t } = useTranslation()

  return (
    <div className={cn('flex h-full min-h-0 flex-col p-3', className)}>
      <div className='mb-3 px-2'>
        <p className='text-sm font-semibold'>
          {t('Public chat conversations')}
        </p>
        <p className='text-muted-foreground mt-0.5 text-xs'>
          {t('Public chat stored in this browser tab')}
        </p>
      </div>
      <Button
        className='mb-3 w-full justify-start gap-2'
        disabled={disabled}
        onClick={onCreateSession}
        variant='outline'
      >
        <PlusIcon className='size-4' />
        {t('Public chat new conversation')}
      </Button>
      <nav
        aria-label={t('Public chat conversations')}
        className='min-h-0 flex-1 space-y-1 overflow-y-auto'
      >
        {sessions.map((session) => {
          const isActive = session.id === activeSessionId
          return (
            <Button
              aria-current={isActive ? 'page' : undefined}
              className={cn(
                'h-auto w-full justify-start gap-2 px-2.5 py-2 text-left',
                isActive && 'bg-accent text-accent-foreground'
              )}
              disabled={disabled}
              key={session.id}
              onClick={() => onSelectSession(session.id)}
              variant='ghost'
            >
              <MessageSquareIcon className='size-4 shrink-0' />
              <span className='min-w-0 flex-1 truncate'>
                {session.title ?? t('Public chat new conversation')}
              </span>
            </Button>
          )
        })}
      </nav>
    </div>
  )
}
