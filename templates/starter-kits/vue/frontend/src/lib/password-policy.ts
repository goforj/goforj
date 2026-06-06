function parseBoolean(value: string | undefined, fallback: boolean) {
  if (value == null || value === '') {
    return fallback
  }
  switch (value.toLowerCase()) {
    case '1':
    case 'true':
    case 'yes':
    case 'on':
      return true
    case '0':
    case 'false':
    case 'no':
    case 'off':
      return false
    default:
      return fallback
  }
}

function parseNumber(value: string | undefined, fallback: number) {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : fallback
}

export const passwordPolicy = {
  minLength: parseNumber(import.meta.env.VITE_AUTH_PASSWORD_MIN_LENGTH, 8),
  requireUpper: parseBoolean(import.meta.env.VITE_AUTH_PASSWORD_REQUIRE_UPPER, true),
  requireLower: parseBoolean(import.meta.env.VITE_AUTH_PASSWORD_REQUIRE_LOWER, false),
  requireNumber: parseBoolean(import.meta.env.VITE_AUTH_PASSWORD_REQUIRE_NUMBER, false),
  requireSymbol: parseBoolean(import.meta.env.VITE_AUTH_PASSWORD_REQUIRE_SYMBOL, true),
}

export function passwordRequirements() {
  const requirements: string[] = []
  if (passwordPolicy.minLength > 0) {
    requirements.push(`at least ${passwordPolicy.minLength} characters`)
  }
  if (passwordPolicy.requireUpper) {
    requirements.push('an uppercase letter')
  }
  if (passwordPolicy.requireLower) {
    requirements.push('a lowercase letter')
  }
  if (passwordPolicy.requireNumber) {
    requirements.push('a number')
  }
  if (passwordPolicy.requireSymbol) {
    requirements.push('a symbol')
  }
  return requirements
}

export function passwordRequirementsText() {
  const requirements = passwordRequirements()
  if (requirements.length === 0) {
    return 'Choose a strong password.'
  }
  if (requirements.length === 1) {
    return `Password must include ${requirements[0]}.`
  }
  const head = requirements.slice(0, -1).join(', ')
  const tail = requirements[requirements.length - 1]
  return `Password must include ${head}, and ${tail}.`
}
