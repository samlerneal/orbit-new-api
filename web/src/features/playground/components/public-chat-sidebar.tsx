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
import { MessageSquareIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import type { PublicChatSession } from '../lib/public-chat-sessions'

type PublicChatSidebarProps = {
  activeSessionId: string
  className?: string
  disabled?: boolean
  onCreateSession: () => void
  onClearSessions: () => void
  onDeleteSession: (sessionId: string) => void
  onSelectSession: (sessionId: string) => void
  sessions: PublicChatSession[]
}

// Shared by desktop sidebar and the mobile Sheet: generation locks every
// destructive/session-changing action while leaving the stop control outside.
// eslint-disable-next-line react/only-export-components
export function canChangePublicChatSessions(disabled: boolean): boolean {
  return !disabled
}

export function PublicChatSidebar({
  activeSessionId,
  className,
  disabled = false,
  onCreateSession,
  onClearSessions,
  onDeleteSession,
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
          {t('Public chat stored in this browser')}
        </p>
      </div>
      <Button
        className='mb-3 w-full justify-start gap-2'
        disabled={!canChangePublicChatSessions(disabled)}
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
            <div className='flex gap-1' key={session.id}>
              <Button
                aria-current={isActive ? 'page' : undefined}
                className={cn(
                  'h-auto min-w-0 flex-1 justify-start gap-2 px-2.5 py-2 text-left',
                  isActive && 'bg-accent text-accent-foreground'
                )}
                disabled={disabled}
                onClick={() => onSelectSession(session.id)}
                variant='ghost'
              >
                <MessageSquareIcon className='size-4 shrink-0' />
                <span className='min-w-0 flex-1 truncate'>
                  {session.title ?? t('Public chat new conversation')}
                </span>
              </Button>
              <AlertDialog>
                <AlertDialogTrigger
                  render={
                    <Button
                      aria-label={t('Public chat delete conversation')}
                      disabled={disabled}
                      size='icon-sm'
                      variant='ghost'
                    >
                      <Trash2Icon className='size-4' />
                    </Button>
                  }
                />
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>
                      {t('Public chat delete conversation')}
                    </AlertDialogTitle>
                    <AlertDialogDescription>
                      {t('Public chat delete confirmation')}
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
                    <AlertDialogAction
                      disabled={disabled}
                      onClick={() => onDeleteSession(session.id)}
                    >
                      {t('Delete')}
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </div>
          )
        })}
      </nav>
      <AlertDialog>
        <AlertDialogTrigger
          render={
            <Button className='mt-3' disabled={disabled} variant='ghost'>
              {t('Public chat clear conversations')}
            </Button>
          }
        />
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('Public chat clear conversations')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('Public chat clear confirmation')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction disabled={disabled} onClick={onClearSessions}>
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
