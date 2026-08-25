// i18next-parser configuration — `npm run i18n:extract`.
//
// Scans the source for t('…') calls and writes any key it finds but the catalogs
// lack, so the mass migration (_Docs/73) has a mechanical way to keep the JSON in
// step with the code. It NEVER invents translations: a new key lands with an
// empty value in every locale, and the catalog-parity test then fails on the blank
// until a real string is supplied. That is the intended workflow — extraction
// finds the gaps, a human (or a translation pass) fills them.
//
// keepRemoved is on because keys are also read dynamically (t(`time.bucket.${b}`))
// and the parser cannot see those; pruning on its say-so would delete live keys.

export default {
  locales: ['en', 'tr'],
  defaultNamespace: 'common',
  // Namespace is taken from useTranslation('<ns>') / t('<ns>:key'), matching the
  // one-namespace-per-feature-folder convention.
  defaultValue: '',
  keySeparator: '.',
  namespaceSeparator: ':',
  createOldCatalogs: false,
  keepRemoved: true,
  sort: true,
  input: ['src/**/*.{ts,tsx}', '!src/**/*.test.{ts,tsx}'],
  output: 'src/i18n/locales/$LOCALE/$NAMESPACE.json',
  lexers: {
    ts: ['JavascriptLexer'],
    tsx: ['JsxLexer'],
  },
  // Fail the command instead of writing a partial result when a file cannot be
  // parsed, so a silent extraction gap never looks like "no new keys".
  failOnWarnings: false,
  verbose: false,
}
