import js from '@eslint/js'
import { defineConfig } from 'eslint/config'
import vue from 'eslint-plugin-vue'
import tseslint from 'typescript-eslint'

// Only rules that catch mistakes: formatting and style are not checked.
export default defineConfig(
  {
    ignores: [
      'build*/',
      'core/',
      'deployment/',
      'dist/',
      'playwright-report/',
      'res/',
      'src-tauri/',
      'test-results/',
      'third_party/',
      // Written by tygo from the Go sources of the core.
      'src/platform/core-types.ts',
      'src/platform/profile-types.ts',
    ],
  },
  js.configs.recommended,
  tseslint.configs.recommendedTypeChecked,
  vue.configs['flat/essential'],
  {
    files: ['**/*.{ts,vue}'],
    languageOptions: {
      parserOptions: {
        parser: tseslint.parser,
        project: ['./tsconfig.eslint.json', './tsconfig.node.json'],
        tsconfigRootDir: import.meta.dirname,
        extraFileExtensions: ['.vue'],
      },
    },
  },
  { ...tseslint.configs.eslintRecommended, files: ['**/*.vue'] },
  {
    files: ['**/*.js'],
    extends: [tseslint.configs.disableTypeChecked],
  },
  {
    rules: {
      'array-callback-return': 'error',
      eqeqeq: ['error', 'smart'],
      'no-constructor-return': 'error',
      'no-self-compare': 'error',
      'no-template-curly-in-string': 'error',
      'no-unmodified-loop-condition': 'error',
      'no-unreachable-loop': 'error',
    },
  },
  {
    files: ['**/*.{ts,vue}'],
    rules: {
      // TypeScript reports undefined names and unused code itself (noUnusedLocals, noUnusedParameters).
      'no-undef': 'off',
      '@typescript-eslint/no-unused-vars': 'off',
      // An async function without await still turns a throw into a rejection, as the core bridge rejects.
      '@typescript-eslint/require-await': 'off',
      // The code has no classes; the functions of useI18n are declared as methods but do not use this.
      '@typescript-eslint/unbound-method': 'off',

      // The core bridge rejects with a CoreError, a plain object; a caught error is passed on as it is.
      '@typescript-eslint/prefer-promise-reject-errors': [
        'error',
        { allow: [{ from: 'file', name: 'CoreError' }], allowThrowingUnknown: true },
      ],
      // {} is an unknown value without null and undefined, such as a value read from JSON: its text is wanted as it is.
      '@typescript-eslint/no-base-to-string': ['error', { ignoredTypeNames: ['Error', 'RegExp', 'URL', 'URLSearchParams', '{}'] }],
      '@typescript-eslint/restrict-template-expressions': [
        'error',
        { allowAny: false, allowBoolean: false, allowNever: false, allowNullish: false, allowNumber: true, allowRegExp: false },
      ],
      '@typescript-eslint/return-await': ['error', 'error-handling-correctness-only'],
      '@typescript-eslint/switch-exhaustiveness-check': 'error',

      // Single-word names are safe: every component is imported, and vue-tsc rejects an unknown one.
      'vue/multi-word-component-names': 'off',
      'vue/no-setup-props-reactivity-loss': 'error',
      'vue/no-template-target-blank': 'error',
      'vue/no-unused-emit-declarations': 'error',
      'vue/no-unused-properties': 'error',
      'vue/no-unused-refs': 'error',
      'vue/no-v-html': 'error',
    },
  },
  {
    // The stub core fails as the core bridge does: with CoreError objects rather than Error.
    files: ['src/platform/stub*.ts'],
    rules: { '@typescript-eslint/only-throw-error': 'off' },
  },
)
