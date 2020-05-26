/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sbft

import (
	"encoding/pem"
	"fmt"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/consensus"
	sbft_impl "github.com/hyperledger/fabric/orderer/consensus/sbft/sbft_impl"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"
	etcdtosbft "github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/raft"
	//"go.etcd.io/etcd/raft"
)

const (
	BYTE = 1 << (10 * iota)
	KILOBYTE
	MEGABYTE
	GIGABYTE
	TERABYTE
)

const (
	//BatchSize if option is not provided
	BatchSize = 2
)

//go:generate counterfeiter -o mocks/configurator.go . Configurator

// Configurator is used to configure the communication layer
// when the chain starts.
type Configurator interface {
	Configure(channel string, newNodes []cluster.RemoteNode)
}

//go:generate counterfeiter -o mocks/mock_rpc.go . RPC

// RPC is used to mock the transport layer in tests.
type RPC interface {
	SendConsensus(dest uint64, msg *orderer.ConsensusRequest) error
	SendSubmit(dest uint64, request *orderer.SubmitRequest) error
}

//go:generate counterfeiter -o mocks/mock_blockpuller.go . BlockPuller

// BlockPuller is used to pull blocks from other OSN
type BlockPuller interface {
	PullBlock(seq uint64) *common.Block
	HeightsByEndpoints() (map[string]uint64, error)
	Close()
}

// CreateBlockPuller is a function to create BlockPuller on demand.
// It is passed into chain initializer so that tests could mock this.
type CreateBlockPuller func() (BlockPuller, error)

// Options contains all the configurations relevant to the chain.
type Options struct {
	NodeID    uint64
	Logger    *flogging.FabricLogger
	BatchSize int

	// BlockMetdata and Consenters should only be modified while under lock
	// of sbftMetadataLock
	BlockMetadata *etcdtosbft.BlockMetadata
	Consenters    map[uint64]*etcdtosbft.Consenter

	// MigrationInit is set when the node starts right after consensus-type migration
	MigrationInit bool
	Cert          []byte
}

type submit struct {
	req *orderer.SubmitRequest
}

// Chain implements consensus.Chain interface.
type Chain struct {
	configurator Configurator
	rpc          RPC
	nodeID       uint64
	channelID    string
	//	ActiveNodes  atomic.Value

	//

	// In this place use our node structure

	//
	Node *honeybadgerbft.ChainImpl

	submitC chan *submit
	haltC   chan struct{} // Signals to goroutines that the chain is halting
	doneC   chan struct{} // Closes when the chain halts
	startC  chan struct{} // Closes when the node is started

	errorCLock sync.RWMutex
	errorC     chan struct{} // returned by Errored()

	sbftMetadataLock sync.RWMutex
	//	confChangeInProgress *raftpb.ConfChange
	configInflight bool // this is true when there is config block or ConfChange in flight
	support        consensus.ConsenterSupport
	lastBlock      *common.Block
	createPuller   CreateBlockPuller // func used to create BlockPuller on demand
	opts           Options
	logger         *flogging.FabricLogger
	haltCallback   func()
	bc             *blockCreator
}

// NewChain constructs a chain object.
func NewChain(
	support consensus.ConsenterSupport,
	opts Options,
	conf Configurator,
	rpc RPC,
	f CreateBlockPuller,
	haltCallback func(),
	observeC chan<- raft.SoftState) (*Chain, error) {

	lg := opts.Logger.With("channel", support.ChainID(), "node", opts.NodeID)

	b := support.Block(support.Height() - 1)
	if b == nil {
		return nil, errors.Errorf("failed to get last block")
	}

	c := &Chain{
		configurator: conf,
		rpc:          rpc,
		channelID:    support.ChainID(),
		nodeID:       opts.NodeID,
		submitC:      make(chan *submit),
		haltC:        make(chan struct{}),
		doneC:        make(chan struct{}),
		startC:       make(chan struct{}),
		errorC:       make(chan struct{}),
		support:      support,
		lastBlock:    b,
		createPuller: f,
		haltCallback: haltCallback,
		logger:       lg,
		opts:         opts,
	}

	//	c.ActiveNodes.Store([]uint64{})

	//

	//Is this like starting new server with IP from proxyserver.go [newserver()]
	//Or can call Newnode from node.go

	//
	c.Node = sbft_impl.NewNode(c.opts.BatchSize, int(c.nodeID), len(c.opts.Consenters))

	return c, nil
}

// Start instructs the orderer to begin serving the chain and keep it current.
func (c *Chain) Start() {
	c.logger.Infof("Starting sbft node")

	if err := c.configureComm(); err != nil {
		c.logger.Errorf("Failed to start chain, aborting: +%v", err)
		close(c.doneC)
		return
	}

	isJoin := c.support.Height() > 1
	if isJoin && c.opts.MigrationInit {
		isJoin = false
		c.logger.Infof("Consensus-type migration detected, starting new raft node on an existing channel; height=%d", c.support.Height())
	}
	c.bc = &blockCreator{
		hash:   c.lastBlock.Header.Hash(),
		number: c.lastBlock.Header.Number,
		logger: c.logger,
	}

	// close(c.errorC)

	//

	//If we call start server there. There is no need to start the node here. Depends on, from where the
	//containing fxn is being called

	//

	c.Node.Start()
	go c.run()
	go c.send()
	go c.outputtxns()
	close(c.startC)
	// time.Sleep(50 * time.Millisecond)
}

// Order submits normal type transactions for ordering.
func (c *Chain) Order(env *common.Envelope, configSeq uint64) error {
	// c.Metrics.NormalProposalsReceived.Add(1)
	return c.Submit(&orderer.SubmitRequest{LastValidationSeq: configSeq, Payload: env, Channel: c.channelID}, c.nodeID)
}

// Configure submits config type transactions for ordering.
func (c *Chain) Configure(env *common.Envelope, configSeq uint64) error {
	//c.Metrics.ConfigProposalsReceived.Add(1)
	//TODO
	if err := c.checkConfigUpdateValidity(env); err != nil {
		c.logger.Warnf("Rejected config: %s", err)
		//c.Metrics.ProposalFailures.Add(1)
		return err
	}
	return c.Submit(&orderer.SubmitRequest{LastValidationSeq: configSeq, Payload: env, Channel: c.channelID}, c.nodeID)
}

//TODO
// Validate the config update for being of Type A or Type B as described in the design doc.
func (c *Chain) checkConfigUpdateValidity(ctx *common.Envelope) error {
	var err error
	payload, err := utils.UnmarshalPayload(ctx.Payload)
	if err != nil {
		return err
	}
	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return err
	}

	if chdr.Type != int32(common.HeaderType_ORDERER_TRANSACTION) &&
		chdr.Type != int32(common.HeaderType_CONFIG) {
		return errors.Errorf("config transaction has unknown header type: %s", common.HeaderType(chdr.Type))
	}

	if chdr.Type == int32(common.HeaderType_ORDERER_TRANSACTION) {
		newChannelConfig, err := utils.UnmarshalEnvelope(payload.Data)
		if err != nil {
			return err
		}

		payload, err = utils.UnmarshalPayload(newChannelConfig.Payload)
		if err != nil {
			return err
		}
	}

	configUpdate, err := configtx.UnmarshalConfigUpdateFromPayload(payload)
	if err != nil {
		return err
	}

	metadata, err := MetadataFromConfigUpdate(configUpdate)
	if err != nil {
		return err
	}

	if metadata == nil {
		return nil // ConsensusType is not updated
	}

	if err = CheckConfigMetadata(metadata); err != nil {
		return err
	}

	switch chdr.Type {
	case int32(common.HeaderType_ORDERER_TRANSACTION):
		c.sbftMetadataLock.RLock()
		set := MembershipByCert(c.opts.Consenters)
		c.sbftMetadataLock.RUnlock()

		for _, c := range metadata.Consenters {
			if _, exits := set[string(c.ClientTlsCert)]; !exits {
				return errors.Errorf("new channel has consenter that is not part of system consenter set")
			}
		}

		return nil

	case int32(common.HeaderType_CONFIG):
		c.sbftMetadataLock.RLock()
		_, err = ComputeMembershipChanges(c.opts.BlockMetadata, c.opts.Consenters, metadata.Consenters)
		c.sbftMetadataLock.RUnlock()

		return err

	default:
		// panic here because we have just check header type and return early
		c.logger.Panicf("Programming error, unknown header type")
	}

	return nil
}

// WaitReady blocks when the chain:
// - is catching up with other nodes using snapshot
//
// In any other case, it returns right away.
func (c *Chain) WaitReady() error {
	if err := c.isRunning(); err != nil {
		return err
	}

	select {
	case c.submitC <- nil:
	case <-c.doneC:
		return errors.Errorf("chain is stopped")
	}

	return nil
}

// Errored returns a channel that closes when the chain stops.
func (c *Chain) Errored() <-chan struct{} {
	c.errorCLock.RLock()
	defer c.errorCLock.RUnlock()
	return c.errorC
}

// Halt stops the chain.
func (c *Chain) Halt() {
	select {
	case <-c.startC:
	default:
		c.logger.Warnf("Attempted to halt a chain that has not started")
		return
	}

	select {
	// case c.haltC <- struct{}{}:
	case <-c.doneC:
		return
	default:
		close(c.doneC)

		if c.haltCallback != nil {
			c.haltCallback()
		}
	}
}

func (c *Chain) isRunning() error {
	select {
	case <-c.startC:
	default:
		return errors.Errorf("chain is not started")
	}

	select {
	case <-c.doneC:
		return errors.Errorf("chain is stopped")
	default:
	}

	return nil
}

// Consensus passes the given ConsensusRequest message to the raft.Node instance
func (c *Chain) Consensus(req *orderer.ConsensusRequest, sender uint64) error {
	if err := c.isRunning(); err != nil {
		return err
	}

	stepMsg := &ab.HoneyBadgerBFTMessage{}
	if err := proto.Unmarshal(req.Payload, stepMsg); err != nil {
		return fmt.Errorf("failed to unmarshal StepRequest payload to Raft Message: %s", err)
	}
	//TODO do metadata no need for now
	// addlogger("\n\nSENDING TO sbFT NODE[%+v]\n[%+v]\n\n", c.nodeID, stepMsg)
	c.Node.MsgChan.Receive <- stepMsg

	return nil
}

// Submit forwards the incoming request to:
// - the local serveRequest goroutine if this is leader
// - the actual leader via the transport mechanism
// The call fails if there's no leader elected yet.
func (c *Chain) Submit(req *orderer.SubmitRequest, sender uint64) error {
	if err := c.isRunning(); err != nil {
		//c.Metrics.ProposalFailures.Add(1)
		return err
	}
	if sender == c.nodeID {
		for id := range c.opts.Consenters {
			if c.nodeID != id {
				c.rpc.SendSubmit(id, req)
			}
		}
	}
	//TODO : decide whether client will send to all or the nodes will transfer to each other
	select {
	case c.submitC <- &submit{req}:
	case <-c.doneC:
		//		c.Metrics.ProposalFailures.Add(1)
		return errors.Errorf("chain is stopped")
	}

	return nil
}

//TODO working for now can add different go routine for each node/orderer
func (c *Chain) send() {
	for {
		select {
		case msg := <-c.Node.MsgChan.Send:
			msg.ChainId = c.channelID
			if msg.Sender == msg.Receiver {
				c.Node.MsgChan.Receive <- &msg
			}
			// fmt.Printf("\n\nCHAIN.GO msg to send in \n%+v\n\n", msg)
			payload, err := proto.Marshal(&msg)
			if err != nil {
				continue
			}
			//todo make a thread for each recver
			c.rpc.SendConsensus(msg.Receiver, &orderer.ConsensusRequest{Channel: c.channelID, Payload: payload})
		case <-c.doneC:
			return
		}
	}
}

//TODO add timer here
func (c *Chain) outputtxns() {
	var timer <-chan time.Time
	for {
		select {
		case txn := <-c.Node.MsgChan.Outtxn:

			batches, pending, err := c.ordered(&orderer.SubmitRequest{Payload: txn})
			if err != nil {
				c.logger.Errorf("Failed to order message: %s", err)
				continue
			}

			c.propose(c.bc, batches...)
			switch {
			case timer != nil && !pending:
				// Timer is already running but there are no messages pending, stop the timer
				timer = nil
			case timer == nil && pending:
				// Timer is not already running and there are messages pending, so start it
				timer = time.After(c.support.SharedConfig().BatchTimeout())
				c.logger.Debugf("Just began %s batch timer", c.support.SharedConfig().BatchTimeout().String())
			default:
				// Do nothing when:
				// 1. Timer is already running and there are messages pending
				// 2. Timer is not set and there are no messages pending
			}
		case <-timer:
			//clear the timer
			timer = nil

			batch := c.support.BlockCutter().Cut()
			if len(batch) == 0 {
				c.logger.Warningf("Batch timer expired with no pending requests, this might indicate a bug")
				continue
			}
			batches := [][]*common.Envelope{}
			if len(batch) != 0 {
				batches = append(batches, batch)
			}
			c.propose(c.bc, batches...)

		case <-c.doneC:
			return
		}
	}
}

func (c *Chain) run() {
	//TODO : our forever loop comes here
	// 1. how to send
	// 2. how to recv

	// submitC = c.submitC

	for {
		select {
		case s := <-c.submitC:
			if s == nil {
				// polled by `WaitReady`
				continue
			}
			// var err error
			msg := s.req
			// seq := c.support.Sequence()
			if c.isConfig(msg.Payload) {
				// ConfigMsg
				// if msg.LastValidationSeq < seq {
				// 	c.logger.Warnf("Config message was validated against %d, although current config seq has advanced (%d)", msg.LastValidationSeq, seq)
				// 	msg.Payload, _, err = c.support.ProcessConfigMsg(msg.Payload)
				// 	if err != nil {
				// 		continue
				// 	}
				// }
				c.logger.Info("Received config transaction, pause accepting transaction till it is committed")
				c.Node.Enqueue(msg.Payload, true)
				continue
				// submitC = nil
			}
			// it is a normal message
			// if msg.LastValidationSeq < seq {
			// 	c.logger.Warnf("Normal message was validated against %d, although current config seq has advanced (%d)", msg.LastValidationSeq, seq)
			// 	if _, err := c.support.ProcessNormalMsg(msg.Payload); err != nil {
			// 		continue
			// 	}
			// }
			c.Node.Enqueue(msg.Payload, false)

		case <-c.doneC:
			select {
			case <-c.errorC: // avoid closing closed channel
			default:
				close(c.errorC)
			}

			c.logger.Infof("Stop serving requests")

			return
		}
	}
}

func (c *Chain) writeBlock(block *common.Block, index uint64) {
	c.logger.Debugf("**********writting BLock***************")
	if block.Header.Number > c.lastBlock.Header.Number+1 {
		c.logger.Panicf("Got block [%d], expect block [%d]", block.Header.Number, c.lastBlock.Header.Number+1)
	} else if block.Header.Number < c.lastBlock.Header.Number+1 {
		c.logger.Infof("Got block [%d], expect block [%d], this node was forced to catch up", block.Header.Number, c.lastBlock.Header.Number+1)
		return
	}

	c.lastBlock = block

	c.logger.Infof("Writing block [%d] (Raft index: %d) to ledger", block.Header.Number, index)

	if utils.IsConfigBlock(block) {
		c.writeConfigBlock(block, index)
		return
	}

	c.sbftMetadataLock.Lock()
	c.opts.BlockMetadata.RaftIndex = index
	m := utils.MarshalOrPanic(c.opts.BlockMetadata)
	c.sbftMetadataLock.Unlock()

	c.support.WriteBlock(block, m)
}

//TODO kal subah
// Orders the envelope in the `msg` content. SubmitRequest.
// Returns
//   -- batches [][]*common.Envelope; the batches cut,
//   -- pending bool; if there are envelopes pending to be ordered,
//   -- err error; the error encountered, if any.
// It takes care of config messages as well as the revalidation of messages if the config sequence has advanced.
func (c *Chain) ordered(msg *orderer.SubmitRequest) (batches [][]*common.Envelope, pending bool, err error) {
	seq := c.support.Sequence()

	if c.isConfig(msg.Payload) {
		// ConfigMsg
		if msg.LastValidationSeq < seq {
			c.logger.Warnf("Config message was validated against %d, although current config seq has advanced (%d)", msg.LastValidationSeq, seq)
			msg.Payload, _, err = c.support.ProcessConfigMsg(msg.Payload)
			if err != nil {
				return nil, true, errors.Errorf("bad config message: %s", err)
			}

			if err = c.checkConfigUpdateValidity(msg.Payload); err != nil {
				return nil, true, errors.Errorf("bad config message: %s", err)
			}
		}
		batch := c.support.BlockCutter().Cut()
		batches = [][]*common.Envelope{}
		if len(batch) != 0 {
			batches = append(batches, batch)
		}
		batches = append(batches, []*common.Envelope{msg.Payload})
		return batches, false, nil
	}
	// it is a normal message
	if msg.LastValidationSeq < seq {
		c.logger.Warnf("Normal message was validated against %d, although current config seq has advanced (%d)", msg.LastValidationSeq, seq)
		if _, err := c.support.ProcessNormalMsg(msg.Payload); err != nil {
			return nil, true, errors.Errorf("bad normal message: %s", err)
		}
	}

	//TODO add support ordered call here
	// batch := c.support.BlockCutter().Cut()
	// batches = [][]*common.Envelope{}
	// if len(batch) != 0 {
	// 	batches = append(batches, batch)
	// }
	// batches = append(batches, []*common.Envelope{msg.Payload})
	batches, pending = c.support.BlockCutter().Ordered(msg.Payload)
	return batches, pending, nil

}

func (c *Chain) propose(bc *blockCreator, batches ...[]*common.Envelope) {
	for _, batch := range batches {
		b := bc.createNextBlock(batch)
		c.logger.Infof("Created block [%d]", b.Header.Number)
		c.writeBlock(b, c.nodeID)
	}

	return
}

// func (c *Chain) detectConfChange(block *common.Block) *MembershipChanges {
// 	// If config is targeting THIS channel, inspect consenter set and
// 	// propose raft ConfChange if it adds/removes node.
// 	configMetadata := c.newConfigMetadata(block)

// 	if configMetadata == nil {
// 		return nil
// 	}

// if configMetadata.Options != nil &&
// 	configMetadata.Options.SnapshotIntervalSize != 0 &&
// 	configMetadata.Options.SnapshotIntervalSize != c.sizeLimit {
// 	c.logger.Infof("Update snapshot interval size to %d bytes (was %d)",
// 		configMetadata.Options.SnapshotIntervalSize, c.sizeLimit)
// 	c.sizeLimit = configMetadata.Options.SnapshotIntervalSize
// }

// changes, err := ComputeMembershipChanges(c.opts.BlockMetadata, c.opts.Consenters, configMetadata.Consenters)
// if err != nil {
// 	c.logger.Panicf("illegal configuration change detected: %s", err)
// }

// if changes.Rotated() {
// 	c.logger.Infof("Config block [%d] rotates TLS certificate of node %d", block.Header.Number, changes.RotatedNode)
// }

// return changes
// }

func (c *Chain) isConfig(env *common.Envelope) bool {
	h, err := utils.ChannelHeader(env)
	if err != nil {
		c.logger.Panicf("failed to extract channel header from envelope")
	}

	return h.Type == int32(common.HeaderType_CONFIG) || h.Type == int32(common.HeaderType_ORDERER_TRANSACTION)
}

func (c *Chain) configureComm() error {
	// Reset unreachable map when communication is reconfigured
	//c.Node.unreachableLock.Lock()
	//c.Node.unreachable = make(map[uint64]struct{})
	//c.Node.unreachableLock.Unlock()

	nodes, err := c.remotePeers()
	if err != nil {
		return err
	}

	c.configurator.Configure(c.channelID, nodes)
	return nil
}

func (c *Chain) remotePeers() ([]cluster.RemoteNode, error) {
	c.sbftMetadataLock.RLock()
	defer c.sbftMetadataLock.RUnlock()

	var nodes []cluster.RemoteNode
	for nodeID, consenter := range c.opts.Consenters {
		// No need to know yourself
		if nodeID == c.nodeID {
			continue
		}
		serverCertAsDER, err := pemToDER(consenter.ServerTlsCert, nodeID, "server", c.logger)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		clientCertAsDER, err := pemToDER(consenter.ClientTlsCert, nodeID, "client", c.logger)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		nodes = append(nodes, cluster.RemoteNode{
			ID:            nodeID,
			Endpoint:      fmt.Sprintf("%s:%d", consenter.Host, consenter.Port),
			ServerTLSCert: serverCertAsDER,
			ClientTLSCert: clientCertAsDER,
		})
	}
	return nodes, nil
}

func pemToDER(pemBytes []byte, id uint64, certType string, logger *flogging.FabricLogger) ([]byte, error) {
	bl, _ := pem.Decode(pemBytes)
	if bl == nil {
		logger.Errorf("Rejecting PEM block of %s TLS cert for node %d, offending PEM is: %s", certType, id, string(pemBytes))
		return nil, errors.Errorf("invalid PEM block")
	}
	return bl.Bytes, nil
}

// writeConfigBlock writes configuration blocks into the ledger in
// addition extracts updates about raft replica set and if there
// are changes updates cluster membership as well
func (c *Chain) writeConfigBlock(block *common.Block, index uint64) {
	hdr, err := ConfigChannelHeader(block)
	if err != nil {
		c.logger.Panicf("Failed to get config header type from config block: %s", err)
	}

	c.configInflight = false

	switch common.HeaderType(hdr.Type) {
	case common.HeaderType_CONFIG:
		// configMembership := c.detectConfChange(block)

		c.sbftMetadataLock.Lock()
		c.opts.BlockMetadata.RaftIndex = index
		// if configMembership != nil {
		// 	c.opts.BlockMetadata = configMembership.NewBlockMetadata
		// 	c.opts.Consenters = configMembership.NewConsenters
		// }
		c.sbftMetadataLock.Unlock()

		blockMetadataBytes := utils.MarshalOrPanic(c.opts.BlockMetadata)
		// write block with metadata
		c.support.WriteConfigBlock(block, blockMetadataBytes)

		// if configMembership == nil {
		// 	return
		// }

		// update membership
		// if configMembership.ConfChange != nil {
		// 	// We need to propose conf change in a go routine, because it may be blocked if raft node
		// 	// becomes leaderless, and we should not block `serveRequest` so it can keep consuming applyC,
		// 	// otherwise we have a deadlock.
		// 	go func() {
		// 		// ProposeConfChange returns error only if node being stopped.
		// 		// This proposal is dropped by followers because DisableProposalForwarding is enabled.
		// 		if err := c.Node.ProposeConfChange(context.TODO(), *configMembership.ConfChange); err != nil {
		// 			c.logger.Warnf("Failed to propose configuration update to Raft node: %s", err)
		// 		}
		// 	}()

		// 	c.confChangeInProgress = configMembership.ConfChange

		// 	switch configMembership.ConfChange.Type {
		// 	case raftpb.ConfChangeAddNode:
		// 		c.logger.Infof("Config block just committed adds node %d, pause accepting transactions till config change is applied", configMembership.ConfChange.NodeID)
		// 	case raftpb.ConfChangeRemoveNode:
		// 		c.logger.Infof("Config block just committed removes node %d, pause accepting transactions till config change is applied", configMembership.ConfChange.NodeID)
		// 	default:
		// 		c.logger.Panic("Programming error, encountered unsupported raft config change")
		// 	}

		// 	c.configInflight = true
		// } else if configMembership.Rotated() {
		// 	lead := atomic.LoadUint64(&c.lastKnownLeader)
		// 	if configMembership.RotatedNode == lead {
		// 		c.logger.Infof("Certificate of Raft leader is being rotated, attempt leader transfer before reconfiguring communication")
		// 		go func() {
		// 			c.Node.abdicateLeader(lead)
		// 			if err := c.configureComm(); err != nil {
		// 				c.logger.Panicf("Failed to configure communication: %s", err)
		// 			}
		// 		}()
		// 	} else {
		// 		if err := c.configureComm(); err != nil {
		// 			c.logger.Panicf("Failed to configure communication: %s", err)
		// 		}
		// 	}
		// }

	case common.HeaderType_ORDERER_TRANSACTION:
		// If this config is channel creation, no extra inspection is needed
		c.logger.Debugf("pagal")
		c.sbftMetadataLock.Lock()
		c.opts.BlockMetadata.RaftIndex = index
		m := utils.MarshalOrPanic(c.opts.BlockMetadata)
		c.sbftMetadataLock.Unlock()

		c.support.WriteConfigBlock(block, m)

	default:
		c.logger.Panicf("Programming error: unexpected config type: %s", common.HeaderType(hdr.Type))
	}
}

// getInFlightConfChange returns ConfChange in-flight if any.
// It returns confChangeInProgress if it is not nil. Otherwise
// it returns ConfChange from the last committed block (might be nil).
// func (c *Chain) getInFlightConfChange() *raftpb.ConfChange {
// 	if c.confChangeInProgress != nil {
// 		return c.confChangeInProgress
// 	}

// 	if c.lastBlock.Header.Number == 0 {
// 		return nil // nothing to failover just started the chain
// 	}

// 	if !utils.IsConfigBlock(c.lastBlock) {
// 		return nil
// 	}

// 	// extracting current Raft configuration state
// 	confState := c.Node.ApplyConfChange(raftpb.ConfChange{})

// 	if len(confState.Nodes) == len(c.opts.BlockMetadata.ConsenterIds) {
// 		// Raft configuration change could only add one node or
// 		// remove one node at a time, if raft conf state size is
// 		// equal to membership stored in block metadata field,
// 		// that means everything is in sync and no need to propose
// 		// config update.
// 		return nil
// 	}

// 	return ConfChange(c.opts.BlockMetadata, confState)
// }

// newMetadata extract config metadata from the configuration block
func (c *Chain) newConfigMetadata(block *common.Block) *etcdraft.ConfigMetadata {
	metadata, err := ConsensusMetadataFromConfigBlock(block)
	if err != nil {
		c.logger.Panicf("error reading consensus metadata: %s", err)
	}
	return metadata
}

// func (c *Chain) suspectEviction() bool {
// 	if c.isRunning() != nil {
// 		return false
// 	}

// 	return atomic.LoadUint64(&c.lastKnownLeader) == uint64(0)
// }

// func (c *Chain) newEvictionSuspector() *evictionSuspector {
// 	return &evictionSuspector{
// 		amIInChannel:               ConsenterCertificate(c.opts.Cert).IsConsenterOfChannel,
// 		evictionSuspicionThreshold: c.opts.EvictionSuspicion,
// 		writeBlock:                 c.support.Append,
// 		createPuller:               c.createPuller,
// 		height:                     c.support.Height,
// 		triggerCatchUp:             c.triggerCatchup,
// 		logger:                     c.logger,
// 		halt: func() {
// 			c.Halt()
// 		},
// 	}
// }

// func (c *Chain) triggerCatchup(sn *raftpb.Snapshot) {
// 	select {
// 	case c.snapC <- sn:
// 	case <-c.doneC:
// 	}
// }
