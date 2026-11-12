import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import ts from 'typescript'

const source = await readFile(new URL('../src/eats/model.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText
const { activityHistory, orderedHistory, traceURL } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

test('broker reordering and duplicates preserve the latest business milestone', () => {
  const history = [{ step: 64, stage: 'delivered' }, { step: 25, stage: 'retrying' }, { step: 10, stage: 'preparing' }, { step: 25, stage: 'retrying' }]
  const result = orderedHistory(history)
  assert.deepEqual(result.map(update => update.step), [10, 25, 64])
  assert.equal(result.at(-1).stage, 'delivered')
  assert.equal(history.length, 4)
})

test('Grafana link selects the provisioned Tempo datasource and exact order trace', () => {
  const id = '0123456789abcdef0123456789abcdef'
  const url = new URL(traceURL('http://localhost:13001/', id))
  assert.equal(url.pathname, '/explore')
  const pane = JSON.parse(url.searchParams.get('panes')).order
  assert.equal(pane.datasource, 'goforj-tempo')
  assert.equal(pane.queries[0].query, id)
  assert.equal(pane.queries[0].queryType, 'traceql')
})

// Compile the actual Vue scene with the starter's own compiler and SSR runtime.
const { parse, compileScript } = await import('@vue/compiler-sfc')
const { createSSRApp } = await import('vue')
const { renderToString } = await import('vue/server-renderer')
const sceneSource = await readFile(new URL('../src/eats/KitchenScene.vue', import.meta.url), 'utf8')
const { descriptor } = parse(sceneSource)
const script = compileScript(descriptor, { id: 'eats-kitchen-test', inlineTemplate: true, templateOptions: { ssr: true } })
const sceneJS = ts.transpileModule(script.content, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText
const resolvedSceneJS = sceneJS.replace(/from (["'])(vue(?:\/server-renderer)?)\1/g, (_match, _quote, name) => `from ${JSON.stringify(import.meta.resolve(name))}`)
const { default: KitchenScene } = await import(`data:text/javascript;base64,${Buffer.from(resolvedSceneJS).toString('base64')}`)

for (const [stage, visual, label] of [
  ['received', 'order-ticket', 'printed pizza order'],
  ['preparing', 'dough', 'tomato and basil toppings'],
  ['baking', 'oven-flames', 'glowing stone oven'],
  ['ready', 'pickup-box', 'boxed on the pickup counter'],
  ['retrying', 'oven-steam', 'burned pizza'],
]) {
  test(`${stage} renders its own accessible kitchen scene from the service stage`, async () => {
    const html = await renderToString(createSSRApp(KitchenScene, { stage }))
    assert.ok(html.includes(`class="${visual}"`), `missing ${stage} visual`)
    assert.ok(html.includes(label), `missing ${stage} accessible description`)
    assert.ok(html.includes('role="img"'))
    if (stage !== 'received') assert.ok(!html.includes('class="order-ticket"'))
    if (stage !== 'preparing') assert.ok(!html.includes('class="dough"'))
    if (stage !== 'ready') assert.ok(!html.includes('class="pickup-box"'))
  })
}


test('activity groups courier updates while keeping a retried cooking sequence visible', () => {
  const history = [
    { step: 10, stage: 'preparing', service: 'kitchen' },
    { step: 20, stage: 'baking', service: 'kitchen' },
    { step: 25, stage: 'retrying', service: 'kitchen' },
    { step: 30, stage: 'preparing', service: 'kitchen' },
    { step: 40, stage: 'baking', service: 'kitchen' },
    { step: 50, stage: 'ready', service: 'kitchen' },
    ...Array.from({ length: 20 }, (_, index) => ({ step: 60 + index, stage: 'delivering', service: 'delivery' })),
    { step: 80, stage: 'delivered', service: 'delivery' },
  ]
  assert.deepEqual(activityHistory(history).map(update => update.step), [10, 20, 25, 30, 40, 50, 60, 80])
  assert.equal(history.length, 27)
  assert.deepEqual(activityHistory([]), [])
})
