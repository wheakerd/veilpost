import type { Directive } from 'vue'

// Liquid glass (styles/_glass.scss): pieces marked by v-glass get the pane
// material, its edge lights and the pointer light. Where the browser can apply an SVG filter to the
// backdrop (Chromium), the surface also refracts what lies beneath it: a
// displacement map, drawn for the element's own size and corner radius,
// bends the backdrop near the edges like the bevel of a lens. Elsewhere the
// glass keeps its blur, rim and sheen.

export interface GlassOptions {
    // false turns the material off, for surfaces that are glass only on some
    // layouts.
    enabled?: boolean
    // false keeps only the refraction, for a lens that sits over controls.
    material?: boolean
    // Width of the refracting bevel in pixels; defaults to a third of the
    // shorter side, at most 18.
    depth?: number
    // Strength of the bend in pixels; defaults to the depth.
    scale?: number
}

interface GlassState {
    options: GlassOptions
    observer?: ResizeObserver
    filter?: SVGFilterElement
    size?: string
}

const svgNamespace = 'http://www.w3.org/2000/svg'
const states = new WeakMap<HTMLElement, GlassState>()
let filters: SVGSVGElement | undefined
let nextId = 0
let tracking = false

// Only Chromium applies url() filters to the backdrop; Safari and Firefox
// would drop the whole backdrop-filter declaration.
function supportsLens() {
    const data = (
        navigator as Navigator & {
            userAgentData?: { brands: { brand: string }[] }
        }
    ).userAgentData
    return Boolean(data?.brands.some((entry) => entry.brand === 'Chromium'))
}
const lensSupported = typeof navigator !== 'undefined' && supportsLens()

function filterRoot() {
    if (!filters) {
        filters = document.createElementNS(svgNamespace, 'svg')
        filters.setAttribute('aria-hidden', 'true')
        filters.setAttribute('width', '0')
        filters.setAttribute('height', '0')
        // Out of the flow and of no size, so it never extends the page; not
        // display: none, which would disable the filters it holds.
        filters.style.position = 'fixed'
        filters.style.top = '0'
        filters.style.left = '0'
        filters.style.width = '0'
        filters.style.height = '0'
        filters.style.overflow = 'hidden'
        filters.style.pointerEvents = 'none'
        document.body.append(filters)
    }
    return filters
}

// Each pixel stores where to sample the backdrop from, as an offset in the
// red (x) and green (y) channels around the neutral 128. Inside the bevel the
// sample moves inwards along the edge normal, strongest at the edge.
function displacementMap(
    width: number,
    height: number,
    radius: number,
    depth: number,
) {
    const canvas = document.createElement('canvas')
    canvas.width = width
    canvas.height = height
    const context = canvas.getContext('2d')
    if (!context) return undefined
    const image = context.createImageData(width, height)
    const halfWidth = width / 2
    const halfHeight = height / 2
    const corner = Math.min(radius, halfWidth, halfHeight)
    for (let y = 0; y < height; y++) {
        for (let x = 0; x < width; x++) {
            const px = x + 0.5 - halfWidth
            const py = y + 0.5 - halfHeight
            // Signed distance to the rounded rectangle, negative inside.
            const qx = Math.abs(px) - (halfWidth - corner)
            const qy = Math.abs(py) - (halfHeight - corner)
            const outside = Math.hypot(Math.max(qx, 0), Math.max(qy, 0))
            const inside = Math.min(Math.max(qx, qy), 0)
            const edge = corner - outside - inside
            let dx = 0
            let dy = 0
            if (edge < depth) {
                let nx = 0
                let ny = 0
                if (qx > 0 && qy > 0) {
                    const length = Math.hypot(qx, qy)
                    nx = qx / length
                    ny = qy / length
                } else if (qx > qy) nx = 1
                else ny = 1
                const falloff = 1 - Math.max(edge, 0) / depth
                const strength = falloff * falloff
                dx = -nx * Math.sign(px) * strength
                dy = -ny * Math.sign(py) * strength
            }
            const index = (y * width + x) * 4
            image.data[index] = 128 + dx * 127
            image.data[index + 1] = 128 + dy * 127
            image.data[index + 2] = 128
            image.data[index + 3] = 255
        }
    }
    context.putImageData(image, 0, 0)
    return canvas.toDataURL()
}

function drawLens(element: HTMLElement, state: GlassState) {
    const width = Math.round(element.offsetWidth)
    const height = Math.round(element.offsetHeight)
    if (!width || !height) return
    const radius =
        parseFloat(getComputedStyle(element).borderTopLeftRadius) || 0
    const size = `${width}x${height}x${radius}`
    if (size === state.size) return
    state.size = size
    const depth =
        state.options.depth ?? Math.min(18, Math.min(width, height) / 3)
    const map = displacementMap(width, height, radius, depth)
    if (!map) return
    if (!state.filter) {
        const id = `liquid-glass-${++nextId}`
        const filter = document.createElementNS(svgNamespace, 'filter')
        filter.id = id
        for (const [name, value] of Object.entries({
            x: '0',
            y: '0',
            filterUnits: 'userSpaceOnUse',
            primitiveUnits: 'userSpaceOnUse',
            'color-interpolation-filters': 'sRGB',
        }))
            filter.setAttribute(name, value)
        const source = document.createElementNS(svgNamespace, 'feImage')
        source.setAttribute('x', '0')
        source.setAttribute('y', '0')
        source.setAttribute('preserveAspectRatio', 'none')
        source.setAttribute('result', 'map')
        const bend = document.createElementNS(svgNamespace, 'feDisplacementMap')
        bend.setAttribute('in', 'SourceGraphic')
        bend.setAttribute('in2', 'map')
        bend.setAttribute('xChannelSelector', 'R')
        bend.setAttribute('yChannelSelector', 'G')
        filter.append(source, bend)
        filterRoot().append(filter)
        state.filter = filter
        element.style.setProperty('--glass-lens', `url(#${id})`)
        element.dataset.glassLens = ''
    }
    const [source, bend] = state.filter.children
    state.filter.setAttribute('width', String(width))
    state.filter.setAttribute('height', String(height))
    source?.setAttribute('width', String(width))
    source?.setAttribute('height', String(height))
    source?.setAttribute('href', map)
    bend?.setAttribute('scale', String(state.options.scale ?? depth))
}

// Pieces of glass that carry the pointer light in their face
// (styles/_glass.scss). Text and link buttons have no glass.
const lightTargets = [
    '.el-button:not(.is-text, .is-link)',
    '[data-glass]',
    '.page-tab',
    '.app-manage-link',
].join(', ')

// Lights the piece of glass under the pointer, as Liquid Glass lights up
// where it is touched: one delegated listener marks the piece and writes
// --app-light-x and --app-light-y from its corner, and CSS owns the look. A
// mouse or pen lights a piece while it hovers; a touch only while pressed.
export function installGlassLight() {
    if (tracking) return
    tracking = true
    let lit: HTMLElement | null = null
    let frame = 0
    let pending: PointerEvent | null = null
    const release = () => {
        lit?.removeAttribute('data-app-light')
        lit = null
    }
    const light = (event: PointerEvent) => {
        const target = event.target
        const piece =
            target instanceof Element
                ? target.closest<HTMLElement>(lightTargets)
                : null
        if (piece !== lit) {
            release()
            piece?.setAttribute('data-app-light', '')
            lit = piece
        }
        if (!piece) return
        const box = piece.getBoundingClientRect()
        piece.style.setProperty(
            '--app-light-x',
            `${event.clientX - box.left}px`,
        )
        piece.style.setProperty('--app-light-y', `${event.clientY - box.top}px`)
    }
    document.addEventListener(
        'pointermove',
        (event) => {
            if (event.pointerType === 'touch') return
            pending = event
            if (frame) return
            frame = requestAnimationFrame(() => {
                frame = 0
                if (pending) light(pending)
                pending = null
            })
        },
        { passive: true },
    )
    // A press places the light at once, so the bloom starts where it landed.
    document.addEventListener('pointerdown', light, { passive: true })
    document.addEventListener(
        'pointerup',
        (event) => {
            if (event.pointerType === 'touch') release()
        },
        { passive: true },
    )
    document.addEventListener(
        'pointerout',
        (event) => {
            if (!event.relatedTarget) release()
        },
        { passive: true },
    )
}

// One shared lens for the floating sheets, which Element Plus renders and a
// directive cannot reach: a map stretched over each sheet, of which only the
// rim band shows (the sheet's masked ::before in styles/_glass.scss).
export function installSheetLens() {
    if (!lensSupported || document.getElementById('veilpost-sheet-lens')) return
    const map = displacementMap(256, 256, 48, 72)
    if (!map) return
    const filter = document.createElementNS(svgNamespace, 'filter')
    filter.id = 'veilpost-sheet-lens'
    for (const [name, value] of Object.entries({
        x: '0',
        y: '0',
        width: '100%',
        height: '100%',
        'color-interpolation-filters': 'sRGB',
    }))
        filter.setAttribute(name, value)
    const source = document.createElementNS(svgNamespace, 'feImage')
    source.setAttribute('href', map)
    source.setAttribute('preserveAspectRatio', 'none')
    source.setAttribute('result', 'map')
    const bend = document.createElementNS(svgNamespace, 'feDisplacementMap')
    bend.setAttribute('in', 'SourceGraphic')
    bend.setAttribute('in2', 'map')
    bend.setAttribute('scale', '40')
    bend.setAttribute('xChannelSelector', 'R')
    bend.setAttribute('yChannelSelector', 'G')
    filter.append(source, bend)
    filterRoot().append(filter)
    document.documentElement.dataset.glassLens = ''
}

function attach(element: HTMLElement, options: GlassOptions) {
    const state: GlassState = { options }
    states.set(element, state)
    if (options.material !== false) element.dataset.glass = ''
    if (!lensSupported) return
    state.observer = new ResizeObserver(() => {
        drawLens(element, state)
    })
    state.observer.observe(element)
}

function detach(element: HTMLElement) {
    const state = states.get(element)
    if (!state) return
    state.observer?.disconnect()
    state.filter?.remove()
    states.delete(element)
    delete element.dataset.glass
    delete element.dataset.glassLens
    element.style.removeProperty('--glass-lens')
}

export const vGlass: Directive<HTMLElement, GlassOptions | undefined> = {
    mounted(element, binding) {
        const options = binding.value ?? {}
        if (options.enabled !== false) attach(element, options)
    },
    updated(element, binding) {
        const options = binding.value ?? {}
        const attached = states.has(element)
        if (options.enabled === false) {
            if (attached) detach(element)
        } else if (!attached) attach(element, options)
    },
    beforeUnmount(element) {
        detach(element)
    },
}
