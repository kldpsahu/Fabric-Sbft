/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/stretchr/testify/assert"
)

func TestBlockFileScanSmallTxOnly(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	ledgerid := "testLedger"
	blkfileMgrWrapper := newTestBlockfileWrapper(env, ledgerid)
	bg, gb := testutil.NewBlockGenerator(t, ledgerid, false)
	blocks := []*common.Block{gb}
	blocks = append(blocks, bg.NextTestBlock(0, 0))
	blocks = append(blocks, bg.NextTestBlock(0, 0))
	blocks = append(blocks, bg.NextTestBlock(0, 0))
	blkfileMgrWrapper.addBlocks(blocks)
	blkfileMgrWrapper.close()

	filePath := deriveBlockfilePath(env.provider.conf.getLedgerBlockDir(ledgerid), 0)
	_, fileSize, err := util.FileExists(filePath)
	assert.NoError(t, err)

	lastBlockBytes, endOffsetLastBlock, numBlocks, err := scanForLastCompleteBlock(env.provider.conf.getLedgerBlockDir(ledgerid), 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, len(blocks), numBlocks)
	assert.Equal(t, fileSize, endOffsetLastBlock)

	expectedLastBlockBytes, _, err := serializeBlock(blocks[len(blocks)-1])
	assert.NoError(t, err)
	assert.Equal(t, expectedLastBlockBytes, lastBlockBytes)
}

func TestBlockFileScanSmallTxLastTxIncomplete(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	ledgerid := "testLedger"
	blkfileMgrWrapper := newTestBlockfileWrapper(env, ledgerid)
	bg, gb := testutil.NewBlockGenerator(t, ledgerid, false)
	blocks := []*common.Block{gb}
	blocks = append(blocks, bg.NextTestBlock(0, 0))
	blocks = append(blocks, bg.NextTestBlock(0, 0))
	blocks = append(blocks, bg.NextTestBlock(0, 0))
	blkfileMgrWrapper.addBlocks(blocks)
	blkfileMgrWrapper.close()

	filePath := deriveBlockfilePath(env.provider.conf.getLedgerBlockDir(ledgerid), 0)
	_, fileSize, err := util.FileExists(filePath)
	assert.NoError(t, err)

	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0660)
	assert.NoError(t, err)
	defer file.Close()
	err = file.Truncate(fileSize - 1)
	assert.NoError(t, err)

	lastBlockBytes, _, numBlocks, err := scanForLastCompleteBlock(env.provider.conf.getLedgerBlockDir(ledgerid), 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, len(blocks)-1, numBlocks)

	expectedLastBlockBytes, _, err := serializeBlock(blocks[len(blocks)-2])
	assert.NoError(t, err)
	assert.Equal(t, expectedLastBlockBytes, lastBlockBytes)
}
