<template>
  <div class="space-y-6">
    <div class="space-y-1">
      <h2 class="text-xl font-semibold tracking-tight">Profile</h2>
      <p class="text-sm text-muted-foreground">Update the account details exposed by generated auth.</p>
    </div>

    <form class="space-y-6" @submit.prevent="submitProfileUpdate">
      <div class="grid gap-2">
        <Label for="name">Name</Label>
        <Input id="name" v-model="name" autocomplete="name" placeholder="Full name" />
      </div>

      <div class="grid gap-2">
        <Label for="email">Email address</Label>
        <Input id="email" v-model="email" type="email" autocomplete="username" placeholder="Email address" />
      </div>

      <p class="text-sm text-muted-foreground">
        {{
          authState.user?.email_verified_at
            ? 'Your email address is verified and active for account recovery, sign-in alerts, and product notifications.'
            : 'Your email address is active for sign-in and notifications. Request verification after changing it.'
        }}
      </p>

      <StatusMessage v-if="errorMessage">{{ errorMessage }}</StatusMessage>

      <div class="flex items-center gap-4">
        <Button :disabled="saving">
          <LoaderCircle v-if="saving" class="size-4 animate-spin" />
          {{ saving ? 'Saving…' : 'Save' }}
        </Button>
      </div>
    </form>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { LoaderCircle } from '@lucide/vue'
import StatusMessage from '@/components/StatusMessage.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { authState, loadCurrentUser, updateProfile } from '@/lib/auth'

const name = ref('')
const email = ref('')
const saving = ref(false)
const errorMessage = ref('')

function syncFromAuthState() {
  name.value = authState.user?.display_name || authState.user?.username || ''
  email.value = authState.user?.email || ''
}

watch(() => authState.user, syncFromAuthState, { immediate: true })

onMounted(async () => {
  if (!authState.user) {
    await loadCurrentUser()
  }
  syncFromAuthState()
})

async function submitProfileUpdate() {
  errorMessage.value = ''
  if (!name.value.trim() || !email.value.trim()) {
    errorMessage.value = 'Name and email are required.'
    return
  }
  saving.value = true
  try {
    await updateProfile(name.value.trim(), email.value.trim())
    syncFromAuthState()
    toast.success('Profile updated')
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Unable to update your profile.'
    toast.error('Profile update failed', {
      description: errorMessage.value,
    })
  } finally {
    saving.value = false
  }
}
</script>
