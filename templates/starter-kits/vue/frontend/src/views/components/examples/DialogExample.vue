<script setup lang="ts">
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
</script>

<template>
  <Dialog>
    <DialogTrigger as-child>
      <Button>Open invite dialog</Button>
    </DialogTrigger>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Invite a teammate</DialogTitle>
        <DialogDescription>
          Use the same field patterns inside dialogs that the rest of the app uses.
        </DialogDescription>
      </DialogHeader>

      <div class="grid gap-4 py-2">
        <div class="grid gap-2">
          <Label>Email address</Label>
          <Input placeholder="person@example.com" />
        </div>
        <div class="grid gap-2">
          <Label>Role</Label>
          <Select default-value="admin">
            <SelectTrigger class="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="admin">Administrator</SelectItem>
              <SelectItem value="editor">Editor</SelectItem>
              <SelectItem value="viewer">Viewer</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      <DialogFooter>
        <DialogClose as-child>
          <Button variant="outline">Cancel</Button>
        </DialogClose>
        <Button>Send invite</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
