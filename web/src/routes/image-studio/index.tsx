import { createFileRoute, redirect } from '@tanstack/react-router'

import { PublicLayout } from '@/components/layout'
import { sanitizeAuthRedirect } from '@/features/auth/lib/auth-redirect'
import { ImageStudio } from '@/features/image-studio'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/image-studio/')({
  beforeLoad: ({ location }) => {
    const { auth } = useAuthStore.getState()
    if (!auth.user || !auth.accessToken) {
      throw redirect({
        to: '/sign-in',
        search: {
          redirect: getImageStudioSignInRedirect(
            location.href,
            window.location.origin
          ),
        },
      })
    }
  },
  component: ImageStudioPage,
})

export function getImageStudioSignInRedirect(
  locationHref: string,
  origin: string
) {
  return sanitizeAuthRedirect(locationHref, origin) ?? '/image-studio'
}

function ImageStudioPage() {
  return (
    <PublicLayout showMainContainer={false}>
      <ImageStudio />
    </PublicLayout>
  )
}
