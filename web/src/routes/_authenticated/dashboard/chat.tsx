/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { createFileRoute, redirect } from '@tanstack/react-router'

import { Main } from '@/components/layout'
import { PublicChat } from '@/features/playground'
import { isSidebarModuleEnabled } from '@/lib/nav-modules'

export const Route = createFileRoute('/_authenticated/dashboard/chat')({
  beforeLoad: () => {
    if (!isSidebarModuleEnabled('chat', 'chat')) {
      throw redirect({ to: '/dashboard' })
    }
  },
  component: DashboardChatPage,
})

function DashboardChatPage() {
  return (
    <Main className='p-0'>
      <PublicChat embedded />
    </Main>
  )
}
