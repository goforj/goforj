<script setup lang="ts">
import { computed, ref } from 'vue'
const props = defineProps<{ position: number; transitionMs: number }>()
const route = ref<SVGPathElement>()
const courierTransform = computed(() => {
 if (!route.value) return 'translate(255 142)'
 const point = route.value.getPointAtLength(route.value.getTotalLength() * Math.min(100, Math.max(0, props.position)) / 100)
 return `translate(${point.x} ${point.y - 16})`
})
</script>
<template>
<svg viewBox="0 0 900 260" role="img" aria-label="Pizza shop, neighborhood route, and a gopher courier delivering your order">
<defs><pattern id="grass" width="24" height="24" patternUnits="userSpaceOnUse"><circle cx="3" cy="3" r="1" fill="#dbe5cf"/></pattern><filter id="shadow"><feDropShadow dx="0" dy="4" stdDeviation="3" flood-opacity=".09"/></filter></defs>
<rect width="900" height="260" fill="url(#grass)"/>
<path ref="route" d="M255 158 H370 Q390 158 390 136 V100 Q390 78 420 78 H560 Q580 78 580 100 V157 Q580 178 605 178 H740" fill="none" stroke="#fffdf5" stroke-width="40" stroke-linejoin="round"/>
<path d="M255 158 H370 Q390 158 390 136 V100 Q390 78 420 78 H560 Q580 78 580 100 V157 Q580 178 605 178 H740" fill="none" stroke="#d7ddcd" stroke-width="2" stroke-dasharray="6 8"/>
<g filter="url(#shadow)"><rect x="125" y="85" width="145" height="93" rx="5" fill="#fff9e8"/><path d="M112 89 L195 42 L281 89Z" fill="#d45f3e"/><rect x="120" y="90" width="155" height="24" rx="3" fill="#e76b48"/><path d="M132 90V114 M158 90V114 M184 90V114 M210 90V114 M236 90V114 M262 90V114" stroke="#ffe3b4" stroke-width="13"/><rect x="141" y="129" width="45" height="32" rx="3" fill="#c4d5b7"/><rect x="208" y="121" width="37" height="57" rx="4" fill="#53775b"/><circle cx="236" cy="149" r="2" fill="#ffe29a"/><rect x="147" y="67" width="97" height="20" rx="4" fill="#fff8df"/><text x="195" y="81" font-size="11" text-anchor="middle" font-family="monospace" fill="#924c32">GOPHER EATS</text></g>
<g class="pizza-spin" id="pizza-art" :style="{ opacity: position > 0 ? 0 : 1 }"><circle cx="224" cy="126" r="21" fill="#edbd6f"/><circle cx="224" cy="126" r="17" fill="#ea6c43"/><circle cx="224" cy="126" r="14" fill="#ffdc83"/><g fill="#c85439"><circle cx="217" cy="120" r="3"/><circle cx="232" cy="124" r="3"/><circle cx="221" cy="133" r="3"/></g><g fill="#527b4b"><ellipse cx="224" cy="117" rx="2" ry="4" transform="rotate(35 224 117)"/><ellipse cx="230" cy="133" rx="2" ry="4" transform="rotate(-20 230 133)"/></g></g>
<g filter="url(#shadow)"><rect x="728" y="111" width="83" height="68" rx="3" fill="#f5ead0"/><path d="M716 113 L770 67 L824 113Z" fill="#6f8a70"/><rect x="759" y="143" width="23" height="36" rx="3" fill="#cc8b59"/><rect x="740" y="124" width="17" height="15" rx="2" fill="#bbd4d4"/><rect x="787" y="124" width="14" height="15" rx="2" fill="#bbd4d4"/><rect x="793" y="74" width="10" height="22" fill="#73836e"/></g>
<g fill="#b2c895"><circle cx="340" cy="58" r="19"/><circle cx="660" cy="89" r="23"/><circle cx="470" cy="180" r="18"/><circle cx="853" cy="133" r="20"/></g><g stroke="#7f946f" stroke-width="4"><path d="M340 58v28 M660 89v35 M470 180v25 M853 133v28"/></g>
<g :transform="courierTransform" id="courier" :style="{ transitionDuration: `${transitionMs}ms` }"><ellipse cx="0" cy="23" rx="27" ry="6" fill="#29382c15"/><circle cx="-15" cy="16" r="9" fill="#334b42"/><circle cx="18" cy="16" r="9" fill="#334b42"/><path d="M-16 5H12L21 -3 M-10 5L-3 16H13" fill="none" stroke="#e56840" stroke-width="7" stroke-linecap="round"/><rect x="-27" y="-17" width="23" height="17" rx="3" fill="#efbe6d"/><ellipse cx="4" cy="-13" rx="13" ry="17" fill="#79b9c6"/><circle cx="-6" cy="-27" r="5" fill="#79b9c6"/><circle cx="14" cy="-27" r="5" fill="#79b9c6"/><circle cx="4" cy="-25" r="14" fill="#79b9c6"/><ellipse cx="0" cy="-27" rx="5" ry="6" fill="#fff"/><ellipse cx="10" cy="-27" rx="5" ry="6" fill="#fff"/><circle cx="1" cy="-26" r="2" fill="#25382f"/><circle cx="11" cy="-26" r="2" fill="#25382f"/><ellipse cx="5" cy="-19" rx="4" ry="2" fill="#3e7081"/><rect x="2" y="-18" width="3" height="5" rx="1" fill="white"/><rect x="6" y="-18" width="3" height="5" rx="1" fill="white"/></g>
<text x="192" y="201" text-anchor="middle" font-size="10" font-family="monospace" fill="#718269">THE KITCHEN</text><text x="770" y="201" text-anchor="middle" font-size="10" font-family="monospace" fill="#718269">YOUR HAPPY PLACE</text>
</svg>
</template>
