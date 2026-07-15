<template>
  <section class="grid gap-6">
    <Card class="border-border/60">
      <CardHeader>
        <div class="flex flex-wrap items-center gap-2">
          <Badge>Components</Badge>
          <Badge variant="outline">Forms</Badge>
        </div>
        <CardTitle class="text-3xl">Form and input patterns</CardTitle>
        <CardDescription class="max-w-3xl">
          Validation, selection, token entry, and staged setup examples arranged the way account settings, onboarding, and admin forms usually need them.
        </CardDescription>
      </CardHeader>
    </Card>

    <div class="grid gap-4 xl:grid-cols-2">
      <Card class="xl:col-span-2">
        <CardHeader>
          <CardTitle>Checkout and billing fields</CardTitle>
          <CardDescription>
            Lead with a real transactional flow: billing identity, payment details, billing address, and order notes in one surface.
          </CardDescription>
        </CardHeader>
        <CardContent class="grid gap-6 xl:grid-cols-[1.15fr_0.85fr]">
          <form class="grid gap-5" @submit.prevent>
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
                  <NativeSelect v-model="nativeRegion">
                    <NativeSelectOption value="us">United States</NativeSelectOption>
                    <NativeSelectOption value="ca">Canada</NativeSelectOption>
                    <NativeSelectOption value="eu">European Union</NativeSelectOption>
                  </NativeSelect>
                </Field>
                <label class="flex items-center gap-3 rounded-lg border p-3 md:col-span-2">
                  <Checkbox v-model="billingMatchesShipping" />
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

          <div class="grid gap-4">
            <div class="rounded-xl border p-4">
              <p class="text-sm font-medium text-muted-foreground">Order summary</p>
              <div class="mt-4 grid gap-3">
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

            <div class="rounded-xl border p-4">
              <p class="font-medium">Purchase controls</p>
              <div class="mt-4 grid gap-4">
                <Field orientation="responsive">
                  <FieldLabel>Billing cadence</FieldLabel>
                  <FieldContent>
                    <FieldDescription>Segmented controls work well for pricing or plan cadence selection.</FieldDescription>
                  </FieldContent>
                  <ToggleGroup v-model="runtimeMode" type="single" class="justify-start">
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
                  <Checkbox v-model="featureChecks.analytics" />
                  <span class="text-sm">Email me receipts and renewal reminders</span>
                </label>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Account and project settings</CardTitle>
          <CardDescription>
            A more realistic settings editor with validation, profile ownership, access controls, and a release summary.
          </CardDescription>
        </CardHeader>
        <CardContent class="grid gap-6">
          <form class="grid gap-5" @submit.prevent="submitForm">
            <div class="flex items-start justify-between gap-4 rounded-xl border p-4">
              <div class="grid gap-1">
                <p class="font-medium">Project status</p>
                <p class="max-w-xl text-sm text-muted-foreground">
                  Use a dedicated settings row for simple on or off preferences instead of forcing a tiny switch into a descriptive field grid.
                </p>
              </div>
              <div class="flex shrink-0 items-center gap-3">
                <span class="text-sm text-muted-foreground">{{ notificationsEnabled ? 'Enabled' : 'Disabled' }}</span>
                <Switch v-model="notificationsEnabled" />
              </div>
            </div>

            <div class="grid gap-4 md:grid-cols-2">
              <FormField v-slot="{ componentField }" name="name">
                <FormItem>
                  <FormLabel>Project name</FormLabel>
                  <FormControl>
                    <Input v-bind="componentField" placeholder="GoForj Admin" />
                  </FormControl>
                  <FormDescription>Validation messages are wired through the local form helpers.</FormDescription>
                  <FormMessage />
                </FormItem>
              </FormField>

              <FormField v-slot="{ componentField }" name="ownerEmail">
                <FormItem>
                  <FormLabel>Owner email</FormLabel>
                  <FormControl>
                    <Input v-bind="componentField" placeholder="team@example.com" />
                  </FormControl>
                  <FormDescription>Use this pattern for profile forms, invite flows, and account settings.</FormDescription>
                  <FormMessage />
                </FormItem>
              </FormField>
            </div>

            <div class="grid gap-4 md:grid-cols-2">
              <div class="grid gap-2">
                <Label>API token</Label>
                <InputGroup>
                  <InputGroupAddon>API</InputGroupAddon>
                  <InputGroupInput value="token_live_4c4c..." />
                  <InputGroupButton variant="outline">Copy</InputGroupButton>
                </InputGroup>
              </div>

              <div class="grid gap-2">
                <Label>Starter profile</Label>
                <Select v-model="starterProfile">
                  <SelectTrigger class="w-full">
                    <SelectValue placeholder="Choose a profile" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="vue">Vue + Shadcn-Vue</SelectItem>
                    <SelectItem value="auth">Auth shell</SelectItem>
                    <SelectItem value="dashboard">Dashboard baseline</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <FieldSet class="grid gap-3 rounded-xl border p-4">
              <FieldLegend>Release controls</FieldLegend>
              <FieldGroup class="grid gap-3">
                <Field orientation="responsive">
                  <FieldLabel>Environment</FieldLabel>
                  <FieldContent>
                    <FieldDescription>Use native selects for lower-importance environment defaults.</FieldDescription>
                  </FieldContent>
                  <NativeSelect v-model="nativeRegion">
                    <NativeSelectOption value="iad">US East</NativeSelectOption>
                    <NativeSelectOption value="sfo">US West</NativeSelectOption>
                    <NativeSelectOption value="fra">Europe</NativeSelectOption>
                  </NativeSelect>
                </Field>
                <FieldSeparator />
                <Field orientation="responsive">
                  <FieldLabel>Frontend stack</FieldLabel>
                  <FieldContent>
                    <FieldDescription>Comboboxes help when stack or provider selection grows beyond a handful of options.</FieldDescription>
                  </FieldContent>
                  <Combobox v-model="selectedFramework">
                    <ComboboxAnchor class="w-full">
                      <div class="relative">
                        <ComboboxInput class="pr-9" placeholder="Search framework..." />
                        <ComboboxTrigger as-child>
                          <Button variant="ghost" size="icon" class="absolute top-1 right-1 size-7">
                            <ChevronsUpDown class="size-4" />
                          </Button>
                        </ComboboxTrigger>
                      </div>
                    </ComboboxAnchor>
                    <ComboboxList class="w-[var(--reka-combobox-trigger-width)]">
                      <ComboboxEmpty>No framework found.</ComboboxEmpty>
                      <ComboboxViewport>
                        <ComboboxGroup>
                          <ComboboxItem v-for="framework in frameworks" :key="framework" :value="framework">
                            {{ framework }}
                            <ComboboxItemIndicator class="ml-auto">
                              <Check class="size-4" />
                            </ComboboxItemIndicator>
                          </ComboboxItem>
                        </ComboboxGroup>
                      </ComboboxViewport>
                    </ComboboxList>
                  </Combobox>
                </Field>
              </FieldGroup>
            </FieldSet>

            <div class="grid gap-4 rounded-xl border p-4">
              <div class="grid gap-2">
                <Label>Summary</Label>
                <Textarea v-model="summary" rows="4" placeholder="Describe the product screen you want to build next." />
              </div>

              <div class="grid gap-3 sm:grid-cols-2">
                <label class="flex items-center gap-3 rounded-lg border p-3">
                  <Checkbox v-model="featureChecks.analytics" />
                  <span class="text-sm">Analytics</span>
                </label>
                <label class="flex items-center gap-3 rounded-lg border p-3">
                  <Checkbox v-model="featureChecks.auditLogs" />
                  <span class="text-sm">Audit logs</span>
                </label>
              </div>

              <RadioGroup v-model="deploymentMode" class="grid gap-2">
                <label class="flex items-center gap-3 rounded-lg border p-3">
                  <RadioGroupItem value="local" />
                  <span class="text-sm">Local deploy</span>
                </label>
                <label class="flex items-center gap-3 rounded-lg border p-3">
                  <RadioGroupItem value="cloud" />
                  <span class="text-sm">Managed deploy</span>
                </label>
              </RadioGroup>
            </div>

            <div class="flex flex-wrap gap-2">
              <Button type="submit">Save settings</Button>
              <Button type="button" variant="outline" @click="notifyPreview">Preview toast</Button>
            </div>
          </form>
        </CardContent>
      </Card>

      <Card>
          <CardHeader>
            <CardTitle>Input groups and assistant surfaces</CardTitle>
            <CardDescription>
              Search, URL, prompt, and handle-entry patterns tend to rely on `input-group` more than plain text inputs.
            </CardDescription>
          </CardHeader>
          <CardContent class="grid gap-5">
            <div class="grid gap-4 md:grid-cols-2">
              <div class="grid gap-2 md:col-span-2">
                <Label>Command bar</Label>
                <InputGroup class="[--radius:9999px]">
                  <InputGroupAddon>
                    <Search class="size-4" />
                  </InputGroupAddon>
                  <InputGroupInput placeholder="Search docs, commands, and project resources..." />
                  <InputGroupAddon align="inline-end">
                    <InputGroupText>Ctrl K</InputGroupText>
                  </InputGroupAddon>
                </InputGroup>
              </div>

              <div class="grid gap-2">
                <Label>Search input</Label>
                <InputGroup>
                  <InputGroupAddon>
                    <Search class="size-4" />
                  </InputGroupAddon>
                  <InputGroupInput placeholder="Search..." />
                  <InputGroupAddon align="inline-end">12 results</InputGroupAddon>
                </InputGroup>
              </div>

              <div class="grid gap-2">
                <Label>URL input</Label>
                <InputGroup>
                  <InputGroupAddon>
                    <InputGroupText>https://</InputGroupText>
                  </InputGroupAddon>
                  <InputGroupInput placeholder="example.com" class="!pl-1" />
                  <InputGroupAddon align="inline-end">
                    <InputGroupButton variant="ghost" aria-label="Info">
                      <Info class="size-4" />
                    </InputGroupButton>
                  </InputGroupAddon>
                </InputGroup>
              </div>

              <div class="grid gap-2 md:col-span-2">
                <Label>Message composer</Label>
                <InputGroup class="h-auto">
                  <InputGroupTextarea placeholder="Ask, Search or Chat..." class="min-h-24" />
                  <InputGroupAddon align="block-end" class="w-full justify-start gap-2 border-t pt-2">
                    <Button variant="outline" size="icon" class="rounded-full" aria-label="Add attachment">
                      <Plus class="size-4" />
                    </Button>
                    <Button variant="ghost" size="sm" class="rounded-full px-2">Auto</Button>
                    <span class="ml-auto text-sm text-muted-foreground">52% used</span>
                    <Button size="icon" class="rounded-full" :disabled="true" aria-label="Send">
                      <ArrowUp class="size-4" />
                    </Button>
                  </InputGroupAddon>
                </InputGroup>
              </div>

              <div class="grid gap-2 md:col-span-2 rounded-xl border p-4">
                <div class="grid gap-1">
                  <p class="font-medium">Assistant handoff</p>
                  <p class="text-sm text-muted-foreground">
                    Compose richer prompts with source scope, mode selection, and a named destination before sending.
                  </p>
                </div>

                <InputGroup class="h-auto">
                  <InputGroupTextarea placeholder="Describe what the assistant should investigate, change, or summarize." class="min-h-28" />
                  <InputGroupAddon align="block-start" class="w-full justify-start gap-2 border-b pb-2">
                    <Button variant="outline" size="sm" class="rounded-full px-3">@ Add context</Button>
                    <Badge variant="outline">frontend</Badge>
                    <Badge variant="outline">billing</Badge>
                    <Badge variant="outline">starter-kit</Badge>
                  </InputGroupAddon>
                  <InputGroupAddon align="block-end" class="w-full justify-start gap-2 border-t pt-2">
                    <Button variant="ghost" size="sm" class="rounded-full px-2">Auto</Button>
                    <Button variant="ghost" size="sm" class="rounded-full px-2">All sources</Button>
                    <span class="ml-auto text-sm text-muted-foreground">Draft response mode</span>
                    <Button size="icon" class="rounded-full" aria-label="Send handoff">
                      <ArrowUp class="size-4" />
                    </Button>
                  </InputGroupAddon>
                </InputGroup>
              </div>

              <div class="grid gap-2">
                <Label>Secure input</Label>
                <InputGroup class="[--radius:9999px]">
                  <InputGroupAddon>
                    <InputGroupButton variant="secondary" aria-label="Info">
                      <Info class="size-4" />
                    </InputGroupButton>
                  </InputGroupAddon>
                  <InputGroupAddon class="!pl-1">
                    <InputGroupText>https://</InputGroupText>
                  </InputGroupAddon>
                  <InputGroupInput placeholder="example.com" class="!pl-1" />
                  <InputGroupAddon align="inline-end">
                    <InputGroupButton variant="ghost" aria-label="Favorite">
                      <Star class="size-4" />
                    </InputGroupButton>
                  </InputGroupAddon>
                </InputGroup>
              </div>

              <div class="grid gap-2">
                <Label>Webhook endpoint</Label>
                <InputGroup>
                  <InputGroupAddon>
                    <InputGroupText>POST</InputGroupText>
                  </InputGroupAddon>
                  <InputGroupInput placeholder="/api/v1/webhooks/stripe" />
                  <InputGroupAddon align="inline-end">
                    <InputGroupButton variant="outline">Copy</InputGroupButton>
                  </InputGroupAddon>
                </InputGroup>
              </div>

              <div class="grid gap-2">
                <Label>Verified handle</Label>
                <InputGroup>
                  <InputGroupInput placeholder="@shadcn" />
                  <InputGroupAddon align="inline-end">
                    <div class="flex size-4 items-center justify-center rounded-full bg-primary text-primary-foreground">
                      <Check class="size-3" />
                    </div>
                  </InputGroupAddon>
                </InputGroup>
              </div>

              <div class="grid gap-2 md:col-span-2 rounded-xl border p-4">
                <div class="grid gap-1 md:grid-cols-[1fr_auto] md:items-start">
                  <div class="grid gap-1">
                    <p class="font-medium">Domain and callback routing</p>
                    <p class="text-sm text-muted-foreground">
                      Configuration screens often mix URL prefixes, callback paths, and verification helpers in one block.
                    </p>
                  </div>
                  <Button variant="outline" size="sm">Validate</Button>
                </div>

                <div class="mt-4 grid gap-3">
                  <InputGroup>
                    <InputGroupAddon>
                      <InputGroupText>https://</InputGroupText>
                    </InputGroupAddon>
                    <InputGroupInput placeholder="app.example.com" />
                    <InputGroupAddon align="inline-end">
                      <InputGroupText>Primary domain</InputGroupText>
                    </InputGroupAddon>
                  </InputGroup>

                  <InputGroup>
                    <InputGroupAddon>
                      <InputGroupText>Callback</InputGroupText>
                    </InputGroupAddon>
                    <InputGroupInput placeholder="/auth/callback" />
                    <InputGroupAddon align="inline-end">
                      <div class="flex size-4 items-center justify-center rounded-full bg-primary text-primary-foreground">
                        <Check class="size-3" />
                      </div>
                    </InputGroupAddon>
                  </InputGroup>
                </div>
              </div>
            </div>
          </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Identity and access verification</CardTitle>
          <CardDescription>
            OTP, backup PINs, invite tags, and account-security prompts belong together when the flow is about trust and activation.
          </CardDescription>
        </CardHeader>
        <CardContent class="grid gap-5">
          <div class="grid gap-3">
            <Item variant="outline">
              <ItemContent>
                <ItemTitle>Two-factor authentication</ItemTitle>
                <ItemDescription>Verify via email or phone number.</ItemDescription>
              </ItemContent>
              <ItemActions>
                <Button size="sm">Enable</Button>
              </ItemActions>
            </Item>

            <Item as="a" href="#" variant="outline" size="sm">
              <ItemMedia>
                <BadgeCheck class="size-5" />
              </ItemMedia>
              <ItemContent>
                <ItemTitle>Your profile has been verified.</ItemTitle>
              </ItemContent>
              <ItemActions>
                <ChevronRight class="size-4" />
              </ItemActions>
            </Item>
          </div>

          <div class="grid gap-4 rounded-xl border p-4">
            <div class="grid gap-2">
              <Label>Invite tags</Label>
              <TagsInput v-model="tags">
                <TagsInputItem v-for="tag in tags" :key="tag" :value="tag">
                  <TagsInputItemText />
                  <TagsInputItemDelete />
                </TagsInputItem>
                <TagsInputInput placeholder="Add tag..." />
              </TagsInput>
            </div>

            <div class="grid gap-2">
              <Label>Email verification</Label>
              <InputOTP v-model="otpCode" :maxlength="6">
                <template #default="{ slots }">
                  <InputOTPGroup>
                    <InputOTPSlot v-for="(_, index) in slots.slice(0, 3)" :key="index" :index="index" />
                  </InputOTPGroup>
                  <InputOTPSeparator />
                  <InputOTPGroup>
                    <InputOTPSlot v-for="(_, index) in slots.slice(3, 6)" :key="index + 3" :index="index + 3" />
                  </InputOTPGroup>
                </template>
              </InputOTP>
            </div>

            <div class="grid gap-2">
              <Label>Backup PIN</Label>
              <PinInput v-model="pinCode" class="justify-between">
                <PinInputGroup class="gap-2">
                  <PinInputSlot v-for="index in 4" :key="index" :index="index - 1" />
                </PinInputGroup>
              </PinInput>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Recovery and session controls</CardTitle>
          <CardDescription>
            Keep trusted fallback contacts, recovery routing, and active device actions beside the identity surface instead of burying them inside it.
          </CardDescription>
        </CardHeader>
        <CardContent class="grid gap-5">
          <div class="rounded-xl border p-4">
            <div class="grid gap-1">
              <p class="font-medium">Access recovery</p>
              <p class="text-sm text-muted-foreground">Security flows usually need trusted fallback contacts and recovery routing.</p>
            </div>

            <div class="mt-4 grid gap-4">
              <div class="grid gap-2">
                <Label>Recovery email</Label>
                <Input placeholder="security@example.com" />
              </div>

              <div class="grid gap-2">
                <Label>Recovery phone</Label>
                <InputGroup>
                  <InputGroupAddon>
                    <InputGroupText>+1</InputGroupText>
                  </InputGroupAddon>
                  <InputGroupInput placeholder="415 555 0188" />
                  <InputGroupAddon align="inline-end">
                    <Badge variant="outline">Verified</Badge>
                  </InputGroupAddon>
                </InputGroup>
              </div>

              <div class="grid gap-2">
                <Label>Recovery method</Label>
                <RadioGroup v-model="deploymentMode" class="grid gap-2">
                  <label class="flex items-center gap-3 rounded-lg border p-3">
                    <RadioGroupItem value="local" />
                    <span class="text-sm">Email first</span>
                  </label>
                  <label class="flex items-center gap-3 rounded-lg border p-3">
                    <RadioGroupItem value="cloud" />
                    <span class="text-sm">SMS first</span>
                  </label>
                </RadioGroup>
              </div>
            </div>
          </div>

          <div class="rounded-xl border p-4">
            <div class="grid gap-1">
              <p class="font-medium">Session and device controls</p>
              <p class="text-sm text-muted-foreground">Pair switches and device-level actions with account protection surfaces.</p>
            </div>

            <div class="mt-4 grid gap-3">
              <Field orientation="horizontal">
                <FieldContent>
                  <FieldLabel>Require device approval</FieldLabel>
                  <FieldDescription>New sign-ins must be approved from a trusted session.</FieldDescription>
                </FieldContent>
                <Switch v-model="notificationsEnabled" />
              </Field>

              <FieldSeparator />

              <Field orientation="horizontal">
                <FieldContent>
                  <FieldLabel>Session timeout</FieldLabel>
                  <FieldDescription>Automatically expire inactive sessions after 30 minutes.</FieldDescription>
                </FieldContent>
                <Switch v-model="wallpaperTinting" />
              </Field>

              <FieldSeparator />

              <Item variant="outline" size="sm">
                <ItemContent>
                  <ItemTitle>MacBook Pro · San Francisco</ItemTitle>
                  <ItemDescription>Last active 2 minutes ago on Chrome 136.</ItemDescription>
                </ItemContent>
                <ItemActions>
                  <Button variant="outline" size="sm">Revoke</Button>
                </ItemActions>
              </Item>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card class="xl:col-span-2">
        <CardHeader>
          <CardTitle>Environment controls and staged rollout</CardTitle>
          <CardDescription>
            Radio cards, quantity controls, segmented selection, and staged setup examples work best when they are tied to an actual launch workflow.
          </CardDescription>
        </CardHeader>
        <CardContent class="grid gap-6 xl:grid-cols-[1fr_0.92fr]">
          <div class="grid gap-5">
            <FieldSeparator>Appearance Settings</FieldSeparator>

            <FieldSet>
              <FieldLegend>Compute Environment</FieldLegend>
              <FieldDescription>Select the compute environment for your cluster.</FieldDescription>
              <RadioGroup v-model="computeEnvironment" class="grid gap-2">
                <label class="flex items-start gap-3 rounded-xl border p-4 transition has-[:checked]:border-primary/30 has-[:checked]:bg-primary/8">
                  <div class="grid flex-1 gap-1">
                    <span class="font-medium">Kubernetes</span>
                    <span class="text-sm text-muted-foreground">Run GPU workloads on a K8s configured cluster. This is the default.</span>
                  </div>
                  <RadioGroupItem value="kubernetes" />
                </label>
                <label class="flex items-start gap-3 rounded-xl border p-4 transition has-[:checked]:border-primary/30 has-[:checked]:bg-primary/8">
                  <div class="grid flex-1 gap-1">
                    <span class="font-medium">Virtual Machine</span>
                    <span class="text-sm text-muted-foreground">Access a VM configured cluster to run workloads. (Coming soon)</span>
                  </div>
                  <RadioGroupItem value="vm" />
                </label>
              </RadioGroup>
            </FieldSet>

            <FieldSeparator />

            <Field orientation="horizontal">
              <FieldContent>
                <FieldLabel>Number of GPUs</FieldLabel>
                <FieldDescription>You can add more later.</FieldDescription>
              </FieldContent>
              <div class="flex shrink-0 items-center gap-2">
                <Input class="h-8 w-14 font-mono" :model-value="String(gpuCount)" />
                <Button variant="outline" size="icon" aria-label="Decrement" @click="gpuCount = Math.max(1, gpuCount - 1)">
                  <Minus class="size-4" />
                </Button>
                <Button variant="outline" size="icon" aria-label="Increment" @click="gpuCount += 1">
                  <Plus class="size-4" />
                </Button>
              </div>
            </Field>

            <FieldSeparator />

            <Field orientation="horizontal">
              <FieldContent>
                <FieldLabel>Wallpaper Tinting</FieldLabel>
                <FieldDescription>Allow the wallpaper to be tinted.</FieldDescription>
              </FieldContent>
              <Switch v-model="wallpaperTinting" />
            </Field>

            <FieldSet>
              <FieldLegend>How did you hear about us?</FieldLegend>
              <FieldDescription>Select the option that best describes how you heard about us.</FieldDescription>
              <div class="flex flex-wrap gap-2 [--radius:9999px]">
                <button
                  v-for="option in audienceOptions"
                  :key="option"
                  type="button"
                  class="inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-sm transition"
                  :class="selectedAudience.includes(option) ? 'border-primary/30 bg-primary/10 text-foreground' : 'border-border bg-background'"
                  @click="toggleAudienceOption(option)"
                >
                  <span
                    class="flex size-4 items-center justify-center rounded-full border transition"
                    :class="selectedAudience.includes(option) ? 'border-primary bg-primary text-primary-foreground' : 'border-input'"
                  >
                    <Check v-if="selectedAudience.includes(option)" class="size-3" />
                  </span>
                  {{ option }}
                </button>
              </div>
            </FieldSet>
          </div>

          <div class="grid gap-6">
            <div class="grid gap-4 rounded-xl border p-4">
              <div class="grid gap-2">
                <p class="font-medium">Rollout controls</p>
                <p class="text-sm text-muted-foreground">Use sliders and segmented controls to tune staged releases and internal launches.</p>
              </div>

              <Field orientation="responsive">
                <FieldLabel>Release channel</FieldLabel>
                <FieldContent>
                  <FieldDescription>Toggle between local builds and a connected cloud environment.</FieldDescription>
                </FieldContent>
                <ToggleGroup v-model="runtimeMode" type="single" class="justify-start">
                  <ToggleGroupItem value="local">Internal</ToggleGroupItem>
                  <ToggleGroupItem value="cloud">Public</ToggleGroupItem>
                </ToggleGroup>
              </Field>

              <FieldSeparator />

              <Field orientation="responsive">
                <FieldLabel>Feature density</FieldLabel>
                <FieldContent>
                  <FieldDescription>A slider or number field works well for sizing and quota style controls.</FieldDescription>
                </FieldContent>
                <div class="w-full max-w-xs space-y-3">
                  <Slider v-model="density" :max="100" :step="5" />
                </div>
              </Field>
            </div>

            <div class="grid gap-6 rounded-xl border p-4">
              <Stepper v-model="activeStep" class="flex-col gap-2">
                <StepperItem v-for="step in setupSteps" :key="step.step" :step="step.step" class="items-start">
                  <div class="grid w-full gap-2">
                    <StepperTrigger as-child>
                      <button class="grid w-full grid-cols-[auto_1fr] items-start gap-3 rounded-xl border border-transparent p-3 text-left transition group-data-[state=active]:border-primary/25 group-data-[state=active]:bg-primary/10 group-data-[state=completed]:bg-muted/30 group-data-[state=inactive]:opacity-65">
                        <StepperIndicator class="mt-0.5 group-data-[state=inactive]:bg-transparent group-data-[state=inactive]:text-muted-foreground/60">
                          <CircleCheckBig v-if="activeStep > step.step" class="size-4" />
                          <span v-else>{{ step.step }}</span>
                        </StepperIndicator>
                        <div class="grid gap-1">
                          <StepperTitle class="whitespace-normal leading-tight group-data-[state=inactive]:text-muted-foreground">{{ step.title }}</StepperTitle>
                          <StepperDescription class="text-sm group-data-[state=inactive]:text-muted-foreground/80">{{ step.description }}</StepperDescription>
                        </div>
                      </button>
                    </StepperTrigger>
                    <StepperSeparator v-if="step.step !== setupSteps.length" class="ml-4 h-8 w-px group-data-[state=inactive]:bg-muted/40" />
                  </div>
                </StepperItem>
              </Stepper>

              <div class="grid gap-3 rounded-xl border p-4">
                <div class="flex items-center justify-between text-sm">
                  <span class="text-muted-foreground">Project setup</span>
                  <span class="font-medium">{{ progressValue }}%</span>
                </div>
                <Progress :model-value="progressValue" />
                <div class="flex flex-wrap gap-2">
                  <Toggle variant="outline" pressed aria-label="Toggle preview">Preview</Toggle>
                  <Toggle variant="outline" aria-label="Toggle logs">Logs</Toggle>
                  <Toggle variant="outline" aria-label="Toggle metrics">Metrics</Toggle>
                </div>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

    </div>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useForm } from 'vee-validate'
import { toTypedSchema } from '@vee-validate/zod'
import { toast } from 'vue-sonner'
import { z } from 'zod'
import { ArrowUp, BadgeCheck, Check, ChevronRight, ChevronsUpDown, CircleCheckBig, Info, Minus, Plus, Search, Star } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Combobox,
  ComboboxAnchor,
  ComboboxEmpty,
  ComboboxGroup,
  ComboboxInput,
  ComboboxItem,
  ComboboxItemIndicator,
  ComboboxList,
  ComboboxTrigger,
  ComboboxViewport,
} from '@/components/ui/combobox'
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
import { FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput, InputGroupText, InputGroupTextarea } from '@/components/ui/input-group'
import { InputOTP, InputOTPGroup, InputOTPSeparator, InputOTPSlot } from '@/components/ui/input-otp'
import { Item, ItemActions, ItemContent, ItemDescription, ItemMedia, ItemTitle } from '@/components/ui/item'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { NumberField, NumberFieldContent, NumberFieldDecrement, NumberFieldIncrement, NumberFieldInput } from '@/components/ui/number-field'
import { PinInput, PinInputGroup, PinInputSlot } from '@/components/ui/pin-input'
import { Progress } from '@/components/ui/progress'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Slider } from '@/components/ui/slider'
import { Stepper, StepperDescription, StepperIndicator, StepperItem, StepperSeparator, StepperTitle, StepperTrigger } from '@/components/ui/stepper'
import { Switch } from '@/components/ui/switch'
import { TagsInput, TagsInputInput, TagsInputItem, TagsInputItemDelete, TagsInputItemText } from '@/components/ui/tags-input'
import { Textarea } from '@/components/ui/textarea'
import { Toggle } from '@/components/ui/toggle'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

const notificationsEnabled = ref(true)
const density = ref([60])
const deploymentMode = ref('local')
const starterProfile = ref('vue')
const nativeRegion = ref('iad')
const selectedFramework = ref('Vue')
const runtimeMode = ref('local')
const seatCount = ref(8)
const summary = ref('Use this page as a working reference while you shape your own product surfaces.')
const activeStep = ref(2)
const tags = ref(['admin', 'finance', 'ops'])
const otpCode = ref('')
const pinCode = ref(['1', '4', '8', '2'])
const progressValue = 72
const expiryMonth = ref('')
const expiryYear = ref('')
const billingMatchesShipping = ref(true)
const computeEnvironment = ref('kubernetes')
const gpuCount = ref(8)
const wallpaperTinting = ref(true)
const selectedAudience = ref(['Social Media'])

const featureChecks = ref({
  analytics: true,
  auditLogs: false,
})

const frameworks = ['Vue', 'Nuxt', 'React', 'Svelte', 'Laravel Blade']
const months = ['01', '02', '03', '04', '05', '06', '07', '08', '09', '10', '11', '12']
const years = ['2026', '2027', '2028', '2029', '2030']
const audienceOptions = ['Social Media', 'Search Engine', 'Referral', 'Other']

const setupSteps = [
  { step: 1, title: 'Choose the shell', description: 'Sidebar, navbar, and route structure.' },
  { step: 2, title: 'Connect auth', description: 'Sign-in, me lookup, and logout.' },
  { step: 3, title: 'Ship the first workflow', description: 'Replace examples with product behavior.' },
]

const componentSchema = toTypedSchema(z.object({
  name: z.string().min(3, 'Project name must be at least 3 characters.'),
  ownerEmail: z.string().email('Enter a valid email address.'),
}))

const { handleSubmit } = useForm({
  validationSchema: componentSchema,
  initialValues: {
    name: 'GoForj Starter Kit',
    ownerEmail: 'team@example.com',
  },
})

const submitForm = handleSubmit((values) => {
  toast.success(`Saved ${values.name}`, {
    description: `Owner: ${values.ownerEmail}`,
  })
})

function notifyPreview() {
  toast.success('Starter preview saved', {
    description: 'This toast is rendered by the local shadcn-vue sonner wrapper.',
  })
}

function toggleAudienceOption(option: string) {
  if (selectedAudience.value.includes(option)) {
    selectedAudience.value = selectedAudience.value.filter(item => item !== option)
    return
  }

  selectedAudience.value = [...selectedAudience.value, option]
}
</script>
