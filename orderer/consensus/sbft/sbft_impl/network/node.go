package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"../consensus"
)

const f = 1

type Node struct {
	Block           *consensus.RequestBlock
	NodeIP          string
	NodeID          string
	NodeTable       map[string]string // key=nodeID, value=url
	RevNodeTable    map[string]string
	View            *View
	CurrentState    *consensus.State
	CommittedMsgs   []*consensus.RequestBlock // kinda block.
	MsgBuffer       *MsgBuffer
	MsgEntrance     chan interface{}
	MsgDelivery     chan interface{}
	MsgTSS          chan string
	Alarm           chan bool
	MsgTSCC         []*consensus.MessageTSCC
	MsgTSEC         []*consensus.MessageTSEC
	MsgTSCP         []*consensus.MessageTSCP
	Keys            *consensus.Keys
	N               int
	ProposedViewMap map[int]int
}

type MsgBuffer struct {
	RequestBlocks         []*consensus.RequestBlock
	PrePrepareMsgs        []*consensus.PrePrepareMsg
	SignShareMsgs         []*consensus.SignShareMsg
	PrepareMsgs           []*consensus.PrepareMsg
	CommitMsgs            []*consensus.CommitMsg
	FastCommitProofMsgs   []*consensus.FastCommitProofMsg
	SlowCommitProofMsgs   []*consensus.SlowCommitProofMsg
	SignStateMsgs         []*consensus.SignStateMsg
	FullExecuteProofMsgs  []*consensus.FullExecuteProofMsg
	KeyMsgs               []*consensus.Keys
	ViewChangeInitMsgs    []*consensus.ViewChangeInitMsg
	ViewChangeExecuteMsgs []*consensus.ViewChangeExecuteMsg
}

var start time.Time
var end time.Time
var SignStateDone bool = false
var CommitDone bool = false
var SignShareDone bool = false

const FPT = time.Millisecond * 30
const SPT = time.Millisecond * 50

type View struct {
	ID          int
	Primary     string
	C_collector string
	E_collector string
}

const ResolvingTimeDuration = time.Millisecond * 1000 // 1 second, for clering out buffered msgs.

func NewNode(nodeIP string) *Node {
	const viewID = 0 // temporary.

	node := &Node{
		NodeIP:       nodeIP,
		Block:        nil,
		NodeID:       " ",
		NodeTable:    nil,
		RevNodeTable: nil,
		View: &View{
			ID:          viewID,
			Primary:     "0",
			C_collector: "1",
			E_collector: "2",
		},

		// Consensus-related struct
		CurrentState:  nil,
		CommittedMsgs: make([]*consensus.RequestBlock, 0),
		MsgBuffer: &MsgBuffer{
			RequestBlocks:         make([]*consensus.RequestBlock, 0),
			PrePrepareMsgs:        make([]*consensus.PrePrepareMsg, 0),
			SignShareMsgs:         make([]*consensus.SignShareMsg, 0),
			PrepareMsgs:           make([]*consensus.PrepareMsg, 0),
			CommitMsgs:            make([]*consensus.CommitMsg, 0),
			FastCommitProofMsgs:   make([]*consensus.FastCommitProofMsg, 0),
			SlowCommitProofMsgs:   make([]*consensus.SlowCommitProofMsg, 0),
			SignStateMsgs:         make([]*consensus.SignStateMsg, 0),
			FullExecuteProofMsgs:  make([]*consensus.FullExecuteProofMsg, 0),
			KeyMsgs:               make([]*consensus.Keys, 0),
			ViewChangeInitMsgs:    make([]*consensus.ViewChangeInitMsg, 0),
			ViewChangeExecuteMsgs: make([]*consensus.ViewChangeExecuteMsg, 0),
		},

		// Channels
		MsgEntrance: make(chan interface{}, 100),
		MsgDelivery: make(chan interface{}, 100),
		MsgTSS:      make(chan string, 100),
		Alarm:       make(chan bool, 100),

		//Msg-TS Queue

		MsgTSCC: make([]*consensus.MessageTSCC, 0),
		MsgTSEC: make([]*consensus.MessageTSEC, 0),
		MsgTSCP: make([]*consensus.MessageTSCP, 0),

		//Keys
		Keys: nil,

		N:               0,
		ProposedViewMap: make(map[int]int),
	}

	// Start message dispatcher
	go node.dispatchMsg()

	// Start alarm trigger
	go node.alarmToDispatcher()

	// Start message resolver
	go node.resolveMsg()

	go node.requestkeys(nodeIP)

	return node
}

//----------------------------------------------------
func (node *Node) requestkeys(NodeID string) error {

	jsonMsg, err := json.Marshal(NodeID)
	if err != nil {
		return err
	}
	fmt.Println("node request sent for keys")
	send("localhost:1109"+"/RequestKey", jsonMsg)

	return nil
}

//-----------------------------------------------------
func (node *Node) GetKeys(Msg *consensus.Keys) error {
	node.Keys = Msg

	// fmt.Println(node.Keys.KeyShare_Sigma)
	node.RevNodeTable = makerevnodetable(Msg.TotalNodes)
	node.NodeTable = makenodetable(Msg.TotalNodes)

	node.NodeID = node.RevNodeTable[node.NodeIP]
	node.N = Msg.TotalNodes
	node.View.ID = Msg.ViewID
	node.View.Primary = strconv.Itoa((node.View.ID * 3) % node.N)
	node.View.C_collector = strconv.Itoa((node.View.ID*3 + 1) % node.N)
	node.View.E_collector = strconv.Itoa((node.View.ID*3 + 2) % node.N)

	node.CurrentState = nil
	// fmt.Println(node.Keys)
	fmt.Println("Keys Loaded Successfully")
	return nil
}

//-----------------------------------------------------
func (node *Node) GetViewChangeInitMsg(Msg *consensus.ViewChangeInitMsg) error {
	fmt.Println("View change Init msg received at nodes")
	viewchangeexecute := node.ViewChangeExecute(Msg)

	node.BroadcastViewChange(viewchangeexecute, "/ViewChangeExecute") //Todo: Changing this broadcast status

	return nil
}

//-----------------------------------------------------
func (node *Node) GetViewChangeExecuteMsg(Msg *consensus.ViewChangeExecuteMsg) error {
	// fmt.Println("View change Execute msg received at nodes")
	node.ProposedViewMap[Msg.ProposedViewID]++

	if node.ProposedViewMap[Msg.ProposedViewID] > (node.N)/2 { //temporary
		node.CurrentState = nil
		if node.NodeID == node.View.Primary {
			jsonMsg, err := json.Marshal(Msg.Block)
			if err != nil {
				return err
			}
			fmt.Println("&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&")
			send(node.NodeTable[strconv.Itoa((Msg.ProposedViewID*3)%node.N)]+"/Block", jsonMsg)
		}

		if node.NodeID == strconv.Itoa((Msg.ProposedViewID*3)%node.N) {
			jsonMsg, err := json.Marshal(Msg)
			if err != nil {
				return err
			}
			send(node.NodeTable["KS"]+"/ViewChangeExecute", jsonMsg)
			flag := false

			fmt.Println("Waiting for new keys load for all the nodes")

			for {
				if flag {
					break
				}
				select {
				case msg := <-node.MsgTSS:
					fmt.Println("******Keys for new view loaded successfully*****")
					fmt.Println(msg)
					flag = true
					break
				}
			}

			fmt.Println("******Breaking loop*****")
		}
		node.View.ID = Msg.ProposedViewID
		node.View.Primary = strconv.Itoa((node.View.ID * 3) % node.N)
		node.View.C_collector = strconv.Itoa((node.View.ID*3 + 1) % node.N)
		node.View.E_collector = strconv.Itoa((node.View.ID*3 + 2) % node.N)
		// node.requestkeys(node.NodeIP)
		fmt.Println("+++++++++++New View Loaded with Primary:")
		fmt.Println(node.View.Primary)
		fmt.Println(node.View.C_collector)
		fmt.Println(node.View.E_collector)

		for k := range node.ProposedViewMap {
			delete(node.ProposedViewMap, k)
		}
	}

	// viewchangeexecute := node.CurrentState.ViewChangeExecute(Msg)

	return nil
}

/*---------------------------------------------------------------------------------*/

func (node *Node) ViewChangeInit(ViewID int, NodeID string, Block *consensus.RequestBlock) *consensus.ViewChangeInitMsg {

	// state.CurrentStage = ViewChangeInitiated
	return &consensus.ViewChangeInitMsg{
		CurrViewID:     ViewID,
		ProposedViewID: ViewID + 1,
		NodeID:         NodeID,
		Block:          Block,
	}
}

/*---------------------------------------------------------------------------------*/

func (node *Node) ViewChangeExecute(Msg *consensus.ViewChangeInitMsg) *consensus.ViewChangeExecuteMsg {

	return &consensus.ViewChangeExecuteMsg{
		CurrViewID:     Msg.CurrViewID,
		ProposedViewID: Msg.ProposedViewID,
		NodeID:         Msg.NodeID,
		Block:          Msg.Block,
	}
}

/*--------------------------------------------------------------------*/
func (node *Node) Broadcast(msg interface{}, path string) map[string]error {

	errorMap := make(map[string]error)

	for nodeID, url := range node.NodeTable {
		if nodeID == node.NodeID || nodeID == node.View.E_collector || nodeID == node.View.C_collector || nodeID == "KS" { //remove collector nodes from broadcast
			continue
		}
		fmt.Println(nodeID)
		jsonMsg, err := json.Marshal(msg)
		if err != nil {
			errorMap[nodeID] = err
			continue
		}

		fmt.Println(url + path)
		send(url+path, jsonMsg)
	}

	if len(errorMap) == 0 {
		return nil
	} else {
		return errorMap
	}
}

/*--------------------------------------------------------------------*/
func (node *Node) BroadcastViewChange(msg interface{}, path string) map[string]error {

	errorMap := make(map[string]error)

	for nodeID, url := range node.NodeTable {
		if nodeID == "KS" {
			continue
		}
		fmt.Println(nodeID)
		jsonMsg, err := json.Marshal(msg)
		if err != nil {
			errorMap[nodeID] = err
			continue
		}

		fmt.Println(url + path)
		send(url+path, jsonMsg)
	}

	if len(errorMap) == 0 {
		return nil
	} else {
		return errorMap
	}
}

/*--------------------------------------------------------------------*/

// GetReq can be called when the node's CurrentState is nil.
// Consensus start procedure for the Primary.
func (node *Node) GetReq(reqMsg *consensus.RequestBlock) error {
	LogMsg(reqMsg)

	// Create a new state for the new consensus.
	//Todo: New req lead to the creation of new state

	// fmt.Println("*#************4. The new request has been resolved *******#")
	err := node.createStateForNewConsensus()
	if err != nil {
		return err
	}

	// Start the consensus process. Seperate consensus for another transaction
	prePrepareMsg, err := node.CurrentState.StartConsensus(reqMsg) //current state has been changed to new state
	if err != nil {
		return err
	}

	LogStage(fmt.Sprintf("Consensus Process (ViewID:%d)", node.CurrentState.ViewID), false)

	// Send getPrePrepare message
	if prePrepareMsg != nil {
		node.Broadcast(prePrepareMsg, "/preprepare") //Todo: Changing this broadcast status
		LogStage("Pre-prepare", true)
	}

	return nil
}

/*--------------------------------------------------------------------*/

// GetPrePrepare can be called when the node's CurrentState is nil.
// Consensus start procedure for normal participants.
func (node *Node) GetPrePrepare(prePrepareMsg *consensus.PrePrepareMsg) error {
	LogMsg(prePrepareMsg)

	// Create a new state for the new consensus. All the nodes other than leader and collectors are also creating new states

	// fmt.Println("+++++++1++++++++")
	err := node.createStateForNewConsensus()
	if err != nil {
		return err
	}
	// fmt.Println("++++++++2+++++++")

	//--------------------------Insted of sending just keys. Sign the msg and send it
	// Calculate h = H(s||v||r)[Taking just r], TS_FP: σi(h),  TS_SP: τi(h)

	var Sigma_sign string
	var Tou_sign string

	s := consensus.Hash([]byte(fmt.Sprintf("%v", prePrepareMsg.Block)))
	// fmt.Printf("Hash sent for block")
	// fmt.Printf(s)
	SignPacket := makesignpacket(s, node.Keys.KeyShare_Sigma, node.NodeID, "Sigma")
	flag := false

	// fmt.Println("++++++++3+++++++")

	jsonMsg, err := json.Marshal(SignPacket)
	if err != nil {
		return err
	}
	fmt.Println("node request sent for Sigma sign")
	send(node.NodeTable["KS"]+"/SignKeys", jsonMsg)

	// fmt.Println("++++++++4+++++++")

	for {
		if flag {
			break
		}
		select {
		case msg := <-node.MsgTSS:
			// fmt.Println("******HERE2222*****")
			Sigma_sign = msg
			flag = true
			break
		}
	}

	// fmt.Println("++++++++5+++++++")
	SignPacket = makesignpacket(s, node.Keys.KeyShare_Tou, node.NodeID, "Tou")

	flag = false

	jsonMsg1, err1 := json.Marshal(SignPacket)
	if err1 != nil {
		return err1
	}
	fmt.Println("node request sent for Tou Sign")
	send(node.NodeTable["KS"]+"/SignKeys", jsonMsg1)

	for {
		if flag {
			break
		}
		select {
		case msg := <-node.MsgTSS:
			// fmt.Println("******HERE2222*****")
			Tou_sign = msg
			flag = true
			break
		}
	}

	fmt.Println("*******Sign Received")

	// fmt.Println(Sigma_sign)

	// fmt.Println(Tou_sign)

	//--------------------------------------------------------------------------
	SignShareMsg, err := node.CurrentState.PrePrepare(prePrepareMsg, Sigma_sign, Tou_sign)
	if err != nil {
		return err
	}
	fmt.Println("+++++++3++++++++")

	if SignShareMsg != nil {
		// Attach node ID to the message
		SignShareMsg.NodeID = node.NodeID
		jsonMsg, err := json.Marshal(SignShareMsg)
		if err != nil {
			return err
		}
		// fmt.Println("+++++++4++++++++")

		LogStage("Pre-prepare", true)
		send(node.NodeTable[node.View.C_collector]+"/SignShare", jsonMsg)
		LogStage("SignShareMsg", false)
		fmt.Println(node.CurrentState)
	}

	return nil
}

/*--------------------------------------------------------------------*/

func (node *Node) GetSignShare(SignShareMsg *consensus.SignShareMsg) error {
	LogMsg(SignShareMsg)

	//Will make a queue to keep msgs and corresponding arrival times and when
	//the difference between the arrival of first msg and current msg is greater than the waiting time
	//we will stop accepting msgs and send the queue further
	//FP: if Queue size > 2f else SP.

	// fmt.Println(SignShareDone)
	if SignShareDone == true || node.CurrentState != nil { //To avoid further msgs to start new slowcommitProof
		return nil
	}

	if len(node.MsgTSCC) == 0 || time.Now().Sub(node.MsgTSCC[0].TS) < FPT {
		node.MsgTSCC = append(node.MsgTSCC, &consensus.MessageTSCC{*SignShareMsg, time.Now()})
		fmt.Println("&&&&&&&&&&&&TimeDiff&&&&&&&&&&&&&&")
		fmt.Println(time.Now().Sub(node.MsgTSCC[0].TS))
		if len(node.MsgTSCC) < node.N-3 { //all nodes are here
			return nil
		}
	}

	SignShareDone = true

	err := node.createStateForNewConsensus()
	if err != nil {
		return err
	}

	if len(node.MsgTSCC) > 3*f {

		fmt.Println("+++++++++FP+++++++++")

		//Send all the signs ti[h] and also groupkey_sigma to the keystore node. Veirfy there and return signature

		var Sigma_FullSign string

		s := consensus.Hash([]byte(fmt.Sprintf("%v", node.MsgTSCC[0].WaitMsg.Block)))

		fmt.Printf("Hash sent for block")
		// fmt.Printf(s)

		AccumulatePacket := AccumulateFP(node.MsgTSCC, s, node.NodeID, node.Keys.GroupKey_Sigma)

		flag := false

		fmt.Println("++++++++3+++++++")

		jsonMsg, err := json.Marshal(AccumulatePacket)
		if err != nil {
			return err
		}
		fmt.Println("Collector request sent for Accumulation[FP]")
		send(node.NodeTable["KS"]+"/Accumulate", jsonMsg)

		fmt.Println("++++++++4+++++++")

		for {
			if flag {
				break
			}
			select {
			case msg := <-node.MsgTSS:
				fmt.Println("******HERE2222*****")
				Sigma_FullSign = msg
				flag = true
				break
			}
		}

		fmt.Println("***************FP*********")
		FastCommitProofMsg, err := node.CurrentState.SignShareFP(node.MsgTSCC, Sigma_FullSign)
		// fmt.Println("***************CompletedFP*********")
		if err != nil {
			return err
		}

		// when the prepare msg has been reached to more than 2f
		if FastCommitProofMsg != nil {
			// Attach node ID to the message
			FastCommitProofMsg.NodeID = node.NodeID

			LogStage("FastCommitProof", true)
			node.Broadcast(FastCommitProofMsg, "/FastCommitProof")
			LogStage("SignState", false)
		}
		//Trying to make nodes status nil for the next txn
		node.MsgTSCC = node.MsgTSCC[:0]
		node.CurrentState = nil
		node.MsgBuffer.CommitMsgs = node.MsgBuffer.CommitMsgs[:0]
		node.MsgBuffer.FastCommitProofMsgs = node.MsgBuffer.FastCommitProofMsgs[:0]
		node.MsgBuffer.FullExecuteProofMsgs = node.MsgBuffer.FullExecuteProofMsgs[:0]
		node.MsgBuffer.PrePrepareMsgs = node.MsgBuffer.PrePrepareMsgs[:0]
		node.MsgBuffer.PrepareMsgs = node.MsgBuffer.PrepareMsgs[:0]
		node.MsgBuffer.RequestBlocks = node.MsgBuffer.RequestBlocks[:0]
		node.MsgBuffer.SignShareMsgs = node.MsgBuffer.SignShareMsgs[:0]
		node.MsgBuffer.SignStateMsgs = node.MsgBuffer.SignStateMsgs[:0]
		node.MsgBuffer.SlowCommitProofMsgs = node.MsgBuffer.SlowCommitProofMsgs[:0]
		fmt.Println(node.CurrentState)
		SignShareDone = false
		/////////////////////////////////////////////////////
		return nil

	} else if len(node.MsgTSCC) >= 2*f {
		fmt.Println(len(node.MsgTSCC))

		fmt.Println("+++++++++SP+++++++++")

		var Tou_FullSign string

		s := consensus.Hash([]byte(fmt.Sprintf("%v", node.MsgTSCC[0].WaitMsg.Block)))

		// fmt.Printf("Hash sent for block")
		// fmt.Printf(s)

		AccumulatePacket := AccumulateSP(node.MsgTSCC, s, node.NodeID, node.Keys.GroupKey_Tou)

		flag := false

		// fmt.Println("++++++++3+++++++")

		jsonMsg, err := json.Marshal(AccumulatePacket)
		if err != nil {
			return err
		}
		fmt.Println("Collector request sent for Accumulation[SP]")
		send(node.NodeTable["KS"]+"/Accumulate", jsonMsg)

		// fmt.Println("++++++++4+++++++")

		for {
			if flag {
				break
			}
			select {
			case msg := <-node.MsgTSS:
				// fmt.Println("******HERE2222*****")
				Tou_FullSign = msg
				flag = true
				break
			}
		}

		PrepareMsg, err := node.CurrentState.SignShareSP(node.MsgTSCC, Tou_FullSign)
		// fmt.Println("+++++++++++++++++++++++++++++++")
		if err != nil {
			// fmt.Println("+++++++++++++++++++++++++++++++")
			return err
		}
		//fmt.Println("+++++++++SSSPComplete+++++++++")
		if PrepareMsg != nil {
			// Attach node ID to the message
			PrepareMsg.NodeID = node.NodeID

			LogStage("Prepare", true)
			node.Broadcast(PrepareMsg, "/prepare")
			LogStage("Commit", false)
		}
		SignShareDone = false
		node.MsgTSCC = node.MsgTSCC[:0]
		/////////////////////////////////////////////////////

		return nil

	} else { //Timer expired but not enough signs, even to make it to the slow path
		fmt.Println("+++++++++ViewChange Initiating+++++++++")

		viewchangeinitMsg := node.ViewChangeInit(node.View.ID, node.NodeID, SignShareMsg.Block)

		jsonMsg, err := json.Marshal(viewchangeinitMsg)
		if err != nil {
			return err
		}
		send(node.NodeTable["KS"]+"/ViewChangeInit", jsonMsg)

		node.BroadcastViewChange(viewchangeinitMsg, "/ViewChangeInit")

		return nil
	}

}

/*--------------------------------------------------------------------*/

func (node *Node) GetPrepare(prepareMsg *consensus.PrepareMsg) error {
	LogMsg(prepareMsg)

	var TouTou_sign string

	s := consensus.Hash([]byte(fmt.Sprintf("%v", prepareMsg.Prepare_TS_SP))) //Taking sig of Full tou sig
	SignPacket := makesignpacket(s, node.Keys.KeyShare_Tou, node.NodeID, "Tou")
	flag := false

	fmt.Println("++++++++3+++++++")

	jsonMsg, err := json.Marshal(SignPacket)
	if err != nil {
		return err
	}
	fmt.Println("node request sent for TouTou sign")
	send(node.NodeTable["KS"]+"/SignKeys", jsonMsg)

	fmt.Println("++++++++4+++++++")

	for {
		if flag {
			break
		}
		select {
		case msg := <-node.MsgTSS:
			fmt.Println("******HERE2222*****")
			TouTou_sign = msg
			flag = true
			break
		}
	}

	fmt.Println("++++++++5+++++++")

	CommitMsg, err := node.CurrentState.Prepare(prepareMsg, TouTou_sign)
	if err != nil {
		return err
	}

	// when the prepare msg has been reached to more than 2f
	if CommitMsg != nil {
		// Attach node ID to the message
		CommitMsg.NodeID = node.NodeID
		jsonMsg, err := json.Marshal(CommitMsg)
		if err != nil {
			return err
		}
		LogStage("Prepare", true)
		send(node.NodeTable[node.View.C_collector]+"/Commit", jsonMsg)
		LogStage("Commit", false)
	}

	return nil
}

/*--------------------------------------------------------------------*/

func (node *Node) GetFastCommitProof(FastCommitProofMsg *consensus.FastCommitProofMsg) error {
	LogMsg(FastCommitProofMsg)

	var Pi_sign string

	s := consensus.Hash([]byte(fmt.Sprintf("%v", FastCommitProofMsg.Block)))
	SignPacket := makesignpacket(s, node.Keys.KeyShare_Pi, node.NodeID, "Pi")
	flag := false

	fmt.Println("++++++++3+++++++")

	jsonMsg, err := json.Marshal(SignPacket)
	if err != nil {
		return err
	}
	fmt.Println("node request sent for sign")
	send(node.NodeTable["KS"]+"/SignKeys", jsonMsg)

	fmt.Println("++++++++4+++++++")

	for {
		if flag {
			break
		}
		select {
		case msg := <-node.MsgTSS:
			fmt.Println("******HERE2222*****")
			Pi_sign = msg
			flag = true
			break
		}
	}

	fmt.Println("++++++++5+++++++")

	SignStateMsg, CommittedMsg, err := node.CurrentState.SignStateFC(FastCommitProofMsg, Pi_sign)
	if err != nil {
		return err
	}

	if SignStateMsg != nil {
		node.CommittedMsgs = append(node.CommittedMsgs, CommittedMsg)
		// Attach node ID to the message
		SignStateMsg.NodeID = node.NodeID
		jsonMsg, err := json.Marshal(SignStateMsg)
		if err != nil {
			return err
		}
		LogStage("FastCommitProof", true)
		send(node.NodeTable[node.View.E_collector]+"/SignState", jsonMsg)
		LogStage("SignState", false)
		// fmt.Println(node.CurrentState)
	}

	return nil
}

/*--------------------------------------------------------------------*/

func (node *Node) GetCommit(CommitMsg *consensus.CommitMsg) error {
	LogMsg(CommitMsg)
	fmt.Println("++++++++0.1+++++++")
	if CommitDone == true || node.CurrentState == nil || node.CurrentState.CurrentStage == consensus.CommitTriggered { //To avoid further msgs to start new slowcommitProof
		return nil
	}
	fmt.Println("++++++++0.2+++++++")

	if len(node.MsgTSCP) == 0 || time.Now().Sub(node.MsgTSCP[0].TS) < SPT {
		node.MsgTSCP = append(node.MsgTSCP, &consensus.MessageTSCP{*CommitMsg, time.Now()})
		if len(node.MsgTSCP) < node.N-3 { //all nodes are here///////HARD-CODED
			return nil
		}
	}
	fmt.Println("++++++++0.3+++++++")
	CommitDone = true

	var TouTou_FullSign string

	fmt.Println("++++++++1+++++++")
	s := consensus.Hash([]byte(fmt.Sprintf("%v", node.MsgTSCP[0].WaitMsg.Prepare_TS_SP)))

	// fmt.Printf("Hash sent for block")
	// fmt.Printf(s)

	fmt.Println("++++++++2+++++++")
	AccumulatePacket := AccumulateTouTou(node.MsgTSCP, s, node.NodeID, node.Keys.GroupKey_Tou)

	flag := false

	fmt.Println("++++++++3+++++++")

	jsonMsg, err := json.Marshal(AccumulatePacket)
	if err != nil {
		return err
	}
	fmt.Println("Collector request sent for Accumulation[TouTou]")
	send(node.NodeTable["KS"]+"/Accumulate", jsonMsg)

	fmt.Println("++++++++4+++++++")

	for {
		if flag {
			break
		}
		select {
		case msg := <-node.MsgTSS:
			fmt.Println("******HERE2222*****")
			TouTou_FullSign = msg
			flag = true
			break
		}
	}
	//send the entire array of signshare msgs to calculate sig
	SlowCommitProofMsg, err := node.CurrentState.Commit(node.MsgTSCP, TouTou_FullSign)
	if err != nil {
		return err
	}
	if SlowCommitProofMsg != nil {
		// Attach node ID to the message
		LogStage("CommitMsg", true)
		node.Broadcast(SlowCommitProofMsg, "/SlowCommitProof")
		LogStage("SlowCommitProof", false)
	}

	//Trying to make nodes status nil for the next txn

	node.CurrentState = nil
	node.MsgTSCP = node.MsgTSCP[:0]
	node.MsgBuffer.CommitMsgs = node.MsgBuffer.CommitMsgs[:0]
	node.MsgBuffer.FastCommitProofMsgs = node.MsgBuffer.FastCommitProofMsgs[:0]
	node.MsgBuffer.FullExecuteProofMsgs = node.MsgBuffer.FullExecuteProofMsgs[:0]
	node.MsgBuffer.PrePrepareMsgs = node.MsgBuffer.PrePrepareMsgs[:0]
	node.MsgBuffer.PrepareMsgs = node.MsgBuffer.PrepareMsgs[:0]
	node.MsgBuffer.RequestBlocks = node.MsgBuffer.RequestBlocks[:0]
	node.MsgBuffer.SignShareMsgs = node.MsgBuffer.SignShareMsgs[:0]
	node.MsgBuffer.SignStateMsgs = node.MsgBuffer.SignStateMsgs[:0]
	node.MsgBuffer.SlowCommitProofMsgs = node.MsgBuffer.SlowCommitProofMsgs[:0]
	fmt.Println(node.CurrentState)
	CommitDone = false
	/////////////////////////////////////////////////////
	return nil
}

/*--------------------------------------------------------------------*/

func (node *Node) GetSlowCommitProof(SlowCommitProofMsg *consensus.SlowCommitProofMsg) error {
	LogMsg(SlowCommitProofMsg)

	var Pi_sign string

	s := consensus.Hash([]byte(fmt.Sprintf("%v", SlowCommitProofMsg.Block)))
	SignPacket := makesignpacket(s, node.Keys.KeyShare_Pi, node.NodeID, "Pi")
	flag := false

	fmt.Println("++++++++3+++++++")

	jsonMsg, err := json.Marshal(SignPacket)
	if err != nil {
		return err
	}
	fmt.Println("node request sent for sign")
	send(node.NodeTable["KS"]+"/SignKeys", jsonMsg)

	fmt.Println("++++++++4+++++++")

	for {
		if flag {
			break
		}
		select {
		case msg := <-node.MsgTSS:
			fmt.Println("******HERE2222*****")
			Pi_sign = msg
			flag = true
			break
		}
	}

	fmt.Println("++++++++5+++++++")

	SignStateMsg, CommittedMsg, err := node.CurrentState.SignStateSC(SlowCommitProofMsg, Pi_sign)
	if err != nil {
		return err
	}

	if SignStateMsg != nil {
		// Attach node ID to the message
		// Save the last version of committed messages to node.
		node.CommittedMsgs = append(node.CommittedMsgs, CommittedMsg)
		// println(len(node.CommittedMsgs))
		SignStateMsg.NodeID = node.NodeID
		jsonMsg, err := json.Marshal(SignStateMsg)
		if err != nil {
			return err
		}
		LogStage("SlowCommitProof", true)
		send(node.NodeTable[node.View.E_collector]+"/SignState", jsonMsg)
		LogStage("SignState", false)
	}

	return nil
}

/*--------------------------------------------------------------------*/

func (node *Node) GetSignState(SignStateMsg *consensus.SignStateMsg) error {
	LogMsg(SignStateMsg)

	// fmt.Println(SignStateDone)
	if SignStateDone == true || node.CurrentState != nil { //To avoid further msgs to start new signstate
		return nil
	}
	if len(node.MsgTSEC) == 0 || time.Now().Sub(node.MsgTSEC[0].TS) < SPT {
		node.MsgTSEC = append(node.MsgTSEC, &consensus.MessageTSEC{*SignStateMsg, time.Now()})
		if len(node.MsgTSEC) < node.N-3 { //all nodes are here, Technically just requires f nodes
			return nil
		}
	}
	SignStateDone = true

	err := node.createStateForNewConsensus()
	if err != nil {
		return err
	}

	var Pi_FullSign string

	s := consensus.Hash([]byte(fmt.Sprintf("%v", node.MsgTSEC[0].WaitMsg.Block)))

	fmt.Printf("Hash sent for block")
	fmt.Printf(s)

	AccumulatePacket := AccumulatePi(node.MsgTSEC, s, node.NodeID, node.Keys.GroupKey_Pi)

	flag := false

	fmt.Println("++++++++3+++++++")

	jsonMsg, err := json.Marshal(AccumulatePacket)
	if err != nil {
		return err
	}
	fmt.Println("Collector request sent for Accumulation[]")
	send(node.NodeTable["KS"]+"/Accumulate", jsonMsg)

	fmt.Println("++++++++4+++++++")

	for {
		if flag {
			break
		}
		select {
		case msg := <-node.MsgTSS:
			fmt.Println("******HERE2222*****")
			Pi_FullSign = msg
			flag = true
			break
		}
	}

	FullExecuteProofMsg, err := node.CurrentState.FullExecuteProof(node.MsgTSEC, Pi_FullSign)
	if err != nil {
		return err
	}

	if FullExecuteProofMsg != nil {
		// Attach node ID to the message
		//FullExecuteProofMsg.NodeID = node.NodeID
		LogStage("SignState", true)
		node.Broadcast(FullExecuteProofMsg, "/FullExecuteProof")
		LogStage("FullExecuteProof", false)
	}
	// fmt.Println(node.CurrentState)
	//Trying to make nodes status nil for the next txn
	node.MsgTSEC = node.MsgTSEC[:0]
	node.CurrentState = nil
	node.MsgBuffer.CommitMsgs = node.MsgBuffer.CommitMsgs[:0]
	node.MsgBuffer.FastCommitProofMsgs = node.MsgBuffer.FastCommitProofMsgs[:0]
	node.MsgBuffer.FullExecuteProofMsgs = node.MsgBuffer.FullExecuteProofMsgs[:0]
	node.MsgBuffer.PrePrepareMsgs = node.MsgBuffer.PrePrepareMsgs[:0]
	node.MsgBuffer.PrepareMsgs = node.MsgBuffer.PrepareMsgs[:0]
	node.MsgBuffer.RequestBlocks = node.MsgBuffer.RequestBlocks[:0]
	node.MsgBuffer.SignShareMsgs = node.MsgBuffer.SignShareMsgs[:0]
	node.MsgBuffer.SignStateMsgs = node.MsgBuffer.SignStateMsgs[:0]
	node.MsgBuffer.SlowCommitProofMsgs = node.MsgBuffer.SlowCommitProofMsgs[:0]
	/////////////////////////////////////////////////////
	fmt.Println(node.CurrentState)
	SignStateDone = false
	return nil
}

/*--------------------------------------------------------------------*/

func (node *Node) GetFullExecuteProof(msg *consensus.FullExecuteProofMsg) error {
	//LogMsg(msg)

	fmt.Printf("Result: Executed succesfully by %s\n", msg.NodeID)
	end = time.Now()
	fmt.Println(end.Sub(start))
	println("The number of Blocks Successfully Committed %s\n", len(node.CommittedMsgs))
	fmt.Println("*#************************#*")
	// fmt.Println(node.CurrentState)
	//Trying to make nodes status nil for the next txn
	node.CurrentState = nil
	node.CurrentState = nil
	node.MsgBuffer.CommitMsgs = node.MsgBuffer.CommitMsgs[:0]
	node.MsgBuffer.FastCommitProofMsgs = node.MsgBuffer.FastCommitProofMsgs[:0]
	node.MsgBuffer.FullExecuteProofMsgs = node.MsgBuffer.FullExecuteProofMsgs[:0]
	node.MsgBuffer.PrePrepareMsgs = node.MsgBuffer.PrePrepareMsgs[:0]
	node.MsgBuffer.PrepareMsgs = node.MsgBuffer.PrepareMsgs[:0]
	node.MsgBuffer.RequestBlocks = node.MsgBuffer.RequestBlocks[:0]
	node.MsgBuffer.SignShareMsgs = node.MsgBuffer.SignShareMsgs[:0]
	node.MsgBuffer.SignStateMsgs = node.MsgBuffer.SignStateMsgs[:0]
	node.MsgBuffer.SlowCommitProofMsgs = node.MsgBuffer.SlowCommitProofMsgs[:0]
	/////////////////////////////////////////////////////
	fmt.Println(node.CurrentState)
	return nil

}

/*--------------------------------------------------------------------*/

func (node *Node) createStateForNewConsensus() error {
	// Check if there is an ongoing consensus process.

	if node.CurrentState != nil {
		return errors.New("another consensus is ongoing")
	}

	// Get the last sequence ID
	var lastSequenceID int64
	if len(node.CommittedMsgs) == 0 {
		lastSequenceID = -1
	} else {
		// println(len(node.CommittedMsgs))
		lastSequenceID = node.CommittedMsgs[len(node.CommittedMsgs)-1].SequenceID
	}

	// println("++++++1.1++++++")
	// Create a new state for this new consensus process in the Primary.
	//Todo: It must be the leader, so new state is just in leader. what about other. their state is same as prev
	node.CurrentState = consensus.CreateState(node.View.ID, lastSequenceID)

	LogStage("Create the replica status", true)

	return nil
}

/*--------------------------------------------------------------------*/

func (node *Node) dispatchMsg() {
	for {
		select {
		case msg := <-node.MsgEntrance:
			// fmt.Println("******HERE2222*****")
			err := node.routeMsg(msg)
			if err != nil {
				fmt.Println(err)
				// TODO: send err to ErrorChannel
			}
		case <-node.Alarm:
			err := node.routeMsgWhenAlarmed()
			if err != nil {
				fmt.Println(err)
				// TODO: send err to ErrorChannel
			}
		}
	}
}

/*--------------------------------------------------------------------*/

//Routing or buffering msgs accroding to the current state of node
func (node *Node) routeMsg(msg interface{}) []error {
	switch msg.(type) {
	case *consensus.RequestBlock: //Req msgs from the channel
		fmt.Println("*#************2.1. Dispatch msg*******#*")

		if node.CurrentState == nil || node.CurrentState.CurrentStage == consensus.FastCommitProved || node.CurrentState.CurrentStage == consensus.SlowCommitProved {
			// Copy buffered messages first.
			//TODO: Last two conditions in case primary can't get nil due to E-collector wait step
			if node.CurrentState != nil {
				node.CurrentState = nil
			}
			// fmt.Println("*#************3. Currstate = NULL(msg will go to Msg delivery channel) *******#*")

			msgs := make([]*consensus.RequestBlock, len(node.MsgBuffer.RequestBlocks))
			copy(msgs, node.MsgBuffer.RequestBlocks)

			// Append a newly arrived message.
			msgs = append(msgs, msg.(*consensus.RequestBlock))

			// Empty the buffer.
			node.MsgBuffer.RequestBlocks = make([]*consensus.RequestBlock, 0)

			// Send messages.
			//Todo: Sending all the messages from the buffer at once. Capacity of the channel no being considered
			node.MsgDelivery <- msgs
		} else {
			// fmt.Println("*#************3. Currstate != NULL(msg will go to req buffer) *******#*")
			//Second request is being diverted here, the Currstate has not being changed to null again
			node.MsgBuffer.RequestBlocks = append(node.MsgBuffer.RequestBlocks, msg.(*consensus.RequestBlock))
			//Todo: This buffer is not working as of now
		}
	case *consensus.PrePrepareMsg:
		if node.CurrentState == nil {
			// Copy buffered messages first.
			msgs := make([]*consensus.PrePrepareMsg, len(node.MsgBuffer.PrePrepareMsgs))
			copy(msgs, node.MsgBuffer.PrePrepareMsgs)

			// Append a newly arrived message.
			msgs = append(msgs, msg.(*consensus.PrePrepareMsg))

			// Empty the buffer.
			node.MsgBuffer.PrePrepareMsgs = make([]*consensus.PrePrepareMsg, 0)

			// Send messages.
			node.MsgDelivery <- msgs
		} else {
			node.MsgBuffer.PrePrepareMsgs = append(node.MsgBuffer.PrePrepareMsgs, msg.(*consensus.PrePrepareMsg))
		}
	case *consensus.SignShareMsg:
		if node.CurrentState != nil { // The collector will be at nil state
			//fmt.Println(node.CurrentState)
			//fmt.Println("------------A---------------")
			node.MsgBuffer.SignShareMsgs = append(node.MsgBuffer.SignShareMsgs, msg.(*consensus.SignShareMsg))
		} else {
			// fmt.Println("**State while recieving SignShare Msg **")
			//fmt.Println("------------B---------------")
			// Copy buffered messages first.
			msgs := make([]*consensus.SignShareMsg, len(node.MsgBuffer.PrepareMsgs))
			copy(msgs, node.MsgBuffer.SignShareMsgs)

			// Append a newly arrived message.
			msgs = append(msgs, msg.(*consensus.SignShareMsg))

			// Empty the buffer.
			node.MsgBuffer.SignShareMsgs = make([]*consensus.SignShareMsg, 0)

			//fmt.Println("------------Sign share till end---------------")

			// Send messages.
			node.MsgDelivery <- msgs
		}
	case *consensus.PrepareMsg:
		if node.CurrentState == nil || node.CurrentState.CurrentStage != consensus.PrePrepared { // As the status of non-collector nodes is still Preprepared
			// fmt.Println("**State while recieving Prepare Msg 1 **")
			node.MsgBuffer.PrepareMsgs = append(node.MsgBuffer.PrepareMsgs, msg.(*consensus.PrepareMsg))
		} else {
			// Copy buffered messages first.
			// fmt.Println("**State while recieving Prepare Msg 2 **")
			msgs := make([]*consensus.PrepareMsg, len(node.MsgBuffer.PrepareMsgs))
			copy(msgs, node.MsgBuffer.PrepareMsgs)

			// Append a newly arrived message.
			msgs = append(msgs, msg.(*consensus.PrepareMsg))

			// Empty the buffer.
			node.MsgBuffer.PrepareMsgs = make([]*consensus.PrepareMsg, 0)

			// Send messages.
			node.MsgDelivery <- msgs
		}
	case *consensus.CommitMsg:
		if node.CurrentState.CurrentStage != consensus.SignShared {
			node.MsgBuffer.CommitMsgs = append(node.MsgBuffer.CommitMsgs, msg.(*consensus.CommitMsg))
		} else {
			// Copy buffered messages first.
			msgs := make([]*consensus.CommitMsg, len(node.MsgBuffer.CommitMsgs))
			copy(msgs, node.MsgBuffer.CommitMsgs)

			// Append a newly arrived message.
			msgs = append(msgs, msg.(*consensus.CommitMsg))

			// Empty the buffer.
			node.MsgBuffer.CommitMsgs = make([]*consensus.CommitMsg, 0)

			// Send messages.
			node.MsgDelivery <- msgs
		}
	case *consensus.SlowCommitProofMsg:
		if node.CurrentState == nil || node.CurrentState.CurrentStage != consensus.Prepared {
			node.MsgBuffer.SlowCommitProofMsgs = append(node.MsgBuffer.SlowCommitProofMsgs, msg.(*consensus.SlowCommitProofMsg))
		} else {
			// Copy buffered messages first.
			msgs := make([]*consensus.SlowCommitProofMsg, len(node.MsgBuffer.SlowCommitProofMsgs))
			copy(msgs, node.MsgBuffer.SlowCommitProofMsgs)

			// Append a newly arrived message.
			msgs = append(msgs, msg.(*consensus.SlowCommitProofMsg))

			// Empty the buffer.
			node.MsgBuffer.SlowCommitProofMsgs = make([]*consensus.SlowCommitProofMsg, 0)

			// Send messages.
			node.MsgDelivery <- msgs
		}
	case *consensus.FastCommitProofMsg:
		if node.CurrentState == nil || node.CurrentState.CurrentStage != consensus.PrePrepared { // As the status of non-collector nodes is still Preprepared
			fmt.Println(node.CurrentState)
			// fmt.Println("Stuck Here1")
			node.MsgBuffer.FastCommitProofMsgs = append(node.MsgBuffer.FastCommitProofMsgs, msg.(*consensus.FastCommitProofMsg))
		} else {
			// Copy buffered messages first.
			msgs := make([]*consensus.FastCommitProofMsg, len(node.MsgBuffer.FastCommitProofMsgs))
			copy(msgs, node.MsgBuffer.FastCommitProofMsgs)

			// Append a newly arrived message.
			msgs = append(msgs, msg.(*consensus.FastCommitProofMsg))

			// Empty the buffer.
			node.MsgBuffer.FastCommitProofMsgs = make([]*consensus.FastCommitProofMsg, 0)

			// Send messages.
			node.MsgDelivery <- msgs
		}
	case *consensus.SignStateMsg:
		if node.CurrentState != nil {
			// fmt.Println("****HERE3311****")
			node.MsgBuffer.SignStateMsgs = append(node.MsgBuffer.SignStateMsgs, msg.(*consensus.SignStateMsg))
		} else {
			// fmt.Println("****HERE3322****")
			// Copy buffered messages first.
			msgs := make([]*consensus.SignStateMsg, len(node.MsgBuffer.SignStateMsgs))
			copy(msgs, node.MsgBuffer.SignStateMsgs)

			// Append a newly arrived message.
			msgs = append(msgs, msg.(*consensus.SignStateMsg))

			// Empty the buffer.
			node.MsgBuffer.SignStateMsgs = make([]*consensus.SignStateMsg, 0)

			// Send messages.
			node.MsgDelivery <- msgs
		}

	case *consensus.FullExecuteProofMsg:
		if node.CurrentState == nil || !(node.CurrentState.CurrentStage == consensus.FastCommitProved || node.CurrentState.CurrentStage == consensus.SlowCommitProved) {
			//fmt.Println("****Here3.1*******")
			node.MsgBuffer.FullExecuteProofMsgs = append(node.MsgBuffer.FullExecuteProofMsgs, msg.(*consensus.FullExecuteProofMsg))
		} else {
			// Copy buffered messages first.
			//fmt.Println("****Here3.2*******")

			msgs := make([]*consensus.FullExecuteProofMsg, len(node.MsgBuffer.FullExecuteProofMsgs))
			copy(msgs, node.MsgBuffer.FullExecuteProofMsgs)

			// Append a newly arrived message.
			msgs = append(msgs, msg.(*consensus.FullExecuteProofMsg))

			// Empty the buffer.
			node.MsgBuffer.FullExecuteProofMsgs = make([]*consensus.FullExecuteProofMsg, 0)

			// Send messages.
			node.MsgDelivery <- msgs
		}

	case *consensus.Keys:
		msgs := make([]*consensus.Keys, len(node.MsgBuffer.KeyMsgs))
		copy(msgs, node.MsgBuffer.KeyMsgs)

		// Append a newly arrived message.
		msgs = append(msgs, msg.(*consensus.Keys))

		// Empty the buffer.
		node.MsgBuffer.KeyMsgs = make([]*consensus.Keys, 0)

		// Send messages.
		node.MsgDelivery <- msgs
		fmt.Println("read from msg channel")
		// node.MsgDelivery <- msg

	case *consensus.ViewChangeInitMsg:
		msgs := make([]*consensus.ViewChangeInitMsg, len(node.MsgBuffer.ViewChangeInitMsgs))
		copy(msgs, node.MsgBuffer.ViewChangeInitMsgs)

		// Append a newly arrived message.
		msgs = append(msgs, msg.(*consensus.ViewChangeInitMsg))

		// Empty the buffer.
		node.MsgBuffer.ViewChangeInitMsgs = make([]*consensus.ViewChangeInitMsg, 0)

		// Send messages.
		node.MsgDelivery <- msgs
		fmt.Println("read  Init msg from msg channel")

	case *consensus.ViewChangeExecuteMsg:
		msgs := make([]*consensus.ViewChangeExecuteMsg, len(node.MsgBuffer.ViewChangeExecuteMsgs))
		copy(msgs, node.MsgBuffer.ViewChangeExecuteMsgs)

		// Append a newly arrived message.
		msgs = append(msgs, msg.(*consensus.ViewChangeExecuteMsg))

		// Empty the buffer.
		node.MsgBuffer.ViewChangeExecuteMsgs = make([]*consensus.ViewChangeExecuteMsg, 0)

		// Send messages.
		node.MsgDelivery <- msgs
		fmt.Println("read Execute VC msgs from msg channel")
	}

	//Doubt : The msgs are being put on the stream on the basis of the current state of the node.
	//How will it be able to perform operations concurrently?

	return nil
}

/*--------------------------------------------------------------------*/

//Basically clearing the buffered msg(In case no new msg triggered)
//Todo: the code is not handling the buffer requests
func (node *Node) routeMsgWhenAlarmed() []error {
	//fmt.Println("****Buffer request %******")
	// if node.CurrentState != nil {
	// 	fmt.Println(node.CurrentState)
	// }

	if node.CurrentState == nil {
		// Check RequestBlocks, send them.
		//req msgs from buffer
		if len(node.MsgBuffer.RequestBlocks) != 0 {
			msgs := make([]*consensus.RequestBlock, len(node.MsgBuffer.RequestBlocks))
			copy(msgs, node.MsgBuffer.RequestBlocks)

			node.MsgDelivery <- msgs
		}

		// Check PrePrepareMsgs, send them.
		if len(node.MsgBuffer.PrePrepareMsgs) != 0 {
			msgs := make([]*consensus.PrePrepareMsg, len(node.MsgBuffer.PrePrepareMsgs))
			copy(msgs, node.MsgBuffer.PrePrepareMsgs)

			node.MsgDelivery <- msgs
		}

		if len(node.MsgBuffer.KeyMsgs) != 0 {
			msgs := make([]*consensus.Keys, len(node.MsgBuffer.KeyMsgs))
			copy(msgs, node.MsgBuffer.KeyMsgs)

			node.MsgDelivery <- msgs
		}
	} else {
		switch node.CurrentState.CurrentStage {

		case consensus.PrePrepared:
			// Check PrepareMsgs, send them.
			if len(node.MsgBuffer.SignShareMsgs) != 0 {
				msgs := make([]*consensus.SignShareMsg, len(node.MsgBuffer.SignShareMsgs))
				copy(msgs, node.MsgBuffer.SignShareMsgs)

				node.MsgDelivery <- msgs
			}
		case consensus.SignShared:
			// Check PrepareMsgs, send them.
			if len(node.MsgBuffer.FullExecuteProofMsgs) != 0 {
				msgs := make([]*consensus.FullExecuteProofMsg, len(node.MsgBuffer.FullExecuteProofMsgs))
				copy(msgs, node.MsgBuffer.FullExecuteProofMsgs)

				node.MsgDelivery <- msgs
			}
			if len(node.MsgBuffer.SlowCommitProofMsgs) != 0 {
				msgs := make([]*consensus.SlowCommitProofMsg, len(node.MsgBuffer.SlowCommitProofMsgs))
				copy(msgs, node.MsgBuffer.SlowCommitProofMsgs)

				node.MsgDelivery <- msgs
			}
		case consensus.FastCommitProved:
			// Check PrepareMsgs, send them.
			if len(node.MsgBuffer.SignStateMsgs) != 0 {
				msgs := make([]*consensus.SignStateMsg, len(node.MsgBuffer.SignStateMsgs))
				copy(msgs, node.MsgBuffer.SignStateMsgs)

				node.MsgDelivery <- msgs
			}
		case consensus.Committed:
			// Check PrepareMsgs, send them.
			if len(node.MsgBuffer.CommitMsgs) != 0 {
				msgs := make([]*consensus.CommitMsg, len(node.MsgBuffer.CommitMsgs))
				copy(msgs, node.MsgBuffer.CommitMsgs)

				node.MsgDelivery <- msgs
			}
		case consensus.SlowCommitProved:
			// Check PrepareMsgs, send them.
			if len(node.MsgBuffer.SlowCommitProofMsgs) != 0 {
				msgs := make([]*consensus.SlowCommitProofMsg, len(node.MsgBuffer.SlowCommitProofMsgs))
				copy(msgs, node.MsgBuffer.SlowCommitProofMsgs)

				node.MsgDelivery <- msgs
			}
		case consensus.CommitTriggered:
			// Check PrepareMsgs, send them.
			if len(node.MsgBuffer.FullExecuteProofMsgs) != 0 {
				msgs := make([]*consensus.FullExecuteProofMsg, len(node.MsgBuffer.FullExecuteProofMsgs))
				copy(msgs, node.MsgBuffer.FullExecuteProofMsgs)
				node.MsgDelivery <- msgs
			}

		case consensus.ViewChangeInitiated:
			// Check PrepareMsgs, send them.
			if node.CurrentState != nil && len(node.MsgBuffer.ViewChangeInitMsgs) != 0 {
				msgs := make([]*consensus.ViewChangeInitMsg, len(node.MsgBuffer.ViewChangeInitMsgs))
				copy(msgs, node.MsgBuffer.ViewChangeInitMsgs)

				fmt.Println("read Init VC msgs at alarm buffer")
				node.MsgDelivery <- msgs
			}

		case consensus.ViewChangeExecuted:
			// Check PrepareMsgs, send them.
			if node.CurrentState != nil && len(node.MsgBuffer.ViewChangeExecuteMsgs) != 0 {
				msgs := make([]*consensus.ViewChangeExecuteMsg, len(node.MsgBuffer.ViewChangeExecuteMsgs))
				copy(msgs, node.MsgBuffer.ViewChangeExecuteMsgs)

				fmt.Println("read Execute VC msgs at alarm buffer")
				node.MsgDelivery <- msgs
			}

		}
	}

	return nil
}

func (node *Node) resolveMsg() {
	for {
		//doubt: Get buffered messages from the dispatcher.
		//Taking msgs from the next channel(MsgDelivery)
		msgs := <-node.MsgDelivery
		switch msgs.(type) {
		case []*consensus.RequestBlock:
			//Request msgs from the dispatche
			errs := node.resolveRequestBlock(msgs.([]*consensus.RequestBlock))
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}
		case []*consensus.PrePrepareMsg:
			errs := node.resolvePrePrepareMsg(msgs.([]*consensus.PrePrepareMsg))
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}

		case []*consensus.SignShareMsg:
			errs := node.resolveSignShareMsg(msgs.([]*consensus.SignShareMsg))
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}

		case []*consensus.FastCommitProofMsg:
			errs := node.resolveFastCommitProofMsg(msgs.([]*consensus.FastCommitProofMsg))
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}

		case []*consensus.SlowCommitProofMsg:
			errs := node.resolveSlowCommitProofMsg(msgs.([]*consensus.SlowCommitProofMsg))
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}

		case []*consensus.PrepareMsg:
			errs := node.resolvePrepareMsg(msgs.([]*consensus.PrepareMsg))
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}

		case []*consensus.CommitMsg:
			errs := node.resolveCommitMsg(msgs.([]*consensus.CommitMsg))
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}

		case []*consensus.SignStateMsg:
			errs := node.resolveSignStateMsg(msgs.([]*consensus.SignStateMsg))
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}

		case []*consensus.FullExecuteProofMsg:
			errs := node.resolveFullExecuteProofMsg(msgs.([]*consensus.FullExecuteProofMsg))
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}

		case []*consensus.Keys:
			fmt.Println("At resolve")
			errs := node.resolveKeys(msgs.([]*consensus.Keys))
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println("Msgchannel")
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}

		case []*consensus.ViewChangeInitMsg:
			fmt.Println("At resolve")
			errs := node.resolveViewChangeInitMsg(msgs.([]*consensus.ViewChangeInitMsg))
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println("Msgchannel")
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}

		case []*consensus.ViewChangeExecuteMsg:
			fmt.Println("At resolve")
			errs := node.resolveViewChangeExecuteMsg(msgs.([]*consensus.ViewChangeExecuteMsg))
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println("Msgchannel")
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}

		}

	}
}

/*--------------------------------------------------------------------*/

func (node *Node) alarmToDispatcher() {
	for {
		time.Sleep(ResolvingTimeDuration)
		node.Alarm <- true
	}
}

//
//
//
// const jt = time.Millisecond * 10000

// func (node *Node) jugad() {
// 	for {
// 		time.Sleep(jt)
// 		node.CurrentState = nil
// 	}
// }

//
//
//

/*--------------------------------------------------------------------*/

func (node *Node) resolvePrePrepareMsg(msgs []*consensus.PrePrepareMsg) []error {
	errs := make([]error, 0)

	// Resolve messages
	for _, PrePrepareMsg := range msgs {
		err := node.GetPrePrepare(PrePrepareMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

/*--------------------------------------------------------------------*/

func (node *Node) resolveRequestBlock(msgs []*consensus.RequestBlock) []error {
	errs := make([]error, 0)

	// These are the request msg entry point from the dispatcher
	for _, reqMsg := range msgs {
		err := node.GetReq(reqMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

/*--------------------------------------------------------------------*/

func (node *Node) resolveSignShareMsg(msgs []*consensus.SignShareMsg) []error {
	errs := make([]error, 0)

	// Resolve messages
	for _, SignShareMsg := range msgs {
		err := node.GetSignShare(SignShareMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

/*--------------------------------------------------------------------*/

func (node *Node) resolveFastCommitProofMsg(msgs []*consensus.FastCommitProofMsg) []error {
	errs := make([]error, 0)

	// Resolve messages
	for _, FastCommitProofMsg := range msgs {
		err := node.GetFastCommitProof(FastCommitProofMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

func (node *Node) resolveSlowCommitProofMsg(msgs []*consensus.SlowCommitProofMsg) []error {
	errs := make([]error, 0)

	// Resolve messages
	for _, SlowCommitProofMsg := range msgs {
		err := node.GetSlowCommitProof(SlowCommitProofMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

/*--------------------------------------------------------------------*/

func (node *Node) resolvePrepareMsg(msgs []*consensus.PrepareMsg) []error {
	errs := make([]error, 0)

	// Resolve messages
	for _, PrepareMsg := range msgs {
		err := node.GetPrepare(PrepareMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

/*--------------------------------------------------------------------*/

func (node *Node) resolveCommitMsg(msgs []*consensus.CommitMsg) []error {
	errs := make([]error, 0)

	// Resolve messages
	for _, CommitMsg := range msgs {
		err := node.GetCommit(CommitMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

/*--------------------------------------------------------------------*/

func (node *Node) resolveSignStateMsg(msgs []*consensus.SignStateMsg) []error {
	errs := make([]error, 0)

	// Resolve messages
	for _, SignStateMsg := range msgs {
		err := node.GetSignState(SignStateMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

/*--------------------------------------------------------------------*/

func (node *Node) resolveFullExecuteProofMsg(msgs []*consensus.FullExecuteProofMsg) []error {
	errs := make([]error, 0)

	// Resolve messages
	for _, FullExecuteProofMsg := range msgs {
		err := node.GetFullExecuteProof(FullExecuteProofMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

/*--------------------------------------------------------------------*/

func (node *Node) resolveKeys(msgs []*consensus.Keys) []error {
	errs := make([]error, 0)

	// Resolve messages
	for _, Keys := range msgs {
		err := node.GetKeys(Keys)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

/*--------------------------------------------------------------------*/

func (node *Node) resolveViewChangeInitMsg(msgs []*consensus.ViewChangeInitMsg) []error {
	errs := make([]error, 0)

	// Resolve messages
	for _, ViewChangeInitMsg := range msgs {
		err := node.GetViewChangeInitMsg(ViewChangeInitMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

/*--------------------------------------------------------------------*/

func (node *Node) resolveViewChangeExecuteMsg(msgs []*consensus.ViewChangeExecuteMsg) []error {
	errs := make([]error, 0)

	// Resolve messages
	for _, ViewChangeExecuteMsg := range msgs {
		err := node.GetViewChangeExecuteMsg(ViewChangeExecuteMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

/*--------------------------------------------------------------------*/

func makesignpacket(reqhash string, sign string, nodeID string, t string) *consensus.SignPacket {
	return &consensus.SignPacket{
		Request: reqhash,
		Key:     sign,
		NodeID:  nodeID,
		Type:    t,
	}
}

/*--------------------------------------------------------------------*/

func AccumulateFP(msg []*consensus.MessageTSCC, reqhash string, NodeID string, groupkey string) *consensus.AccumulatePacket {

	shares := make([]string, len(msg))
	memID := make([]string, len(msg))
	for i := 0; i < len(msg); i++ {
		shares[i] = msg[i].WaitMsg.TS_FP
	}
	for i := 0; i < len(msg); i++ {
		memID[i] = msg[i].WaitMsg.NodeID
	}
	return &consensus.AccumulatePacket{
		Request:   reqhash,
		KeyShares: shares,
		NodeID:    NodeID,
		GroupKey:  groupkey,
		MemberID:  memID,
		Path:      "FP",
	}
}

/*--------------------------------------------------------------------*/

func AccumulateSP(msg []*consensus.MessageTSCC, reqhash string, NodeID string, groupkey string) *consensus.AccumulatePacket {

	shares := make([]string, len(msg))
	memID := make([]string, len(msg))
	for i := 0; i < len(msg); i++ {
		shares[i] = msg[i].WaitMsg.TS_SP
	}
	for i := 0; i < len(msg); i++ {
		memID[i] = msg[i].WaitMsg.NodeID
	}
	return &consensus.AccumulatePacket{
		Request:   reqhash,
		KeyShares: shares,
		NodeID:    NodeID,
		GroupKey:  groupkey,
		MemberID:  memID,
		Path:      "SP",
	}
}

/*--------------------------------------------------------------------*/

func AccumulatePi(msg []*consensus.MessageTSEC, reqhash string, NodeID string, groupkey string) *consensus.AccumulatePacket {

	shares := make([]string, len(msg))
	memID := make([]string, len(msg))
	for i := 0; i < len(msg); i++ {
		shares[i] = msg[i].WaitMsg.SS_TS
	}
	for i := 0; i < len(msg); i++ {
		memID[i] = msg[i].WaitMsg.NodeID
	}
	return &consensus.AccumulatePacket{
		Request:   reqhash,
		KeyShares: shares,
		NodeID:    NodeID,
		GroupKey:  groupkey,
		MemberID:  memID,
		Path:      "Pi",
	}
}

/*--------------------------------------------------------------------*/

func AccumulateTouTou(msg []*consensus.MessageTSCP, reqhash string, NodeID string, groupkey string) *consensus.AccumulatePacket {

	shares := make([]string, len(msg))
	memID := make([]string, len(msg))
	for i := 0; i < len(msg); i++ {
		shares[i] = msg[i].WaitMsg.C_TS_SP
	}
	for i := 0; i < len(msg); i++ {
		memID[i] = msg[i].WaitMsg.NodeID
	}
	return &consensus.AccumulatePacket{
		Request:   reqhash,
		KeyShares: shares,
		NodeID:    NodeID,
		GroupKey:  groupkey,
		MemberID:  memID,
		Path:      "SP",
	}
}

func makenodetable(n int) map[string]string {
	m := make(map[string]string)
	m["KS"] = "localhost:1109"
	for i := 0; i < n; i++ {
		m[strconv.Itoa(i)] = "localhost:" + strconv.Itoa(1110+i)
		fmt.Println(strconv.Itoa(i))
		fmt.Println("localhost:" + strconv.Itoa(1110+i))
	}
	return m
}

func makerevnodetable(n int) map[string]string {
	m := make(map[string]string)
	m["localhost:1109"] = "KS"
	for i := 0; i < n; i++ {
		m["localhost:"+strconv.Itoa(1110+i)] = strconv.Itoa(i)
		// fmt.Println(strconv.Itoa(i))
		// fmt.Println("localhost:" + strconv.Itoa(1110+i))
	}
	return m
}
