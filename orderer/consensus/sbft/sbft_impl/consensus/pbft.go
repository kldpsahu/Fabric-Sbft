package consensus

type PBFT interface {
	StartConsensus(request *RequestBlock) (*PrePrepareMsg, error)
	PrePrepare(prePrepareMsg *PrePrepareMsg) (*SignShareMsg, error)
	SignShareFP(MessageTSCC []*MessageTSCC) (*FastCommitProofMsg, error)
	SignShareSP(MessageTSCC []*MessageTSCC) (*PrepareMsg, error)
	Prepare(prepareMsg *PrepareMsg) (*CommitMsg, error)
	Commit(prepareMsg *PrepareMsg) (*SlowCommitProofMsg, error)
	SignStateFC(SignStateMsg *SignStateMsg) (*FullExecuteProofMsg, error)
	SignStateSC(SignStateMsg *SignStateMsg) (*FullExecuteProofMsg, error)
	FullExecuteProof(FullExecuteProofMsg *FullExecuteProofMsg) (bool error)
	StoreKey(msg string) (*Keys, error)
}
