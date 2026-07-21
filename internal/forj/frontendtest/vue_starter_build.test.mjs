import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath, pathToFileURL } from 'node:url'

const testDir = path.dirname(fileURLToPath(import.meta.url))
const repositoryRoot = path.resolve(testDir, '../../..')
const frontendRoot = path.join(repositoryRoot, 'templates/starter-kits/vue/frontend')
const requireFromFrontend = createRequire(path.join(frontendRoot, 'package.json'))
const viteEntry = requireFromFrontend.resolve('vite')
const { build, createLogger } = await import(pathToFileURL(viteEntry).href)
const { lucideIconImports } = await import(pathToFileURL(path.join(frontendRoot, 'goforj-lucide-imports.mjs')).href)

test('the production graph resolves only used Lucide icons and retains the component showroom', async () => {
  let moduleIDs = []

  await build({
    root: frontendRoot,
    configFile: path.join(frontendRoot, 'vite.config.ts'),
    logLevel: 'silent',
    plugins: [{
      name: 'goforj:test-module-graph',
      generateBundle() {
        moduleIDs = [...this.getModuleIds()]
      },
    }],
    build: {
      reportCompressedSize: false,
      write: false,
    },
  })

  const normalized = moduleIDs.map((id) => id.replaceAll('\\', '/'))
  const lucideModules = normalized.filter((id) => id.includes('/node_modules/@lucide/vue/'))
  assert.ok(!lucideModules.some((id) => id.endsWith('/dist/esm/lucide-vue.mjs')), 'the Lucide package barrel entered the production graph')
  assert.ok(lucideModules.some((id) => id.endsWith('/dist/esm/icons/loader-circle.mjs')), 'the Loader2Icon alias was not resolved')
  assert.ok(lucideModules.length < 100, `expected fewer than 100 Lucide modules, received ${lucideModules.length}`)
  for (const view of ['Overview', 'Forms', 'Navigation', 'Overlays', 'Data']) {
    assert.ok(normalized.some((id) => id.endsWith(`/src/views/components/Components${view}View.vue`)), `Components${view}View.vue was removed from the production graph`)
  }
  assert.ok(moduleIDs.length < 1500, `expected fewer than 1500 total modules, received ${moduleIDs.length}`)
})

test('local Lucide aliases resolve to their direct icon module', async () => {
  let moduleIDs = []

  await build({
    root: frontendRoot,
    configFile: false,
    logLevel: 'silent',
    plugins: [
      virtualEntry(`import { Calendar as CalendarIcon } from '@lucide/vue'; console.log(CalendarIcon);`),
      lucideIconImports(),
      {
        name: 'goforj:test-lucide-alias-graph',
        generateBundle() {
          moduleIDs = [...this.getModuleIds()]
        },
      },
    ],
    build: {
      minify: false,
      reportCompressedSize: false,
      write: false,
      rollupOptions: {
        input: 'virtual:goforj-lucide-test',
      },
    },
  })

  const normalized = moduleIDs.map((id) => id.replaceAll('\\', '/'))
  assert.ok(normalized.some((id) => id.endsWith('/dist/esm/icons/calendar.mjs')), 'the locally aliased Calendar icon was not resolved')
  assert.ok(!normalized.some((id) => id.endsWith('/dist/esm/lucide-vue.mjs')), 'the Lucide package barrel entered the aliased import graph')
})

test('catalog imports retain Lucide compatibility with an actionable warning', async () => {
  const warnings = await buildFallbackFixture(`import { icons } from '@lucide/vue'; console.log(icons.Check);`)

  assert.ok(warnings.some((warning) => warning.includes('export "icons" is not an individual icon')), `expected an optimizer fallback warning, received ${JSON.stringify(warnings)}`)
})

test('namespace imports retain Lucide compatibility with an actionable warning', async () => {
  const warnings = await buildFallbackFixture(`import * as Lucide from '@lucide/vue'; console.log(Lucide.Check);`)

  assert.ok(warnings.some((warning) => warning.includes('namespace')), `expected an optimizer fallback warning, received ${JSON.stringify(warnings)}`)
})

async function buildFallbackFixture(source) {
  const warnings = []
  const logger = createLogger('silent')
  logger.warn = (message) => warnings.push(String(message))

  await build({
    root: frontendRoot,
    configFile: false,
    customLogger: logger,
    plugins: [
      {
        ...virtualEntry(source),
        name: 'goforj:test-lucide-fallback-entry',
      },
      lucideIconImports(),
    ],
    build: {
      minify: false,
      reportCompressedSize: false,
      write: false,
      rollupOptions: {
        input: 'virtual:goforj-lucide-test',
      },
    },
  })
  return warnings
}

function virtualEntry(source) {
  return {
    name: 'goforj:test-lucide-entry',
    resolveId(id) {
      return id === 'virtual:goforj-lucide-test' ? `\0${id}` : null
    },
    load(id) {
      return id === '\0virtual:goforj-lucide-test' ? source : null
    },
  }
}
