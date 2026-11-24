// Defined by rsbuild.config.ts from package.json.
declare const __APP_VERSION__: string

// Lets plain TypeScript (ESLint's type checker) resolve .vue imports;
// vue-tsc reads the components themselves.
declare module '*.vue' {
    import type { DefineComponent } from 'vue'
    const component: DefineComponent
    export default component
}
