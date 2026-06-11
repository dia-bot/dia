// Package customcommands holds the type-only model for Dia's programmable
// per-guild slash commands: the JSONB document shape (Definition, Step,
// Expr), the per-run Scope, the publish-time validator, and the
// templating-engine adapter that powers conditional expressions.
//
// The runtime engine lives in customcommands/exec; the worker plugin glue
// (CommandFallback / component intercepts / scheduler worker) lives in
// customcommands/runtime. Splitting the package this way keeps the data
// model importable from the API layer and the validator runnable inside the
// dashboard without dragging the Discord runtime along.
package customcommands
