<template>
  <div class="mb-6 flex items-start justify-between gap-6">
    <div>
      <slot name="title" />
      <slot name="description" />
    </div>
    <div>
      <slot name="action" />
    </div>
  </div>
</template>
