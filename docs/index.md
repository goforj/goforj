---
# https://vitepress.dev/reference/default-theme-home-page
layout: home

hero:
  name: "GoForj"
  text: "Build faster. Ship smarter. Go development tools forged for productivity."
  image:
    src: ./assets/goforj-logo-v3.png
    alt: GoForj
  actions:
    - theme: brand
      text: Docs
      link: /about
    - theme: alt
      text: API Examples
      link: /api-examples

features:
  - title: Zero-Setup Scaffolding
    icon: 🚀
    details: Generate Go models, commands, controllers, jobs, and more with a single CLI command to eliminate boilerplate and speed up development.

  - title: Relationship-Aware Model Generation
    icon: 🧩
    details: Define database relationships in YAML and generate accurate GORM models with associations, constraints, and table metadata.

  - title: Command & Job Generators
    icon: ⚙️
    details: Create structured CLI commands and background job workers ready for Asynq integration in seconds.

  - title: HTTP Controller Generator
    icon: 🌐
    details: Generate RESTful HTTP controllers with route definitions, method stubs, and wire registration.

  - title: Go Module Awareness
    icon: 📦
    details: Automatically respects your project’s go.mod and generates files into the correct package namespaces.

  - title: Wire Dependency Injection Integration
    icon: 🪢
    details: Auto-inject generated commands, controllers, and jobs into your existing Wire dependency graph.

---

