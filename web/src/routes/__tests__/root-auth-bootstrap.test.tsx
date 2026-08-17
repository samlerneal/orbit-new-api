import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'

import { GlobalRegistrator } from '@happy-dom/global-registrator'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  RouterProvider,
  createMemoryHistory,
  createRoute,
  createRouter,
} from '@tanstack/react-router'
import axios from 'axios'
import * as React from 'react'
import ReactDOM from 'react-dom/client'

import { useAuthStore } from '@/stores/auth-store'

before(() => GlobalRegistrator.register({ url: 'http://localhost' }))
after(() => GlobalRegistrator.unregister())

type DeferredResponse = {
  resolve: (value: { status: number; data: unknown }) => void
  promise: Promise<{ status: number; data: unknown }>
}

function createDeferredResponse(): DeferredResponse {
  let resolve!: DeferredResponse['resolve']
  const promise = new Promise<{ status: number; data: unknown }>((next) => {
    resolve = next
  })
  return { promise, resolve }
}

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

test('transient auth recovery blocks the protected child and Retry invalidates through pending', async () => {
  const reactEnvironment = globalThis as typeof globalThis & {
    IS_REACT_ACT_ENVIRONMENT?: boolean
  }
  const hadReactActEnvironment = Object.hasOwn(
    reactEnvironment,
    'IS_REACT_ACT_ENVIRONMENT'
  )
  const reactActEnvironment = reactEnvironment.IS_REACT_ACT_ENVIRONMENT
  reactEnvironment.IS_REACT_ACT_ENVIRONMENT = true
  localStorage.setItem('setup_status_checked', 'true')
  useAuthStore.getState().auth.reset('idle')

  const originalCreate = axios.create
  const refresh = createDeferredResponse()
  let requestCount = 0
  axios.create = ((config) => {
    const client = originalCreate(config)
    if (config?.withCredentials) {
      client.post = () => {
        requestCount += 1
        if (requestCount === 1) return Promise.reject(new Error('offline'))
        return refresh.promise as never
      }
    }
    return client
  }) as typeof axios.create

  const { Route } = await import('../__root')
  let protectedBeforeLoadCount = 0
  const protectedRoute = createRoute({
    getParentRoute: () => Route,
    path: 'protected',
    beforeLoad: () => {
      protectedBeforeLoadCount += 1
    },
    component: () => <p>Protected content</p>,
  })
  const queryClient = new QueryClient()
  const router = createRouter({
    routeTree: Route.addChildren([protectedRoute]),
    history: createMemoryHistory({ initialEntries: ['/protected'] }),
    context: { queryClient },
  })
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
    await React.act(async () => {
      await waitForDocumentCondition(
        () => document.querySelector('[role="alert"]') !== null,
        'root route did not render the transient auth recovery UI'
      )
    })

    assert.equal(protectedBeforeLoadCount, 0)
    assert.equal(
      container
        .querySelector('[role="alert"]')
        ?.textContent?.includes('Request failed'),
      true
    )

    const retry = container.querySelector('button')
    assert.ok(retry)
    await React.act(async () => {
      retry.click()
    })
    await React.act(async () => {
      await waitForDocumentCondition(
        () => document.querySelector('[aria-busy="true"]') !== null,
        'Retry did not render the auth recovery pending UI'
      )
    })
    assert.equal(container.querySelector('[aria-busy="true"]') !== null, true)

    refresh.resolve({
      status: 200,
      data: {
        success: true,
        data: {
          access_token: 'test-access-token',
          token_type: 'Bearer',
          access_expires_at: Math.floor(Date.now() / 1000) + 60,
          user: { id: 1, username: 'tester', role: 1 },
          session: {
            sid: 'test-session',
            current: true,
            login_method: 'test',
            ip: '',
            user_agent: '',
            created_at: 0,
            last_active_at: 0,
            expires_at: 0,
          },
        },
      },
    })
    await React.act(async () => {
      await waitForDocumentCondition(
        () => container.textContent?.includes('Protected content') === true,
        'protected route did not render after refresh succeeded'
      )
    })
    assert.equal(requestCount, 2)
    assert.equal(protectedBeforeLoadCount, 1)
    assert.equal(container.textContent?.includes('Protected content'), true)
  } finally {
    await React.act(async () => root.unmount())
    ;(router as unknown as { dispose?: () => void }).dispose?.()
    queryClient.clear()
    container.remove()
    axios.create = originalCreate
    useAuthStore.getState().auth.reset('idle')
    localStorage.removeItem('setup_status_checked')
    if (hadReactActEnvironment) {
      reactEnvironment.IS_REACT_ACT_ENVIRONMENT = reactActEnvironment
    } else {
      delete reactEnvironment.IS_REACT_ACT_ENVIRONMENT
    }
  }
})
