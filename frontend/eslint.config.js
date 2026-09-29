import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import i18next from 'eslint-plugin-i18next'
import { defineConfig, globalIgnores } from 'eslint/config'

// Every application screen uses the locale catalogs. Keep new visible JSX copy
// from silently bypassing them; tests intentionally contain language fixtures.
const I18N_MIGRATED = ['src/**/*.{ts,tsx}']

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
      // Downgraded 2026-08-24 (61 hits, deliberate async-fetch lifecycles). The
      // 2026-09-22 pass moved the reset-on-key-change cases to useKeyedReset
      // (render-phase adjust) and the loaders to callback-only fetches, leaving
      // seven that genuinely belong in an effect: they call a parent's setter
      // (AgentsView, useSessionsController), write localStorage (App recs,
      // useWorkspaces, TaskBoard glow), restore once via a ref (FlowsPanel) or
      // must also fire on mount (App chat-open nag). Kept as a warning so new
      // violations stay visible.
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
          // Direct JSX text is checked. Source-key and catalog tests complement
          // this guard; machine attributes, examples and imports are not copy.
          mode: 'jsx-text-only',
          'jsx-components': { exclude: ['Trans', 'code', 'pre'] },
          words: {
            exclude: [/^[\p{P}\p{S}\p{N}\p{M}\s]+$/u, '[A-Z_-]+', '^TionHarness$'],
          },
        },
      ],
    },
  },
  {
    // Test harnesses publish the latest hook result through module-scope
    // bindings (this repo has no renderHook library). The compiler-backed
    // globals rule targets app code, not that scaffolding.
    files: ['**/*.test.{ts,tsx}'],
    rules: {
      'react-hooks/globals': 'off',
      'i18next/no-literal-string': 'off',
    },
  },
])
