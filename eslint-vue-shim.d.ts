// Only the TypeScript project of ESLint (tsconfig.eslint.json) reads this file. ESLint checks types with plain
// TypeScript, which cannot read a .vue file: without this declaration every imported component is an unresolved type.
// vue-tsc reads the components themselves and must not see it, or an import of a missing .vue file would pass.
declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent
  export default component
}
