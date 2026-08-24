import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

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
])
