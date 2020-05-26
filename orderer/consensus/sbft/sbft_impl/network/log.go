package network

import (
	"fmt"

	"../consensus"
)

func LogMsg(msg interface{}) {
	switch msg.(type) {
	case *consensus.RequestMsg:
		reqMsg := msg.(*consensus.RequestMsg)
		fmt.Printf("[REQUEST] ClientID: %s, Timestamp: %d, Operation: %s\n", reqMsg.ClientID, reqMsg.Timestamp, reqMsg.Operation)
	case *consensus.PrePrepareMsg:
		prePrepareMsg := msg.(*consensus.PrePrepareMsg)
		fmt.Printf("[PREPREPARE] SequenceID: %d\n", prePrepareMsg.SequenceID)

	case *consensus.SignShareMsg:
		SignShareMsg := msg.(*consensus.SignShareMsg)
		fmt.Printf("[REQUEST] NodeID: %s, SigFP: %s, SigSP: %s\n", SignShareMsg.NodeID, SignShareMsg.TS_FP, SignShareMsg.TS_SP)

	case *consensus.PrepareMsg:
		PrepareMsg := msg.(*consensus.PrepareMsg)
		fmt.Printf("[REQUEST] NodeID: %s, SP-PrepareSign: %s\n", PrepareMsg.NodeID, PrepareMsg.Prepare_TS_SP)

	case *consensus.CommitMsg:
		PrepareMsg := msg.(*consensus.CommitMsg)
		fmt.Printf("[REQUEST] NodeID: %s, SP-CommitSign: %s\n", PrepareMsg.NodeID, PrepareMsg.C_TS_SP)

	case *consensus.FastCommitProofMsg:
		FastCommitProofMsg := msg.(*consensus.FastCommitProofMsg)
		fmt.Printf("[REQUEST] NodeID: %s, SigFP: %s\n", FastCommitProofMsg.NodeID, FastCommitProofMsg.FC_TS_FP)

	case *consensus.SlowCommitProofMsg:
		SlowCommitProofMsg := msg.(*consensus.SlowCommitProofMsg)
		fmt.Printf("[REQUEST] NodeID: %s, SigFP: %s\n", SlowCommitProofMsg.NodeID, SlowCommitProofMsg.SC_TS_SP)

	case *consensus.SignStateMsg:
		SignStateMsg := msg.(*consensus.SignStateMsg)
		fmt.Printf("[REQUEST] NodeID: %s, SigSS: %s\n", SignStateMsg.NodeID, SignStateMsg.SS_TS)

	case *consensus.FullExecuteProofMsg:
		FullExecuteProofMsg := msg.(*consensus.FullExecuteProofMsg)
		fmt.Printf("[REQUEST] NodeID: %s, SigFP: %s\n", FullExecuteProofMsg.NodeID, FullExecuteProofMsg.FS_TS)
	}
}

func LogStage(stage string, isDone bool) {
	if isDone {
		fmt.Printf("[STAGE-DONE] %s\n", stage)
	} else {
		fmt.Printf("[STAGE-BEGIN] %s\n", stage)
	}
}
