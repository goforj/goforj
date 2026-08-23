# Go 1.27 Generic Methods API Design

## Status

- Design status: implemented and merged in the sibling libraries
- Research date: 2026-07-13; stable revalidation: 2026-08-23
- Language status: shipped in stable Go 1.27.0 on 2026-08-19
- Scope: GoForj-owned sibling libraries with receiver-owned generic operations
- Release policy decision: a minimum-Go increase ships in a minor release, never
  a patch; a major is reserved for incompatible API or behavior changes
- Generated-application adoption is deferred consumer work, not a release gate
  for the library APIs

Merged rollout pull requests: [ship#3](https://github.com/goforj/ship/pull/3),
[collection#5](https://github.com/goforj/collection/pull/5),
[collection#6](https://github.com/goforj/collection/pull/6),
[collection#7](https://github.com/goforj/collection/pull/7),
[collection#8](https://github.com/goforj/collection/pull/8),
[collection#9](https://github.com/goforj/collection/pull/9),
[collection#10](https://github.com/goforj/collection/pull/10),
[cache#9](https://github.com/goforj/cache/pull/9),
[httpx#10](https://github.com/goforj/httpx/pull/10),
[execx#10](https://github.com/goforj/execx/pull/10), and
[queue#13](https://github.com/goforj/queue/pull/13).

## Decision Summary

GoForj adopts generic methods only where a concrete receiver already owns the
operation.

The first adoption wave delivers 33 public generic method surfaces:

- 9 additive methods in `cache`
- 10 methods on the redesigned `collection/v4.Slice`
- 14 additive methods in `httpx`

Ship also converted one internal receiver-style generic function as the
low-risk canary.

The cache and HTTPX migrations are additive:

- add the generic method
- keep the current package function as a compatibility entry point
- preserve behavior, error contracts, and encoding policy
- update documentation generators to distinguish package functions from methods

Collection is the deliberate exception. Its pre-Go-1.27 pointer wrapper and
mutation contracts were already candidates for a major cleanup, so v2 remains
available while v4 provides a coherent named-slice API rather than preserving
two competing surfaces inside one major version.

Generic methods should not be forced onto interface-first APIs. In particular:

- keep `events.API` unchanged
- keep `web.Context`, `web.WebSocketConn`, and `web.Router` unchanged
- keep `storage.Storage` byte-oriented and unchanged
- keep cache's interface contracts non-generic

`execx.DecodeChain.As[T]` and queue payload result methods are adjacent additions,
not migrations of existing generic package functions. They use the same
result-owned method capability to remove caller-side destination pointers.

The Go 1.27 floor does not by itself trigger new major module paths. Additive
adopters should use their next minor release and announce the new floor
prominently. Collection uses a major version because its slice-backed redesign
changes source APIs and ownership behavior, not merely because it requires Go
1.27. Patch releases must not raise the minimum Go version.

## Why This Fits GoForj

GoForj separates generated application policy from reusable sibling primitives:

- sibling repos own reusable cache, collection, HTTP, queue, web, events, and
  storage behavior
- `goforj` owns templates, generated composition, wiring, and developer workflow

Generic methods do not change that ownership. The method belongs in the sibling
repo that owns the concrete type. Generated consumers can adopt released methods
later without blocking the libraries that declare them.

This matters most where generated applications already expose concrete objects:

- cache managers return `*cache.Cache`
- HTTP clients are `*httpx.Client`
- job handlers receive concrete `queue.Message` values

It matters much less where generated applications intentionally depend on abstractions:

- events expose `events.API`
- storage accessors return `storage.Storage`
- HTTP handlers receive `web.Context`

The existing abstraction boundary is more valuable than method-call syntax.

## What Go 1.27 Changes

### New syntax

Before Go 1.27, a method can use type parameters declared by its receiver type,
but it cannot declare new type parameters of its own:

```go
type Collection[T any] struct {
	items []T
}

// This is legal before Go 1.27 because T belongs to Collection.
func (c *Collection[T]) Filter(fn func(T) bool) *Collection[T]
```

A type-changing operation therefore has to be a package function:

```go
func MapTo[T, R any](
	c *Collection[T],
	fn func(T) R,
) *Collection[R]
```

Go 1.27 permits the method to declare `R`:

```go
func (c *Collection[T]) MapTo[R any](
	fn func(T) R,
) *Collection[R]
```

The proposal extends method declarations with an optional type-parameter list.
Calls use the same explicit type arguments and inference rules as generic
functions. Method values and method expressions are generic functions when the
selected method is generic.

The standard library's first documented example is `math/rand/v2.Rand.N`, which
places the existing typed operation in the namespace of a particular random
source.

### What does not change

Generic methods are concrete methods only.

The following remain true:

- interface methods cannot declare type parameters
- a generic concrete method does not implement a same-named non-generic
  interface method
- generic methods are unavailable through reflective method discovery; an
  explicitly instantiated method value can still be reflected as an ordinary
  function value
- methods can only be declared on local receiver types
- type inference does not infer a result-only type parameter from assignment
  context
- a method-specific type parameter cannot strengthen the constraint of a
  receiver type parameter
- Go still has no method overloading

These limits drive most of the decisions in this design.

### Inference examples

An input normally supplies enough information:

```go
c.Set("profile:42", profile, time.Minute) // T inferred as Profile
```

A result-only type still needs brackets:

```go
profile, ok, err := c.Get[Profile]("profile:42")
```

That is not a defect in generic methods; it is the same inference rule generic
functions use today.

## Goals

- Move receiver-owned generic operations into the receiver's namespace.
- Restore left-to-right fluent composition where package functions currently
  interrupt it.
- Improve discoverability through completion on concrete values.
- Remove the repeated instance parameter from APIs that already require a
  concrete instance.
- Preserve existing behavior and compatibility for additive migrations, and use
  a major-version boundary when a broader redesign is intentional.
- Keep GoForj's interface and driver boundaries intact.
- Record explicit no-change decisions for every audited primary GoForj module.

## Non-Goals

- Redesign every API merely because generic methods are available.
- Add generic type parameters to input-only `any` methods without a meaningful
  type relationship.
- Replace interfaces with concrete types to gain method syntax.
- Promise performance improvements; the main benefits are organization,
  composition, discoverability, and compile-time result typing.
- Remove JSON, codec, or reflection work that remains semantically necessary.
- Remove current package functions from additive migrations in the first
  adoption wave.
- Raise the Go version of sibling modules that receive no useful generic method.

## Adoption Test

A candidate should normally satisfy all of these conditions:

1. Real callers already hold a concrete local type.
2. The operation semantically belongs to that type.
3. A method-specific type parameter varies from call to call.
4. The type parameter relates an input to an output, changes a result type, or
   removes caller-side destination assertions or pointers.
5. The method remains useful without passing through an interface.
6. The name does not collide with a materially different existing method.
7. Reflection is not required to discover or invoke the method.

A generic function whose first parameter is the owning concrete instance is the
strongest migration signal. It is not an automatic rule: foreign receiver types,
receiver-constraint limitations, and symmetric multi-input operations can still
make the package function the correct shape.

## Audit Summary

The table covers the primary public API repositories. Nested driver, core,
testing, integration, docs, and examples modules were also searched for the
receiver-workaround pattern; they produced no additional public candidates.
Examples include the cache, events, storage, and mail auxiliary modules. Their
Go directives and release tags may still need coordinated updates when they
select an affected Go 1.27 dependency.

| Repository | Audited Go | Generic receiver-style audit | Stable decision |
|---|---:|---:|---|
| `cache` | 1.24.4 | 9 | Implemented as methods; function entry points remain. |
| `collection` | 1.24.4 | 11 direct candidates, 10 false positives | Implemented 10 generic methods on `Slice`; returning `[]Pair[T, U]` makes `Zip` viable. |
| `httpx/v2` | 1.24.4 | 14 | Implemented `Client` methods; package verbs and `Do[T]` remain. |
| `ship` | 1.26.1 | 1 internal | Implemented as the stable compiler canary. |
| `execx` | 1.24.4 | 0 | Implemented `DecodeChain.As[T]` as new API. |
| `queue` | 1.24.4 | 0 | Implemented `Job.PayloadAs[T]` and `Message.PayloadAs[T]`; retain `Bind`. |
| `events` | 1.24.0 | 0 | Do not put generic methods on the interface-led surface. |
| `web` | 1.25.0 | 0 | Prefer generic package helpers, which are legal today. |
| `storage` | 1.24.4 | 0 | No core change; keep raw-byte interface. |
| `str/v2` | 1.24.0 | 0 | No change. |
| `env/v2` | 1.24.4 | 0 | No migration; evaluate any new typed parser separately. |
| `crypt` | 1.24.4 | 0 | No migration; structured-value methods are separate scope. |
| `null/v6` | 1.22.12 | 0 | Constructors stay functions; do not diverge the proxy fork. |
| `atlas` | 1.25.0 | 0 | No change. |
| `godump` | 1.18 | 0 | No change. |
| `goforj` | 1.25.0 | 0 | Tooling and generated-consumer work only. |
| `mail` | 1.25.0 | 0 | No change. |
| `metrics` | 1.25.0 | 0 | No change. |
| `scheduler/v2` | 1.24.4 | 0 | No change. |
| `wgo` | 1.24.0 | 0 | No change. |

The public generic-method count is therefore 33, concentrated in three repos.
That concentration kept the rollout deliberate rather than turning it into a
mechanical all-repo Go version bump.

## Direct Migration: Cache

### Previous workaround

`cache.Cache` is concrete, but typed operations are package functions because
their `T` changes per key and call:

```go
profile, ok, err := cache.Get[Profile](
	c.WithContext(ctx),
	"profile:42",
)

err = cache.Set(
	c.WithContext(ctx),
	"profile:42",
	profile,
	time.Minute,
)
```

GoForj templates use this shape repeatedly in auth, inspections, application
settings, notifications, and monitoring code.

### Implemented methods

```go
func (c *Cache) GetJSON[T any](key string) (T, bool, error)
func (c *Cache) Get[T any](key string) (T, bool, error)

func (c *Cache) RefreshAhead[T any](
	key string,
	ttl time.Duration,
	refreshAhead time.Duration,
	fn func() (T, error),
) (T, error)

func (c *Cache) RefreshAheadValueWithCodec[T any](
	key string,
	ttl time.Duration,
	refreshAhead time.Duration,
	fn func() (T, error),
	codec ValueCodec[T],
) (T, error)

func (c *Cache) SetJSON[T any](
	key string,
	value T,
	ttl time.Duration,
) error

func (c *Cache) Set[T any](
	key string,
	value T,
	ttl time.Duration,
) error

func (c *Cache) Pull[T any](key string) (T, bool, error)

func (c *Cache) RememberStale[T any](
	key string,
	ttl time.Duration,
	staleTTL time.Duration,
	fn func() (T, error),
) (T, bool, error)

func (c *Cache) Remember[T any](
	key string,
	ttl time.Duration,
	fn func() (T, error),
) (T, error)
```

The custom-codec function currently takes `context.Context` before `*Cache`.
The method follows the rest of the concrete cache API and uses the context
bound by `WithContext`. Its legacy function preserves its explicit-context
contract through the existing private context-aware implementation.

This helper is therefore the one cache migration that is not a mechanical
first-argument move. The method and compatibility function enter the same
private implementation with their respective resolved contexts. Parity tests
cover nil-context normalization, cancellation, and the context delivered to
cache observers.

### Resulting usage

```go
profile, ok, err := c.WithContext(ctx).Get[Profile]("profile:42")
err = c.WithContext(ctx).Set("profile:42", profile, time.Minute)

profile, err = c.WithContext(ctx).Remember(
	"profile:42",
	time.Minute,
	loadProfile,
)
```

`Set`, `SetJSON`, `Remember`, `RememberStale`, `RefreshAhead`, and
`RefreshAheadValueWithCodec` normally infer `T` from a value, callback, or
codec. `Get`, `GetJSON`, and `Pull` require an explicit result type.

### Compatibility

Keep all nine current functions and forward them to the methods or their shared
private implementation:

```go
func Get[T any](c *Cache, key string) (T, bool, error) {
	return c.Get[T](key)
}
```

`CacheAPI` and its smaller interfaces remain non-generic. This is consistent
with today's typed functions, which already require `*Cache` rather than
`CacheAPI`.

Do not add new typed batch or codec variants merely to expand the method set.
Evaluate those from real use cases.

## Direct Migration: Collection v4

### Final API boundary

The initial additive implementation proved the Go 1.27 method syntax on the v2
pointer wrapper. The final design went further: collection v4 replaces
`*Collection[T]` with `Slice[T]`, a named slice that works directly with `len`,
indexing, slicing, `range`, standard-library helpers, and iterator adapters.

The old workaround required package functions or specialized names for
type-changing operations:

```go
names := collection.MapTo(
	collection.New(users).Filter(active),
	func(user User) string { return user.Name },
)
```

The v4 surface uses ordinary fluent methods:

```go
groups := collection.New(users).
	Filter(active).
	Map(func(user User) string { return user.Name }).
	UniqueBy(strings.ToLower).
	GroupBy(func(name string) int { return len(name) })
```

### Implemented generic methods

```go
func (c Slice[T]) Map[R any](fn func(T) R) Slice[R]
func (c Slice[T]) MinBy[K Number | ~string](keyFn func(T) K) (T, bool)
func (c Slice[T]) MaxBy[K Number | ~string](keyFn func(T) K) (T, bool)
func (c Slice[T]) ToMap[K comparable, V any](keyFn func(T) K, valueFn func(T) V) map[K]V
func (c Slice[T]) GroupBy[K comparable](keyFn func(T) K) map[K][]T
func (c Slice[T]) CountBy[K comparable](keyFn func(T) K) map[K]int
func (c Slice[T]) UniqueBy[K comparable](keyFn func(T) K) Slice[T]
func (c Slice[T]) ZipWith[U, R any](other []U, fn func(T, U) R) Slice[R]
func (c Slice[T]) Reduce[R any](initial R, fn func(R, T) R) R
func (c Slice[T]) Zip[U any](other []U) []Pair[T, U]
```

`Map` and `Filter` are pure, while `Transform` and `Retain` make in-place work
explicit. `Reduce` accepts a heterogeneous accumulator directly. Built-in slice
results mark collection boundaries without another wrapper type.

### Why `Zip` became viable

The stable compiler rejected the intermediate pointer-wrapper shape because
returning `*Collection[Tuple[T, U]]` recursively expanded a method set containing
`Zip` again. The final slice-backed design returns `[]Pair[T, U]`, so the result
does not recursively instantiate the receiver's generic method set. `Zip` is
therefore a valid v4 method rather than a permanent package-function exception.

### Compatibility and module path

This is not an additive change inside v2. Collection v4 changes type names,
method names, mutation behavior, ownership contracts, and the semantic import
path to `github.com/goforj/collection/v4`. Existing consumers can remain on v2;
the v4 migration guide provides method-by-method before-and-after examples.

Functions whose receiver element must itself be comparable remain package
functions, including `CountByValue`, `Difference`, `Intersect`,
`SymmetricDifference`, `Union`, and `UniqueComparable`. Constructors and map
boundaries such as `New`, `FromMap`, and `Times` also remain functions where the
receiver shape would not improve the API.

## Direct Migration: HTTPX

### Previous workaround

`httpx.Client` is concrete and one client intentionally decodes many unrelated
response types. Before this rollout, all typed verbs therefore took the client
as their first argument:

```go
response, err := httpx.Post[CreateUser, CreateUserResponse](
	client,
	"/users",
	CreateUser{Name: "Ana"},
)
```

There are 14 direct candidates:

- `Get`, `Post`, `Put`, `Patch`, `Delete`, `Head`, and `Options`
- `GetCtx`, `PostCtx`, `PutCtx`, `PatchCtx`, `DeleteCtx`, `HeadCtx`, and
  `OptionsCtx`

### Implemented methods

```go
func (c *Client) Get[Out any](
	url string,
	opts ...Option,
) (Out, error)

func (c *Client) Post[Out any](
	url string,
	body any,
	opts ...Option,
) (Out, error)

func (c *Client) Put[Out any](
	url string,
	body any,
	opts ...Option,
) (Out, error)

func (c *Client) Patch[Out any](
	url string,
	body any,
	opts ...Option,
) (Out, error)

func (c *Client) Delete[Out any](
	url string,
	opts ...Option,
) (Out, error)

func (c *Client) Head[Out any](
	url string,
	opts ...Option,
) (Out, error)

func (c *Client) Options[Out any](
	url string,
	opts ...Option,
) (Out, error)
```

Matching context methods use `ctx context.Context` as the first ordinary
argument.

The new body methods need only `Out`. The current `In` parameter is unconstrained
and is immediately passed into an `any` body path, so it establishes no useful
compile-time relationship. Removing it from the new method also fixes calls
that currently need `Post[any, Response](client, url, nil)`.

```go
response, err := client.Post[CreateUserResponse](
	"/users",
	CreateUser{Name: "Ana"},
)
```

The existing two-parameter package function remains compatible. Both entry
points call the shared request engine directly, preserving the same hot path:

```go
func Post[In, Out any](
	client *Client,
	url string,
	body In,
	opts ...Option,
) (Out, error) {
	out, _, err := do[Out](client, nil, methodPost, url, body, opts)
	return out, err
}
```

Keep `Head[Out]` as an exact migration in this compatibility wave even though a
decoded body is a weak contract for a HEAD response. A different response-metadata
API would be a separate HTTPX design; mixing it into this migration would lose
behavior parity with the shared request engine.

### `Do[T]` stays a function

`Do[T]` accepts `*req.Request`, a non-local type, and does not use `httpx.Client`.
HTTPX cannot declare a method on that receiver. Attaching it to `Client` would
invent ownership and potentially change error-mapper behavior, so it remains a
package-level escape hatch.

## Internal Canary: Ship

Before the canary change, Ship had one internal instance-passing generic
function:

```go
func Decode[T any](cmd Command) (T, error)
```

Its 27 production call sites were converted to:

```go
func (cmd Command) Decode[T any]() (T, error)
```

```go
// Before
request, err := lighthouse.Decode[BenchmarkRunRequest](cmd)

// After
request, err := cmd.Decode[BenchmarkRunRequest]()
```

Because the package is internal, Ship migrated atomically without a public
compatibility wrapper. It served as the post-stable canary for compiler, editor,
lint, documentation, and CI readiness before the public sibling changes merged.

## Adjacent Addition: Execx

Before this addition, Execx had no receiver-passing generic function. Its decode
chain instead ended in a caller-owned pointer:

```go
var payload Payload
err := execx.Command(...).
	DecodeJSON().
	FromStdout().
	Trim().
	Into(&payload)
```

The implemented addition is:

```go
func (d *DecodeChain) As[T any]() (T, error)
```

```go
payload, err := execx.Command(...).
	DecodeJSON().
	FromStdout().
	Trim().
	As[Payload]()
```

This removes the temporary variable, pointer ceremony, and caller-triggered
non-pointer destination error. It preserves the same decoder, source selection,
trimming, and error behavior.

`Into(any)` remains available for compatibility and for callers that
intentionally populate an existing object. `Decoder.Decode([]byte, any)` remains
non-generic because a decoder implementation must handle many destination types
through an interface.

## Adjacent Addition: Queue

Before this addition, Queue had no receiver-passing generic function, while its
concrete payload owners exposed pointer-based binding:

```go
var payload EmailPayload
if err := msg.Bind(&payload); err != nil {
	return err
}
```

The methods are implemented on both public concrete payload values:

```go
func (j Job) PayloadAs[T any]() (T, error)
func (m Message) PayloadAs[T any]() (T, error)
```

The queue API changed after the initial audit: `queue.Message` is now a concrete
root type rather than an alias of `bus.Context`. The method belongs directly on
that public type:

```go
payload, err := msg.PayloadAs[EmailPayload]()
if err != nil {
	return err
}
```

`PayloadAs` avoids collisions with the existing `Job.Payload(any)` setter and
the existing `Bind(any)` methods.

Queue's receiver-aware documentation tooling can index both methods directly as
`Job.PayloadAs` and `Message.PayloadAs`; no alias-specific documentation path is
needed.

Keep `Bind(any)` permanently. Lower-level queue interfaces and adapters rely on
it, and a generic method cannot replace an interface method.

Do not genericize `Payload(any)`, `PayloadJSON(any)`, or dispatch setters. They
are input-only APIs and an unconstrained type parameter would add no safety.

A later queue-specific design may consider typed handler registration:

```go
func (q *Queue) RegisterPayload[T any](
	jobType string,
	handler func(context.Context, Message, T) error,
)
```

That work needs decisions about binding-error wrapping, job metadata, GoForj's
generated queue manager, and whether a typed job descriptor should connect
dispatch to registration. Keep it separate from the completed `PayloadAs`
addition.

## Interface-First Repositories

### Events

The common event surface is `events.API`, including `WithContext`, `Publish`, and
`Subscribe`. Generic interface methods are illegal, and `WithContext` returns
the interface, so a generic concrete method on `Bus` would disappear in normal
injected usage.

Do not replace `Subscribe(any)` or add a privileged concrete-only subscription
tier. If compile-time validation of a canonical handler shape is useful, use a
generic package helper:

```go
func Subscribe[T any](
	bus API,
	handler func(context.Context, T) error,
) (Subscription, error)
```

That helper is legal before Go 1.27 and works with real and fake implementations.
`Publish(any)` is input-only and gains no useful invariant from `Publish[T]`.

### Web

The attractive typed operations live on `web.Context` and `web.WebSocketConn`,
which are interfaces implemented by adapters. Generic methods cannot be added to
those contracts.

Use package helpers instead:

```go
func Bind[T any](ctx Context) (T, error)
func Value[T any](ctx Context, key string) (T, bool)
func ReadJSON[T any](conn WebSocketConn) (T, error)
```

These helpers are also legal before Go 1.27. They could improve generated
handler code without weakening the web abstraction, but they are independent of
this migration.

Do not add a `web.Typed(ctx)` wrapper solely to recover method syntax.

### Storage

`storage.Storage` intentionally moves bytes across driver boundaries. Its
manager and GoForj accessors return that interface.

Do not add `Storage[T]`, change `Manager.Disk` to return a concrete facade, or
conflate byte access with serialization merely to create a generic method.
Generic JSON helpers could be package functions today if structured object
storage becomes an explicit product requirement. That is a codec-scope decision,
not a Go 1.27 migration.

## Explicit No-Change Decisions

- `str`: `String` is already fluent and method-based. `Parse[T]` would be less
  discoverable than `Int`, `Float64`, and `Bool` and would require new runtime
  parsing policy.
- `env`: `Scope` already has typed, named getters. A generic `Value[T]` would be
  a new scalar parsing abstraction, not a receiver-function migration.
- `crypt`: `Cipher` already owns encryption methods. Typed JSON encryption may
  be useful later but adds serialization policy and is separate scope.
- `null`: `NewValue`, `ValueFrom`, and `ValueFromPtr` are constructors and have
  no existing receiver. `Value[T].Map[U]` is a valid language example but would
  be new API in a proxy fork that should track upstream.
- `atlas`: its production generic helper is private and stateless; attaching it
  to the server would invent a receiver relationship.
- `godump`: `Dumper` already mirrors the package-level default-instance API.
- `mail`: builders, mailers, and drivers already own their operations.
- `metrics`: `Registry` already owns fixed metric constructors.
- `scheduler`: its fluent surface is already method-based.
- `wgo`: no typed transformation or receiver-function workaround was found.
- `wire`: no API migration candidate exists; it is tooling that must understand
  the new syntax.
- `goforj`: its only generic function is a private map-key sorter. Built-in maps
  cannot receive local methods. GoForj's role is generator and consumer rollout.

Unaffected repos should retain their existing Go version floors.

## Compatibility Policy

### Keep package functions

For additive public migrations in cache and HTTPX, the old and new forms
coexist:

```go
httpx.Get[User](client, url)
client.Get[User](url)
```

The method becomes the preferred documentation form. The function remains a
compatibility entry point backed by the same implementation where practical.

Do not immediately deprecate the functions. The method form is an ergonomic
improvement, not a correctness fix. Reconsider deprecation only after real usage
and migration data, and only under each module's compatibility policy.

Collection does not use compatibility wrappers inside v4. Its old API remains
available on v2, while v4 presents only the final slice-backed surface. Keeping
both collection designs in the same major version would undermine the clean
boundary that justified the breaking release.

### Minimum Go version

Source that declares a generic method requires Go 1.27 language support. Each
adopting module must set a clear Go 1.27 minimum rather than expose a
toolchain-dependent public method set through version build tags.

This language change is backward-compatible with existing Go source, but a
module's minimum-toolchain increase is still a consumer compatibility event.
The GoForj release policy is:

- a patch release must not raise the minimum Go version
- a minor release may raise the minimum Go version when the change is prominent
  in release notes and installation documentation
- a minimum-Go increase alone does not require a new major module path
- a major release remains reserved for incompatible source API, contract, or
  behavior changes
- consumers that cannot move to the new toolchain can remain pinned to the
  previous minor line

This policy must be published in the affected sibling repos before, or as part
of, the first Go 1.27 release. README Go-version badges, CI matrices, and
`go.mod` directives must remain aligned; the merged rollout changes updated
them together.

Do not bump every sibling module in lockstep. Modules that declare the new
methods, plus modules that select dependencies requiring Go 1.27, need the new
floor. Repositories that do neither should retain their current floor.

GoForj itself imports several affected modules. Consuming their new releases and
emitting calls to the methods therefore requires a coordinated GoForj minimum-Go
update and updated generated project metadata.

Collection resolved its existing semantic-import-versioning mismatch by
publishing the redesigned API at `github.com/goforj/collection/v4`. That major
version is justified by the source, ownership, and mutation changes in the
slice-backed API; the Go 1.27 floor alone would not have justified it.

### No conditional public API

Do not keep a lower module version and expose methods only from `go1.27`-tagged
files. Even if a build-tag arrangement can be made to compile, users would see a
different public method set based on toolchain version, and documentation,
completion, examples, and support would become ambiguous.

## Tooling Readiness

### Documentation generators

The additive strategy creates a package function and method with the same bare
name. Documentation tooling must use receiver-qualified identities such as
`Get` and `Client.Get`.

The merged implementations updated the affected generators before regenerating
docs:

- `collection/docs/readme`
- `collection/docs/gen`
- `httpx/docs/readme`
- `httpx/docs/examplegen`

Those generators now use receiver-qualified identities and paths. Collection's
v4 generator also understands the named-slice receiver and the final generic
method signatures. `cache` and `queue` already had receiver-aware identity logic
and generated their new method entries without collisions.

`execx/docs/readme` and `execx/docs/examplegen` also key by bare name, but the
implemented `DecodeChain.As` has no current name collision, so no generator
identity change is required for that API.

After every public API change, regenerate and verify README, examples, and any
API indexes as part of the release—not as follow-up work.

### Deferred generated consumers

These library PRs declare and call ordinary receiver methods. GoForj template
adoption is outside this release wave and is not a library release gate.

Any later generated-consumer change should be designed and validated separately.
It should not be bundled into the reusable-library API correction.

### Reflection

Do not add any discovery path that relies on `reflect.Type.MethodByName` or method
indexes for generic methods. The accepted design intentionally excludes them
from reflection.

Existing reflection that decodes a concrete value, invokes legacy event
handlers, or inspects input shapes does not automatically disappear when a
method becomes generic. Remove reflection only where the new API actually makes
the runtime check unnecessary, as with `execx.As[T]` caller destinations.

## Stable Revalidation Outcome

The 2026-08-23 revalidation confirmed the final syntax, inference, interface,
and reflection rules against Go 1.27.0. The completed implementations also
settled two repository-level design questions:

1. Collection's intermediate pointer wrapper could not support `Zip` as a
   generic method because its result recursively expanded the receiver method
   set. The final slice-backed v4 API returns `[]Pair[T, U]`, so `Slice.Zip` is
   valid and the exception is no longer necessary.
2. Queue retired the public `bus.Context` alias arrangement. Payload result
   methods belong directly to concrete root `Job` and `Message` values.

Each public-library change updated its relevant module floors, installation
guidance, CI, tests, benchmarks, and generated documentation. No interface
boundary was replaced merely to obtain method syntax.

Cache and HTTPX benchmarks found allocation parity between their compatibility
functions and method forms. Execx and queue likewise preserved the costs of
their prior result paths. Collection v4 was benchmarked against the previous
implementation and `lo`; its generated comparison table distinguishes
equivalent work from explicitly labeled ownership and API trade-offs, and
dedicated regression benchmarks track its mutable zero-allocation paths.

## Rollout Plan

### Phase 1: stable compiler canary — complete

Ship's internal `lighthouse.Decode[T](cmd)` was converted to `cmd.Decode[T]()`,
with production call sites and method value/expression coverage updated.

### Phase 2: direct public migrations — implementations merged

Implemented in this order:

1. `collection`
2. `cache`
3. `httpx`

The additive cache and HTTPX implementations:

1. made docs tooling receiver-aware first
2. added methods over the existing shared implementation
3. retained package-function wrappers
4. added behavior-parity tests for both call forms
5. regenerated documentation and examples
6. ran unit, contract, integration, vet, and lint checks appropriate to each repo

Collection used the same documentation, test, and performance disciplines, but
shipped its broader slice-backed redesign behind the v4 module boundary rather
than retaining v2 wrappers.

### Phase 3: focused new APIs — implementations merged

1. Added `execx.DecodeChain.As[T]`.
2. Added queue `Job.PayloadAs[T]` and `Message.PayloadAs[T]`.

`web.Bind[T]` and related helpers remain an independent API-design question.

Collection v4 is published. Cache, HTTPX, execx, and queue still need releases
from their merged commits according to each repository's minimum-Go release
policy.

### Deferred: GoForj generated consumers

In a later generated-consumer change:

1. bump GoForj dependencies and its Go version together
2. replace generated cache helper calls with receiver methods
3. prefer HTTP client methods in tests and examples
4. prefer queue payload result methods in generated handlers
5. update starter kits and demo templates
6. render and test disposable Apps only under `/tmp`
7. verify generated documentation, build, and runtime behavior

Do not change generated event, storage, or web receiver types merely to expose
generic methods.

## Validation Matrix

Every adopting repo should validate:

- explicit result type arguments
- type inference from values and callbacks
- method values and method expressions
- pointer and value receivers as applicable
- nil-receiver behavior where an existing compatibility function accepted a nil
  pointer
- exact result and error parity where method and compatibility-function forms
  coexist
- allocation-count and timing comparisons between compatibility forms, or
  equivalent old/new operations for a major redesign, with an established
  main-branch baseline for hot paths
- documentation identity for same-named functions and methods
- reflection-dependent code paths do not expect the generic method
- lint, vet, static analysis, and generated examples

Use the GoForj cache defaults for Go commands:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go vet ./...
```

For a later GoForj consumer change:

- release or locally replace the sibling module intentionally
- render into `/tmp`, never into the GoForj repository
- build and test the emitted App
- verify any generated API artifacts affected by that separate change
- inspect generated README/example output in every changed sibling

## Risks

### Tool lag

Each library must validate its own compiler, docs, examples, lint, and CI paths.
Generated-App tooling is validated when GoForj actually begins emitting the new
calls; it is not coupled to declaration-only library releases.

### Interface confusion

Adding a generic method only to a concrete implementation can create two unequal
API tiers. Avoid that in event, web, storage, and driver contracts.

### Baseline churn

Cache, HTTPX, execx, and queue are source-additive, but their Go 1.27 module
floors may exclude consumers on older toolchains. Collection v4 separately has
source, module-path, ownership, and mutation compatibility changes. Treat each
kind of compatibility event explicitly rather than attributing the collection
major version to the toolchain floor.

### Documentation collisions

Where functions and methods coexist, bare-name generators can silently merge
their docs. Receiver-aware identity is a hard prerequisite. Collection v4 does
not carry duplicate compatibility entry points inside the new major version.

### Generic novelty

An unconstrained type parameter on an input-only method often provides no more
safety than `any`. Require a real type relationship or result benefit.

### Accidental semantic changes

Moving a function into a namespace must not quietly change context binding,
default-client behavior, codecs, observation names, nil handling, or errors.
Methods and wrappers should share one implementation.

### Accidental performance regressions

An additive method can still make a compatibility function exceed the compiler's
inline budget or add a wrapper frame to a hot path. Preserve allocation counts,
benchmark both call forms, and retain a direct implementation when delegation
measurably regresses an established performance-sensitive entry point. For a
breaking redesign such as Collection v4, benchmark equivalent operations against
the previous major and relevant alternatives, including allocation-sensitive
in-place paths.

## Final Recommendation

The merged implementations use generic methods as a focused API correction for
the receiver-first workarounds in `cache` and `httpx`, and as a core capability
of Collection's clean slice-backed v4 API.

The feature's best GoForj outcome is not “put generics everywhere.” It is:

- concrete resources read naturally from left to right
- type-changing collection pipelines remain fluent
- HTTP calls live on the client that performs them
- old cache and HTTPX function calls keep working
- collection v2 remains available while v4 exposes one coherent API
- interface-led primitives keep their abstraction strength
- generated applications can adopt the new syntax in a separate consumer change

That gives GoForj a materially cleaner public surface while keeping unrelated
interface-led libraries and generated applications out of the language rollout.

## Sources

- [Go 1.27 release notes](https://go.dev/doc/go1.27)
- [Accepted generic methods proposal, golang/go#77273](https://github.com/golang/go/issues/77273)
- [Official Go downloads, including Go 1.27.0](https://go.dev/dl/)
- [Go module version numbering](https://go.dev/doc/modules/version-numbers)
- [Go major-version guidance](https://go.dev/doc/modules/major-version)
- [Go modules reference: minimum Go version](https://go.dev/ref/mod#go-mod-file-go)
- Local API audit of GoForj-owned sibling modules, revalidated 2026-08-23
- [Intermediate pointer-wrapper `Zip` instantiation cycle, golang/go#80109](https://github.com/golang/go/issues/80109)
