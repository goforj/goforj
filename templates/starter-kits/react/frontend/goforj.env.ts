import fs from "node:fs"
import path from "node:path"

type FrontendEnvOptions = {
  env: Record<string, string>
  frontendDir: string
  projectRoot: string
}

type FrontendEnv = {
  appName: string
  backendTarget: string
  define: Record<string, string>
}

const defaultApp = "app"

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

function resolveApp(frontendDir: string): string {
  return path.basename(path.dirname(frontendDir)) || defaultApp
}

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

function frontendEnvValue(env: Record<string, string>, appPrefix: string, key: string): string {
  return env[`${appPrefix}_FRONTEND_${key}`] || env[`FRONTEND_${key}`] || env[`VITE_${key}`] || ""
}

function targetHTTPPort(projectRoot: string, appName: string): number {
  const apps = discoverApps(projectRoot)
  const index = apps.indexOf(appName)
  return 3000 + Math.max(index, 0)
}

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

function defineMissing(define: Record<string, string>, key: string, value: string) {
  const envKey = `import.meta.env.${key}`
  if (!(envKey in define)) {
    define[envKey] = JSON.stringify(value)
  }
}

function envPrefix(value: string): string {
  return value.replace(/[^a-zA-Z0-9]+/g, "_").replace(/^_+|_+$/g, "").toUpperCase() || defaultApp.toUpperCase()
}
