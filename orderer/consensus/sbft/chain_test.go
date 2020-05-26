/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sbft_test

import (
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os/user"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	consensusmocks "github.com/hyperledger/fabric/orderer/consensus/mocks"
	"github.com/hyperledger/fabric/orderer/consensus/sbft"
	"github.com/hyperledger/fabric/orderer/consensus/sbft/mocks"
	mockblockcutter "github.com/hyperledger/fabric/orderer/mocks/common/blockcutter"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	raftprotos "github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	interval            = 100 * time.Millisecond
	LongEventualTimeout = 10 * time.Second

	// 10 is the default setting of ELECTION_TICK.
	// We used to have a small number here (2) to reduce the time for test - we don't
	// need to tick node 10 times to trigger election - however, we are using another
	// mechanism to trigger it now which does not depend on time: send an artificial
	// MsgTimeoutNow to node.
	ELECTION_TICK  = 10
	HEARTBEAT_TICK = 1
)

func init() {
	factory.InitFactories(nil)
}

// for some test cases we chmod file/dir to test failures caused by exotic permissions.
// however this does not work if tests are running as root, i.e. in a container.
func skipIfRoot() {
	u, err := user.Current()
	Expect(err).NotTo(HaveOccurred())
	if u.Uid == "0" {
		Skip("you are running test as root, there's no way to make files unreadable")
	}
}

var _ = Describe("Chain", func() {
	var (
		env       *common.Envelope
		channelID string
		tlsCA     tlsgen.CA
		// logger    *flogging.FabricLogger
	)

	BeforeEach(func() {
		tlsCA, _ = tlsgen.NewCA()
		channelID = "test-channel"
		// logger = flogging.NewFabricLogger(zap.NewExample())
		env = &common.Envelope{
			Payload: marshalOrPanic(&common.Payload{
				Header: &common.Header{ChannelHeader: marshalOrPanic(&common.ChannelHeader{Type: int32(common.HeaderType_MESSAGE), ChannelId: channelID})},
				Data:   []byte("TEST_MESSAGE"),
			}),
		}
	})

	Describe("3-node Raft cluster", func() {
		var (
			network        *network
			channelID      string
			timeout        time.Duration
			dataDir        string
			c0, c1, c2, c3 *chain
			raftMetadata   *raftprotos.BlockMetadata
			consenters     map[uint64]*raftprotos.Consenter
		)

		BeforeEach(func() {
			// var err error

			channelID = "multi-node-channel"
			timeout = 10 * time.Second
			raftMetadata = &raftprotos.BlockMetadata{
				ConsenterIds:    []uint64{0, 1, 2, 3},
				NextConsenterId: 4,
			}

			consenters = map[uint64]*raftprotos.Consenter{
				0: {
					Host:          "localhost",
					Port:          7051,
					ClientTlsCert: clientTLSCert(tlsCA),
					ServerTlsCert: serverTLSCert(tlsCA),
				},
				1: {
					Host:          "localhost",
					Port:          7051,
					ClientTlsCert: clientTLSCert(tlsCA),
					ServerTlsCert: serverTLSCert(tlsCA),
				},
				2: {
					Host:          "localhost",
					Port:          7051,
					ClientTlsCert: clientTLSCert(tlsCA),
					ServerTlsCert: serverTLSCert(tlsCA),
				},
				3: {
					Host:          "localhost",
					Port:          7051,
					ClientTlsCert: clientTLSCert(tlsCA),
					ServerTlsCert: serverTLSCert(tlsCA),
				},
			}

			network = createNetwork(timeout, channelID, dataDir, raftMetadata, consenters)
			c0 = network.chains[0]
			c1 = network.chains[1]
			c2 = network.chains[2]
			c3 = network.chains[3]
		})

		AfterEach(func() {

		})

		// When("reconfiguring raft cluster", func() {
		// 	const (
		// 		defaultTimeout = 5 * time.Second
		// 	)
		// 	var (
		// 		options = &raftprotos.Options{
		// 			TickInterval:         "500ms",
		// 			ElectionTick:         10,
		// 			HeartbeatTick:        1,
		// 			MaxInflightBlocks:    5,
		// 			SnapshotIntervalSize: 200,
		// 		}
		// 		updateRaftConfigValue = func(metadata *raftprotos.ConfigMetadata) map[string]*common.ConfigValue {
		// 			return map[string]*common.ConfigValue{
		// 				"ConsensusType": {
		// 					Version: 1,
		// 					Value: marshalOrPanic(&orderer.ConsensusType{
		// 						Metadata: marshalOrPanic(metadata),
		// 					}),
		// 				},
		// 			}
		// 		}
		// 		addConsenterConfigValue = func() map[string]*common.ConfigValue {
		// 			metadata := &raftprotos.ConfigMetadata{Options: options}
		// 			for _, consenter := range consenters {
		// 				metadata.Consenters = append(metadata.Consenters, consenter)
		// 			}

		// 			newConsenter := &raftprotos.Consenter{
		// 				Host:          "localhost",
		// 				Port:          7050,
		// 				ServerTlsCert: serverTLSCert(tlsCA),
		// 				ClientTlsCert: clientTLSCert(tlsCA),
		// 			}
		// 			metadata.Consenters = append(metadata.Consenters, newConsenter)
		// 			return updateRaftConfigValue(metadata)
		// 		}
		// 		removeConsenterConfigValue = func(id uint64) map[string]*common.ConfigValue {
		// 			metadata := &raftprotos.ConfigMetadata{Options: options}
		// 			for nodeID, consenter := range consenters {
		// 				if nodeID == id {
		// 					continue
		// 				}
		// 				metadata.Consenters = append(metadata.Consenters, consenter)
		// 			}
		// 			return updateRaftConfigValue(metadata)
		// 		}
		// 		createChannelEnv = func(metadata *raftprotos.ConfigMetadata) *common.Envelope {
		// 			configEnv := newConfigEnv("another-channel",
		// 				common.HeaderType_CONFIG,
		// 				newConfigUpdateEnv(channelID, nil, updateRaftConfigValue(metadata)))

		// 			// Wrap config env in Orderer transaction
		// 			return &common.Envelope{
		// 				Payload: marshalOrPanic(&common.Payload{
		// 					Header: &common.Header{
		// 						ChannelHeader: marshalOrPanic(&common.ChannelHeader{
		// 							Type:      int32(common.HeaderType_ORDERER_TRANSACTION),
		// 							ChannelId: channelID,
		// 						}),
		// 					},
		// 					Data: marshalOrPanic(configEnv),
		// 				}),
		// 			}
		// 		}
		// 	)
		// })

		When("3/3 nodes are running", func() {
			JustBeforeEach(func() {
				network.init()
				network.start()
			})

			AfterEach(func() {
				network.stop()
			})

			It("orders envelope on leader", func() {
				By("instructed to cut next block")
				c0.cutter.CutNext = true
				err := c0.Order(env, 0)
				Expect(err).ToNot(HaveOccurred())
				c1.cutter.CutNext = true
				// err = c1.Order(env, 0)
				// Expect(err).ToNot(HaveOccurred())
				c2.cutter.CutNext = true
				// err = c2.Order(env, 0)
				// Expect(err).ToNot(HaveOccurred())
				c3.cutter.CutNext = true
				// err = c3.Order(env, 0)
				// Expect(err).ToNot(HaveOccurred())
				time.Sleep(10 * time.Second)
				network.exec(
					func(c *chain) {
						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
					})

				// By("respect batch timeout")
				// c0.cutter.CutNext = false
				// c1.cutter.CutNext = false
				// c2.cutter.CutNext = false
				// c3.cutter.CutNext = false

				// err = c1.Order(env, 0)
				// Expect(err).ToNot(HaveOccurred())
				// Eventually(c1.cutter.CurBatch, LongEventualTimeout).Should(HaveLen(1))

				// // // c1.clock.WaitForNWatchersAndIncrement(timeout, 2)
				// time.Sleep(10 * time.Second)
				// network.exec(
				// 	func(c *chain) {
				// 		Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
				// 	})
			})

		})
	})
})

// 		When("3/3 nodes are running", func() {
// 			JustBeforeEach(func() {
// 				network.init()
// 				network.start()
// 				network.elect(1)
// 			})

// 			AfterEach(func() {
// 				network.stop()
// 			})

// 			It("correctly sets the cluster size and leadership metrics", func() {
// 				// the network should see only one leadership change
// 				network.exec(func(c *chain) {
// 					Expect(c.fakeFields.fakeLeaderChanges.AddCallCount()).Should(Equal(1))
// 					Expect(c.fakeFields.fakeLeaderChanges.AddArgsForCall(0)).Should(Equal(float64(1)))
// 					Expect(c.fakeFields.fakeClusterSize.SetCallCount()).Should(Equal(1))
// 					Expect(c.fakeFields.fakeClusterSize.SetArgsForCall(0)).To(Equal(float64(3)))
// 				})
// 				// c1 should be the leader
// 				Expect(c1.fakeFields.fakeIsLeader.SetCallCount()).Should(Equal(2))
// 				Expect(c1.fakeFields.fakeIsLeader.SetArgsForCall(1)).Should(Equal(float64(1)))
// 				// c2 and c3 should continue to remain followers
// 				Expect(c2.fakeFields.fakeIsLeader.SetCallCount()).Should(Equal(1))
// 				Expect(c2.fakeFields.fakeIsLeader.SetArgsForCall(0)).Should(Equal(float64(0)))
// 				Expect(c3.fakeFields.fakeIsLeader.SetCallCount()).Should(Equal(1))
// 				Expect(c3.fakeFields.fakeIsLeader.SetArgsForCall(0)).Should(Equal(float64(0)))
// 			})

// 			It("orders envelope on leader", func() {
// 				By("instructed to cut next block")
// 				c1.cutter.CutNext = true
// 				err := c1.Order(env, 0)
// 				Expect(err).ToNot(HaveOccurred())
// 				Expect(c1.fakeFields.fakeNormalProposalsReceived.AddCallCount()).To(Equal(1))
// 				Expect(c1.fakeFields.fakeNormalProposalsReceived.AddArgsForCall(0)).To(Equal(float64(1)))

// 				network.exec(
// 					func(c *chain) {
// 						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
// 					})

// 				By("respect batch timeout")
// 				c1.cutter.CutNext = false

// 				err = c1.Order(env, 0)
// 				Expect(err).ToNot(HaveOccurred())
// 				Expect(c1.fakeFields.fakeNormalProposalsReceived.AddCallCount()).To(Equal(2))
// 				Expect(c1.fakeFields.fakeNormalProposalsReceived.AddArgsForCall(1)).To(Equal(float64(1)))
// 				Eventually(c1.cutter.CurBatch, LongEventualTimeout).Should(HaveLen(1))

// 				c1.clock.WaitForNWatchersAndIncrement(timeout, 2)
// 				network.exec(
// 					func(c *chain) {
// 						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
// 					})
// 			})

// 			It("orders envelope on follower", func() {
// 				By("instructed to cut next block")
// 				c1.cutter.CutNext = true
// 				err := c2.Order(env, 0)
// 				Expect(err).ToNot(HaveOccurred())
// 				Expect(c2.fakeFields.fakeNormalProposalsReceived.AddCallCount()).To(Equal(1))
// 				Expect(c2.fakeFields.fakeNormalProposalsReceived.AddArgsForCall(0)).To(Equal(float64(1)))
// 				Expect(c1.fakeFields.fakeNormalProposalsReceived.AddCallCount()).To(Equal(0))

// 				network.exec(
// 					func(c *chain) {
// 						Eventually(func() int { return c.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
// 					})

// 				By("respect batch timeout")
// 				c1.cutter.CutNext = false

// 				err = c2.Order(env, 0)
// 				Expect(err).ToNot(HaveOccurred())
// 				Expect(c2.fakeFields.fakeNormalProposalsReceived.AddCallCount()).To(Equal(2))
// 				Expect(c2.fakeFields.fakeNormalProposalsReceived.AddArgsForCall(1)).To(Equal(float64(1)))
// 				Expect(c1.fakeFields.fakeNormalProposalsReceived.AddCallCount()).To(Equal(0))
// 				Eventually(c1.cutter.CurBatch, LongEventualTimeout).Should(HaveLen(1))

// 				c1.clock.WaitForNWatchersAndIncrement(timeout, 2)
// 				network.exec(
// 					func(c *chain) {
// 						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
// 					})
// 			})

// 			When("MaxInflightBlocks is reached", func() {
// 				BeforeEach(func() {
// 					network.exec(func(c *chain) { c.opts.MaxInflightBlocks = 1 })
// 				})

// 				It("waits for in flight blocks to be committed", func() {
// 					c1.cutter.CutNext = true
// 					// disconnect c1 to disrupt consensus
// 					network.disconnect(1)

// 					Expect(c1.Order(env, 0)).To(Succeed())

// 					doneProp := make(chan struct{})
// 					go func() {
// 						defer GinkgoRecover()
// 						Expect(c1.Order(env, 0)).To(Succeed())
// 						close(doneProp)
// 					}()
// 					// expect second `Order` to block
// 					Consistently(doneProp).ShouldNot(BeClosed())
// 					network.exec(func(c *chain) {
// 						Consistently(c.support.WriteBlockCallCount).Should(BeZero())
// 					})

// 					network.connect(1)
// 					c1.clock.Increment(interval)

// 					Eventually(doneProp, LongEventualTimeout).Should(BeClosed())
// 					network.exec(func(c *chain) {
// 						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
// 					})
// 				})

// 				It("resets block in flight when steps down from leader", func() {
// 					c1.cutter.CutNext = true
// 					c2.cutter.CutNext = true
// 					// disconnect c1 to disrupt consensus
// 					network.disconnect(1)

// 					Expect(c1.Order(env, 0)).To(Succeed())

// 					doneProp := make(chan struct{})
// 					go func() {
// 						defer GinkgoRecover()

// 						Expect(c1.Order(env, 0)).To(Succeed())
// 						close(doneProp)
// 					}()
// 					// expect second `Order` to block
// 					Consistently(doneProp).ShouldNot(BeClosed())
// 					network.exec(func(c *chain) {
// 						Consistently(c.support.WriteBlockCallCount).Should(BeZero())
// 					})

// 					network.elect(2)
// 					Expect(c3.Order(env, 0)).To(Succeed())
// 					Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(0))
// 					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
// 					Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))

// 					network.connect(1)
// 					c2.clock.Increment(interval)

// 					Eventually(doneProp, LongEventualTimeout).Should(BeClosed())
// 					network.exec(func(c *chain) {
// 						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
// 					})
// 				})
// 			})

// 			When("leader is disconnected", func() {
// 				It("proactively steps down to follower", func() {
// 					network.disconnect(1)

// 					By("Ticking leader until it steps down")
// 					Eventually(func() <-chan raft.SoftState {
// 						c1.clock.Increment(interval)
// 						return c1.observe
// 					}, LongEventualTimeout).Should(Receive(Equal(raft.SoftState{Lead: 0, RaftState: raft.StateFollower})))

// 					By("Ensuring it does not accept message due to the cluster being leaderless")
// 					err := c1.Order(env, 0)
// 					Expect(err).To(MatchError("no Raft leader"))

// 					network.elect(2)

// 					// c1 should have lost leadership
// 					Expect(c1.fakeFields.fakeIsLeader.SetCallCount()).Should(Equal(3))
// 					Expect(c1.fakeFields.fakeIsLeader.SetArgsForCall(2)).Should(Equal(float64(0)))
// 					// c2 should become the leader
// 					Expect(c2.fakeFields.fakeIsLeader.SetCallCount()).Should(Equal(2))
// 					Expect(c2.fakeFields.fakeIsLeader.SetArgsForCall(1)).Should(Equal(float64(1)))
// 					// c2 should continue to remain follower
// 					Expect(c3.fakeFields.fakeIsLeader.SetCallCount()).Should(Equal(1))

// 					network.join(1, true)
// 					network.exec(func(c *chain) {
// 						Expect(c.fakeFields.fakeLeaderChanges.AddCallCount()).Should(Equal(3))
// 						Expect(c.fakeFields.fakeLeaderChanges.AddArgsForCall(2)).Should(Equal(float64(1)))
// 					})

// 					err = c1.Order(env, 0)
// 					Expect(err).NotTo(HaveOccurred())
// 				})

// 				It("does not deadlock if propose is blocked", func() {
// 					signal := make(chan struct{})
// 					c1.cutter.CutNext = true
// 					c1.support.SequenceStub = func() uint64 {
// 						signal <- struct{}{}
// 						<-signal
// 						return 0
// 					}

// 					By("Sending a normal transaction")
// 					Expect(c1.Order(env, 0)).To(Succeed())

// 					Eventually(signal).Should(Receive())
// 					network.disconnect(1)

// 					By("Ticking leader till it steps down")
// 					Eventually(func() raft.SoftState {
// 						c1.clock.Increment(interval)
// 						return c1.Node.Status().SoftState
// 					}).Should(StateEqual(0, raft.StateFollower))

// 					close(signal)

// 					Eventually(c1.observe).Should(Receive(StateEqual(0, raft.StateFollower)))
// 					c1.support.SequenceStub = nil
// 					network.exec(func(c *chain) {
// 						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(0))
// 					})

// 					By("Re-electing 1 as leader")
// 					network.connect(1)
// 					network.elect(1)

// 					By("Sending another normal transaction")
// 					Expect(c1.Order(env, 0)).To(Succeed())

// 					network.exec(func(c *chain) {
// 						Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
// 					})
// 				})
// 			})

// 			When("follower is disconnected", func() {
// 				It("should return error when receiving an env", func() {
// 					network.disconnect(2)

// 					errorC := c2.Errored()
// 					Consistently(errorC).ShouldNot(BeClosed()) // assert that errorC is not closed

// 					By("Ticking node 2 until it becomes pre-candidate")
// 					Eventually(func() <-chan raft.SoftState {
// 						c2.clock.Increment(interval)
// 						return c2.observe
// 					}, LongEventualTimeout).Should(Receive(Equal(raft.SoftState{Lead: 0, RaftState: raft.StatePreCandidate})))

// 					Eventually(errorC).Should(BeClosed())
// 					err := c2.Order(env, 0)
// 					Expect(err).To(HaveOccurred())
// 					Expect(c2.fakeFields.fakeNormalProposalsReceived.AddCallCount()).To(Equal(1))
// 					Expect(c2.fakeFields.fakeNormalProposalsReceived.AddArgsForCall(0)).To(Equal(float64(1)))
// 					Expect(c1.fakeFields.fakeNormalProposalsReceived.AddCallCount()).To(Equal(0))

// 					network.connect(2)
// 					c1.clock.Increment(interval)
// 					Expect(errorC).To(BeClosed())

// 					Eventually(c2.Errored).ShouldNot(BeClosed())
// 				})
// 			})

// 			It("leader retransmits lost messages", func() {
// 				// This tests that heartbeats will trigger leader to retransmit lost MsgApp

// 				c1.cutter.CutNext = true

// 				network.disconnect(1) // drop MsgApp

// 				err := c1.Order(env, 0)
// 				Expect(err).ToNot(HaveOccurred())

// 				network.exec(
// 					func(c *chain) {
// 						Consistently(func() int { return c.support.WriteBlockCallCount() }).Should(Equal(0))
// 					})

// 				network.connect(1) // reconnect leader

// 				c1.clock.Increment(interval) // trigger a heartbeat
// 				network.exec(
// 					func(c *chain) {
// 						Eventually(func() int { return c.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
// 					})
// 			})

// 			It("allows the leader to create multiple normal blocks without having to wait for them to be written out", func() {
// 				// this ensures that the created blocks are not written out
// 				network.disconnect(1)

// 				c1.cutter.CutNext = true
// 				for i := 0; i < 3; i++ {
// 					Expect(c1.Order(env, 0)).To(Succeed())
// 				}

// 				Consistently(c1.support.WriteBlockCallCount).Should(Equal(0))

// 				network.connect(1)

// 				// After FAB-13722, leader would pause replication if it gets notified that message
// 				// delivery to certain node is failed, i.e. connection refused. Replication to that
// 				// follower is resumed if leader receives a MsgHeartbeatResp from it.
// 				// We could certainly repeatedly tick leader to trigger heartbeat broadcast, but we
// 				// would also risk a slow leader stepping down due to excessive ticks.
// 				//
// 				// Instead, we can simply send artificial MsgHeartbeatResp to leader to resume.
// 				m2 := &raftpb.Message{To: c1.id, From: c2.id, Type: raftpb.MsgHeartbeatResp}
// 				c1.Consensus(&orderer.ConsensusRequest{Channel: channelID, Payload: utils.MarshalOrPanic(m2)}, c2.id)
// 				m3 := &raftpb.Message{To: c1.id, From: c3.id, Type: raftpb.MsgHeartbeatResp}
// 				c1.Consensus(&orderer.ConsensusRequest{Channel: channelID, Payload: utils.MarshalOrPanic(m3)}, c3.id)

// 				network.exec(func(c *chain) {
// 					Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(3))
// 				})
// 			})

// 			It("new leader should wait for in-fight blocks to commit before accepting new env", func() {
// 				// Scenario: when a node is elected as new leader and there are still in-flight blocks,
// 				// it should not immediately start accepting new envelopes, instead it should wait for
// 				// those in-flight blocks to be committed, otherwise we may create uncle block which
// 				// forks and panicks chain.
// 				//
// 				// Steps:
// 				// - start raft cluster with three nodes and genesis block0
// 				// - order env1 on c1, which creates block1
// 				// - drop MsgApp from 1 to 3
// 				// - drop second round of MsgApp sent from 1 to 2, so that block1 is only committed on c1
// 				// - disconnect c1 and elect c2
// 				// - order env2 on c2. This env must NOT be immediately accepted, otherwise c2 would create
// 				//   an uncle block1 based on block0.
// 				// - c2 commits block1
// 				// - c2 accepts env2, and creates block2
// 				// - c2 commits block2
// 				c1.cutter.CutNext = true
// 				c2.cutter.CutNext = true

// 				step1 := c1.getStepFunc()
// 				c1.setStepFunc(func(dest uint64, msg *orderer.ConsensusRequest) error {
// 					stepMsg := &raftpb.Message{}
// 					Expect(proto.Unmarshal(msg.Payload, stepMsg)).NotTo(HaveOccurred())

// 					if dest == 3 {
// 						return nil
// 					}

// 					if stepMsg.Type == raftpb.MsgApp && len(stepMsg.Entries) == 0 {
// 						return nil
// 					}

// 					return step1(dest, msg)
// 				})

// 				Expect(c1.Order(env, 0)).NotTo(HaveOccurred())

// 				Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
// 				Consistently(c2.support.WriteBlockCallCount).Should(Equal(0))
// 				Consistently(c3.support.WriteBlockCallCount).Should(Equal(0))

// 				network.disconnect(1)

// 				step2 := c2.getStepFunc()
// 				c2.setStepFunc(func(dest uint64, msg *orderer.ConsensusRequest) error {
// 					stepMsg := &raftpb.Message{}
// 					Expect(proto.Unmarshal(msg.Payload, stepMsg)).NotTo(HaveOccurred())

// 					if stepMsg.Type == raftpb.MsgApp && len(stepMsg.Entries) != 0 && dest == 3 {
// 						for _, ent := range stepMsg.Entries {
// 							if len(ent.Data) != 0 {
// 								return nil
// 							}
// 						}
// 					}
// 					return step2(dest, msg)
// 				})

// 				network.elect(2)

// 				go func() {
// 					defer GinkgoRecover()
// 					Expect(c2.Order(env, 0)).NotTo(HaveOccurred())
// 				}()

// 				Consistently(c2.support.WriteBlockCallCount).Should(Equal(0))
// 				Consistently(c3.support.WriteBlockCallCount).Should(Equal(0))

// 				c2.setStepFunc(step2)
// 				c2.clock.Increment(interval)

// 				Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
// 				Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))

// 				b, _ := c2.support.WriteBlockArgsForCall(0)
// 				Expect(b.Header.Number).To(Equal(uint64(1)))
// 				b, _ = c2.support.WriteBlockArgsForCall(1)
// 				Expect(b.Header.Number).To(Equal(uint64(2)))
// 			})

// 			Context("handling config blocks", func() {
// 				var configEnv *common.Envelope
// 				BeforeEach(func() {
// 					values := map[string]*common.ConfigValue{
// 						"BatchTimeout": {
// 							Version: 1,
// 							Value: marshalOrPanic(&orderer.BatchTimeout{
// 								Timeout: "3ms",
// 							}),
// 						},
// 					}
// 					configEnv = newConfigEnv(channelID,
// 						common.HeaderType_CONFIG,
// 						newConfigUpdateEnv(channelID, nil, values),
// 					)
// 				})

// 				It("holds up block creation on leader once a config block has been created and not written out", func() {
// 					// this ensures that the created blocks are not written out
// 					network.disconnect(1)

// 					c1.cutter.CutNext = true
// 					// config block
// 					err := c1.Order(configEnv, 0)
// 					Expect(err).NotTo(HaveOccurred())

// 					// to avoid data races since we are accessing these within a goroutine
// 					tempEnv := env
// 					tempC1 := c1

// 					done := make(chan struct{})

// 					// normal block
// 					go func() {
// 						defer GinkgoRecover()

// 						// This should be blocked if config block is not committed
// 						err := tempC1.Order(tempEnv, 0)
// 						Expect(err).NotTo(HaveOccurred())

// 						close(done)
// 					}()

// 					Consistently(done).ShouldNot(BeClosed())

// 					network.connect(1)
// 					c1.clock.Increment(interval)

// 					network.exec(
// 						func(c *chain) {
// 							Eventually(func() int { return c.support.WriteConfigBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
// 						})

// 					network.exec(
// 						func(c *chain) {
// 							Eventually(func() int { return c.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
// 						})
// 				})

// 				It("continues creating blocks on leader after a config block has been successfully written out", func() {
// 					c1.cutter.CutNext = true
// 					// config block
// 					err := c1.Configure(configEnv, 0)
// 					Expect(err).NotTo(HaveOccurred())
// 					network.exec(
// 						func(c *chain) {
// 							Eventually(func() int { return c.support.WriteConfigBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
// 						})

// 					// normal block following config block
// 					err = c1.Order(env, 0)
// 					Expect(err).ToNot(HaveOccurred())
// 					network.exec(
// 						func(c *chain) {
// 							Eventually(func() int { return c.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
// 						})
// 				})
// 			})

// 			When("Snapshotting is enabled", func() {
// 				BeforeEach(func() {
// 					c1.opts.SnapshotIntervalSize = 1
// 					c1.opts.SnapshotCatchUpEntries = 1
// 				})

// 				It("keeps running if some entries in memory are purged", func() {
// 					// Scenario: snapshotting is enabled on node 1 and it purges memory storage
// 					// per every snapshot. Cluster should be correctly functioning.

// 					i, err := c1.opts.MemoryStorage.FirstIndex()
// 					Expect(err).NotTo(HaveOccurred())
// 					Expect(i).To(Equal(uint64(1)))

// 					c1.cutter.CutNext = true

// 					err = c1.Order(env, 0)
// 					Expect(err).ToNot(HaveOccurred())

// 					network.exec(
// 						func(c *chain) {
// 							Eventually(func() int { return c.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
// 						})

// 					Eventually(c1.opts.MemoryStorage.FirstIndex, LongEventualTimeout).Should(BeNumerically(">", i))
// 					i, err = c1.opts.MemoryStorage.FirstIndex()
// 					Expect(err).NotTo(HaveOccurred())

// 					err = c1.Order(env, 0)
// 					Expect(err).ToNot(HaveOccurred())

// 					network.exec(
// 						func(c *chain) {
// 							Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
// 						})

// 					Eventually(c1.opts.MemoryStorage.FirstIndex, LongEventualTimeout).Should(BeNumerically(">", i))
// 					i, err = c1.opts.MemoryStorage.FirstIndex()
// 					Expect(err).NotTo(HaveOccurred())

// 					err = c1.Order(env, 0)
// 					Expect(err).ToNot(HaveOccurred())

// 					network.exec(
// 						func(c *chain) {
// 							Eventually(c.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(3))
// 						})

// 					Eventually(c1.opts.MemoryStorage.FirstIndex, LongEventualTimeout).Should(BeNumerically(">", i))
// 				})

// 				It("lagged node can catch up using snapshot", func() {
// 					network.disconnect(2)
// 					c1.cutter.CutNext = true

// 					c2Lasti, _ := c2.opts.MemoryStorage.LastIndex()
// 					var blockCnt int
// 					// Order blocks until first index of c1 memory is greater than last index of c2,
// 					// so a snapshot will be sent to c2 when it rejoins network
// 					Eventually(func() bool {
// 						c1Firsti, _ := c1.opts.MemoryStorage.FirstIndex()
// 						if c1Firsti > c2Lasti+1 {
// 							return true
// 						}

// 						Expect(c1.Order(env, 0)).To(Succeed())
// 						blockCnt++
// 						Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(blockCnt))
// 						Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(blockCnt))
// 						return false
// 					}, LongEventualTimeout).Should(BeTrue())

// 					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(0))

// 					network.join(2, false)

// 					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(blockCnt))
// 					indices := sbft.ListSnapshots(logger, c2.opts.SnapDir)
// 					Expect(indices).To(HaveLen(1))
// 					gap := indices[0] - c2Lasti
// 					Expect(c2.puller.PullBlockCallCount()).To(Equal(int(gap)))

// 					// chain should keeps functioning
// 					Expect(c2.Order(env, 0)).To(Succeed())

// 					network.exec(
// 						func(c *chain) {
// 							Eventually(func() int { return c.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(blockCnt + 1))
// 						})
// 				})
// 			})

// 			Context("failover", func() {
// 				It("follower should step up as leader upon failover", func() {
// 					network.stop(1)
// 					network.elect(2)

// 					By("order envelope on new leader")
// 					c2.cutter.CutNext = true
// 					err := c2.Order(env, 0)
// 					Expect(err).ToNot(HaveOccurred())

// 					// block should not be produced on chain 1
// 					Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(0))

// 					// block should be produced on chain 2 & 3
// 					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
// 					Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))

// 					By("order envelope on follower")
// 					err = c3.Order(env, 0)
// 					Expect(err).ToNot(HaveOccurred())

// 					// block should not be produced on chain 1
// 					Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(0))

// 					// block should be produced on chain 2 & 3
// 					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
// 					Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(2))
// 				})

// 				It("follower cannot be elected if its log is not up-to-date", func() {
// 					network.disconnect(2)

// 					c1.cutter.CutNext = true
// 					err := c1.Order(env, 0)
// 					Expect(err).NotTo(HaveOccurred())

// 					Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
// 					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(0))
// 					Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))

// 					network.disconnect(1)
// 					network.connect(2)

// 					// node 2 has not caught up with other nodes
// 					for tick := 0; tick < 2*ELECTION_TICK-1; tick++ {
// 						c2.clock.Increment(interval)
// 						Consistently(c2.observe).ShouldNot(Receive(Equal(2)))
// 					}

// 					// When PreVote is enabled, node 2 would fail to collect enough
// 					// PreVote because its index is not up-to-date. Therefore, it
// 					// does not cause leader change on other nodes.
// 					Consistently(c3.observe).ShouldNot(Receive())
// 					network.elect(3) // node 3 has newest logs among 2&3, so it can be elected
// 				})

// 				It("PreVote prevents reconnected node from disturbing network", func() {
// 					network.disconnect(2)

// 					c1.cutter.CutNext = true
// 					err := c1.Order(env, 0)
// 					Expect(err).NotTo(HaveOccurred())

// 					Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))
// 					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(0))
// 					Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(1))

// 					network.connect(2)

// 					for tick := 0; tick < 2*ELECTION_TICK-1; tick++ {
// 						c2.clock.Increment(interval)
// 						Consistently(c2.observe).ShouldNot(Receive(Equal(2)))
// 					}

// 					Consistently(c1.observe).ShouldNot(Receive())
// 					Consistently(c3.observe).ShouldNot(Receive())
// 				})

// 				It("follower can catch up and then campaign with success", func() {
// 					network.disconnect(2)

// 					c1.cutter.CutNext = true
// 					for i := 0; i < 10; i++ {
// 						err := c1.Order(env, 0)
// 						Expect(err).NotTo(HaveOccurred())
// 					}

// 					Eventually(c1.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(10))
// 					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(0))
// 					Eventually(c3.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(10))

// 					network.join(2, false)
// 					Eventually(c2.support.WriteBlockCallCount, LongEventualTimeout).Should(Equal(10))

// 					network.disconnect(1)
// 					network.elect(2)
// 				})

// 				It("purges blockcutter, stops timer and discards created blocks if leadership is lost", func() {
// 					// enqueue one transaction into 1's blockcutter to test for purging of block cutter
// 					c1.cutter.CutNext = false
// 					err := c1.Order(env, 0)
// 					Expect(err).ToNot(HaveOccurred())
// 					Eventually(c1.cutter.CurBatch, LongEventualTimeout).Should(HaveLen(1))

// 					// no block should be written because env is not cut into block yet
// 					c1.clock.WaitForNWatchersAndIncrement(interval, 2)
// 					Consistently(c1.support.WriteBlockCallCount).Should(Equal(0))

// 					network.disconnect(1)
// 					network.elect(2)
// 					network.join(1, true)

// 					Eventually(c1.clock.WatcherCount, LongEventualTimeout).Should(Equal(1)) // blockcutter time is stopped
// 					Eventually(c1.cutter.CurBatch, LongEventualTimeout).Should(HaveLen(0))
// 					// the created block should be discarded since there is a leadership change
// 					Consistently(c1.support.WriteBlockCallCount).Should(Equal(0))

// 					network.disconnect(2)
// 					network.elect(1)

// 					err = c1.Order(env, 0)
// 					Expect(err).ToNot(HaveOccurred())

// 					// The following group of assertions is redundant - it's here for completeness.
// 					// If the blockcutter has not been reset, fast-forwarding 1's clock to 'timeout', should result in the blockcutter firing.
// 					// If the blockcucter has been reset, fast-forwarding won't do anything.
// 					//
// 					// Put differently:
// 					//
// 					// correct:
// 					// stop         start                      fire
// 					// |--------------|---------------------------|
// 					//    n*intervals              timeout
// 					// (advanced in election)
// 					//
// 					// wrong:
// 					// unstop                   fire
// 					// |---------------------------|
// 					//          timeout
// 					//
// 					//              timeout-n*interval   n*interval
// 					//                 |-----------|----------------|
// 					//                             ^                ^
// 					//                at this point of time     it should fire
// 					//                timer should not fire     at this point

// 					c1.clock.WaitForNWatchersAndIncrement(timeout-interval, 2)
// 					Eventually(func() int { return c1.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(0))
// 					Eventually(func() int { return c3.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(0))

// 					c1.clock.Increment(interval)
// 					Eventually(func() int { return c1.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
// 					Eventually(func() int { return c3.support.WriteBlockCallCount() }, LongEventualTimeout).Should(Equal(1))
// 				})

// 				It("stale leader should not be able to propose block because of lagged term", func() {
// 					network.disconnect(1)
// 					network.elect(2)
// 					network.connect(1)

// 					c1.cutter.CutNext = true
// 					err := c1.Order(env, 0)
// 					Expect(err).NotTo(HaveOccurred())

// 					network.exec(
// 						func(c *chain) {
// 							Consistently(c.support.WriteBlockCallCount).Should(Equal(0))
// 						})
// 				})

// 				It("aborts waiting for block to be committed upon leadership lost", func() {
// 					network.disconnect(1)

// 					c1.cutter.CutNext = true
// 					err := c1.Order(env, 0)
// 					Expect(err).NotTo(HaveOccurred())

// 					network.exec(
// 						func(c *chain) {
// 							Consistently(c.support.WriteBlockCallCount).Should(Equal(0))
// 						})

// 					network.elect(2)
// 					network.connect(1)

// 					c2.clock.Increment(interval)
// 					// this check guarantees that signal on resignC is consumed in commitBatches method.
// 					Eventually(c1.observe, LongEventualTimeout).Should(Receive(Equal(raft.SoftState{Lead: 2, RaftState: raft.StateFollower})))
// 				})
// 			})
// 		})
// 	})
// })

func nodeConfigFromMetadata(consenterMetadata *raftprotos.ConfigMetadata) []cluster.RemoteNode {
	var nodes []cluster.RemoteNode
	for i, consenter := range consenterMetadata.Consenters {
		// For now, skip ourselves
		if i == 0 {
			continue
		}
		serverDER, _ := pem.Decode(consenter.ServerTlsCert)
		clientDER, _ := pem.Decode(consenter.ClientTlsCert)
		node := cluster.RemoteNode{
			ID:            uint64(i),
			Endpoint:      "localhost:7050",
			ServerTLSCert: serverDER.Bytes,
			ClientTLSCert: clientDER.Bytes,
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func createMetadata(nodeCount int, tlsCA tlsgen.CA) *raftprotos.ConfigMetadata {
	md := &raftprotos.ConfigMetadata{Options: &raftprotos.Options{
		TickInterval:      time.Duration(interval).String(),
		ElectionTick:      ELECTION_TICK,
		HeartbeatTick:     HEARTBEAT_TICK,
		MaxInflightBlocks: 5,
	}}
	for i := 0; i < nodeCount; i++ {
		md.Consenters = append(md.Consenters, &raftprotos.Consenter{
			Host:          "localhost",
			Port:          7050,
			ServerTlsCert: serverTLSCert(tlsCA),
			ClientTlsCert: clientTLSCert(tlsCA),
		})
	}
	return md
}

func serverTLSCert(tlsCA tlsgen.CA) []byte {
	cert, err := tlsCA.NewServerCertKeyPair("localhost")
	if err != nil {
		panic(err)
	}
	return cert.Cert
}

func clientTLSCert(tlsCA tlsgen.CA) []byte {
	cert, err := tlsCA.NewClientCertKeyPair()
	if err != nil {
		panic(err)
	}
	return cert.Cert
}

// marshalOrPanic serializes a protobuf message and panics if this
// operation fails
func marshalOrPanic(pb proto.Message) []byte {
	data, err := proto.Marshal(pb)
	if err != nil {
		panic(err)
	}
	return data
}

// helpers to facilitate tests
type stepFunc func(dest uint64, msg *orderer.ConsensusRequest) error

type chain struct {
	id uint64

	stepLock sync.RWMutex
	step     stepFunc

	support      *consensusmocks.FakeConsenterSupport
	cutter       *mockblockcutter.Receiver
	configurator *mocks.FakeConfigurator
	rpc          *mocks.FakeRPC
	opts         sbft.Options
	puller       *mocks.FakeBlockPuller

	// store written blocks to be returned by mock block puller
	ledgerLock            sync.RWMutex
	ledger                map[uint64]*common.Block
	ledgerHeight          uint64
	lastConfigBlockNumber uint64

	unstarted chan struct{}
	stopped   chan struct{}

	*sbft.Chain
}

func newChain(timeout time.Duration, channel string, dataDir string, id uint64, raftMetadata *raftprotos.BlockMetadata, consenters map[uint64]*raftprotos.Consenter) *chain {
	rpc := &mocks.FakeRPC{}
	opts := sbft.Options{
		NodeID:        uint64(id),
		BlockMetadata: raftMetadata,
		Consenters:    consenters,
		Logger:        flogging.NewFabricLogger(zap.NewExample()),
	}

	support := &consensusmocks.FakeConsenterSupport{}
	support.ChainIDReturns(channel)
	support.SharedConfigReturns(&mockconfig.Orderer{BatchTimeoutVal: timeout})

	cutter := mockblockcutter.NewReceiver()
	close(cutter.Block)
	support.BlockCutterReturns(cutter)

	// upon leader change, lead is reset to 0 before set to actual
	// new leader, i.e. 1 -> 0 -> 2. Therefore 2 numbers will be
	// sent on this chan, so we need size to be 2
	//observe := make(chan raft.SoftState, 2)

	configurator := &mocks.FakeConfigurator{}
	puller := &mocks.FakeBlockPuller{}

	ch := make(chan struct{})
	close(ch)

	c := &chain{
		id:           id,
		support:      support,
		cutter:       cutter,
		rpc:          rpc,
		opts:         opts,
		unstarted:    ch,
		stopped:      make(chan struct{}),
		configurator: configurator,
		puller:       puller,
		ledger: map[uint64]*common.Block{
			0: getSeedBlock(), // Very first block
		},
		ledgerHeight: 1,
		//fakeFields:   fakeFields,
	}

	// receives normal blocks and metadata and appends it into
	// the ledger struct to simulate write behaviour
	appendNormalBlockToLedger := func(b *common.Block, meta []byte) {
		c.ledgerLock.Lock()
		defer c.ledgerLock.Unlock()

		b = proto.Clone(b).(*common.Block)
		bytes, err := proto.Marshal(&common.Metadata{Value: meta})
		Expect(err).NotTo(HaveOccurred())
		b.Metadata.Metadata[common.BlockMetadataIndex_ORDERER] = bytes

		lastConfigValue := utils.MarshalOrPanic(&common.LastConfig{Index: c.lastConfigBlockNumber})
		b.Metadata.Metadata[common.BlockMetadataIndex_LAST_CONFIG] = utils.MarshalOrPanic(&common.Metadata{
			Value: lastConfigValue,
		})

		c.ledger[b.Header.Number] = b
		if c.ledgerHeight < b.Header.Number+1 {
			c.ledgerHeight = b.Header.Number + 1
		}
	}

	// receives config blocks and metadata and appends it into
	// the ledger struct to simulate write behaviour
	appendConfigBlockToLedger := func(b *common.Block, meta []byte) {
		c.ledgerLock.Lock()
		defer c.ledgerLock.Unlock()

		b = proto.Clone(b).(*common.Block)
		bytes, err := proto.Marshal(&common.Metadata{Value: meta})
		Expect(err).NotTo(HaveOccurred())
		b.Metadata.Metadata[common.BlockMetadataIndex_ORDERER] = bytes

		c.lastConfigBlockNumber = b.Header.Number

		lastConfigValue := utils.MarshalOrPanic(&common.LastConfig{Index: c.lastConfigBlockNumber})
		b.Metadata.Metadata[common.BlockMetadataIndex_LAST_CONFIG] = utils.MarshalOrPanic(&common.Metadata{
			Value: lastConfigValue,
		})

		c.ledger[b.Header.Number] = b
		if c.ledgerHeight < b.Header.Number+1 {
			c.ledgerHeight = b.Header.Number + 1
		}
	}

	c.support.WriteBlockStub = appendNormalBlockToLedger
	c.support.WriteConfigBlockStub = appendConfigBlockToLedger

	// returns current ledger height
	c.support.HeightStub = func() uint64 {
		c.ledgerLock.RLock()
		defer c.ledgerLock.RUnlock()
		return c.ledgerHeight
	}

	// reads block from the ledger
	c.support.BlockStub = func(number uint64) *common.Block {
		c.ledgerLock.RLock()
		defer c.ledgerLock.RUnlock()
		return c.ledger[number]
	}

	return c
}

func (c *chain) init() {
	ch, err := sbft.NewChain(
		c.support,
		c.opts,
		c.configurator,
		c.rpc,
		func() (sbft.BlockPuller, error) { return c.puller, nil },
		nil,
		nil,
	)
	Expect(err).NotTo(HaveOccurred())
	c.Chain = ch
}

func (c *chain) start() {
	c.unstarted = nil
	c.Start()
}

func (c *chain) setStepFunc(f stepFunc) {
	c.stepLock.Lock()
	c.step = f
	c.stepLock.Unlock()
}

func (c *chain) getStepFunc() stepFunc {
	c.stepLock.RLock()
	defer c.stepLock.RUnlock()
	return c.step
}

type network struct {
	sync.RWMutex

	leader uint64
	chains map[uint64]*chain

	// links simulates the configuration of comm layer (link is bi-directional).
	// if links[left][right] == true, right can send msg to left.
	links map[uint64]map[uint64]bool
	// connectivity determines if a node is connected to network. This is used for tests
	// to simulate network partition.
	connectivity map[uint64]bool
}

func (n *network) link(from []uint64, to uint64) {
	links := make(map[uint64]bool)
	for _, id := range from {
		links[id] = true
	}

	n.Lock()
	defer n.Unlock()

	n.links[to] = links
}

func (n *network) linked(from, to uint64) bool {
	n.RLock()
	defer n.RUnlock()

	return n.links[to][from]
}

func (n *network) connect(id uint64) {
	n.Lock()
	defer n.Unlock()

	n.connectivity[id] = true
}

func (n *network) disconnect(id uint64) {
	n.Lock()
	defer n.Unlock()

	n.connectivity[id] = false
}

func (n *network) connected(id uint64) bool {
	n.RLock()
	defer n.RUnlock()

	return n.connectivity[id]
}

func (n *network) addChain(c *chain) {
	n.connect(c.id) // chain is connected by default

	c.step = func(dest uint64, msg *orderer.ConsensusRequest) error {
		if !n.linked(c.id, dest) {
			return errors.Errorf("connection refused")
		}

		if !n.connected(c.id) || !n.connected(dest) {
			return errors.Errorf("connection lost")
		}

		n.RLock()
		target := n.chains[dest]
		n.RUnlock()
		go func() {
			defer GinkgoRecover()
			target.Consensus(msg, c.id)
		}()
		return nil
	}

	c.rpc.SendConsensusStub = func(dest uint64, msg *orderer.ConsensusRequest) error {
		c.stepLock.RLock()
		defer c.stepLock.RUnlock()
		return c.step(dest, msg)
	}

	c.rpc.SendSubmitStub = func(dest uint64, msg *orderer.SubmitRequest) error {
		if !n.linked(c.id, dest) {
			return errors.Errorf("connection refused")
		}

		if !n.connected(c.id) || !n.connected(dest) {
			return errors.Errorf("connection lost")
		}

		n.RLock()
		target := n.chains[dest]
		n.RUnlock()
		go func() {
			defer GinkgoRecover()
			target.Submit(msg, c.id)
		}()
		return nil
	}

	c.puller.PullBlockStub = func(i uint64) *common.Block {
		n.RLock()
		leaderChain := n.chains[n.leader]
		n.RUnlock()

		leaderChain.ledgerLock.RLock()
		defer leaderChain.ledgerLock.RUnlock()
		block := leaderChain.ledger[i]
		return block
	}

	c.puller.HeightsByEndpointsStub = func() (map[string]uint64, error) {
		n.RLock()
		leader := n.chains[n.leader]
		n.RUnlock()

		if leader == nil {
			return nil, errors.Errorf("ledger not available")
		}

		leader.ledgerLock.RLock()
		defer leader.ledgerLock.RUnlock()
		return map[string]uint64{"leader": leader.ledgerHeight}, nil
	}

	c.configurator.ConfigureCalls(func(channel string, nodes []cluster.RemoteNode) {
		var ids []uint64
		for _, node := range nodes {
			ids = append(ids, node.ID)
		}
		n.link(ids, c.id)
	})

	n.Lock()
	defer n.Unlock()
	n.chains[c.id] = c
}

func createNetwork(timeout time.Duration, channel string, dataDir string, raftMetadata *raftprotos.BlockMetadata, consenters map[uint64]*raftprotos.Consenter) *network {
	n := &network{
		chains:       make(map[uint64]*chain),
		connectivity: make(map[uint64]bool),
		links:        make(map[uint64]map[uint64]bool),
	}

	for _, nodeID := range raftMetadata.ConsenterIds {
		dir, err := ioutil.TempDir(dataDir, fmt.Sprintf("node-%d-", nodeID))
		Expect(err).NotTo(HaveOccurred())

		m := proto.Clone(raftMetadata).(*raftprotos.BlockMetadata)
		n.addChain(newChain(timeout, channel, dir, nodeID, m, consenters))
	}

	return n
}

// tests could alter configuration of a chain before creating it
func (n *network) init() {
	n.exec(func(c *chain) { c.init() })
}

func (n *network) start(ids ...uint64) {
	nodes := ids
	if len(nodes) == 0 {
		for i := range n.chains {
			nodes = append(nodes, i)
		}
	}

	for _, id := range nodes {
		n.chains[id].start()

		// When the Raft node bootstraps, it produces a ConfChange
		// to add itself, which needs to be consumed with Ready().
		// If there are pending configuration changes in raft,
		// it refused to campaign, no matter how many ticks supplied.
		// This is not a problem in production code because eventually
		// raft.Ready will be consumed as real time goes by.
		//
		// However, this is problematic when using fake clock and artificial
		// ticks. Instead of ticking raft indefinitely until raft.Ready is
		// consumed, this check is added to indirectly guarantee
		// that first ConfChange is actually consumed and we can safely
		// proceed to tick raft.

	}
}

func (n *network) stop(ids ...uint64) {
	nodes := ids
	if len(nodes) == 0 {
		for i := range n.chains {
			nodes = append(nodes, i)
		}
	}

	for _, id := range nodes {
		c := n.chains[id]
		c.Halt()
		Eventually(c.Errored).Should(BeClosed())
		select {
		case <-c.stopped:
		default:
			close(c.stopped)
		}
	}
}

func (n *network) exec(f func(c *chain), ids ...uint64) {
	if len(ids) == 0 {
		for _, c := range n.chains {
			f(c)
		}

		return
	}

	for _, i := range ids {
		f(n.chains[i])
	}
}

// sets the configEnv var declared above
func newConfigEnv(chainID string, headerType common.HeaderType, configUpdateEnv *common.ConfigUpdateEnvelope) *common.Envelope {
	return &common.Envelope{
		Payload: marshalOrPanic(&common.Payload{
			Header: &common.Header{
				ChannelHeader: marshalOrPanic(&common.ChannelHeader{
					Type:      int32(headerType),
					ChannelId: chainID,
				}),
			},
			Data: marshalOrPanic(&common.ConfigEnvelope{
				LastUpdate: &common.Envelope{
					Payload: marshalOrPanic(&common.Payload{
						Header: &common.Header{
							ChannelHeader: marshalOrPanic(&common.ChannelHeader{
								Type:      int32(common.HeaderType_CONFIG_UPDATE),
								ChannelId: chainID,
							}),
						},
						Data: marshalOrPanic(configUpdateEnv),
					}), // common.Payload
				}, // LastUpdate
			}),
		}),
	}
}

func newConfigUpdateEnv(chainID string, oldValues, newValues map[string]*common.ConfigValue) *common.ConfigUpdateEnvelope {
	return &common.ConfigUpdateEnvelope{
		ConfigUpdate: marshalOrPanic(&common.ConfigUpdate{
			ChannelId: chainID,
			ReadSet: &common.ConfigGroup{
				Groups: map[string]*common.ConfigGroup{
					"Orderer": {
						Values: oldValues,
					},
				},
			},
			WriteSet: &common.ConfigGroup{
				Groups: map[string]*common.ConfigGroup{
					"Orderer": {
						Values: newValues,
					},
				},
			}, // WriteSet
		}),
	}
}

func getSeedBlock() *common.Block {
	return &common.Block{
		Header:   &common.BlockHeader{},
		Data:     &common.BlockData{Data: [][]byte{[]byte("foo")}},
		Metadata: &common.BlockMetadata{Metadata: make([][]byte, 4)},
	}
}

func noOpBlockPuller() (sbft.BlockPuller, error) {
	bp := &mocks.FakeBlockPuller{}
	return bp, nil
}
