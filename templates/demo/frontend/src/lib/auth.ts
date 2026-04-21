import { computed, ref } from 'vue'

export type AuthUser = {
  id: number
  username: string
  email: string
  display_name?: string
}

const currentUser = ref<AuthUser | null>(null)
const loaded = ref(false)
let meRequest: Promise<AuthUser | null> | null = null

function loginRedirectURL() {
  const current = `${window.location.pathname}${window.location.search}${window.location.hash}`
  return `/login?redirect=${encodeURIComponent(current || '/monitors')}`
}

function redirectToLogin() {
  if (window.location.pathname === '/login') return
  window.location.assign(loginRedirectURL())
}

async function loadCurrentUser(force = false): Promise<AuthUser | null> {
  if (!force && loaded.value && currentUser.value) {
    return currentUser.value
  }
  if (!force && meRequest) {
    return meRequest
  }
  meRequest = (async () => {
    try {
      const res = await fetch('/api/v1/auth/me', { cache: 'no-store' })
      if (!res.ok) {
        currentUser.value = null
        loaded.value = true
        return null
      }
      const data = await res.json().catch(() => ({}))
      currentUser.value = (data?.user ?? null) as AuthUser | null
      loaded.value = true
      return currentUser.value
    } finally {
      meRequest = null
    }
  })()
  return meRequest
}

export async function ensureAuthenticated() {
  return Boolean(await loadCurrentUser())
}

export async function signIn(login: string, password: string) {
  const res = await fetch('/api/v1/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ login, password }),
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    throw new Error(typeof data?.error === 'string' ? data.error : 'Sign in failed')
  }
  currentUser.value = (data?.user ?? null) as AuthUser | null
  loaded.value = true
  return currentUser.value
}

export async function signOut() {
  try {
    await fetch('/api/v1/auth/logout', { method: 'POST' })
  } finally {
    currentUser.value = null
    loaded.value = true
  }
}

export async function apiFetch(input: RequestInfo | URL, init?: RequestInit) {
  const res = await fetch(input, init)
  if (res.status === 401) {
    currentUser.value = null
    loaded.value = true
    redirectToLogin()
  }
  return res
}

export function useAuthState() {
  return {
    currentUser,
    isAuthenticated: computed(() => Boolean(currentUser.value)),
    loaded: computed(() => loaded.value),
  }
}
