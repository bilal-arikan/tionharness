import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import i18next from 'eslint-plugin-i18next'
import { defineConfig, globalIgnores } from 'eslint/config'

// Directories whose user-visible strings have already been moved into the i18n
// catalogs (see _Docs/73). Inside these, a literal string in JSX is an error, so
// migrated screens cannot silently regress. The list GROWS as the migration
// proceeds — adding a path here is the last step of migrating that feature.
//
// It is an allowlist rather than a global rule because the other ~450 files still
// hold ~2.2k Turkish literals; turning the rule on everywhere at once would bury
// real violations under known debt.
const I18N_MIGRATED = ['src/i18n/**/*.{ts,tsx}', 'src/shared/lib/{time,format,intl}.ts']

export default defineConfig([
  globalIgnores(['dist']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      globals: globals.browser,
    },
    rules: {
      // Downgraded 2026-08-24 after auditing all 61 hits. They are deliberate
      // cross-entity resets and async-fetch lifecycles — clear the session lists
      // when the workspace changes, close the inline flow view when the active
      // session changes, re-arm the auth gate on entering the chat screen — not
      // the "this state is derivable during render" anti-pattern the rule targets.
      // Rewriting them would mean key-based remounts or reshaping the data flow in
      // useSessionsController/App.tsx, which carries real regression risk and no
      // functional gain. Kept as a warning so genuinely new violations stay visible.
      'react-hooks/set-state-in-effect': 'warn',
      // Small constants (NAV, KIND_ICON, …) are deliberately co-located with the
      // component that owns them. Fast refresh handles constant exports fine, so
      // only flag non-constant non-component exports.
      'react-refresh/only-export-components': ['error', { allowConstantExport: true }],
      // The codebase marks deliberately-unused bindings with a leading underscore
      // (placeholder params kept for call-site compatibility, discarded rest-spread
      // keys). Honour that convention instead of deleting the intent.
      '@typescript-eslint/no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
          caughtErrorsIgnorePattern: '^_',
          destructuredArrayIgnorePattern: '^_',
          ignoreRestSiblings: true,
        },
      ],
    },
  },
  {
    files: I18N_MIGRATED,
    plugins: { i18next },
    rules: {
      'i18next/no-literal-string': [
        'error',
        {
          // Only JSX text and human-facing attributes are checked. Object keys,
          // imports, className strings and the like are machine data, not copy.
          mode: 'jsx-text-only',
        },
      ],
    },
  },
])
