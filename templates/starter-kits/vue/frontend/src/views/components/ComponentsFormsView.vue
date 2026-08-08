<template>
  <section class="grid gap-6">
    <PageHeader
      eyebrow="Components"
      section="Forms"
      title="Form and input patterns"
      description="Validation, selection, token entry, and staged setup examples arranged the way account settings, onboarding, and admin forms usually need them."
    />

    <Showcase
      title="Checkout and billing fields"
      description="One composed flow rather than a single control: billing identity, payment details, address, and order notes on one surface, with the order summary beside them."
      :source="CheckoutFormExampleSource"
    >
      <CheckoutFormExample />
    </Showcase>

    <div class="grid items-start gap-6 xl:grid-cols-2">
      <Specimen
        name="Form"
        description="Validation wired through vee-validate and zod, with messages attached to the field that produced them."
        :also="['Input', 'Button']"
        :source="FormExampleSource"
      >
        <FormExample />
      </Specimen>

      <Specimen
        name="Field"
        description="The label, description, and control wrapper. Orientation decides whether the control sits below the label or beside it."
        :also="['Input', 'Switch']"
        :source="FieldExampleSource"
      >
        <FieldExample />
      </Specimen>

      <Specimen
        name="Select"
        description="A styled listbox, and the native element for lower-importance defaults."
        :also="['NativeSelect', 'Label']"
        :source="SelectExampleSource"
      >
        <SelectExample />
      </Specimen>

      <Specimen
        name="Combobox"
        description="Filterable selection for when the option list outgrows a select."
        :also="['Button']"
        :source="ComboboxExampleSource"
      >
        <ComboboxExample />
      </Specimen>

      <Specimen
        name="RadioGroup"
        description="One choice from several. Radio cards give each option room for a description."
        :also="['FieldSet']"
        :source="RadioGroupExampleSource"
      >
        <RadioGroupExample />
      </Specimen>

      <Specimen
        name="Checkbox"
        description="Independent on and off choices, including a single acknowledgement."
        :also="['Label']"
        :source="CheckboxExampleSource"
      >
        <CheckboxExample />
      </Specimen>

      <Specimen
        name="Switch"
        description="An immediate setting rather than something you submit. Pair it with a descriptive row."
        :source="SwitchExampleSource"
      >
        <SwitchExample />
      </Specimen>

      <Specimen
        name="ToggleGroup"
        description="Segmented selection, and standalone toggles when the options are independent."
        :also="['Toggle', 'Label']"
        :source="ToggleGroupExampleSource"
      >
        <ToggleGroupExample />
      </Specimen>

      <Specimen
        name="Slider"
        description="Continuous values, with progress as the read-only counterpart."
        :also="['Progress', 'Label']"
        :source="SliderExampleSource"
      >
        <SliderExample />
      </Specimen>

      <Specimen
        name="NumberField"
        description="A bounded numeric control with increment and decrement affordances."
        :also="['Label']"
        :source="NumberFieldExampleSource"
      >
        <NumberFieldExample />
      </Specimen>

      <Specimen
        name="Textarea"
        description="Multi-line entry for notes, summaries, and messages."
        :also="['Label']"
        :source="TextareaExampleSource"
      >
        <TextareaExample />
      </Specimen>

      <Specimen
        name="Item"
        description="A row with content and trailing actions. Use for settings lists and account-security prompts."
        :also="['Button']"
        :source="ItemRowExampleSource"
      >
        <ItemRowExample />
      </Specimen>
    </div>

    <Specimen
      name="InputGroup"
      description="Wrap an input with leading or trailing addons — icons, prefixes, buttons, or a whole toolbar. Covers search, URL, token, and prompt entry."
      :also="['Textarea', 'Button', 'Badge']"
      :source="InputGroupExampleSource"
    >
      <InputGroupExample />
    </Specimen>

    <Specimen
      name="Stepper"
      description="Staged setup where each step carries a title, a description, and a completion state."
      :source="StepperExampleSource"
    >
      <StepperExample />
    </Specimen>

    <div class="grid gap-6 lg:grid-cols-3">
      <Specimen
        name="TagsInput"
        description="Free-form token entry for invites, labels, and filters."
        :source="TagsInputExampleSource"
      >
        <TagsInputExample />
      </Specimen>

      <Specimen
        name="InputOTP"
        description="Fixed-length one-time codes with grouped slots."
        :source="InputOTPExampleSource"
      >
        <InputOTPExample />
      </Specimen>

      <Specimen
        name="PinInput"
        description="Short numeric PINs where each digit is its own slot."
        :source="PinInputExampleSource"
      >
        <PinInputExample />
      </Specimen>
    </div>
  </section>
</template>

<script setup lang="ts">
import CheckoutFormExample from './examples/CheckoutFormExample.vue'
import CheckoutFormExampleSource from './examples/CheckoutFormExample.vue?raw'
import FormExample from './examples/FormExample.vue'
import FormExampleSource from './examples/FormExample.vue?raw'
import FieldExample from './examples/FieldExample.vue'
import FieldExampleSource from './examples/FieldExample.vue?raw'
import SelectExample from './examples/SelectExample.vue'
import SelectExampleSource from './examples/SelectExample.vue?raw'
import ComboboxExample from './examples/ComboboxExample.vue'
import ComboboxExampleSource from './examples/ComboboxExample.vue?raw'
import RadioGroupExample from './examples/RadioGroupExample.vue'
import RadioGroupExampleSource from './examples/RadioGroupExample.vue?raw'
import CheckboxExample from './examples/CheckboxExample.vue'
import CheckboxExampleSource from './examples/CheckboxExample.vue?raw'
import SwitchExample from './examples/SwitchExample.vue'
import SwitchExampleSource from './examples/SwitchExample.vue?raw'
import ToggleGroupExample from './examples/ToggleGroupExample.vue'
import ToggleGroupExampleSource from './examples/ToggleGroupExample.vue?raw'
import SliderExample from './examples/SliderExample.vue'
import SliderExampleSource from './examples/SliderExample.vue?raw'
import NumberFieldExample from './examples/NumberFieldExample.vue'
import NumberFieldExampleSource from './examples/NumberFieldExample.vue?raw'
import TextareaExample from './examples/TextareaExample.vue'
import TextareaExampleSource from './examples/TextareaExample.vue?raw'
import ItemRowExample from './examples/ItemRowExample.vue'
import ItemRowExampleSource from './examples/ItemRowExample.vue?raw'
import InputGroupExample from './examples/InputGroupExample.vue'
import InputGroupExampleSource from './examples/InputGroupExample.vue?raw'
import StepperExample from './examples/StepperExample.vue'
import StepperExampleSource from './examples/StepperExample.vue?raw'
import TagsInputExample from './examples/TagsInputExample.vue'
import TagsInputExampleSource from './examples/TagsInputExample.vue?raw'
import InputOTPExample from './examples/InputOTPExample.vue'
import InputOTPExampleSource from './examples/InputOTPExample.vue?raw'
import PinInputExample from './examples/PinInputExample.vue'
import PinInputExampleSource from './examples/PinInputExample.vue?raw'
import PageHeader from '@/components/PageHeader.vue'
import { Showcase, Specimen } from '@/components/showcase'
</script>
