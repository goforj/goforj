# External development artifacts

`forj dev` normally writes its runtime executable, ready stamp, and Go build cache under the project checkout. Set the inherited `FORJ_DEV_ARTIFACT_ROOT` environment variable to an absolute directory outside that checkout to keep those development artifacts elsewhere:

```sh
FORJ_DEV_ARTIFACT_ROOT=/var/tmp/my-project-dev forj dev
```

The runtime executable and ready stamp are written beneath `bin/` in that directory, and the Go build cache is stored at `.gocache`. Managed callers that launch multiple isolated artifact roots may also set `FORJ_DEV_GOCACHE` to one clean absolute path outside the checkout; that path becomes the build `GOCACHE` for every watcher while runtime artifacts remain isolated. The development command still runs with the project checkout as its working directory. Relative paths and a root or managed cache that is the checkout or a child of it are rejected. When `FORJ_DEV_ARTIFACT_ROOT` is unset, `forj dev` retains its existing `./bin` behavior.
