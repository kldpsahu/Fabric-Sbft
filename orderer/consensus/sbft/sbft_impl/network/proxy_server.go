package network

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"../consensus"
)

type Server struct {
	url  string
	node *Node
}

func NewServer(nodeIP string) *Server {
	node := NewNode(nodeIP) //node.go returns node
	server := &Server{nodeIP, node}
	server.setRoute()
	return server
}

func (server *Server) Start() {
	fmt.Printf("Server will be started at %s...\n", server.url)
	if err := http.ListenAndServe(server.url, nil); err != nil {
		fmt.Println(err)
		return
	}
}

func (server *Server) setRoute() {
	http.HandleFunc("/req", server.getReq)
	http.HandleFunc("/preprepare", server.getPrePrepare)
	http.HandleFunc("/prepare", server.getPrepare)
	http.HandleFunc("/Commit", server.getCommit)
	http.HandleFunc("/SignShare", server.getSignShare)
	http.HandleFunc("/FullExecuteProof", server.getFullExecuteProof)
	http.HandleFunc("/SlowCommitProof", server.getSlowCommitProof)
	http.HandleFunc("/FastCommitProof", server.getFastCommitProof)
	http.HandleFunc("/SignState", server.getSignState)
	http.HandleFunc("/ReceiveKey", server.getReceiveKey)
	http.HandleFunc("/ReceiveSign", server.getReceiveSign)
	http.HandleFunc("/ViewChangeInit", server.getViewChangeInit)
	http.HandleFunc("/ViewChangeExecute", server.getViewChangeExecute)
	http.HandleFunc("/Block", server.getBlock)
	http.HandleFunc("/KeyLoadedSignal", server.getKeyLoadedSignal)
}

var Block *consensus.RequestBlock

//Transferring request from client to Msgentrance channel, getting called from setroute above

func (server *Server) getReq(writer http.ResponseWriter, request *http.Request) {
	// fmt.Println(Block)
	if Block == nil {
		Block = new(consensus.RequestBlock)
		Block.Requests = make([]*consensus.RequestMsg, 0)
		Block.SequenceID = -1000
	}
	// fmt.Println(Block)
	// fmt.Println("*#************1. To msgEntrance Channel*******#*")
	var msg *consensus.RequestMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	//fmt.Println(msg)
	if server.node.NodeID != server.node.View.Primary {
		fmt.Println("Request redirected to Primary")
		jsonMsg1, err1 := json.Marshal(msg)
		if err1 != nil {
			fmt.Println(err1)
		}
		send(server.node.NodeTable[server.node.View.Primary]+"/req", jsonMsg1)

	} else if len(Block.Requests) == 1 { //Taking 10 transactions in a block
		start = time.Now()
		server.node.MsgEntrance <- Block
		Block.Requests = Block.Requests[:0]
	} else {
		fmt.Println("Waiting for the Block to complete. Current number of transactions in the block:", len(Block.Requests)+1)
		Block.Requests = append(Block.Requests, msg)
	}

}

//getting called from setroute above
func (server *Server) getPrePrepare(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.PrePrepareMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	server.node.MsgEntrance <- &msg
}

func (server *Server) getPrepare(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.PrepareMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	server.node.MsgEntrance <- &msg
}

func (server *Server) getCommit(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.CommitMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	server.node.MsgEntrance <- &msg
}

func (server *Server) getSignShare(writer http.ResponseWriter, request *http.Request) {
	//fmt.Println("***********YEYEYEYE*****************")
	var msg consensus.SignShareMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	server.node.MsgEntrance <- &msg
}

func (server *Server) getFullExecuteProof(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.FullExecuteProofMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	server.node.MsgEntrance <- &msg
}

func (server *Server) getSlowCommitProof(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.SlowCommitProofMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	server.node.MsgEntrance <- &msg
}

func (server *Server) getFastCommitProof(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.FastCommitProofMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	server.node.MsgEntrance <- &msg
}

func (server *Server) getSignState(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.SignStateMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	// fmt.Println("******HERE111*****")

	server.node.MsgEntrance <- &msg
}

func (server *Server) getReceiveKey(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.Keys
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	// fmt.Println("******HERE111*****")

	server.node.MsgEntrance <- &msg
}

func (server *Server) getReceiveSign(writer http.ResponseWriter, request *http.Request) {
	var msg string
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	// fmt.Println("******HERE111*****")

	server.node.MsgTSS <- msg
}

func (server *Server) getKeyLoadedSignal(writer http.ResponseWriter, request *http.Request) {
	var msg string
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("******HERE111*****")

	server.node.MsgTSS <- msg
}

func (server *Server) getViewChangeInit(writer http.ResponseWriter, request *http.Request) {
	var msg *consensus.ViewChangeInitMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("******ViewChangeInit sent to msgchannel of nodes*****")

	server.node.MsgEntrance <- msg
}

func (server *Server) getViewChangeExecute(writer http.ResponseWriter, request *http.Request) {
	var msg *consensus.ViewChangeExecuteMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("******ViewChangeExecute sent to msgchannel of nodes*****")

	server.node.MsgEntrance <- msg
}

func (server *Server) getBlock(writer http.ResponseWriter, request *http.Request) {
	var msg *consensus.RequestBlock
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("******Block sent to New Leader*****")
	time.Sleep(500 * time.Millisecond) //To let the state be null first
	server.node.MsgEntrance <- msg
}

func send(url string, msg []byte) {
	// fmt.Println("+++++++++++++++")
	//var mut sync.Mutex
	buff := bytes.NewBuffer(msg)
	// fmt.Println("+++++++++++++++")
	//mut.Lock()
	_, err := http.Post("http://"+url, "application/json", buff)
	//mut.Unlock()
	if err != nil {
		// fmt.Println("***************Yahan Atka BC*********")

		fmt.Printf("%v", err)
	}
}
