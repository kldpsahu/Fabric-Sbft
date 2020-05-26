/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stateleveldb

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/stretchr/testify/assert"
)

// TestVDBEnv provides a level db backed versioned db for testing
type TestVDBEnv struct {
	t          testing.TB
	DBProvider *VersionedDBProvider
	dbPath     string
}

// NewTestVDBEnv instantiates and new level db backed TestVDB
func NewTestVDBEnv(t testing.TB) *TestVDBEnv {
	t.Logf("Creating new TestVDBEnv")
	dbPath, err := ioutil.TempDir("", "statelvldb")
	if err != nil {
		t.Fatalf("Failed to create leveldb directory: %s", err)
	}
	dbProvider, err := NewVersionedDBProvider(dbPath)
	assert.NoError(t, err)
	return &TestVDBEnv{t, dbProvider, dbPath}
}

// Cleanup closes the db and removes the db folder
func (env *TestVDBEnv) Cleanup() {
	env.t.Logf("Cleaningup TestVDBEnv")
	env.DBProvider.Close()
	os.RemoveAll(env.dbPath)
}

var (
	// TestEnvDBValueformat exports the constant to be used used for tests
	TestEnvDBValueformat = fullScanIteratorValueFormat
	// TestEnvDBValueDecoder exports the function for decoding the dbvalue bytes
	TestEnvDBValueDecoder = func(dbValue []byte) (*statedb.VersionedValue, error) {
		return decodeValue(dbValue)
	}
)
