# Sample App Prompts

These prompts are small, repeatable application builds for evaluating how effectively an AI agent can work with a stock GoForj project.

The applications intentionally use ordinary product requirements rather than framework-specific implementation instructions. An effective agent should discover and use GoForj's generated structure, maker commands, App composition points, primitives, development workflow, and inspection surfaces.

## Prompt Set

- [PhotoDrop](photodrop.md): a private photo library with people tagging and controlled sharing
- [PocketDesk](pocketdesk.md): a small team support desk
- [Gather](gather.md): event registration with capacity and a waitlist

PhotoDrop is the recommended flagship benchmark. It is immediately relatable,
visually demonstrable, connected to the existing GoForj site, and exercises most
framework components through natural product behavior.

PocketDesk and Gather provide complementary comparisons. PocketDesk emphasizes
ordinary authenticated business workflows, while Gather emphasizes capacity,
concurrency, time-based behavior, and idempotent waitlist promotion.

## Benchmark Setup

Each run should begin from a newly rendered, otherwise unmodified GoForj project. Record the exact choices made in `forj new`, including:

- starter kit
- database driver
- cache driver
- queue driver
- storage driver
- enabled components

Enable every component required by the selected prompt through the normal project configuration and rendering workflow. Do not copy implementation from an earlier benchmark run.

For comparisons across frontend and infrastructure choices, run the same prompt against multiple stock configurations. A useful initial matrix is:

| Variant | Starter kit | Database | Distributed primitives |
| --- | --- | --- | --- |
| Local | templ + htmx | SQLite | Local or memory drivers |
| Vue | Vue | MySQL | Redis-backed drivers |
| React | React | Postgres | Redis-backed drivers |

The matrix is illustrative. Use combinations currently supported by GoForj rather than modifying the framework merely to satisfy a variant.

## Rules For A Valid Run

- Work only in the generated project unless the prompt explicitly identifies a framework defect.
- Use GoForj maker, generation, build, and development commands when an applicable command exists.
- Put custom behavior in documented App-owned extension points.
- Do not patch generated framework files to bypass composition or dependency injection.
- Do not weaken authentication, authorization, validation, or dependency initialization to make tests pass.
- Keep background work in jobs and schedules when the prompt requires it rather than performing it synchronously in controllers.
- Add direct tests for validation branches and failure modes.
- Finish with a clean `forj build` and the relevant unit, integration, and frontend tests passing.
- Document any required manual correction, framework workaround, or missing generator capability.

## Evidence To Capture

For each run, retain:

- elapsed time to the first successful build
- elapsed time to completion
- agent/tool turns or equivalent interaction count
- compile, generation, migration, and test failures
- files edited manually after generation
- GoForj commands used
- framework documentation consulted
- framework defects or missing affordances encountered
- final test results
- whether the rendered application can be rerendered without losing application behavior

The goal is not merely to obtain working code. The benchmark should reveal whether an agent can discover the framework's intended path and remain on it.
