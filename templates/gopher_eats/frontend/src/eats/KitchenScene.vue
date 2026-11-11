<script setup lang="ts">
defineProps<{ stage: string }>()
const labels: Record<string,string> = {
    received: 'A gopher chef receives your printed pizza order at the counter',
    preparing: 'A gopher chef stretches pizza dough while tomato and basil toppings land on it',
    baking: 'Your pizza bakes inside a glowing stone oven with flickering flames',
    retrying: 'A slightly embarrassed gopher chef discovers a burned pizza and starts again',
    ready: 'Your finished pizza is boxed on the pickup counter, with steam rising from it',
  }

</script>

<template>
  <svg class="kitchen-visual" viewBox="0 0 900 260" role="img" :aria-label="labels[stage] || 'The gopher kitchen is making your pizza'">
    <defs>
      <pattern id="kitchen-tiles" width="34" height="34" patternUnits="userSpaceOnUse">
        <rect width="34" height="34" fill="var(--scene-kitchen)" />
        <path d="M34 0H0V34" fill="none" stroke="var(--scene-tile)" stroke-width="1" />
      </pattern>
      <radialGradient id="oven-glow"><stop stop-color="#ffd47b" /><stop offset=".55" stop-color="#f28a42" /><stop offset="1" stop-color="#a3442e" stop-opacity="0" /></radialGradient>
    </defs>
    <rect width="900" height="260" fill="url(#kitchen-tiles)" />
    <rect x="162" y="75" width="102" height="7" rx="3" fill="#d4c2ad" />
    <g fill="#f2dfc6"><rect x="174" y="44" width="20" height="31" rx="4" /><rect x="206" y="50" width="17" height="25" rx="4" /><rect x="235" y="38" width="15" height="37" rx="4" /></g>
    <g fill="#c29269"><rect x="174" y="40" width="20" height="6" rx="2" /><rect x="206" y="46" width="17" height="6" rx="2" /><rect x="235" y="34" width="15" height="6" rx="2" /></g>
    <g fill="#709661"><ellipse cx="722" cy="127" rx="9" ry="22" transform="rotate(-30 722 127)" /><ellipse cx="738" cy="121" rx="9" ry="24" transform="rotate(28 738 121)" /></g>
    <path d="M712 143H748L742 174H718Z" fill="#d59b70" />
    <rect x="198" y="170" width="493" height="13" rx="5" fill="#d9bc8d" />
    <path d="M213 183H676V202H213Z" fill="#f7edd6" />
    <rect x="235" y="202" width="11" height="20" fill="#cfb992" /><rect x="644" y="202" width="11" height="20" fill="#cfb992" />
    <g class="chef" :class="{ kneading: stage === 'preparing', surprised: stage === 'retrying' }">
      <ellipse cx="342" cy="149" rx="37" ry="39" fill="var(--gopher-body)" />
      <path d="M320 127H364L367 172H317Z" fill="#fffdf5" />
      <path d="M330 129V117M354 129V117" stroke="#fffdf5" stroke-width="5" />
      <circle cx="312" cy="77" r="12" fill="var(--gopher-body)" /><circle cx="372" cy="77" r="12" fill="var(--gopher-body)" />
      <ellipse cx="342" cy="92" rx="38" ry="35" fill="var(--gopher-body)" />
      <ellipse cx="328" cy="86" rx="11" ry="14" fill="white" /><ellipse cx="353" cy="86" rx="11" ry="14" fill="white" />
      <g class="chef-eyes"><circle cx="331" cy="88" r="4" fill="var(--foreground)" /><circle cx="356" cy="88" r="4" fill="var(--foreground)" /></g>
      <ellipse cx="342" cy="104" rx="8" ry="4" fill="var(--gopher-nose)" />
      <rect x="336" y="107" width="6" height="10" rx="2" fill="white" /><rect x="343" y="107" width="6" height="10" rx="2" fill="white" />
      <path d="M311 64Q302 44 318 39Q319 23 337 30Q351 16 362 33Q383 28 381 48Q388 61 372 66Z" fill="#fffdf5" />
      <rect x="313" y="59" width="60" height="11" rx="4" fill="#efe8dd" />
      <g class="chef-hands"><path d="M369 136L395 149" stroke="var(--gopher-body)" stroke-width="16" stroke-linecap="round" /><ellipse cx="402" cy="150" rx="14" ry="10" fill="var(--gopher-body)" /><ellipse cx="298" cy="153" rx="14" ry="10" fill="var(--gopher-body)" /></g>
    </g>
    <Transition name="station" mode="out-in">
      <g :key="stage" class="station">
        <g v-if="stage === 'received'" class="order-ticket">
          <rect x="465" y="63" width="150" height="106" rx="7" fill="#fffdf5" stroke="#e7d8cb" stroke-width="2" />
          <rect x="480" y="55" width="120" height="12" rx="5" fill="#c73d2d" />
          <text x="540" y="88" text-anchor="middle" fill="#c73d2d" font-family="monospace" font-size="11">ONE HAPPY PIZZA</text>
          <path d="M489 101H590M489 110H568M489 119H581" stroke="#e5d9cd" stroke-width="3" stroke-linecap="round" />
          <circle cx="540" cy="144" r="13" fill="#fff0e8" /><path d="M534 144L539 149L548 138" fill="none" stroke="#c73d2d" stroke-width="3" stroke-linecap="round" />
        </g>
        <g v-else-if="stage === 'preparing'">
          <g class="flour" fill="#fffdf5"><circle cx="441" cy="144" r="4" /><circle cx="457" cy="151" r="3" /><circle cx="598" cy="149" r="5" /><circle cx="616" cy="156" r="3" /></g>
          <g class="dough"><ellipse cx="525" cy="149" rx="76" ry="22" fill="#e7bf78" /><ellipse cx="525" cy="146" rx="68" ry="17" fill="#f9db98" /><ellipse cx="525" cy="145" rx="57" ry="13" fill="#eb8152" /><ellipse cx="525" cy="144" rx="50" ry="10" fill="#ffdc83" /></g>
          <g class="toppings" fill="#bd543a"><ellipse cx="508" cy="142" rx="6" ry="3" /><ellipse cx="543" cy="140" rx="6" ry="3" /><ellipse cx="526" cy="149" rx="6" ry="3" /></g>
          <g class="basil" fill="#588047"><ellipse cx="496" cy="148" rx="5" ry="2" /><ellipse cx="552" cy="147" rx="5" ry="2" /></g>
          <rect x="425" y="164" width="39" height="5" rx="2" fill="#bd9161" />
        </g>
        <g v-else-if="stage === 'baking' || stage === 'retrying'">
          <ellipse cx="539" cy="171" rx="104" ry="8" fill="#b8836820" />
          <path d="M446 164V112Q446 45 537 45Q628 45 628 112V164Z" fill="#b85d42" />
          <path d="M453 160V112Q453 53 537 53Q621 53 621 112V160Z" fill="#d88760" />
          <path d="M472 157V116Q472 76 537 76Q602 76 602 116V157Z" fill="#472c27" />
          <path d="M482 153V117Q482 87 537 87Q592 87 592 117V153Z" fill="#2e2424" />
          <path d="M446 112H472M602 112H628M456 82L478 91M478 59L493 80M510 47L515 74M564 48L558 74M596 61L581 81M618 84L597 95" stroke="#f1bb8c" stroke-width="5" />
          <path d="M453 136H472M602 136H621" stroke="#b85d42" stroke-width="3" />
          <path d="M481 153V140Q537 125 593 140V153Z" fill="#7f4632" />
          <ellipse v-if="stage === 'baking'" class="hearth-glow" cx="537" cy="135" rx="54" ry="22" fill="url(#oven-glow)" opacity=".55" />
          <g stroke="#432b26" stroke-width="5" stroke-linecap="round"><path d="M488 143L509 149M489 150L507 142M565 147L587 141M566 140L584 150" /></g>
          <g v-if="stage === 'baking'" class="oven-flames">
            <g class="flame-left"><path d="M488 143Q477 131 492 110Q489 125 500 124Q497 111 506 105Q502 126 513 131Q519 147 504 150Z" fill="#f47a35" /><path d="M492 144Q485 135 495 125Q492 137 502 132Q511 142 503 147Z" fill="#ffd66f" /><path d="M498 146Q493 140 500 135Q499 142 505 142L503 147Z" fill="#fff0ac" /></g>
            <g class="flame-right"><path d="M568 149Q553 138 567 120Q566 130 572 130Q583 122 578 108Q596 126 589 138Q599 149 582 152Z" fill="#f47a35" /><path d="M573 147Q566 139 574 130Q574 139 581 136Q589 145 581 150Z" fill="#ffd66f" /><path d="M577 149Q573 144 579 140Q578 146 583 146L581 150Z" fill="#fff0ac" /></g>
          </g>
          <rect x="437" y="158" width="200" height="13" rx="4" fill="#b45b40" />
          <rect x="440" y="158" width="194" height="4" rx="2" fill="#e7a579" />
          <g v-if="stage === 'baking'" class="pizza-peel"><path d="M401 151L515 157" stroke="#b58251" stroke-width="6" stroke-linecap="round" /><ellipse cx="537" cy="155" rx="37" ry="6" fill="#c9c5b9" /><ellipse cx="537" cy="154" rx="34" ry="5" fill="#e6e1d5" /></g>
          <ellipse cx="537" cy="150" rx="33" ry="7" :fill="stage === 'retrying' ? '#6a493a' : '#d99a4c'" />
          <ellipse cx="537" cy="148" rx="29" ry="5" :fill="stage === 'retrying' ? '#49372f' : '#f4c466'" />
          <ellipse v-if="stage === 'baking'" cx="537" cy="147" rx="24" ry="3" fill="#ef8451" />
          <g v-if="stage === 'baking'" fill="#b94432"><ellipse cx="525" cy="146" rx="3" ry="1.5" /><ellipse cx="540" cy="149" rx="3" ry="1.5" /><ellipse cx="549" cy="146" rx="3" ry="1.5" /></g>
          <g class="oven-steam" :stroke="stage === 'retrying' ? '#8d817a' : '#c1a48d'" stroke-width="3" opacity=".7" fill="none" stroke-linecap="round"><path d="M518 36Q511 27 519 17" /><path d="M539 31Q532 21 540 11" /><path d="M560 36Q553 27 561 17" /></g>
        </g>
        <g v-else-if="stage === 'ready'" class="pickup-box">
          <path d="M450 126H600L638 151H487Z" fill="#eacb91" stroke="#cfad75" stroke-width="2" />
          <path d="M487 151H638V174H487Z" fill="#d9b57d" /><path d="M450 126L487 151V174L450 148Z" fill="#cda66f" />
          <text x="545" y="143" text-anchor="middle" font-size="11" font-family="monospace" fill="#7d613e">GOPHER EATS</text>
          <rect x="549" y="153" width="35" height="20" rx="2" fill="#e65c45" /><path d="M558 163L564 168L575 157" stroke="#fff3d5" stroke-width="3" fill="none" />
          <g class="oven-steam" stroke="#fffdf5" stroke-width="4" fill="none" stroke-linecap="round"><path d="M516 105Q508 94 517 83" /><path d="M540 99Q532 88 541 77" /><path d="M563 106Q555 95 564 84" /></g>
          <g class="pickup-bell"><path d="M659 159Q659 141 675 141Q691 141 691 159Z" fill="#e6b55f" /><rect x="656" y="159" width="38" height="6" rx="3" fill="#b98c4d" /><circle cx="675" cy="138" r="4" fill="#b98c4d" /></g>
        </g>
      </g>
    </Transition>
  </svg>
</template>

<style scoped>
.station-enter-active,.station-leave-active {
  transition: opacity .18s ease,transform .18s ease;
}
.station-enter-from,.station-leave-to {
  opacity: 0;
  transform: translateY(5px);
}
.order-ticket {
  animation: ticket-in .5s ease-out both;
  transform-origin: 540px 110px;
}
.kneading .chef-hands {
  animation: knead .7s ease-in-out infinite;
  transform-origin: 350px 150px;
}
.dough {
  animation: stretch .7s ease-in-out infinite;
  transform-origin: 525px 148px;
}
.toppings {
  animation: topping-drop 1.8s ease-out infinite;
  transform-origin: 525px 145px;
}
.basil {
  animation: topping-drop 2.1s .3s ease-out infinite;
}
.flour {
  animation: flour-puff 1.4s ease-out infinite;
}
.flame-left, .flame-right {
  animation: flame .65s ease-in-out infinite alternate;
  transform-box: fill-box;
  transform-origin: center bottom;
}
.flame-right {
  animation-delay: -.3s;
  animation-duration: .85s;
}
.hearth-glow {
  animation: hearth-glow 1.8s ease-in-out infinite alternate;
}
@keyframes hearth-glow {
  to { opacity: .8; }
}
.oven-steam {
  animation: steam 1.7s ease-in-out infinite;
}
.pickup-box {
  animation: ticket-in .45s ease-out both;
  transform-origin: 540px 155px;
}
.pickup-bell {
  animation: ring 1.3s ease-in-out infinite;
  transform-origin: 675px 162px;
}
.surprised .chef-eyes {
  transform: translateY(-2px);
}
@keyframes ticket-in {
  from {
    opacity: 0;
    transform: translateY(-16px) rotate(-5deg);
  }
  to {
    opacity: 1;
    transform: translateY(0) rotate(0);
  }
}
@keyframes knead {
  50% {
    transform: translate(6px,4px);
  }
}
@keyframes stretch {
  50% {
    transform: scale(1.06,.9);
  }
}
@keyframes topping-drop {
  0%,15% {
    opacity: 0;
    transform: translateY(-34px);
  }
  45%,85% {
    opacity: 1;
    transform: translateY(0);
  }
  100% {
    opacity: 0;
  }
}
@keyframes flour-puff {
  0% {
    opacity: 0;
    transform: translateY(4px);
  }
  40% {
    opacity: .8;
  }
  100% {
    opacity: 0;
    transform: translateY(-12px);
  }
}
@keyframes flame {
  from {
    transform: scaleY(.85);
  }
  to {
    transform: scaleY(1.08);
  }
}
@keyframes steam {
  0% {
    opacity: 0;
    transform: translateY(4px);
  }
  40% {
    opacity: .9;
  }
  100% {
    opacity: 0;
    transform: translateY(-9px);
  }
}
@keyframes ring {
  0%,65%,100% {
    transform: rotate(0);
  }
  75% {
    transform: rotate(-7deg);
  }
  85% {
    transform: rotate(7deg);
  }
}
@media(prefers-reduced-motion:reduce) {
  * {
    animation: none!important;
    transition: none!important;
  }
}
</style>
