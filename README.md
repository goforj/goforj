<p align="center">
  <a href="https://goforj.dev">
    <img src="https://raw.githubusercontent.com/goforj/docs/e63d7a15a5c6e4d7b4a2b930d10913433d90da33/docs/public/twitter-banner.png" width="100%" alt="GoForj: The composable stack for building with Go, illustrated by the Foundry workbench">
  </a>
</p>

<p align="center">
  <a href="https://github.com/goforj/goforj/actions/workflows/test.yml?query=branch%3Amain+event%3Apush"><img src="https://img.shields.io/github/actions/workflow/status/goforj/goforj/test.yml?branch=main&amp;event=push&amp;label=CI&amp;logo=githubactions&amp;logoColor=white&amp;style=flat-square&amp;labelColor=17141f" alt="CI status"></a>
  <a href="https://github.com/goforj/goforj/blob/main/.github/workflows/test.yml#L47"><img src="https://img.shields.io/badge/tests-unit%20%2B%20integration-ff8052?style=flat-square&amp;labelColor=17141f" alt="tests: unit + integration"></a>
  <a href="https://github.com/goforj/goforj/blob/main/.github/workflows/test.yml#L119"><img src="https://img.shields.io/badge/runtime-race%20%2B%20containers-ff8052?style=flat-square&amp;labelColor=17141f" alt="runtime: race + containers"></a>
  <a href="https://github.com/goforj/goforj/blob/main/.github/workflows/test.yml#L266"><img src="https://img.shields.io/badge/apps-generation%20%2B%20Lighthouse-ff8052?style=flat-square&amp;labelColor=17141f" alt="apps: generation + Lighthouse"></a>
  <a href="https://github.com/goforj/goforj/blob/main/.github/workflows/test.yml#L83"><img src="https://img.shields.io/badge/platforms-Linux%20%7C%20macOS%20%7C%20Windows-ff8052?style=flat-square&amp;labelColor=17141f" alt="platforms: Linux | macOS | Windows"></a>
</p>

# GoForj

GoForj is a composable application stack for Go. Build HTTP APIs, background workers, scheduled jobs, command-line tools, and web apps with explicit dependency injection and local-first drivers.

The `forj` CLI creates the project structure and wires the components you select. Application code stays in your repository. HTTP, queues, events, cache, storage, mail, and other [first-party libraries](https://goforj.dev/libraries/) can also be used independently.

## Install

Requires Go 1.26 or newer. Make sure Go's binary directory is on your `PATH`.

```sh
go install github.com/goforj/goforj/cmd/forj@latest
forj --help
```

The help output confirms that the CLI is installed. To create a project, run the interactive wizard:

```sh
forj new
```

Follow the [Quickstart](https://goforj.dev/getting-started/quickstart) to choose components, start the app, make a request, and run its tests.

## Explore

- [Project structure](https://goforj.dev/getting-started/project-structure): where application code, configuration, and wiring belong.
- [Runnable scenarios](https://goforj.dev/scenarios/): examples for routes, jobs, events, storage, and observability.
- [Standalone libraries](https://goforj.dev/libraries/): adopt individual Go packages without the framework.
- [Performance](https://goforj.dev/performance): measured latency, throughput, and allocation costs with reproducible workloads.
- [Releases](https://github.com/goforj/goforj/releases) and [upgrade notes](UPGRADING.md): released changes and migration guidance.

GoForj is pre-1.0 and under active development. Review release notes before upgrading an existing project.
