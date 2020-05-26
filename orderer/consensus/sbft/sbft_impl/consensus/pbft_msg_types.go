package consensus

import (
	"time"
)

type RequestMsg struct {
	Timestamp int64  `json:"timestamp"`
	ClientID  string `json:"clientID"`
	Operation string `json:"operation"`
}

type RequestBlock struct {
	Requests   []*RequestMsg
	SequenceID int64 `json:"sequenceID"`
}

type PrePrepareMsg struct {
	ViewID     int           `json:"viewID"`
	NodeID     string        `json:"nodeID"`
	SequenceID int64         `json:"sequenceID"`
	Block      *RequestBlock `json:"RequestBlock"`
}

type SignShareMsg struct {
	ViewID     int           `json:"viewID"`
	SequenceID int64         `json:"sequenceID"`
	NodeID     string        `json:"nodeID"`
	Block      *RequestBlock `json:"RequestBlock"`
	TS_FP      string        `json:"TS_FP"` // fast path: σi(h)
	TS_SP      string        `json:"TS_SP"` // slow path: τi(h)

}

type PrepareMsg struct {
	ViewID        int           `json:"viewID"`
	SequenceID    int64         `json:"sequenceID"`
	NodeID        string        `json:"nodeID"`
	MsgType       string        `json:"msgType"`
	Block         *RequestBlock `json:"RequestBlock"`
	Prepare_TS_SP string        `json:"TS_FP"` // Prepare sig for slow path: τ(h)
}

type FastCommitProofMsg struct {
	ViewID     int   `json:"viewID"`
	SequenceID int64 `json:"sequenceID"`
	//Digest     string `json:"digest"`
	NodeID   string        `json:"nodeID"`
	Block    *RequestBlock `json:"RequestBlock"`
	FC_TS_FP string        `json:"TS_FP"` // fast commit path: σ(h)
}

type CommitMsg struct {
	ViewID        int           `json:"viewID"`
	SequenceID    int64         `json:"sequenceID"`
	NodeID        string        `json:"nodeID"`
	Block         *RequestBlock `json:"RequestBlock"`
	C_TS_SP       string        `json:"TS_SP"` // fast commit path: τi(τ(h))
	Prepare_TS_SP string        `json:"TS_FP"` // Prepare sig for slow path: τ(h)
}

type SlowCommitProofMsg struct {
	ViewID     int   `json:"viewID"`
	SequenceID int64 `json:"sequenceID"`
	//Digest     string `json:"digest"`
	NodeID   string        `json:"nodeID"`
	Block    *RequestBlock `json:"RequestBlock"`
	SC_TS_SP string        `json:"TS_SP"` // fast commit path: τ(τ(h))
}

type SignStateMsg struct {
	SequenceID int64         `json:"sequenceID"`
	NodeID     string        `json:"nodeID"`
	Block      *RequestBlock `json:"RequestBlock"`
	SS_TS      string        `json:"SS_TS"` //  πi(d), d = digest(Ds)
}

type FullExecuteProofMsg struct {
	SequenceID int64         `json:"sequenceID"`
	NodeID     string        `json:"nodeID"`
	Block      *RequestBlock `json:"RequestBlock"`
	FS_TS      string        `json:"SS_TS"` //  π(d), d = digest(Ds)
}

type MessageTSCC struct {
	WaitMsg SignShareMsg
	TS      time.Time
}

type MessageTSEC struct {
	WaitMsg SignStateMsg
	TS      time.Time
}

type MessageTSCP struct {
	WaitMsg CommitMsg
	TS      time.Time
}

type Keys struct {
	PvtKey         string
	KeyShare_Sigma string
	KeyShare_Tou   string
	KeyShare_Pi    string
	GroupKey_Sigma string
	GroupKey_Tou   string
	GroupKey_Pi    string
	TotalNodes     int
	ViewID         int //while sending keys also send the current view to maintain consistency
}

type SignPacket struct {
	Request string
	Key     string
	NodeID  string
	Type    string
}

type AccumulatePacket struct {
	Request   string
	KeyShares []string
	GroupKey  string
	NodeID    string
	MemberID  []string
	Path      string
}

//generator and keys are in string needs to be converted back

type ViewChangeInitMsg struct {
	CurrViewID     int           `json:"CurrViewID"`
	ProposedViewID int           `json:"PropesedViewID"`
	Block          *RequestBlock `json:"RequestBlock"`
	NodeID         string        `json:"nodeID"`
}

type ViewChangeExecuteMsg struct {
	CurrViewID     int           `json:"CurrViewID"`
	Block          *RequestBlock `json:"RequestBlock"`
	ProposedViewID int           `json:"PropesedViewID"`
	NodeID         string        `json:"nodeID"`
}
