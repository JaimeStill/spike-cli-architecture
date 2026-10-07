package app

import (
	"github.com/standards-lab/go-database"
	"github.com/standards-lab/go-database/postgres"
	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-storage/azureblob"

	"github.com/JaimeStill/spike-cli-architecture/graph"
	"github.com/JaimeStill/spike-cli-architecture/lifecycle"
)

// envPrefix is the prefix of every environment variable blobfs reads its
// configuration from: the libraries compose the rest, such as
// BLOBFS_DATABASE_HOST, BLOBFS_STORAGE_ENDPOINT, or BLOBFS_SHUTDOWN_TIMEOUT.
const envPrefix = "BLOBFS"

// infrastructure is the graph's infrastructure nodes: each configuration,
// and the database and object store built from them. Every value is read or
// constructed in its node's constructor, which runs only when a Build
// reaches the node, so configuration is read only for a built System, never
// in [New] and never for a run that builds nothing.
//
// *database.DB and *storage.Store each implement lifecycle.Subsystem, a
// Start (a ping, a probe) and a Shutdown, so the lifecycle starts and shuts
// them down through their own methods.
type infrastructure struct {
	databaseConfig  *graph.Node[database.Config]
	storageConfig   *graph.Node[storage.Config]
	lifecycleConfig *graph.Node[lifecycle.Config]
	database        *graph.Node[*database.DB]
	store           *graph.Node[*storage.Store]
}

// defineInfrastructure defines the infrastructure nodes on g. It constructs
// nothing.
func defineInfrastructure(g *graph.Graph) *infrastructure {
	in := &infrastructure{
		databaseConfig:  g.Define("database config", finalized[database.Config]),
		storageConfig:   g.Define("storage config", finalized[storage.Config]),
		lifecycleConfig: g.Define("lifecycle config", finalized[lifecycle.Config]),
	}
	in.database = g.Define("database", in.newDatabase)
	in.store = g.Define("store", in.newStore)
	return in
}

// finalizer is a configuration finalized from the environment under a
// prefix, as go-core's config convention shapes go-database's, go-storage's,
// and lifecycle's Config.
type finalizer[T any] interface {
	*T
	Finalize(prefix string) error
}

// finalized is a configuration node's constructor: it reads the zero
// configuration's defaults and its environment overrides under envPrefix
// alone, and validates them.
func finalized[T any, P finalizer[T]](*graph.Scope) (T, error) {
	var cfg T
	err := P(&cfg).Finalize(envPrefix)
	return cfg, err
}

// newDatabase constructs the Postgres pool from the database configuration,
// BLOBFS_DATABASE_HOST, _PORT, _NAME, _USER, _PASSWORD, and the pool and
// timeout settings go-database names. It does no I/O: the pool first
// connects in Start, the ping bounded by the configuration's conn_timeout.
func (in *infrastructure) newDatabase(s *graph.Scope) (*database.DB, error) {
	return postgres.New(s.Use(in.databaseConfig))
}

// newStore constructs the object store over the Azure Blob provider from
// the storage configuration, BLOBFS_STORAGE_ENDPOINT, _CONTAINER, _ACCOUNT,
// _KEY, the limits and timeouts go-storage names, and the azureblob options
// under BLOBFS_STORAGE_OPTIONS_. It does no I/O: Start creates the
// container when it is missing and probes the service, both bounded by the
// configuration's request_timeout.
func (in *infrastructure) newStore(s *graph.Scope) (*storage.Store, error) {
	cfg := s.Use(in.storageConfig)
	client, err := azureblob.New(cfg)
	if err != nil {
		return nil, err
	}
	return storage.New(client, cfg), nil
}
