/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sbft

import (
	"bytes"
	"reflect"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/orderer/consensus/inactive"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	etcdtosbft "github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/pkg/errors"
)

//go:generate mockery -dir . -name InactiveChainRegistry -case underscore -output mocks

// InactiveChainRegistry registers chains that are inactive
type InactiveChainRegistry interface {
	// TrackChain tracks a chain with the given name, and calls the given callback
	// when this chain should be created.
	TrackChain(chainName string, genesisBlock *common.Block, createChain func())
}

//go:generate mockery -dir . -name ChainGetter -case underscore -output mocks

// ChainGetter obtains instances of ChainSupport for the given channel
type ChainGetter interface {
	// GetChain obtains the ChainSupport for the given channel.
	// Returns nil, false when the ChainSupport for the given channel
	// isn't found.
	GetChain(chainID string) *multichannel.ChainSupport
}

// Config contains etcdraft configurations
type Config struct {
	BatchSize uint64
}

// Consenter implements etcdraft consenter
type Consenter struct {
	CreateChain           func(chainName string)
	InactiveChainRegistry InactiveChainRegistry
	Dialer                *cluster.PredicateDialer
	Communication         cluster.Communicator
	*Dispatcher
	Chains ChainGetter
	Logger *flogging.FabricLogger
	//sbftConfig Config
	OrdererConfig localconfig.TopLevel
	Cert          []byte
}

// TargetChannel extracts the channel from the given proto.Message.
// Returns an empty string on failure.
func (c *Consenter) TargetChannel(message proto.Message) string {
	switch req := message.(type) {
	case *orderer.ConsensusRequest:
		return req.Channel
	case *orderer.SubmitRequest:
		return req.Channel
	default:
		return ""
	}
}

// ReceiverByChain returns the MessageReceiver for the given channelID or nil
// if not found.
func (c *Consenter) ReceiverByChain(channelID string) MessageReceiver {
	cs := c.Chains.GetChain(channelID)
	if cs == nil {
		return nil
	}
	if cs.Chain == nil {
		c.Logger.Panicf("Programming error - Chain %s is nil although it exists in the mapping", channelID)
	}
	if sbftChain, issbftChain := cs.Chain.(*Chain); issbftChain {
		return sbftChain
	}
	c.Logger.Warningf("Chain %s is of type %v and not sbft.Chain", channelID, reflect.TypeOf(cs.Chain))
	return nil
}

func (c *Consenter) detectSelfID(consenters map[uint64]*etcdtosbft.Consenter) (uint64, error) {
	thisNodeCertAsDER, err := pemToDER(c.Cert, 0, "server", c.Logger)
	if err != nil {
		return 0, err
	}

	var serverCertificates []string
	for nodeID, cst := range consenters {
		serverCertificates = append(serverCertificates, string(cst.ServerTlsCert))

		certAsDER, err := pemToDER(cst.ServerTlsCert, nodeID, "server", c.Logger)
		if err != nil {
			return 0, err
		}

		if bytes.Equal(thisNodeCertAsDER, certAsDER) {
			return nodeID, nil
		}
	}

	c.Logger.Warning("Could not find", string(c.Cert), "among", serverCertificates)
	return 0, cluster.ErrNotInChannel
}

// HandleChain returns a new Chain instance or an error upon failure
func (c *Consenter) HandleChain(support consensus.ConsenterSupport, metadata *common.Metadata) (consensus.Chain, error) {
	m := &etcdtosbft.ConfigMetadata{}
	if err := proto.Unmarshal(support.SharedConfig().ConsensusMetadata(), m); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal consensus metadata")
	}

	if m.Options == nil {
		return nil, errors.New("sbft options have not been provided")
	}

	isMigration := (metadata == nil || len(metadata.Value) == 0) && (support.Height() > 1)
	if isMigration {
		c.Logger.Debugf("Block metadata is nil at block height=%d, it is consensus-type migration", support.Height())
	}

	// determine raft replica set mapping for each node to its id
	// for newly started chain we need to read and initialize raft
	// metadata by creating mapping between conseter and its id.
	// In case chain has been restarted we restore raft metadata
	// information from the recently committed block meta data
	// field.
	blockMetadata, err := ReadBlockMetadata(metadata, m)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read Raft metadata")
	}

	consenters := CreateConsentersMap(blockMetadata, m)

	id, err := c.detectSelfID(consenters)
	if err != nil {
		c.InactiveChainRegistry.TrackChain(support.ChainID(), support.Block(0), func() {
			c.CreateChain(support.ChainID())
		})
		return &inactive.Chain{Err: errors.Errorf("channel %s is not serviced by me", support.ChainID())}, nil
	}

	opts := Options{
		NodeID:        id,
		Logger:        c.Logger,
		BatchSize:     int(m.Options.ElectionTick),
		BlockMetadata: blockMetadata,
		Consenters:    consenters,
		MigrationInit: isMigration,
		Cert:          c.Cert,
	}

	rpc := &cluster.RPC{
		Timeout:       c.OrdererConfig.General.Cluster.RPCTimeout,
		Logger:        c.Logger,
		Channel:       support.ChainID(),
		Comm:          c.Communication,
		StreamsByType: cluster.NewStreamsByType(),
	}
	return NewChain(
		support,
		opts,
		c.Communication,
		rpc,
		func() (BlockPuller, error) { return newBlockPuller(support, c.Dialer, c.OrdererConfig.General.Cluster) },
		func() {
			c.InactiveChainRegistry.TrackChain(support.ChainID(), nil, func() { c.CreateChain(support.ChainID()) })
		},
		nil,
	)
}

// ReadBlockMetadata attempts to read raft metadata from block metadata, if available.
// otherwise, it reads sbft metadata from config metadata supplied.
func ReadBlockMetadata(blockMetadata *common.Metadata, configMetadata *etcdtosbft.ConfigMetadata) (*etcdtosbft.BlockMetadata, error) {
	if blockMetadata != nil && len(blockMetadata.Value) != 0 { // we have consenters mapping from block
		m := &etcdtosbft.BlockMetadata{}
		if err := proto.Unmarshal(blockMetadata.Value, m); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal block's metadata")
		}
		return m, nil
	}

	m := &etcdtosbft.BlockMetadata{
		NextConsenterId: 0,
		ConsenterIds:    make([]uint64, len(configMetadata.Consenters)),
	}
	// need to read consenters from the configuration
	for i := range m.ConsenterIds {
		m.ConsenterIds[i] = m.NextConsenterId
		m.NextConsenterId++
	}

	return m, nil
}

// New creates a sbft Consenter
func New(
	clusterDialer *cluster.PredicateDialer,
	conf *localconfig.TopLevel,
	srvConf comm.ServerConfig,
	srv *comm.GRPCServer,
	r *multichannel.Registrar,
	icr InactiveChainRegistry,
	metricsProvider metrics.Provider,
) *Consenter {
	logger := flogging.MustGetLogger("orderer.consensus.sbft")

	//var cfg Config
	//if err := viperutil.Decode(conf.Consensus, &cfg); err != nil {
	//	logger.Panicf("Failed to decode etcdraft configuration: %s", err)
	//}

	consenter := &Consenter{
		CreateChain: r.CreateChain,
		Cert:        srvConf.SecOpts.Certificate,
		Logger:      logger,
		Chains:      r,
		//EtcdRaftConfig:        cfg,
		OrdererConfig: *conf,
		Dialer:        clusterDialer,
		//Metrics:               NewMetrics(metricsProvider),
		InactiveChainRegistry: icr,
	}
	consenter.Dispatcher = &Dispatcher{
		Logger:        logger,
		ChainSelector: consenter,
	}

	comm := createComm(clusterDialer, consenter, conf.General.Cluster, metricsProvider)
	consenter.Communication = comm
	svc := &cluster.Service{
		CertExpWarningThreshold:          conf.General.Cluster.CertExpirationWarningThreshold,
		MinimumExpirationWarningInterval: cluster.MinimumExpirationWarningInterval,
		StreamCountReporter: &cluster.StreamCountReporter{
			Metrics: comm.Metrics,
		},
		StepLogger: flogging.MustGetLogger("orderer.common.cluster.step"),
		Logger:     flogging.MustGetLogger("orderer.common.cluster"),
		Dispatcher: comm,
	}
	orderer.RegisterClusterServer(srv.Server(), svc)
	consenter.Logger.Infof("sbFT consenter")
	return consenter
}

func createComm(clusterDialer *cluster.PredicateDialer, c *Consenter, config localconfig.Cluster, p metrics.Provider) *cluster.Comm {
	metrics := cluster.NewMetrics(p)
	comm := &cluster.Comm{
		MinimumExpirationWarningInterval: cluster.MinimumExpirationWarningInterval,
		CertExpWarningThreshold:          config.CertExpirationWarningThreshold,
		SendBufferSize:                   config.SendBufferSize,
		Logger:                           flogging.MustGetLogger("orderer.common.cluster"),
		Chan2Members:                     make(map[string]cluster.MemberMapping),
		Connections:                      cluster.NewConnectionStore(clusterDialer, metrics.EgressTLSConnectionCount),
		Metrics:                          metrics,
		ChanExt:                          c,
		H:                                c,
	}
	c.Communication = comm
	return comm
}
