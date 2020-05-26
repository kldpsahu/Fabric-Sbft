/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/statecouchdb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/stateleveldb"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnv - an interface that a test environment implements
type TestEnv interface {
	StartExternalResource()
	Init(t testing.TB)
	GetDBHandle(id string) *DB
	GetName() string
	DBValueFormat() byte
	DecodeDBValue(dbVal []byte) statedb.VersionedValue
	Cleanup()
	StopExternalResource()
}

// Tests will be run against each environment in this array
// For example, to skip CouchDB tests, remove &CouchDBLockBasedEnv{}
var testEnvs = []TestEnv{&LevelDBTestEnv{}, &CouchDBTestEnv{}}

///////////// LevelDB Environment //////////////

// LevelDBTestEnv implements TestEnv interface for leveldb based storage
type LevelDBTestEnv struct {
	t                 testing.TB
	provider          *DBProvider
	bookkeeperTestEnv *bookkeeping.TestEnv
	dbPath            string
}

// Init implements corresponding function from interface TestEnv
func (env *LevelDBTestEnv) Init(t testing.TB) {
	dbPath, err := ioutil.TempDir("", "cstestenv")
	if err != nil {
		t.Fatalf("Failed to create level db storage directory: %s", err)
	}
	env.bookkeeperTestEnv = bookkeeping.NewTestEnv(t)
	dbProvider, err := NewDBProvider(
		env.bookkeeperTestEnv.TestProvider,
		&disabled.Provider{},
		&mock.HealthCheckRegistry{},
		&StateDBConfig{
			&ledger.StateDBConfig{},
			dbPath,
		},
		[]string{"lscc", "_lifecycle"},
	)
	assert.NoError(t, err)
	env.t = t
	env.provider = dbProvider
	env.dbPath = dbPath
}

// StartExternalResource will be an empty implementation for levelDB test environment.
func (env *LevelDBTestEnv) StartExternalResource() {
	// empty implementation
}

// StopExternalResource will be an empty implementation for levelDB test environment.
func (env *LevelDBTestEnv) StopExternalResource() {
	// empty implementation
}

// GetDBHandle implements corresponding function from interface TestEnv
func (env *LevelDBTestEnv) GetDBHandle(id string) *DB {
	db, err := env.provider.GetDBHandle(id)
	assert.NoError(env.t, err)
	return db
}

// GetName implements corresponding function from interface TestEnv
func (env *LevelDBTestEnv) GetName() string {
	return "levelDBTestEnv"
}

// DBValueFormat returns the format used by the stateleveldb for dbvalue
func (env *LevelDBTestEnv) DBValueFormat() byte {
	return stateleveldb.TestEnvDBValueformat
}

// DecodeDBValue decodes the dbvalue bytes for tests
func (env *LevelDBTestEnv) DecodeDBValue(dbVal []byte) statedb.VersionedValue {
	vv, err := stateleveldb.TestEnvDBValueDecoder(dbVal)
	require.NoError(env.t, err)
	return *vv
}

// Cleanup implements corresponding function from interface TestEnv
func (env *LevelDBTestEnv) Cleanup() {
	env.provider.Close()
	env.bookkeeperTestEnv.Cleanup()
	os.RemoveAll(env.dbPath)
}

///////////// CouchDB Environment //////////////

// CouchDBTestEnv implements TestEnv interface for couchdb based storage
type CouchDBTestEnv struct {
	couchAddress      string
	t                 testing.TB
	provider          *DBProvider
	bookkeeperTestEnv *bookkeeping.TestEnv
	redoPath          string
	couchCleanup      func()
	couchDBConfig     *ledger.CouchDBConfig
}

// StartExternalResource starts external couchDB resources.
func (env *CouchDBTestEnv) StartExternalResource() {
	if env.couchAddress != "" {
		return
	}
	env.couchAddress, env.couchCleanup = statecouchdb.StartCouchDB(env.t.(*testing.T), nil)
}

// StopExternalResource stops external couchDB resources.
func (env *CouchDBTestEnv) StopExternalResource() {
	if env.couchAddress != "" {
		env.couchCleanup()
	}
}

// Init implements corresponding function from interface TestEnv
func (env *CouchDBTestEnv) Init(t testing.TB) {
	redoPath, err := ioutil.TempDir("", "pestate")
	if err != nil {
		t.Fatalf("Failed to create redo log directory: %s", err)
	}

	env.t = t
	env.StartExternalResource()

	stateDBConfig := &StateDBConfig{
		StateDBConfig: &ledger.StateDBConfig{
			StateDatabase: "CouchDB",
			CouchDB: &ledger.CouchDBConfig{
				Address:             env.couchAddress,
				Username:            "",
				Password:            "",
				MaxRetries:          3,
				MaxRetriesOnStartup: 20,
				RequestTimeout:      35 * time.Second,
				InternalQueryLimit:  1000,
				MaxBatchUpdateSize:  1000,
				RedoLogPath:         redoPath,
			},
		},
		LevelDBPath: "",
	}

	env.bookkeeperTestEnv = bookkeeping.NewTestEnv(t)
	dbProvider, err := NewDBProvider(
		env.bookkeeperTestEnv.TestProvider,
		&disabled.Provider{},
		&mock.HealthCheckRegistry{},
		stateDBConfig,
		[]string{"lscc", "_lifecycle"},
	)
	assert.NoError(t, err)
	env.provider = dbProvider
	env.redoPath = redoPath
	env.couchDBConfig = stateDBConfig.CouchDB
}

// GetDBHandle implements corresponding function from interface TestEnv
func (env *CouchDBTestEnv) GetDBHandle(id string) *DB {
	db, err := env.provider.GetDBHandle(id)
	assert.NoError(env.t, err)
	return db
}

// GetName implements corresponding function from interface TestEnv
func (env *CouchDBTestEnv) GetName() string {
	return "couchDBTestEnv"
}

// DBValueFormat returns the format used by the stateleveldb for dbvalue
// Not yet implemented
func (env *CouchDBTestEnv) DBValueFormat() byte {
	return byte(0) //To be implemented
}

// DecodeDBValue decodes the dbvalue bytes for tests
// Not yet implemented
func (env *CouchDBTestEnv) DecodeDBValue(dbVal []byte) statedb.VersionedValue {
	return statedb.VersionedValue{} //To be implemented
}

// Cleanup implements corresponding function from interface TestEnv
func (env *CouchDBTestEnv) Cleanup() {
	if env.provider != nil {
		assert.NoError(env.t, statecouchdb.DropApplicationDBs(env.couchDBConfig))
	}
	os.RemoveAll(env.redoPath)
	env.bookkeeperTestEnv.Cleanup()
	env.provider.Close()
}
