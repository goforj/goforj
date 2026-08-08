// Identity for the generated application shell.
//
// appName defaults to the App directory name resolved by Vite, so a project
// generated as `billing-api` shows "Billing Api" rather than a template name.
// Override it with APP_NAME in your env file, or replace the values below
// outright once the product has its own branding.
export const appName: string = import.meta.env.VITE_APP_NAME || 'App'

// Links shown under Resources in the sidebar. These point at GoForj's own
// documentation because they are useful while you are finding your way
// around a freshly generated project. Replace or delete them once they are
// not — they are not required by anything.
export const resourceLinks = [
  { title: 'Repository', url: 'https://github.com/goforj/goforj' },
  { title: 'Documentation', url: 'https://goforj.dev' },
]
