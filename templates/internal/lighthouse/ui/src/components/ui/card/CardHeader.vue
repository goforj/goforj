<template>
  <div data-slot="card-header" class="mb-3 flex items-start justify-between gap-6">
    <div>
      <slot name="title" />
      <slot name="description" />
    </div>
    <div>
      <slot name="action" />
    </div>
  </div>
</template>
