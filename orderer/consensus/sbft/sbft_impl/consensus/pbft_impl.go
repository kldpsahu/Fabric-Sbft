package consensus

import (
	"encoding/json"
	"errors"
	"time"
)

const n = 5

type State struct {
	ViewID         int
	MsgLogs        *MsgLogs
	LastSequenceID int64
	CurrentStage   Stage
}

type MsgLogs struct {
	Blocks               *RequestBlock
	PrePrepareMsgs       map[string]*PrePrepareMsg
	SignShareMsgs        map[string]*SignShareMsg
	PrepareMsgs          map[string]*PrepareMsg
	CommitMsgs           map[string]*CommitMsg
	FastCommitProofMsgs  map[string]*FastCommitProofMsg
	SlowCommitProofMsgs  map[string]*SlowCommitProofMsg
	SignStateMsgs        map[string]*SignStateMsg
	FullExecuteProofMsgs map[string]*FullExecuteProofMsg
}

type Stage int

const (
	Idle        Stage = iota // Node is created successfully, but the consensus process is not started yet.
	PrePrepared              // The RequestBlocks is processed successfully. The node is ready to head to the Prepare stage.
	ViewChangeInitiated
	ViewChangeExecuted
	SignShared
	Prepared
	Committed
	FastCommitProved
	SlowCommitProved
	CommitTriggered
	ExecuteTriggered // Same with `prepared` stage explained in the original paper.
	ExecutedProved   // Same with `committed-local` stage explained in the original paper.
)

// f: # of Byzantine faulty node
// f = (n­1) / 3
// n = 4, in this case.
const f = 1

// lastSequenceID will be -1 if there is no last sequence ID.
func CreateState(viewID int, lastSequenceID int64) *State {
	return &State{
		ViewID: viewID,
		MsgLogs: &MsgLogs{
			Blocks:               nil, //why no preprepare msgs
			PrepareMsgs:          make(map[string]*PrepareMsg),
			CommitMsgs:           make(map[string]*CommitMsg),
			FastCommitProofMsgs:  make(map[string]*FastCommitProofMsg),
			SignShareMsgs:        make(map[string]*SignShareMsg),
			SlowCommitProofMsgs:  make(map[string]*SlowCommitProofMsg),
			SignStateMsgs:        make(map[string]*SignStateMsg),
			FullExecuteProofMsgs: make(map[string]*FullExecuteProofMsg),
		},
		LastSequenceID: lastSequenceID,
		CurrentStage:   Idle,
	}
}

func (state *State) StartConsensus(request *RequestBlock) (*PrePrepareMsg, error) {
	// `sequenceID` will be the index of this message.
	sequenceID := time.Now().UnixNano()

	// Find the unique and largest number for the sequence ID
	if state.LastSequenceID != -1 {
		for state.LastSequenceID >= sequenceID {
			sequenceID += 1
		}
	}

	// Assign a new sequence ID to the request message object.
	request.SequenceID = sequenceID

	// Save RequestBlocks to its logs.
	state.MsgLogs.Blocks = request

	// Get the digest of the request message
	// digest, err := digest(request)
	// if err != nil {
	// 	fmt.Println(err)
	// 	return nil, err
	// }

	// Change the stage to pre-prepared.
	state.CurrentStage = PrePrepared

	return &PrePrepareMsg{
		ViewID:     state.ViewID,
		SequenceID: sequenceID,
		Block:      request,
	}, nil
}

/*--------------------------------------------------------------*/

func (state *State) PrePrepare(prePrepareMsg *PrePrepareMsg, Sigma_sign string, Tou_sign string) (*SignShareMsg, error) {
	// Get RequestBlocks and save it to its logs like the primary.
	state.MsgLogs.Blocks = prePrepareMsg.Block

	// Verify if view and sequence ID are correct.
	//todo: Introducing signatures to rectify
	if !state.verifyMsg(prePrepareMsg.ViewID, prePrepareMsg.SequenceID, prePrepareMsg.Block.Requests) {
		return nil, errors.New("pre-prepare message is corrupted")
	}

	// Change the stage to pre-prepared.

	state.CurrentStage = PrePrepared

	return &SignShareMsg{
		ViewID:     state.ViewID,
		SequenceID: prePrepareMsg.SequenceID,
		Block:      prePrepareMsg.Block,
		TS_FP:      Sigma_sign, //To be calculated
		TS_SP:      Tou_sign,
	}, nil
}

/*--------------------------------------------------------------*/

func (state *State) SignShareFP(MsgTSCC []*MessageTSCC, Tou_FullSign string) (*FastCommitProofMsg, error) {
	//Verify view, seqID and signature of each signshare msg from the array
	// if !state.verifySignShare(SignShareMsg.ViewID, SignShareMsg.SequenceID, SignShareMsg.TS_FP, SignShareMsg.TS_SP) {
	// 	return nil, nil, errors.New("prepare message is corrupted")
	// }
	//Create a  σ(h), from all  σi(h) signs
	// t := 3 * f
	// memberSigns := make([]*pbc.Element, len(MsgTSCC))
	// for i := 0; i < len(MsgTSCC); i++ {
	// 	memberSigns[i] = MsgTSCC[i].WaitMsg.TS_FP
	// }

	// memberIds := mr.Perm(n)[:t]

	// shares := make([]*pbc.Element, t)
	// for i := 0; i < t; i++ {
	// 	shares[i] = memberSigns[memberIds[i]]
	// }

	// Recover the threshold signature.
	// signature := threshold.Threshold_Accumulator(shares, memberIds, keys.Pairing_sigma, keys.Parameters_sigma)

	// //hash of request
	// s := Hash([]byte(fmt.Sprintf("%v", MsgTSCC[0].WaitMsg.Block)))
	// Verify the threshold signature.
	// IsVerified := threshold.Verify(keys.Pairing_sigma, signature, []byte(s), keys.Generator_sigma, keys.GroupKey_Sigma)
	// if !IsVerified {
	// 	fmt.Printf("Verification Failed !!!!! \n")
	// 	return nil, nil
	// }

	// fmt.Printf("Verification Done !!!!! \n")
	// Append msg to its logs
	// state.MsgLogs.SignShareMsgs[MsgTSCC[0].WaitMsg.NodeID] = &MsgTSCC[0].WaitMsg

	//Make combined Signature

	// Change the stage to prepared.
	state.CurrentStage = SignShared

	return &FastCommitProofMsg{
		ViewID:     MsgTSCC[0].WaitMsg.ViewID,
		SequenceID: MsgTSCC[0].WaitMsg.SequenceID,
		FC_TS_FP:   "Dummy",
	}, nil

}

// func (state *State) MakeFC_TS_FP(viewID int64, sequenceID int64, digestGot string) string {

// 	return "dummysig_TS_SP"
// }

// func (state *State) MakePrepare_TS_SP(viewID int64, sequenceID int64, digestGot string) string {

// 	return "dummysig_Prepare_TS_SP"
// }

// func (state *State) verifySignShare(viewID int64, sequenceID int64, TS_FP string, TS_SP string) bool {
// 	// Wrong view. That is, wrong configurations of peers to start the consensus.
// 	if state.ViewID != viewID {
// 		return false
// 	}

// 	// Check if the Primary sent fault sequence number. => Faulty primary.
// 	// TODO: adopt upper/lower bound check.
// 	if state.LastSequenceID != -1 {
// 		if state.LastSequenceID >= sequenceID {
// 			return false
// 		}
// 	}
// 	// Check Signature.

// 	return true
// }

/*--------------------------------------------------------------*/

func (state *State) SignShareSP(MsgTSCC []*MessageTSCC, Tou_Fullsign string) (*PrepareMsg, error) {

	//Verify view, seqID and signature of each signshare msg from the array
	// if !state.verifySignShare(SignShareMsg.ViewID, SignShareMsg.SequenceID, SignShareMsg.TS_FP, SignShareMsg.TS_SP) {
	// 	return nil, nil, errors.New("prepare message is corrupted")
	// }
	//Create a  τ(h), from all  τi(h) signs

	// Append msg to its logs
	// state.MsgLogs.SignShareMsgs[MsgTSCC[0].WaitMsg.NodeID] = &MsgTSCC[0].WaitMsg

	//Make combined Signature

	// Change the stage to prepared.
	state.CurrentStage = SignShared

	return &PrepareMsg{
		ViewID:        state.ViewID,
		SequenceID:    MsgTSCC[0].WaitMsg.SequenceID,
		Prepare_TS_SP: Tou_Fullsign,
	}, nil

}

// func (state *State) MakeFC_TS_FP(viewID int64, sequenceID int64, digestGot string) string {

// 	return "dummysig_TS_SP"
// }

// func (state *State) MakePrepare_TS_SP(viewID int64, sequenceID int64, digestGot string) string {

// 	return "dummysig_Prepare_TS_SP"
// }

// func (state *State) verifySignShare(viewID int64, sequenceID int64, TS_FP string, TS_SP string) bool {
// 	// Wrong view. That is, wrong configurations of peers to start the consensus.
// 	if state.ViewID != viewID {
// 		return false
// 	}

// 	// Check if the Primary sent fault sequence number. => Faulty primary.
// 	// TODO: adopt upper/lower bound check.
// 	if state.LastSequenceID != -1 {
// 		if state.LastSequenceID >= sequenceID {
// 			return false
// 		}
// 	}
// 	// Check Signature.

// 	return true
// }

/*--------------------------------------------------------------*/

//Assuming single prepare msg from c-collector to each node
func (state *State) Prepare(prepareMsg *PrepareMsg, TouTou string) (*CommitMsg, error) {
	//Verify State, Sequence ID and sig τ(h)
	// if !state.verifyMsg(prepareMsg.ViewID, prepareMsg.SequenceID, ) {
	// 	return nil, errors.New("prepare message is corrupted")
	// }

	// Append msg to its logs
	state.MsgLogs.PrepareMsgs[prepareMsg.NodeID] = prepareMsg

	state.CurrentStage = Prepared

	return &CommitMsg{
		ViewID:        state.ViewID,
		SequenceID:    prepareMsg.SequenceID,
		C_TS_SP:       TouTou,
		Prepare_TS_SP: prepareMsg.Prepare_TS_SP,
	}, nil

}

func (state *State) verifyPrepareSig(viewID int, sequenceID int64, Prepare_TS_SP string) bool {
	// Wrong view. That is, wrong configurations of peers to start the consensus.
	if state.ViewID != viewID {
		return false
	}

	// Check if the Primary sent fault sequence number. => Faulty primary.
	// TODO: adopt upper/lower bound check.
	if state.LastSequenceID != -1 {
		if state.LastSequenceID >= sequenceID {
			return false
		}
	}
	// Check Signature.

	return true
}

func (state *State) MakeC_TS_SP(viewID int64, sequenceID int64, digestGot string) string {

	return "dummysig_C_TS_SP"
}

/*---------------------------------------------------------------------------------*/

func (state *State) Commit(MsgTSCP []*MessageTSCP, toutou string) (*SignStateMsg, error) {

	//state.MsgLogs.RequestBlock = SlowCommitProofMsg.RequestBlocks

	// Verify view, sequenceID and each τi(τ(h))
	//todo: Introducing signatures to rectify
	// if !state.verifyMsg(SlowCommitProofMsg.ViewID, SlowCommitProofMsg.SequenceID, SlowCommitProofMsg.SC_TS_SP) {
	// 	return nil, errors.New("SlowCommit message is corrupted") //will take digest instead of Sign
	// }

	// Change the stage to pre-prepared.
	state.CurrentStage = CommitTriggered /////TODO

	//Create Signature τ(τ(h))

	//var SS_TS string = state.MakeSS_TS(SlowCommitProofMsg.ViewID, SlowCommitProofMsg.SequenceID, SlowCommitProofMsg.Requests)

	return &SignStateMsg{
		SequenceID: MsgTSCP[0].WaitMsg.SequenceID,
		SS_TS:      toutou,
	}, nil
}

// func (state *State) MakeSS_TS(viewID int64, sequenceID int64, digestGot string) string {

// 	return "dummysigSS_TS"
// }

// func (state *State) verifyPrepareSig(viewID int64, sequenceID int64, Prepare_TS_SP string) bool {
// 	// Wrong view. That is, wrong configurations of peers to start the consensus.
// 	if state.ViewID != viewID {
// 		return false
// 	}

// 	// Check if the Primary sent fault sequence number. => Faulty primary.
// 	// TODO: adopt upper/lower bound check.
// 	if state.LastSequenceID != -1 {
// 		if state.LastSequenceID >= sequenceID {
// 			return false
// 		}
// 	}
// 	// Check Signature.

// 	return true
// }

func (state *State) MakeSC_TS_SP(viewID int64, sequenceID int64, digestGot string) string {

	return "dummysig_SC_TS_SP"
}

/*---------------------------------------------------------------------------------*/

func (state *State) SignStateFC(FastCommitProofMsg *FastCommitProofMsg, Pi string) (*SignStateMsg, *RequestBlock, error) {

	state.MsgLogs.FastCommitProofMsgs[FastCommitProofMsg.NodeID] = FastCommitProofMsg

	// Verify v, s and sig
	//todo: Introducing signatures to rectify
	// if !state.verifyMsg(FastCommitProofMsg.ViewID, FastCommitProofMsg.SequenceID, FastCommitProofMsg.FC_TS_FP) {
	// 	return nil, errors.New("Fast Commit message is corrupted") //will take digest instead of Sign
	// }

	// Change the stage to pre-prepared.
	// fmt.Println("*&*&&*^%()^(&&)*(&%*^&%&&(*)*&")
	state.CurrentStage = FastCommitProved /////TODO

	//Create sig  πi(d)

	//var SS_TS string = state.MakeSS_TS(FastCommitProofMsg.ViewID, FastCommitProofMsg.SequenceID, FastCommitProofMsg.Block.Requests)

	return &SignStateMsg{
		Block:      FastCommitProofMsg.Block,
		SequenceID: FastCommitProofMsg.SequenceID,
		SS_TS:      Pi,
	}, state.MsgLogs.Blocks, nil
}

// func (state *State) MakeSS_TS(viewID int64, sequenceID int64, digestGot string) string {

// 	return "dummysigSS_TS"
// }

/*---------------------------------------------------------------------------------*/

func (state *State) SignStateSC(SlowCommitProofMsg *SlowCommitProofMsg, Pi string) (*SignStateMsg, *RequestBlock, error) {

	state.MsgLogs.SlowCommitProofMsgs[SlowCommitProofMsg.NodeID] = SlowCommitProofMsg

	// Verify if v, s and sig
	//todo: Introducing signatures to rectify
	// if !state.verifyMsg(SlowCommitProofMsg.ViewID, SlowCommitProofMsg.SequenceID, SlowCommitProofMsg.SC_TS_SP) {
	// 	return nil, errors.New("SlowCommit message is corrupted") //will take digest instead of Sign
	// }

	state.CurrentStage = SlowCommitProved /////TODO

	// Create  πi(d)
	//var SS_TS string = state.MakeSS_TS(SlowCommitProofMsg.ViewID, SlowCommitProofMsg.SequenceID, SlowCommitProofMsg.Block.Requests)

	return &SignStateMsg{
		Block:      SlowCommitProofMsg.Block,
		SequenceID: SlowCommitProofMsg.SequenceID,
		SS_TS:      Pi,
	}, state.MsgLogs.Blocks, nil
}

// func (state *State) MakeSS_TS(viewID int64, sequenceID int64, requests []*RequestMsg) string {

// 	return "dummysigSS_TS"
// }

/*---------------------------------------------------------------------------------*/

func (state *State) FullExecuteProof(MsgTSEC []*MessageTSEC, Pi string) (*FullExecuteProofMsg, error) {
	// if !state.Verifyfullexp(SignStateMsg.SequenceID, SignStateMsg.SS_TS) {
	// 	return false, errors.New("prepare message is corrupted")
	// }

	// Append msg to its logs
	//state.MsgLogs.SignStateMsg[SignStateMsg.NodeID] = SignStateMsg

	//Create  π(d)

	state.CurrentStage = ExecuteTriggered

	return &FullExecuteProofMsg{
		SequenceID: MsgTSEC[0].WaitMsg.SequenceID,
		FS_TS:      Pi,
		Block:      MsgTSEC[0].WaitMsg.Block,
	}, nil

}

/*---------------------------------------------------------------------------------*/

func (state *State) Verifyfullexp(sequenceID int64, sign string) bool {

	// Check if the Primary sent fault sequence number. => Faulty primary.
	// TODO: adopt upper/lower bound check.
	if state.LastSequenceID != -1 {
		if state.LastSequenceID >= sequenceID {
			return false
		}
	}
	// Check Signature.

	return true
}

/*----------------------------------------------------------------------------*/

func (state *State) verifyMsg(viewID int, sequenceID int64, requests []*RequestMsg) bool {
	// Wrong view. That is, wrong configurations of peers to start the consensus.
	// if state.ViewID != viewID {
	// 	return false
	// }

	// Check if the Primary sent fault sequence number. => Faulty primary.
	// TODO: adopt upper/lower bound check.
	// if state.LastSequenceID != -1 {
	// 	if state.LastSequenceID >= sequenceID {
	// 		return false
	// 	}
	// }

	// digest, err := digest(state.MsgLogs.RequestBlock)
	// if err != nil {
	// 	fmt.Println(err)
	// 	return false
	// }

	// // Check digest.
	// if digestGot != digest {
	// 	return false
	// }

	return true
}

//todo: Improve this checking mechanism

// func (state *State) prepared() bool {
// 	if state.MsgLogs.RequestBlock == nil {
// 		return false
// 	}

// 	if len(state.MsgLogs.PrepareMsgs) < 2*f {
// 		return false
// 	}

// 	return true
// }

func digest(object interface{}) (string, error) {
	msg, err := json.Marshal(object)

	if err != nil {
		return "", err
	}

	return Hash(msg), nil
}
