<script setup lang="ts">
import type { Pizza } from './model'
defineProps<{ pizza: Pizza }>()
</script>

<template>
  <svg viewBox="0 0 80 80" aria-hidden="true" class="pizza-art">
    <circle cx="40" cy="41" r="33" fill="#c77b30" />
    <circle cx="40" cy="38" r="33" fill="#edb35f" />
    <circle cx="40" cy="38" r="27" fill="#d95137" />
    <circle cx="40" cy="38" r="24" fill="#ffdc88" />
    <g v-if="pizza === 'pepperoni'" fill="#c8342b" stroke="#ae2a24" stroke-width="1">
      <circle cx="29" cy="25" r="6" /><circle cx="49" cy="25" r="6" />
      <circle cx="21" cy="43" r="6" /><circle cx="41" cy="39" r="6" />
      <circle cx="58" cy="43" r="6" /><circle cx="36" cy="56" r="6" />
      <circle cx="53" cy="56" r="5" />
    </g>
    <g v-else-if="pizza === 'margherita'">
      <g fill="#f7f2dc"><circle cx="28" cy="27" r="7" /><circle cx="51" cy="34" r="8" /><circle cx="33" cy="51" r="8" /></g>
      <g fill="#438959"><ellipse cx="39" cy="23" rx="4" ry="8" transform="rotate(35 39 23)" /><ellipse cx="23" cy="43" rx="4" ry="8" transform="rotate(-35 23 43)" /><ellipse cx="52" cy="53" rx="4" ry="8" transform="rotate(40 52 53)" /></g>
    </g>
    <g v-else>
      <g fill="none" stroke="#4b9664" stroke-width="4"><path d="M23 30Q29 20 34 29" /><path d="M48 24Q60 23 57 35" /><path d="M26 50Q32 60 39 52" /></g>
      <g fill="#e86243"><circle cx="24" cy="42" r="4" /><circle cx="43" cy="33" r="4" /><circle cx="55" cy="51" r="4" /></g>
      <g fill="#674d4a"><circle cx="39" cy="22" r="3" /><circle cx="43" cy="49" r="3" /><circle cx="60" cy="40" r="3" /></g>
    </g>
    <g fill="#fff4ca" opacity=".8"><circle cx="17" cy="22" r="1.5" /><circle cx="64" cy="24" r="1.5" /><circle cx="17" cy="57" r="1.5" /><circle cx="48" cy="68" r="1.5" /></g>
  </svg>
</template>
