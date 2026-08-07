import fs from "node:fs"
import path from "node:path"

// FrontendEnvOptions carries Vite's merged environment together with the paths needed to identify one App in a multi-App project.
type FrontendEnvOptions = {
  env: Record<string, string>
  frontendDir: string
  projectRoot: string
}

// FrontendEnv contains the browser-safe defines and backend proxy target consumed by the starter's Vite configuration.
type FrontendEnv = {
  appName: string
  backendTarget: string
  define: Record<string, string>
}

// defaultApp preserves the conventional primary App identity when discovery cannot inspect the project tree.
const defaultApp = "app"

// resolveGoForjFrontendEnv applies GoForj's shared and per-App environment precedence without exposing unrelated backend variables to the browser.
export function resolveGoForjFrontendEnv(options: FrontendEnvOptions): FrontendEnv {
  const appName = resolveApp(options.frontendDir)
  const appPrefix = envPrefix(appName)
  const define = collectFrontendDefines(options.env, appPrefix)

  defineMissing(define, "VITE_APP_ENV", options.env[`${appPrefix}_APP_ENV`] || options.env.APP_ENV || "local")

  return {
    appName,
    define,
    backendTarget: resolveBackendTarget(options.env, options.projectRoot, appName, appPrefix),
  }
}

// resolveApp derives the App name from cmd/<app>/frontend so additional Apps require no duplicated Vite configuration.
function resolveApp(frontendDir: string): string {
  return path.basename(path.dirname(frontendDir)) || defaultApp
}

// collectFrontendDefines exposes only explicitly frontend-scoped values; App-scoped values are processed last so they override project defaults.
function collectFrontendDefines(env: Record<string, string>, appPrefix: string): Record<string, string> {
  const define: Record<string, string> = {}
  const prefixes = ["FRONTEND_", `${appPrefix}_FRONTEND_`]

  for (const prefix of prefixes) {
    for (const [name, value] of Object.entries(env)) {
      if (!name.startsWith(prefix)) {
        continue
      }
      const key = name.slice(prefix.length)
      if (!/^[A-Z][A-Z0-9_]*$/.test(key)) {
        continue
      }
      define[`import.meta.env.VITE_${key}`] = JSON.stringify(value)
    }
  }

  return define
}

// resolveBackendTarget favors explicit frontend and App URLs before falling back to GoForj's deterministic development port convention.
function resolveBackendTarget(
  env: Record<string, string>,
  projectRoot: string,
  appName: string,
  appPrefix: string,
): string {
  return (
    frontendEnvValue(env, appPrefix, "BACKEND_URL") ||
    env[`${appPrefix}_APP_URL`] ||
    env.APP_URL ||
    `http://localhost:${targetHTTPPort(projectRoot, appName)}`
  )
}

// frontendEnvValue preserves compatibility with direct Vite overrides while preferring GoForj's App-specific and shared frontend namespaces.
function frontendEnvValue(env: Record<string, string>, appPrefix: string, key: string): string {
  return env[`${appPrefix}_FRONTEND_${key}`] || env[`FRONTEND_${key}`] || env[`VITE_${key}`] || ""
}

// targetHTTPPort mirrors GoForj's stable App ordering so an unset proxy target still reaches the matching backend.
function targetHTTPPort(projectRoot: string, appName: string): number {
  const apps = discoverApps(projectRoot)
  const index = apps.indexOf(appName)
  return 3000 + Math.max(index, 0)
}

// discoverApps keeps the default App first and named Apps sorted to make fallback ports stable across filesystems.
function discoverApps(projectRoot: string): string[] {
  const cmdDir = path.join(projectRoot, "cmd")
  let entries: fs.Dirent[]
  try {
    entries = fs.readdirSync(cmdDir, { withFileTypes: true })
  } catch {
    return [defaultApp]
  }

  const apps = entries
    .filter((entry) => entry.isDirectory())
    .map((entry) => entry.name)
    .filter((name) => fs.existsSync(path.join(cmdDir, name, "main.go")))

  const named = Array.from(new Set(apps.filter((name) => name !== defaultApp))).sort()
  return [defaultApp, ...named]
}

// defineMissing lets explicit frontend values win when supplying framework defaults such as VITE_APP_ENV.
function defineMissing(define: Record<string, string>, key: string, value: string) {
  const envKey = `import.meta.env.${key}`
  if (!(envKey in define)) {
    define[envKey] = JSON.stringify(value)
  }
}

// envPrefix converts an App name to the same environment prefix used by GoForj's backend configuration.
function envPrefix(value: string): string {
  return value.replace(/[^a-zA-Z0-9]+/g, "_").replace(/^_+|_+$/g, "").toUpperCase() || defaultApp.toUpperCase()
}
