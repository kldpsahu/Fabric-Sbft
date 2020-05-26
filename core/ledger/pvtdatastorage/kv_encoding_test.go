/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"bytes"
	math "math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDataKeyEncoding(t *testing.T) {
	dataKey1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns1", coll: "coll1", blkNum: 2}, txNum: 5}
	datakey2, err := decodeDatakey(encodeDataKey(dataKey1))
	assert.NoError(t, err)
	assert.Equal(t, dataKey1, datakey2)
}

func TestDatakeyRange(t *testing.T) {
	blockNum := uint64(20)
	startKey, endKey := datakeyRange(blockNum)
	var txNum uint64
	for txNum = 0; txNum < 100; txNum++ {
		keyOfBlock := encodeDataKey(
			&dataKey{
				nsCollBlk: nsCollBlk{ns: "ns", coll: "coll", blkNum: blockNum},
				txNum:     txNum,
			},
		)
		keyOfPreviousBlock := encodeDataKey(
			&dataKey{
				nsCollBlk: nsCollBlk{ns: "ns", coll: "coll", blkNum: blockNum - 1},
				txNum:     txNum,
			},
		)
		keyOfNextBlock := encodeDataKey(
			&dataKey{
				nsCollBlk: nsCollBlk{ns: "ns", coll: "coll", blkNum: blockNum + 1},
				txNum:     txNum,
			},
		)
		assert.Equal(t, bytes.Compare(keyOfPreviousBlock, startKey), -1)
		assert.Equal(t, bytes.Compare(keyOfBlock, startKey), 1)
		assert.Equal(t, bytes.Compare(keyOfBlock, endKey), -1)
		assert.Equal(t, bytes.Compare(keyOfNextBlock, endKey), 1)
	}
}

func TestEligibleMissingdataRange(t *testing.T) {
	blockNum := uint64(20)
	startKey, endKey := eligibleMissingdatakeyRange(blockNum)
	var txNum uint64
	for txNum = 0; txNum < 100; txNum++ {
		keyOfBlock := encodeMissingDataKey(
			&missingDataKey{
				nsCollBlk:  nsCollBlk{ns: "ns", coll: "coll", blkNum: blockNum},
				isEligible: true,
			},
		)
		keyOfPreviousBlock := encodeMissingDataKey(
			&missingDataKey{
				nsCollBlk:  nsCollBlk{ns: "ns", coll: "coll", blkNum: blockNum - 1},
				isEligible: true,
			},
		)
		keyOfNextBlock := encodeMissingDataKey(
			&missingDataKey{
				nsCollBlk:  nsCollBlk{ns: "ns", coll: "coll", blkNum: blockNum + 1},
				isEligible: true,
			},
		)
		assert.Equal(t, bytes.Compare(keyOfNextBlock, startKey), -1)
		assert.Equal(t, bytes.Compare(keyOfBlock, startKey), 1)
		assert.Equal(t, bytes.Compare(keyOfBlock, endKey), -1)
		assert.Equal(t, bytes.Compare(keyOfPreviousBlock, endKey), 1)
	}
}

func TestEncodeDecodeMissingdataKey(t *testing.T) {
	for i := 0; i < 1000; i++ {
		testEncodeDecodeMissingdataKey(t, uint64(i))
	}
	testEncodeDecodeMissingdataKey(t, math.MaxUint64) // corner case
}

func testEncodeDecodeMissingdataKey(t *testing.T, blkNum uint64) {
	key := &missingDataKey{
		nsCollBlk: nsCollBlk{
			ns:     "ns",
			coll:   "coll",
			blkNum: blkNum,
		},
	}

	t.Run("ineligibileKey",
		func(t *testing.T) {
			key.isEligible = false
			decodedKey := decodeMissingDataKey(
				encodeMissingDataKey(key),
			)
			assert.Equal(t, key, decodedKey)
		},
	)

	t.Run("ineligibileKey",
		func(t *testing.T) {
			key.isEligible = true
			decodedKey := decodeMissingDataKey(
				encodeMissingDataKey(key),
			)
			assert.Equal(t, key, decodedKey)
		},
	)
}

func TestBasicEncodingDecoding(t *testing.T) {
	for i := 0; i < 10000; i++ {
		value := encodeReverseOrderVarUint64(uint64(i))
		nextValue := encodeReverseOrderVarUint64(uint64(i + 1))
		if !(bytes.Compare(value, nextValue) > 0) {
			t.Fatalf("A smaller integer should result into greater bytes. Encoded bytes for [%d] is [%x] and for [%d] is [%x]",
				i, i+1, value, nextValue)
		}
		decodedValue, _ := decodeReverseOrderVarUint64(value)
		if decodedValue != uint64(i) {
			t.Fatalf("Value not same after decoding. Original value = [%d], decode value = [%d]", i, decodedValue)
		}
	}
}

func TestDecodingAppendedValues(t *testing.T) {
	appendedValues := []byte{}
	for i := 0; i < 1000; i++ {
		appendedValues = append(appendedValues, encodeReverseOrderVarUint64(uint64(i))...)
	}

	len := 0
	value := uint64(0)
	for i := 0; i < 1000; i++ {
		appendedValues = appendedValues[len:]
		value, len = decodeReverseOrderVarUint64(appendedValues)
		if value != uint64(i) {
			t.Fatalf("expected value = [%d], decode value = [%d]", i, value)
		}
	}
}
