# 1. Generic `Exec[T]` as a free function, not a method

Date: 2026-09-05

Status: Accepted

> **Note:** [PR #15](https://github.com/ppanyukov/azure-resource-graph-go/pull/15)
> renamed `RgClient`/`NewRgClient` (referenced throughout this ADR) to
> `Client`/`NewClient`, to avoid the `rg.RgClient` package-name stutter,
> and added a `ClientOptions` parameter to `NewClient`. The naming below
> is left as originally written since it doesn't affect the decision.

## Context

The natural API shape for a caller-owned client would be:

```go
r := rg.NewRgClient(cred)
items, err := r.Exec[MyRecord](ctx, query)
```

This does not compile. Go methods cannot introduce their own type
parameters — only free functions and type declarations can. A method's
type parameters are limited to whatever its receiver type already
declares. Unlike C#, where a non-generic class can have a generic
method (`class RgClient { public T[] Exec<T>(string query) }`), Go has
no equivalent slot in its generics design. This was a deliberate
scoping decision in the generics proposal (implementation complexity
around method values/expressions and interface satisfiability), not an
oversight, and the Go team has left the door open to revisiting it in
a future release.

We considered four ways to give callers a client bound to their own
`azcore.TokenCredential` (as opposed to the package's shared default
credential) while still supporting generic result types:

1. **Generic wrapper type**: `type RgClient[T any] struct{...}` with a
   real `(*RgClient[T]).Exec(ctx, query) ([]T, error)` method.
   Rejected: binds one client to one result type at construction time.
   Real usage runs many different `T`s (record shapes) through the
   same client/credential, so this would force constructing a new
   client per query type — exactly the construction cost we're trying
   to amortize.

2. **Global mutable init** (`rg.InitRgClient(cred)`, then bare
   `Exec[T]` reads package-level state). Rejected: makes `Exec[T]`
   secretly stateful, breaks under concurrent use with different
   credentials (e.g. multi-tenant callers), and hides which credential
   is in effect at the call site. See "Revisiting option 2" below —
   this was reconsidered in more concrete form (`SetCred` /
   `DefaultClient`) once `RgClient` and `ExecClient` were designed, and
   rejected again with a sharper reason.

3. **Stuff the client into `ExecOptions`** (`ExecOptions.Client`).
   Rejected on semantic grounds: `ExecOptions` is meant to grow with
   *query*-shaped knobs (scopes, subscriptions, management groups,
   paging/facet options) — things that vary per call. A client/
   credential is *identity*-shaped and constant across many calls.
   Mixing the two invites bugs (set `Scopes`, forget `Client`, silently
   fall back to the default credential) and muddies what `ExecOptions`
   is for as it grows.

4. **Context value** (`ctx = rg.WithClient(ctx, r)`). A real, precedented
   pattern (e.g. `oauth2.HTTPClient` context key). Rejected for this
   case: a credential/client is caller-scoped configuration, not
   request-scoped data crossing API boundaries — the usual bar for
   putting something in a `context.Context`. It also turns a
   compile-time-visible dependency into a runtime-only one.

## Decision

`Exec[T]` keeps its current signature and behavior: it uses the
package's shared, lazily-initialized default client
(`azidentity.NewDefaultAzureCredential`, built at most once). This is
existing, already-shipped behavior (both `examples/rg-simple` and
`examples/rg-dumpjson` depend on it) and is real onboarding value for
a public package: `go get`, five lines, `rg.Exec[T](ctx, query, nil)`
works with no setup.

When a caller-supplied credential/client is needed, it is added as a
**separate free function** that takes the client as an explicit
parameter, following the same generic-free-function shape rather than
a method:

```go
func ExecClient[T any](r *RgClient, ctx context.Context, query string, options *ExecOptions) ([]T, error)
```

```go
func NewRgClient(cred azcore.TokenCredential) (*RgClient, error)
```

`ExecOptions` remains reserved for query-shaped parameters only
(scopes, management groups, resource-graph-specific options), not
identity/transport.

`RgClient` has no public mutator (no `SetCred`, nothing else exported
that can be reassigned after construction) — see below.

### Naming: `ExecClient`, not `ExecWithClient`

Follows the stdlib convention for "same operation, one explicit
dependency added": `Query` → `QueryContext`, `Command` →
`CommandContext`, `Ping` → `PingContext` — `Xxx<Noun>`, not
`Xxx<With><Noun>`. The one stdlib counterexample,
`NewRequestWithContext`, is a constructor (`New...`), where "with" reads
naturally as "construct with this extra thing" — a different
grammatical shape than an action verb like `Exec`. Since `Exec` is a
verb, `ExecClient` matches the majority pattern.

### Revisiting option 2: `SetCred` / exported `DefaultClient`

Before settling, we walked through a concrete version of option 2:
`RgClient.SetCred(cred)` plus an exported `rg.DefaultClient *RgClient`
that `Exec[T]` would forward to — mirroring `http.DefaultClient` /
`http.DefaultTransport`. This is a real, precedented Go pattern, and a
working design was sketched (lock-free reads via
`atomic.Pointer[armresourcegraph2.Client]`, lazy default construction
via `sync.Once`, `SetCred` always winning whenever called).

It was rejected anyway, on a narrower and more decisive ground than
"global state is bad in general": **it solves nothing that isn't
already trivially solvable outside the package once `ExecClient` and
`RgClient` exist.** A caller who wants a shared default across many
functions can already write, in their own code:

```go
var rgClient = rg.NewRgClient(myCred) // package-level var in the caller's own package

func doStuff(ctx context.Context) {
    items, _ := rg.ExecClient[MyRecord](rgClient, ctx, query, nil)
}
```

or wrap it in their own `Runner`/service-struct holding `*rg.RgClient`
alongside whatever other Azure SDK clients they already manage — which
is the normal pattern for every other Azure SDK client (nobody expects
`armresourcegraph.Client` or a blob `Client` to have a swappable
package-level default; callers hold and pass their own).

This gives a concrete test for what belongs in the package versus what
doesn't: **if the workaround is a few lines entirely in the caller's
own code, it doesn't need to live in the library.** `ExecClient` fails
this test the other way — there is currently no way for a caller to
supply their own credential without forking `pkg/rg`, so it earns its
place. `SetCred`/`DefaultClient` passes the test in the "don't need
it" direction: it would only add an exported mutable global (visible
and mutable by every transitive dependency of the caller, not just the
caller) for something a private package-level variable already gives
them, with none of the downside.

`SetCred` and `DefaultClient` are therefore explicitly **out of
scope**. `RgClient`'s internal lazy-default-credential path (used only
by bare `Exec[T]`) has no public mutator and is not designed to be
overridden after the fact.

## Consequences

- Callers needing the default credential keep using `Exec[T](ctx,
  query, options)` unchanged — no behavior change, no migration.
- Callers needing their own credential construct an `RgClient` once
  via `NewRgClient(cred)` (amortizing the cost of
  `armruntime.NewPipeline` construction) and pass it explicitly to
  every `ExecClient[T]` call — no hidden global state, no per-call
  client rebuild.
- The call site reads honestly: which credential is in effect is
  visible in the function call, not inferred from init order, global
  state, or context plumbing.
- Callers who want a shared default across many functions in their own
  codebase implement that themselves (package-level var or a wrapper
  type holding `*RgClient`) — this is intentionally not provided by
  `pkg/rg`, per the "solvable outside the package" test above.
