export type AuthUser = {
  id: string
  username: string
  email: string
  display_name?: string
  avatar_url?: string
  email_verified_at?: string | null
  last_login_at?: string | null
  last_seen_at?: string | null
  timezone?: string
  locale?: string
}

type AuthUserResponse = {
  ok?: boolean
  user?: AuthUser
  error?: string
  requires_email_verification?: boolean
  verification_token?: string
}

type OKResponse = {
  ok?: boolean
  error?: string
  message?: string
}

type PasswordResetRequestResponse = OKResponse & {
  reset_token?: string
}

export type RegisterResult = {
  user?: AuthUser
  requires_email_verification: boolean
  verification_token?: string
}

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    credentials: "include",
    headers: {
      "content-type": "application/json",
      ...(init?.headers || {}),
    },
    ...init,
  })
  const payload = await response.json().catch(() => ({}))
  if (!response.ok || payload.ok === false) {
    throw new Error(typeof payload.error === "string" ? payload.error : "Request failed")
  }
  return payload as T
}

export async function currentUser(): Promise<AuthUser | null> {
  try {
    const payload = await api<AuthUserResponse>("/auth/me")
    return payload.user ?? null
  } catch {
    return null
  }
}

export async function login(login: string, password: string, remember: boolean): Promise<AuthUser> {
  const payload = await api<AuthUserResponse>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ login, password, remember }),
  })
  if (!payload.user) {
    throw new Error("Unable to sign in")
  }
  return payload.user
}

export async function register(displayName: string, email: string, password: string, remember: boolean): Promise<RegisterResult> {
  const payload = await api<AuthUserResponse>("/auth/register", {
    method: "POST",
    body: JSON.stringify({ display_name: displayName, email, password, remember }),
  })
  return {
    user: payload.user,
    requires_email_verification: Boolean(payload.requires_email_verification),
    verification_token: payload.verification_token,
  }
}

export async function requestPasswordReset(login: string): Promise<PasswordResetRequestResponse> {
  return api<PasswordResetRequestResponse>("/auth/password-reset/request", {
    method: "POST",
    body: JSON.stringify({ login }),
  })
}

export async function resetPassword(token: string, newPassword: string): Promise<void> {
  await api<OKResponse>("/auth/password-reset/confirm", {
    method: "POST",
    body: JSON.stringify({ token, new_password: newPassword }),
  })
}

export async function verifyEmail(token: string): Promise<AuthUser> {
  const payload = await api<AuthUserResponse>("/auth/email-verification/confirm", {
    method: "POST",
    body: JSON.stringify({ token }),
  })
  if (!payload.user) {
    throw new Error("Unable to verify email")
  }
  return payload.user
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  await api<OKResponse>("/auth/change-password", {
    method: "POST",
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  })
}

export async function updateProfile(displayName: string, email: string): Promise<AuthUser> {
  const payload = await api<AuthUserResponse>("/auth/profile", {
    method: "POST",
    body: JSON.stringify({ display_name: displayName, email }),
  })
  if (!payload.user) {
    throw new Error("Unable to update profile")
  }
  return payload.user
}

export async function logout(): Promise<void> {
  await api("/auth/logout", { method: "POST" })
}
