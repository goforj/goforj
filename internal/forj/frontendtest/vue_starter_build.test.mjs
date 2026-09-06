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
const { build } = await import(pathToFileURL(viteEntry).href)

test('the production graph retains the complete component showroom', async () => {
  let moduleIDs = []

  await build({
    root: frontendRoot,
    configFile: path.join(frontendRoot, 'vite.config.ts'),
    logLevel: 'silent',
    plugins: [{
      name: 'goforj:test-production-module-graph',
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
  for (const view of ['Overview', 'Forms', 'Navigation', 'Overlays', 'Data']) {
    assert.ok(normalized.some((id) => id.endsWith(`/src/views/components/Components${view}View.vue`)), `Components${view}View.vue was removed from the production graph`)
  }
  assert.ok(moduleIDs.length < 3400, `expected fewer than 3400 total modules, received ${moduleIDs.length}`)
})
