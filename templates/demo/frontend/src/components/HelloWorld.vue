<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'

defineProps<{ msg: string }>()

const count = ref(0)
const { t } = useI18n()
</script>

<template>
  <h1>{{ msg }}</h1>

  <div class="card">
    <button type="button" @click="count++">{{ t('helloWorld.countIs', { count }) }}</button>
    <p>
      {{ t('helloWorld.editToTestHmr') }}
    </p>
  </div>

  <p>
    {{ t('helloWorld.checkOut') }}
    <a href="https://vuejs.org/guide/quick-start.html#local" target="_blank"
      >create-vue</a
    >, {{ t('helloWorld.officialStarter') }}
  </p>
  <p>
    {{ t('helloWorld.learnMoreIdeSupport') }}
    <a
      href="https://vuejs.org/guide/scaling-up/tooling.html#ide-support"
      target="_blank"
      >Vue Docs Scaling up Guide</a
    >.
  </p>
  <p class="read-the-docs">{{ t('helloWorld.clickLogos') }}</p>
</template>

<style scoped>
.read-the-docs {
  color: #888;
}
</style>
