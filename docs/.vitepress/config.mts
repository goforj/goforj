import { defineConfig } from 'vitepress'

// https://vitepress.dev/reference/site-config
export default defineConfig({
  title: "GoForj",
  description: "Build faster. Ship smarter. Go development tools forged for productivity.",
  appearance: 'force-dark',

  head: [['link', { rel: 'icon', href: '../assets/goforj-12.png' }]],

  themeConfig: {
    // https://vitepress.dev/reference/default-theme-config
    search: {
      provider: 'local'
    },
    logo: '../assets/goforj-12.png',

    nav: [
      { text: 'Home', link: '/' },
      { text: 'What is GoForj?', link: '/about' }
    ],

    sidebar: [
      {
        text: 'Examples',
        items: [
          { text: 'Pages to Create', link: '/todo' },
          { text: 'What is GoForj?', link: '/about' },
          { text: 'Getting Started', link: '/getting-started' }
        ]
      }
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/vuejs/vitepress' }
    ]
  }
})
