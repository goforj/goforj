# `.goforj.yml` - Configuration Reference

GoForj records project intent in `.goforj.yml`. It is explicit, auditable, and safe to edit. Render and the dev loop read from it; reruns are idempotent.

## Location

- Written to the project root selected in `forj new`.
- Must exist for `forj render`, `forj dev`, and `forj down`.

## Fields

```yaml
project_name: MyApp
module_name: github.com/you/myapp
updated_at: "2025-01-01 12:00:00 MST"
components:
  cli: true
  web_api: true
  web_ui: false
  docker: true
  database: true
  scheduler: false
  jobs: true
pre_dev:
  - name: Run Wire generate
    cmd: go install github.com/google/wire/cmd/wire@latest && cd wire && wire
  - name: Watcher Go Install
    cmd: go install github.com/bokwoon95/wgo@latest
  - name: Run Docker Compose
    cmd: docker-compose up -d              # only added when Docker is selected
  - name: Waiting for Database to be ready # only added when Docker + Database
    cmd: docker-compose exec -T mysql sh -c 'while ! mysqladmin ping -h "mysql" --silent; do sleep .5; done'
dev_down:
  - name: Docker Compose Down              # only added when Docker is selected
    cmd: docker-compose down
dev_watches:
  - name: App
    watch: -verbose -file .env -file .go -xdir forj -xdir _data -xdir ./frontend/node_modules -file .html
    exec: go run main.go http:serve
  - name: Wire
    watch: -file .go -cd ./wire -xfile ./wire/wire_gen.go -xdir forj -postpone
    exec: wire
  - name: Scheduler                        # only when Scheduler selected
    watch: -file .env -file .go -xdir forj -xdir _data -xdir ./frontend/node_modules -file .html
    exec: go run main.go schedule:run
  - name: Jobs                             # only when Jobs selected
    watch: -file .env -file .go -xdir forj -xdir _data -xdir ./frontend/node_modules -file .html
    exec: go run main.go queue:work
  - name: NPM                              # only when Web UI selected and package.json has a dev script
    watch: -cd ./frontend -xdir _data -xdir .
    exec: npm run dev
```

## Components

- `cli` is always true and cannot be turned off.
- Optional flags: `docker`, `web_api`, `web_ui`, `database`, `scheduler`, `jobs`.
- Render uses these to decide which scaffolds to generate. Dev hooks and watches are also derived from them.

## Hooks

- `pre_dev`: runs before watchers start in `forj dev` (wire install/generate, wgo install, Docker up, DB wait when applicable).
- `dev_down`: runs in `forj down` (docker-compose down when Docker is selected).
- `dev_watches`: watchers run during `forj dev` (App, Wire, Scheduler, Jobs, NPM).

All hooks are explicit shell commands; you can edit them to suit your environment.

## Behavior

- `updated_at` is set during wizard finalization.
- Render is idempotent: existing files are skipped and reported.
- `forj render` will install Wire and generate wire code so `go run main.go` works immediately.
- `dev_down` is only populated when Docker is selected to avoid unnecessary teardown steps.

## Editing safely

- You can toggle components and rerun `forj render`; new scaffolds are created, existing files are left in place and reported as skipped.
- You can change hooks if your environment differs (e.g., alternate DB wait commands).
- Keep `.goforj.yml` in version control to document team intent.

## Troubleshooting

- “No dev watches defined”: ensure `.goforj.yml` is present and was written by `forj new`.
- “.goforj.yml: no such file”: run `forj new` and ensure you are in the project root.
- Docker errors in pre_dev/dev_down: start Docker and rerun; adjust commands if needed.
