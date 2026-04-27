<template>
  <div class="space-y-6">
    <div class="space-y-1">
      <h2 class="text-xl font-semibold tracking-tight">Password</h2>
      <p class="text-sm text-muted-foreground">Ensure your account is using a long, random password to stay secure</p>
    </div>

    <form class="space-y-6" @submit.prevent="submitPasswordChange">
      <div class="grid gap-2">
        <Label for="current-password">Current password</Label>
        <Input id="current-password" v-model="currentPassword" type="password" autocomplete="current-password" placeholder="Current password" />
      </div>

      <div class="grid gap-2">
        <Label for="new-password">New password</Label>
        <Input id="new-password" v-model="newPassword" type="password" autocomplete="new-password" placeholder="New password" />
        <p class="text-xs text-muted-foreground">
          {{ passwordRulesText }}
        </p>
      </div>

      <div class="grid gap-2">
        <Label for="confirm-password">Confirm password</Label>
        <Input id="confirm-password" v-model="confirmPassword" type="password" autocomplete="new-password" placeholder="Confirm password" />
      </div>

      <p v-if="errorMessage" class="text-sm text-destructive">
        {{ errorMessage }}
      </p>

      <div class="flex items-center gap-4">
        <Button :disabled="saving">
          {{ saving ? 'Saving…' : 'Save password' }}
        </Button>
      </div>
    </form>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { changePassword } from '@/lib/auth'
import { passwordRequirementsText } from '@/lib/password-policy'

const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const saving = ref(false)
const errorMessage = ref('')
const passwordRulesText = passwordRequirementsText()

async function submitPasswordChange() {
  errorMessage.value = ''

  if (!currentPassword.value || !newPassword.value || !confirmPassword.value) {
    errorMessage.value = 'Enter your current password and confirm the new password.'
    return
  }

  if (newPassword.value !== confirmPassword.value) {
    errorMessage.value = 'New password and confirmation must match.'
    return
  }

  saving.value = true
  try {
    await changePassword(currentPassword.value, newPassword.value)
    currentPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
    toast.success('Password updated')
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Unable to update your password.'
    toast.error('Password update failed', {
      description: errorMessage.value,
    })
  } finally {
    saving.value = false
  }
}
</script>
