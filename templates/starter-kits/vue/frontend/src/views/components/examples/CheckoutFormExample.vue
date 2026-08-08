<script setup lang="ts">
import { ref } from 'vue'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSeparator,
  FieldSet,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  NumberField,
  NumberFieldContent,
  NumberFieldDecrement,
  NumberFieldIncrement,
  NumberFieldInput,
} from '@/components/ui/number-field'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

const expiryMonth = ref('')
const expiryYear = ref('')
const country = ref('us')
const sameAsShipping = ref(true)
const cadence = ref('monthly')
const seatCount = ref(8)
const emailReceipts = ref(true)

const months = ['01', '02', '03', '04', '05', '06', '07', '08', '09', '10', '11', '12']
const years = ['2026', '2027', '2028', '2029', '2030']
</script>

<template>
  <div class="grid gap-6 xl:grid-cols-[1.15fr_0.85fr]">
  <form class="grid content-start gap-6" @submit.prevent>
    <FieldSet>
      <FieldLegend>Payment Method</FieldLegend>
      <FieldDescription>All transactions are secure and encrypted.</FieldDescription>
      <FieldGroup>
        <Field>
          <FieldLabel>Name on Card</FieldLabel>
          <Input placeholder="John Doe" />
        </Field>

        <div class="grid grid-cols-3 gap-4">
          <Field class="col-span-2">
            <FieldLabel>Card Number</FieldLabel>
            <Input placeholder="1234 5678 9012 3456" />
            <FieldDescription>Enter your 16-digit number.</FieldDescription>
          </Field>

          <Field>
            <FieldLabel>CVV</FieldLabel>
            <Input placeholder="123" />
          </Field>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <Field>
            <FieldLabel>Month</FieldLabel>
            <Select v-model="expiryMonth">
              <SelectTrigger class="w-full">
                <SelectValue placeholder="MM" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem v-for="month in months" :key="month" :value="month">{{ month }}</SelectItem>
              </SelectContent>
            </Select>
          </Field>

          <Field>
            <FieldLabel>Year</FieldLabel>
            <Select v-model="expiryYear">
              <SelectTrigger class="w-full">
                <SelectValue placeholder="YYYY" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem v-for="year in years" :key="year" :value="year">{{ year }}</SelectItem>
              </SelectContent>
            </Select>
          </Field>
        </div>
      </FieldGroup>
    </FieldSet>

    <FieldSeparator />

    <FieldSet>
      <FieldLegend>Billing Address</FieldLegend>
      <FieldDescription>The billing address associated with your payment method.</FieldDescription>
      <FieldGroup class="grid gap-4 md:grid-cols-2">
        <Field class="md:col-span-2">
          <FieldLabel>Street address</FieldLabel>
          <Input placeholder="100 Market Street" />
        </Field>
        <Field>
          <FieldLabel>City</FieldLabel>
          <Input placeholder="San Francisco" />
        </Field>
        <Field>
          <FieldLabel>State / Region</FieldLabel>
          <Input placeholder="California" />
        </Field>
        <Field>
          <FieldLabel>Postal code</FieldLabel>
          <Input placeholder="94105" />
        </Field>
        <Field>
          <FieldLabel>Country</FieldLabel>
          <NativeSelect v-model="country">
            <NativeSelectOption value="us">United States</NativeSelectOption>
            <NativeSelectOption value="ca">Canada</NativeSelectOption>
            <NativeSelectOption value="eu">European Union</NativeSelectOption>
          </NativeSelect>
        </Field>
        <label class="flex items-center gap-3 rounded-lg border p-3 md:col-span-2">
          <Checkbox v-model="sameAsShipping" />
          <span class="text-sm">Same as shipping address</span>
        </label>
      </FieldGroup>
    </FieldSet>

    <FieldSeparator />

    <FieldSet>
      <FieldLegend>Order notes</FieldLegend>
      <FieldGroup>
        <Field>
          <FieldLabel>Comments</FieldLabel>
          <Textarea placeholder="Add any purchasing notes, PO references, or delivery requirements." rows="4" />
        </Field>
      </FieldGroup>
    </FieldSet>

    <div class="flex gap-2">
      <Button type="submit">Submit</Button>
      <Button type="button" variant="outline">Cancel</Button>
    </div>
  </form>

  <div class="grid content-start gap-4">
    <div class="grid content-start gap-4 rounded-lg bg-muted/40 p-4">
    <p class="text-sm font-medium leading-none">Order summary</p>
      <div class="grid gap-3">
        <div class="flex items-center justify-between text-sm">
          <span>Starter Kit License</span>
          <span>$299</span>
        </div>
        <div class="flex items-center justify-between text-sm">
          <span>Seats</span>
          <span>{{ seatCount }}</span>
        </div>
        <div class="flex items-center justify-between text-sm">
          <span>Tax</span>
          <span>$24</span>
        </div>
        <div class="border-t pt-3">
          <div class="flex items-center justify-between font-medium">
            <span>Total due today</span>
            <span>$323</span>
          </div>
        </div>
      </div>
    </div>

    <div class="grid gap-4 rounded-lg bg-muted/40 p-4">
    <p class="text-sm font-medium leading-none">Purchase controls</p>
      <div class="grid gap-4">
        <Field orientation="responsive">
          <FieldLabel>Billing cadence</FieldLabel>
          <FieldContent>
            <FieldDescription>Segmented controls work well for pricing or plan cadence selection.</FieldDescription>
          </FieldContent>
          <ToggleGroup v-model="cadence" type="single" class="justify-start">
            <ToggleGroupItem value="monthly">Monthly</ToggleGroupItem>
            <ToggleGroupItem value="annual">Annual</ToggleGroupItem>
          </ToggleGroup>
        </Field>

        <FieldSeparator />

        <Field orientation="responsive">
          <FieldLabel>Seat count</FieldLabel>
          <FieldContent>
            <FieldDescription>Quantity controls belong near checkout and provisioning flows.</FieldDescription>
          </FieldContent>
          <NumberField v-model="seatCount" :min="1" :max="25" class="w-32">
            <NumberFieldContent>
              <NumberFieldDecrement />
              <NumberFieldInput />
              <NumberFieldIncrement />
            </NumberFieldContent>
          </NumberField>
        </Field>

        <FieldSeparator />

        <label class="flex items-center gap-3 rounded-lg border p-3">
          <Checkbox v-model="emailReceipts" />
          <span class="text-sm">Email me receipts and renewal reminders</span>
        </label>
      </div>
    </div>
  </div>
  </div>
</template>
