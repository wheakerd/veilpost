<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useInterfaceMotion } from '@/composables/useInterfaceMotion'

// The sign-in visual: a dotted wireframe globe with a scan line and an orbit
// ring, drawn on a canvas. It is decorative (aria-hidden), shows no node or
// account data, pauses while the page is hidden and holds still when reduced
// motion is requested.

const canvas = ref<HTMLCanvasElement>()
const { reduceMotion } = useInterfaceMotion()

type Point = [x: number, y: number, z: number]
const grid: Point[] = []
const marks: Point[] = []

function unit(latitude: number, longitude: number): Point {
    const lat = (latitude * Math.PI) / 180
    const lon = (longitude * Math.PI) / 180
    return [
        Math.cos(lat) * Math.sin(lon),
        Math.sin(lat),
        Math.cos(lat) * Math.cos(lon),
    ]
}
for (let lat = -75; lat <= 75; lat += 15) {
    const count = Math.round(96 * Math.cos((lat * Math.PI) / 180))
    for (let step = 0; step < count; step++)
        grid.push(unit(lat, (360 * step) / count))
}
for (let lon = 0; lon < 360; lon += 20) {
    for (let lat = -84; lat <= 84; lat += 4) {
        if (lat % 15) grid.push(unit(lat, lon))
    }
}
// Abstract survey marks; they stand for no real place or node.
for (const [lat, lon] of [
    [34, 18],
    [12, 104],
    [-22, 152],
    [48, 236],
    [-8, 298],
    [60, 72],
    [-40, 200],
] as const)
    marks.push(unit(lat, lon))

const tilt = 0.38
const period = 120_000 // one turn every two minutes
const sweep = 7_000 // scan line from top to bottom

let frame = 0
let start = 0
let colors = { a: [20, 108, 100], b: [165, 101, 30], ink: [18, 20, 24] }

function parseColor(value: string): number[] {
    const hex = /^#([\da-f]{6})$/i.exec(value.trim())?.[1]
    if (hex) {
        const n = Number.parseInt(hex, 16)
        return [(n >> 16) & 255, (n >> 8) & 255, n & 255]
    }
    const rgb = value.match(/\d+(\.\d+)?/g)
    return rgb ? rgb.slice(0, 3).map(Number) : [128, 128, 128]
}

function readColors() {
    const style = getComputedStyle(document.documentElement)
    colors = {
        a: parseColor(style.getPropertyValue('--app-primary')),
        b: parseColor(style.getPropertyValue('--app-secondary')),
        ink: parseColor(style.getPropertyValue('--app-text')),
    }
}

function project(point: Point, angle: number) {
    const [x, y, z] = point
    const rx = x * Math.cos(angle) + z * Math.sin(angle)
    const rz = -x * Math.sin(angle) + z * Math.cos(angle)
    const ty = y * Math.cos(tilt) - rz * Math.sin(tilt)
    const tz = y * Math.sin(tilt) + rz * Math.cos(tilt)
    return { x: rx, y: ty, z: tz }
}

function mix(t: number, alpha: number) {
    const c = colors.a.map((value, index) =>
        Math.round(value + ((colors.b[index] ?? value) - value) * t),
    )
    return `rgba(${c[0]}, ${c[1]}, ${c[2]}, ${alpha})`
}

function draw(time: number) {
    const element = canvas.value
    const context = element?.getContext('2d')
    if (!element || !context) return
    const ratio = window.devicePixelRatio || 1
    const width = element.clientWidth
    const height = element.clientHeight
    if (element.width !== Math.round(width * ratio))
        element.width = Math.round(width * ratio)
    if (element.height !== Math.round(height * ratio))
        element.height = Math.round(height * ratio)
    context.setTransform(ratio, 0, 0, ratio, 0, 0)
    context.clearRect(0, 0, width, height)

    const radius = Math.min(width, height) * (width < 760 ? 0.44 : 0.4)
    const cx = width / 2
    const cy = height / 2
    const angle = 0.6 + ((time % period) / period) * Math.PI * 2
    const scan = reduceMotion.value ? 0.18 : 1 - ((time % sweep) / sweep) * 2
    const ink = colors.ink

    // Limb and orbit ring.
    context.lineWidth = 1
    context.strokeStyle = `rgba(${ink[0]}, ${ink[1]}, ${ink[2]}, 0.12)`
    context.beginPath()
    context.arc(cx, cy, radius, 0, Math.PI * 2)
    context.stroke()
    context.save()
    context.translate(cx, cy)
    context.rotate(-0.32)
    context.setLineDash([2, 6])
    context.strokeStyle = mix(0.5, 0.45)
    context.beginPath()
    context.ellipse(0, 0, radius * 1.32, radius * 0.42, 0, 0, Math.PI * 2)
    context.stroke()
    context.setLineDash([])
    const orbit = angle * 2.4
    context.fillStyle = mix(1, 0.9)
    context.fillRect(
        Math.cos(orbit) * radius * 1.32 - 3,
        Math.sin(orbit) * radius * 0.42 - 3,
        6,
        6,
    )
    context.restore()

    // Dotted graticule, brighter on the near side and along the scan line.
    for (const point of grid) {
        const p = project(point, angle)
        const near = p.z > 0
        const lit = Math.max(0, 1 - Math.abs(p.y - scan) / 0.06)
        const alpha = near ? 0.4 + 0.5 * p.z + 0.4 * lit : 0.12
        const size = near ? 1.6 + 1.6 * p.z + 1.4 * lit : 1
        context.fillStyle = mix((p.x + 1) / 2, Math.min(alpha, 1))
        context.fillRect(
            cx + p.x * radius - size / 2,
            cy - p.y * radius - size / 2,
            size,
            size,
        )
    }

    // Scan chord across the sphere.
    if (Math.abs(scan) < 1) {
        const half = radius * Math.sqrt(1 - scan * scan)
        context.strokeStyle = mix(0.5, 0.35)
        context.beginPath()
        context.moveTo(cx - half, cy - scan * radius)
        context.lineTo(cx + half, cy - scan * radius)
        context.stroke()
    }

    // Survey marks on the near side.
    context.lineWidth = 1.25
    for (const point of marks) {
        const p = project(point, angle)
        if (p.z <= 0.05) continue
        const x = cx + p.x * radius
        const y = cy - p.y * radius
        context.strokeStyle = mix(1, 0.4 + 0.6 * p.z)
        context.beginPath()
        context.moveTo(x - 5, y)
        context.lineTo(x + 5, y)
        context.moveTo(x, y - 5)
        context.lineTo(x, y + 5)
        context.stroke()
    }
}

function loop(now: number) {
    if (!start) start = now
    draw(now - start)
    frame = requestAnimationFrame(loop)
}

function stop() {
    cancelAnimationFrame(frame)
    frame = 0
}

function run() {
    stop()
    readColors()
    if (reduceMotion.value || document.hidden) draw(0)
    else frame = requestAnimationFrame(loop)
}

let observer: MutationObserver | undefined
let resize: ResizeObserver | undefined
onMounted(() => {
    run()
    document.addEventListener('visibilitychange', run)
    // Theme or palette changes recolor the globe.
    observer = new MutationObserver(run)
    observer.observe(document.documentElement, {
        attributes: true,
        attributeFilter: ['class', 'data-theme'],
    })
    resize = new ResizeObserver(() => {
        if (!frame) draw(0)
    })
    if (canvas.value) resize.observe(canvas.value)
})
watch(reduceMotion, run)
onBeforeUnmount(() => {
    stop()
    document.removeEventListener('visibilitychange', run)
    observer?.disconnect()
    resize?.disconnect()
})
</script>

<template>
    <canvas ref="canvas" class="lab-globe" aria-hidden="true"></canvas>
</template>
