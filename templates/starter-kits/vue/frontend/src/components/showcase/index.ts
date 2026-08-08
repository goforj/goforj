export { default as ComponentTag } from './ComponentTag.vue'
export { default as Showcase } from './Showcase.vue'
export { default as ShowcaseRow } from './ShowcaseRow.vue'
export { default as Specimen } from './Specimen.vue'
export { default as SpecimenSource } from './SpecimenSource.vue'

/** "Checkout and billing fields" -> "checkout-and-billing-fields" */
export function slugify(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

/** "InputGroup" -> "@/components/ui/input-group" */
export function componentImportPath(name: string): string {
  const dir = name
    .replace(/([a-z0-9])([A-Z])/g, '$1-$2')
    .replace(/([A-Z]+)([A-Z][a-z])/g, '$1-$2')
    .toLowerCase()
  return `@/components/ui/${dir}`
}
