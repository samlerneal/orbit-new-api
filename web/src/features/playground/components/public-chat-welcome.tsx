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
import { BarChart3Icon, Code2Icon, LightbulbIcon, TextIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

type PublicChatWelcomeProps = {
  disabled?: boolean
  logo?: string
  onPromptSelect: (prompt: string) => void
  systemName: string
}

export function PublicChatWelcome({
  disabled = false,
  logo,
  onPromptSelect,
  systemName,
}: PublicChatWelcomeProps) {
  const { t } = useTranslation()
  const prompts = [
    {
      icon: BarChart3Icon,
      label: t('Public chat analyze data'),
      prompt: t('Public chat analyze data prompt'),
    },
    {
      icon: TextIcon,
      label: t('Public chat summarize text'),
      prompt: t('Public chat summarize text prompt'),
    },
    {
      icon: Code2Icon,
      label: t('Public chat explain code'),
      prompt: t('Public chat explain code prompt'),
    },
    {
      icon: LightbulbIcon,
      label: t('Public chat get suggestions'),
      prompt: t('Public chat get suggestions prompt'),
    },
  ]

  return (
    <div className='mx-auto flex w-full max-w-3xl flex-col items-center px-4 py-10 text-center md:py-16'>
      {logo && (
        <div className='bg-card mb-5 flex size-14 items-center justify-center rounded-2xl border shadow-sm'>
          <img
            alt={systemName}
            className='size-9 rounded-xl object-cover'
            src={logo}
          />
        </div>
      )}
      <p className='text-muted-foreground mb-2 text-sm font-medium'>
        {systemName}
      </p>
      <h1 className='text-2xl font-semibold tracking-tight md:text-3xl'>
        {t('Start a conversation')}
      </h1>
      <p className='text-muted-foreground mt-3 max-w-xl text-sm leading-6 md:text-base'>
        {t('Public chat welcome description')}
      </p>
      <div className='mt-8 grid w-full grid-cols-1 gap-2 sm:grid-cols-2'>
        {prompts.map(({ icon: Icon, label, prompt }) => (
          <Button
            className='bg-card h-auto min-h-12 justify-start gap-3 rounded-xl px-4 py-3 text-left shadow-none'
            disabled={disabled}
            key={label}
            onClick={() => onPromptSelect(prompt)}
            variant='outline'
          >
            <Icon className='text-muted-foreground size-4 shrink-0' />
            <span>{label}</span>
          </Button>
        ))}
      </div>
    </div>
  )
}
