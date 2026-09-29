import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import ts from 'typescript'
import { expect, it } from 'vitest'
import { buildResources } from './catalog'
import { LOCALE_CODES } from './locales'

// Catalog parity alone cannot detect a key missing from BOTH languages. Check
// statically identifiable translation calls against the bundled resources too.
// Dynamic keys remain covered by feature render tests and catalog parity.
const sourceRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const resources = buildResources()

function filesIn(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) return filesIn(path)
    return /\.(ts|tsx)$/.test(entry.name) && !/\.(test|spec)\./.test(entry.name) ? [path] : []
  })
}

function literal(node: ts.Node | undefined): string | undefined {
  return node && (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node))
    ? node.text
    : undefined
}

function namespace(node: ts.Node | undefined): string | undefined {
  if (!node) return 'common'
  if (ts.isArrayLiteralExpression(node)) return literal(node.elements[0])
  return literal(node)
}

function hasKey(locale: string, ns: string, key: string): boolean {
  const catalog = resources[locale]?.[ns]
  const read = (path: string) =>
    path
      .split('.')
      .reduce<unknown>(
        (value, segment) =>
          value && typeof value === 'object'
            ? (value as Record<string, unknown>)[segment]
            : undefined,
        catalog,
      )
  return (
    typeof read(key) === 'string' ||
    new Intl.PluralRules(locale)
      .resolvedOptions()
      .pluralCategories.every((category) => typeof read(`${key}_${category}`) === 'string')
  )
}

it('resolves static translation keys in every supported locale', () => {
  const missing: string[] = []
  for (const path of filesIn(sourceRoot)) {
    const source = ts.createSourceFile(
      path,
      readFileSync(path, 'utf8'),
      ts.ScriptTarget.Latest,
      true,
    )
    const bindings = new Map<string, Set<string>>()
    const remember = (name: string, ns: string | undefined) => {
      if (ns) bindings.set(name, new Set([...(bindings.get(name) ?? []), ns]))
    }
    const scanBindings = (node: ts.Node) => {
      if (
        ts.isVariableDeclaration(node) &&
        node.initializer &&
        ts.isCallExpression(node.initializer)
      ) {
        const call = node.initializer
        if (
          ts.isIdentifier(call.expression) &&
          call.expression.text === 'useTranslation' &&
          ts.isObjectBindingPattern(node.name)
        ) {
          for (const binding of node.name.elements) {
            if ((binding.propertyName?.getText(source) ?? binding.name.getText(source)) === 't') {
              remember(binding.name.getText(source), namespace(call.arguments[0]))
            }
          }
        } else if (
          ts.isPropertyAccessExpression(call.expression) &&
          call.expression.name.text === 'getFixedT'
        ) {
          remember(node.name.getText(source), namespace(call.arguments[1]))
        }
      }
      ts.forEachChild(node, scanBindings)
    }
    scanBindings(source)
    const checkKey = (node: ts.Node, key: string, ns: string) => {
      const separator = key.indexOf(':')
      const keyNamespace = separator >= 0 ? key.slice(0, separator) : ns
      const leaf = separator >= 0 ? key.slice(separator + 1) : key
      for (const locale of LOCALE_CODES) {
        if (!hasKey(locale, keyNamespace, leaf)) {
          const line = source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1
          missing.push(`${relative(sourceRoot, path)}:${line} — ${locale}/${keyNamespace}:${leaf}`)
        }
      }
    }
    const scanCalls = (node: ts.Node) => {
      if (
        (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) &&
        node.tagName.getText(source) === 'Trans'
      ) {
        const attribute = (name: string) => {
          const attr = node.attributes.properties.find(
            (item) => ts.isJsxAttribute(item) && item.name.getText(source) === name,
          )
          if (!attr || !ts.isJsxAttribute(attr)) return undefined
          return attr.initializer && ts.isJsxExpression(attr.initializer)
            ? attr.initializer.expression
            : attr.initializer
        }
        const key = literal(attribute('i18nKey'))
        const ns = literal(attribute('ns'))
        // Only explicit namespaces are unambiguous for Trans; a supplied t prop
        // can otherwise override the default namespace at runtime.
        if (key && ns) checkKey(node, key, ns)
      }
      if (ts.isCallExpression(node)) {
        const key = literal(node.arguments[0])
        let ns: string | undefined
        if (ts.isIdentifier(node.expression)) {
          const candidates = bindings.get(node.expression.text)
          if (candidates?.size === 1) ns = [...candidates][0]
        } else if (
          ts.isPropertyAccessExpression(node.expression) &&
          node.expression.name.text === 't' &&
          node.expression.expression.getText(source) === 'i18next'
        ) {
          ns = 'common'
        }
        if (key && ns) {
          const options = node.arguments[1]
          if (options && ts.isObjectLiteralExpression(options)) {
            const property = options.properties.find(
              (item) => ts.isPropertyAssignment(item) && item.name.getText(source) === 'ns',
            )
            if (property && ts.isPropertyAssignment(property)) ns = namespace(property.initializer)
          }
          if (ns) checkKey(node, key, ns)
        }
      }
      ts.forEachChild(node, scanCalls)
    }
    scanCalls(source)
  }
  expect(missing, 'UI translation calls without catalog entries').toEqual([])
})
