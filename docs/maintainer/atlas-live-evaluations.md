# Atlas live evaluations

Live evaluations authenticate the GoForj source identity in their signed `run.json` before starting a provider session. Build an evaluation runner from a checkout with:

```sh
make eval-runner EVAL_RUNNER=/tmp/forj-eval
```

The command requires a clean checkout, exports the selected commit into a private source snapshot, and builds only that snapshot with a full Git revision and explicit clean-state stamp. This prevents concurrent worktree changes from entering a binary attributed to an earlier revision. Use that binary for `atlas:eval` commands when evaluating unpublished changes. Other builds are accepted only when Go build metadata provides an immutable module version or a full revision with an explicit dirty state; incomplete source identities are rejected before a provider session starts.
