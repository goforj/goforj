import { readFile } from 'node:fs/promises'
import path from 'node:path'
import MagicString from 'magic-string'

const lucidePackage = '@lucide/vue'
const lucideVirtualPrefix = 'virtual:goforj-lucide-icon/'

/**
 * lucideIconImports keeps public Lucide imports in source while resolving only the icons a build uses.
 *
 * @returns {import('vite').Plugin}
 */
export function lucideIconImports() {
  /** @type {string | undefined} */
  let lucideEntry
  /** @type {Promise<Map<string, string>> | undefined} */
  let lucideExports
  /** @type {Map<string, string>} */
  const virtualTargets = new Map()
  /** @type {Set<string>} */
  const emittedWarnings = new Set()

  return {
    name: 'goforj:lucide-icon-imports',
    apply: 'build',
    enforce: 'post',

    resolveId(id) {
      return virtualTargets.get(id) ?? null
    },

    async transform(code, id) {
      if (!code.includes(lucidePackage) || normalizeID(id).includes('/node_modules/')) {
        return null
      }

      const program = this.parse(code)
      const imports = program.body.filter((node) => (
        node.type === 'ImportDeclaration' && node.source.value === lucidePackage
      ))
      if (imports.length === 0) {
        return null
      }

      let iconExports
      try {
        if (!lucideExports) {
          lucideExports = loadLucideExports(this, id).then((loaded) => {
            lucideEntry = loaded.entry
            this.addWatchFile(loaded.entry)
            return loaded.exports
          })
        }
        iconExports = await lucideExports
      } catch (error) {
        warnOnce(this, emittedWarnings, `Unable to optimize ${lucidePackage}; using its package entry instead. ${errorMessage(error)}`)
        return null
      }

      const output = new MagicString(code)
      let changed = false

      for (const declaration of imports) {
        if (declaration.specifiers.length === 0 || declaration.specifiers.some((specifier) => specifier.type !== 'ImportSpecifier')) {
          warnOnce(this, emittedWarnings, `${lucidePackage} default, namespace, and side-effect imports cannot be reduced to individual icons.`)
          continue
        }

        const requested = declaration.specifiers.map((specifier) => ({
          imported: moduleExportName(specifier.imported),
          local: specifier.local.name,
        }))
        const unresolved = requested.find(({ imported }) => !iconExports.has(imported))
        if (unresolved) {
          warnOnce(this, emittedWarnings, `${lucidePackage} export ${JSON.stringify(unresolved.imported)} is not an individual icon; this import will use the package entry.`)
          continue
        }

        const replacements = []
        for (const specifier of requested) {
          const target = iconExports.get(specifier.imported)
          const virtualID = `${lucideVirtualPrefix}${encodeURIComponent(specifier.imported)}`
          virtualTargets.set(virtualID, target)
          replacements.push(`import ${specifier.local} from ${JSON.stringify(virtualID)};`)
        }

        output.overwrite(declaration.start, declaration.end, replacements.join('\n'))
        changed = true
      }

      if (!changed) {
        return null
      }
      return {
        code: output.toString(),
        map: output.generateMap({ hires: true, includeContent: true, source: id }),
      }
    },

    watchChange(id) {
      if (lucideEntry && normalizeID(id) === normalizeID(lucideEntry)) {
        lucideEntry = undefined
        lucideExports = undefined
        virtualTargets.clear()
      }
    },
  }
}

/**
 * loadLucideExports uses Lucide's own entry manifest because public aliases do not consistently map to filenames.
 *
 * @param {{resolve: Function, parse: Function}} context
 * @param {string} importer
 * @returns {Promise<{entry: string, exports: Map<string, string>}>}
 */
async function loadLucideExports(context, importer) {
  const resolution = await context.resolve(lucidePackage, importer, { skipSelf: true })
  if (!resolution) {
    throw new Error(`Vite could not resolve ${lucidePackage}`)
  }

  const entry = resolution.id
  const source = await readFile(entry, 'utf8')
  const program = context.parse(source)
  /** @type {Map<string, string>} */
  const exports = new Map()

  for (const declaration of program.body) {
    if (declaration.type !== 'ExportNamedDeclaration' || typeof declaration.source?.value !== 'string') {
      continue
    }
    for (const specifier of declaration.specifiers) {
      if (specifier.type !== 'ExportSpecifier' || moduleExportName(specifier.local) !== 'default') {
        continue
      }
      if (!declaration.source.value.startsWith('.')) {
        continue
      }
      exports.set(moduleExportName(specifier.exported), normalizeID(path.resolve(path.dirname(entry), declaration.source.value)))
    }
  }

  if (exports.size === 0) {
    throw new Error(`${lucidePackage} no longer exposes direct icon modules from ${entry}`)
  }
  return { entry, exports }
}

/**
 * moduleExportName normalizes ESTree's identifier and string-literal export names.
 *
 * @param {{type: string, name?: string, value?: unknown}} node
 * @returns {string}
 */
function moduleExportName(node) {
  if (node.type === 'Identifier' && typeof node.name === 'string') {
    return node.name
  }
  if (typeof node.value === 'string') {
    return node.value
  }
  return ''
}

/**
 * normalizeID makes dependency-path checks consistent across Vite's supported platforms.
 *
 * @param {string} id
 * @returns {string}
 */
function normalizeID(id) {
  return id.replaceAll('\\', '/')
}

/**
 * warnOnce keeps an unsupported import actionable without flooding builds that share it.
 *
 * @param {{warn: Function}} context
 * @param {Set<string>} emitted
 * @param {string} message
 */
function warnOnce(context, emitted, message) {
  if (emitted.has(message)) {
    return
  }
  emitted.add(message)
  context.warn(message)
}

/**
 * errorMessage makes unknown thrown values useful in Vite diagnostics.
 *
 * @param {unknown} error
 * @returns {string}
 */
function errorMessage(error) {
  return error instanceof Error ? error.message : String(error)
}
