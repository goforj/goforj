import { reactive } from 'vue'

export type AuthUser = {
  id: number
  username: string
  email: string
  display_name: string
  avatar_url: string
  active: boolean
  email_verified_at?: string | null
  last_login_at?: string | null
  last_seen_at?: string | null
  timezone?: string
  locale?: string
}

type AuthUserResponse = {
  ok: boolean
  user?: AuthUser
  error?: string
}

type OKResponse = {
  ok: boolean
  error?: string
}

export const authState = reactive({
  bootstrapped: false,
  loading: false,
  user: null as AuthUser | null,
})

async function readJSON<T>(response: Response): Promise<T> {
  const text = await response.text()
  if (!text) {
    return {} as T
  }
  return JSON.parse(text) as T
}

function responseError(payload: { error?: string }, fallback: string) {
  return new Error(payload.error || fallback)
}

export async function loadCurrentUser() {
  authState.loading = true
  try {
    const response = await fetch('/api/v1/auth/me', {
      headers: { Accept: 'application/json' },
      credentials: 'same-origin',
    })
    const payload = await readJSON<AuthUserResponse>(response)
    if (!response.ok || !payload.ok || !payload.user) {
      authState.user = null
      return null
    }
    authState.user = payload.user
    return payload.user
  } finally {
    authState.bootstrapped = true
    authState.loading = false
  }
}

export async function loginWithPassword(login: string, password: string) {
  const response = await fetch('/api/v1/auth/login', {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ login, password }),
  })
  const payload = await readJSON<AuthUserResponse>(response)
  if (!response.ok || !payload.ok) {
    throw responseError(payload, 'Check your credentials and try again.')
  }
  return loadCurrentUser()
}

export async function logout() {
  const response = await fetch('/api/v1/auth/logout', {
    method: 'POST',
    headers: { Accept: 'application/json' },
    credentials: 'same-origin',
  })
  const payload = await readJSON<OKResponse>(response)
  authState.user = null
  authState.bootstrapped = true
  if (!response.ok || !payload.ok) {
    throw responseError(payload, 'Unable to log out.')
  }
}
