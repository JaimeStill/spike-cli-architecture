package app

import "github.com/JaimeStill/spike-cli-architecture/cli"

// The opener hook: tests swap the real openers for recording fakes, and
// mount a probe command group declaring any set of dependencies, which the
// production groups do not cover alone.

type (
	Database   = database
	Store      = objectStore
	Deps       = deps
	Dependency = dependency
)

const (
	DepPostgres = depPostgres
	DepStore    = depStore
)

// SetOpeners replaces a's openers.
func SetOpeners(a *App, postgres func() (Database, error), store func() (Store, error)) {
	a.init.openers = openers{postgres: postgres, store: store}
}

// Mount mounts a command group on a, declaring need, as the composition
// root does.
func Mount(a *App, build func(*Deps) *cli.Command, need ...Dependency) {
	a.mount(build, need...)
}
