// Defined by rsbuild.config.ts from package.json.
declare const __APP_VERSION__: string

interface Window {
    /** Minimum splash duration measured from the initial document's first frame. */
    veilpostBootMinimum: Promise<void>
}

// Lets plain TypeScript (ESLint's type checker) resolve .vue imports;
// vue-tsc reads the components themselves.
declare module '*.vue' {
    import type { DefineComponent } from 'vue'
    const component: DefineComponent
    export default component
}
