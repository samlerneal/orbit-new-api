import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'

import { GlobalRegistrator } from '@happy-dom/global-registrator'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  Outlet,
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
} from '@tanstack/react-router'
import * as React from 'react'
import ReactDOM from 'react-dom/client'

import { useAuthStore } from '@/stores/auth-store'

before(() => GlobalRegistrator.register({ url: 'http://localhost' }))
after(() => GlobalRegistrator.unregister())

function waitForDocumentCondition(
  condition: () => boolean,
  message: string
): Promise<void> {
  if (condition()) return Promise.resolve()

  return new Promise((resolve, reject) => {
    const observer = new MutationObserver(() => {
      if (!condition()) return
      cleanup()
      resolve()
    })
    const timeout = globalThis.setTimeout(() => {
      cleanup()
      reject(new Error(message))
    }, 1_000)
    const cleanup = () => {
      observer.disconnect()
      globalThis.clearTimeout(timeout)
    }

    observer.observe(document.body, { childList: true, subtree: true })
    if (condition()) {
      cleanup()
      resolve()
    }
  })
}

function click(element: Element) {
  element.dispatchEvent(
    new MouseEvent('click', { bubbles: true, cancelable: true })
  )
}

test('the real sidebar trigger opens a 252px overlay and restores focus after Escape', async () => {
  const reactEnvironment = globalThis as typeof globalThis & {
    IS_REACT_ACT_ENVIRONMENT?: boolean
  }
  const hadReactActEnvironment = Object.hasOwn(
    reactEnvironment,
    'IS_REACT_ACT_ENVIRONMENT'
  )
  const reactActEnvironment = reactEnvironment.IS_REACT_ACT_ENVIRONMENT
  reactEnvironment.IS_REACT_ACT_ENVIRONMENT = true
  useAuthStore.getState().auth.setUser({ id: 1, username: 'tester', role: 10 })
  const { AppSidebar } =
    await import('@/components/layout/components/app-sidebar')
  const { Header } = await import('@/components/layout/components/header')
  const { SidebarInset, SidebarProvider } =
    await import('@/components/ui/sidebar')

  function NavigationLayout() {
    return (
      <SidebarProvider defaultOpen={false}>
        <Header />
        <AppSidebar />
        <SidebarInset>
          <Outlet />
        </SidebarInset>
      </SidebarProvider>
    )
  }

  const rootRoute = createRootRoute({ component: NavigationLayout })
  const indexRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/',
    component: () => <p>Page body</p>,
  })
  const settingsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: 'system-settings',
    component: () => <p>System settings</p>,
  })
  const router = createRouter({
    routeTree: rootRoute.addChildren([indexRoute, settingsRoute]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  const queryClient = new QueryClient()
  const container = document.createElement('div')
  document.body.append(container)
  const root = ReactDOM.createRoot(container)

  try {
    await React.act(async () => {
      root.render(
        <QueryClientProvider client={queryClient}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      )
    })
    const trigger = container.querySelector<HTMLButtonElement>(
      '[data-slot="sidebar-trigger"]'
    )
    assert.ok(trigger)
    assert.equal(trigger.getAttribute('aria-expanded'), 'false')
    assert.equal(container.querySelector('[data-slot="sidebar-gap"]'), null)

    await React.act(async () => click(trigger))
    await waitForDocumentCondition(
      () => document.querySelector('#app-navigation-sheet') !== null,
      'sidebar trigger did not open the navigation sheet'
    )
    const sheet = document.querySelector<HTMLElement>('#app-navigation-sheet')
    assert.ok(sheet)
    assert.equal(sheet.className.includes('w-[252px]'), true)
    assert.equal(trigger.getAttribute('aria-controls'), 'app-navigation-sheet')
    assert.equal(trigger.getAttribute('aria-expanded'), 'true')

    const overlay = document.querySelector<HTMLElement>(
      '[data-slot="sheet-overlay"]'
    )
    assert.ok(overlay)
    await React.act(async () => click(overlay))
    await waitForDocumentCondition(
      () => trigger.getAttribute('aria-expanded') === 'false',
      'overlay did not close the navigation sheet'
    )
    assert.equal(trigger.getAttribute('aria-expanded'), 'false')

    await React.act(async () => click(trigger))
    const navigationLink = document.querySelector<HTMLAnchorElement>(
      '#app-navigation-sheet a'
    )
    assert.ok(navigationLink)
    await React.act(async () => click(navigationLink))
    await waitForDocumentCondition(
      () => trigger.getAttribute('aria-expanded') === 'false',
      'navigation link did not close the navigation sheet'
    )
    assert.equal(trigger.getAttribute('aria-expanded'), 'false')

    await React.act(async () => {
      await router.navigate({ to: '/system-settings' })
    })
    await React.act(async () => click(trigger))
    const viewHeaderLink = document.querySelector<HTMLAnchorElement>(
      '#app-navigation-sheet [data-slot="sidebar-header"] a'
    )
    assert.ok(viewHeaderLink)
    await React.act(async () => click(viewHeaderLink))
    await waitForDocumentCondition(
      () => trigger.getAttribute('aria-expanded') === 'false',
      'sidebar view header link did not close the navigation sheet'
    )
    assert.equal(trigger.getAttribute('aria-expanded'), 'false')

    await React.act(async () => click(trigger))
    await React.act(async () =>
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })
      )
    )
    await waitForDocumentCondition(
      () => trigger.getAttribute('aria-expanded') === 'false',
      'Escape did not close the navigation sheet'
    )
    assert.equal(trigger.getAttribute('aria-expanded'), 'false')
    assert.equal(document.activeElement, trigger)
  } finally {
    await React.act(async () => root.unmount())
    ;(router as unknown as { dispose?: () => void }).dispose?.()
    queryClient.clear()
    container.remove()
    useAuthStore.getState().auth.reset('idle')
    if (hadReactActEnvironment) {
      reactEnvironment.IS_REACT_ACT_ENVIRONMENT = reactActEnvironment
    } else {
      delete reactEnvironment.IS_REACT_ACT_ENVIRONMENT
    }
  }
})
