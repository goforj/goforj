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

type MessageResponse = {
  ok: boolean
  error?: string
  message?: string
}

type PasswordResetRequestResponse = {
  ok: boolean
  error?: string
  reset_token?: string
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

export async function loginWithPassword(login: string, password: string, remember = false) {
  const response = await fetch('/api/v1/auth/login', {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ login, password, remember }),
  })
  const payload = await readJSON<AuthUserResponse>(response)
  if (!response.ok || !payload.ok) {
    throw responseError(payload, 'Check your credentials and try again.')
  }
  return loadCurrentUser()
}

export async function registerWithPassword(displayName: string, email: string, password: string, remember = false) {
  const response = await fetch('/api/v1/auth/register', {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ display_name: displayName, email, password, remember }),
  })
  const payload = await readJSON<AuthUserResponse>(response)
  if (!response.ok || !payload.ok) {
    throw responseError(payload, 'Unable to create your account.')
  }
  return loadCurrentUser()
}

export async function requestPasswordReset(login: string) {
  const response = await fetch('/api/v1/auth/password-reset/request', {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ login }),
  })
  const payload = await readJSON<PasswordResetRequestResponse>(response)
  if (!response.ok || !payload.ok) {
    throw responseError(payload, 'Unable to send password reset instructions.')
  }
  return payload
}

export async function resetPassword(token: string, newPassword: string) {
  const response = await fetch('/api/v1/auth/password-reset/confirm', {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ token, new_password: newPassword }),
  })
  const payload = await readJSON<OKResponse>(response)
  if (!response.ok || !payload.ok) {
    throw responseError(payload, 'Unable to reset your password.')
  }
  authState.user = null
  authState.bootstrapped = true
  return payload
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

export async function changePassword(currentPassword: string, newPassword: string) {
  const response = await fetch('/api/v1/auth/change-password', {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({
      current_password: currentPassword,
      new_password: newPassword,
    }),
  })
  const payload = await readJSON<MessageResponse>(response)
  if (!response.ok || !payload.ok) {
    throw responseError(payload, 'Unable to update your password.')
  }
  return payload
}

export async function updateProfile(displayName: string, email: string) {
  const response = await fetch('/api/v1/auth/profile', {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({
      display_name: displayName,
      email,
    }),
  })
  const payload = await readJSON<AuthUserResponse>(response)
  if (!response.ok || !payload.ok || !payload.user) {
    throw responseError(payload, 'Unable to update your profile.')
  }
  authState.user = payload.user
  authState.bootstrapped = true
  return payload.user
}
