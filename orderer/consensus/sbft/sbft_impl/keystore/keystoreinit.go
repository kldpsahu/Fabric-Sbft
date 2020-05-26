package keystore

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strconv"

	"../consensus"

	"../threshold"

	pbc "github.com/Nik-U/pbc"
)

const f = 1
const c = 0

var n int

var sign *pbc.Element

type View struct {
	ID          int
	Primary     string
	C_collector string
	E_collector string
}

// const ResolvingTimeDuration = time.Millisecond * 1000

type keystoreNode struct {
	NodeTable        map[string]string // key=nodeID, value=url
	RevNodeTable     map[string]string
	View             *View
	RequestChannel   chan string
	SignChannel      chan interface{}
	Alarm            chan bool
	Parameters_sigma *pbc.Params
	Generator_sigma  *pbc.Element
	Pairing_sigma    *pbc.Pairing
	Parameters_tou   *pbc.Params
	Generator_tou    *pbc.Element
	Pairing_tou      *pbc.Pairing
	Parameters_pi    *pbc.Params
	Pairing_pi       *pbc.Pairing
	Generator_pi     *pbc.Element
	key_share_sigma  []*pbc.Element
	groupkey_sigma   *pbc.Element
	key_share_tou    []*pbc.Element
	groupkey_tou     *pbc.Element
	key_share_pi     []*pbc.Element
	groupkey_pi      *pbc.Element
	pvt_key          []*rsa.PrivateKey
	pub_key          []*rsa.PublicKey
	NodeID           string
	ProposedViewMap  map[int]int
	// //
	// Sig2 []*pbc.Element
}

type keyserver struct {
	url  string
	node *keystoreNode
}

/*--------------------------------------------------------------------*/

func NewKeyStoreServer(nodeIP string, nodes int) *keyserver {
	n = nodes
	node := NewKeyStoreNode(n)
	server := &keyserver{"localhost:1109", node}
	server.setKeyStoreRoute()
	return server
}

/*--------------------------------------------------------------------*/

func (server *keyserver) Startkeystore() {
	fmt.Printf("Server will be started at %s...\n", server.url)
	if err := http.ListenAndServe(server.url, nil); err != nil {
		fmt.Println(err)
		return
	}

}

/*--------------------------------------------------------------------*/

func (server *keyserver) setKeyStoreRoute() {
	http.HandleFunc("/RequestKey", server.getRequestKey)
	http.HandleFunc("/SignKeys", server.getSignKeys)
	http.HandleFunc("/Accumulate", server.getAccumulate)
	http.HandleFunc("/ViewChangeInit", server.getViewChangeInit)
	http.HandleFunc("/ViewChangeExecute", server.getViewChangeExecute)

}

/*--------------------------------------------------------------------*/

func (server *keyserver) getRequestKey(writer http.ResponseWriter, request *http.Request) {
	fmt.Println("node request recieved")
	var msg string
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	msg = server.node.RevNodeTable[msg]
	server.node.RequestChannel <- msg
}

/*--------------------------------------------------------------------*/

func (server *keyserver) getSignKeys(writer http.ResponseWriter, request *http.Request) {
	fmt.Println("Sign request recieved")
	var msg *consensus.SignPacket
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	server.node.SignChannel <- msg
}

/*--------------------------------------------------------------------*/

func (server *keyserver) getAccumulate(writer http.ResponseWriter, request *http.Request) {
	fmt.Println("Accumulate request recieved")
	var msg *consensus.AccumulatePacket
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("Accumulate request Going to channel")
	server.node.SignChannel <- msg
}

/*--------------------------------------------------------------------*/

func (server *keyserver) getViewChangeInit(writer http.ResponseWriter, request *http.Request) {
	var msg *consensus.ViewChangeInitMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("******ViewChangeInit sent to msgchannel of nodes*****")

	server.node.SignChannel <- msg
}

/*--------------------------------------------------------------------*/

func (server *keyserver) getViewChangeExecute(writer http.ResponseWriter, request *http.Request) {
	var msg *consensus.ViewChangeExecuteMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("******ViewChangeExecute sent to msgchannel of nodes*****")

	server.node.SignChannel <- msg
}

/*--------------------------------------------------------------------*/

func NewKeyStoreNode(nodes int) *keystoreNode {
	const viewID = 0 // temporary.
	node := &keystoreNode{
		NodeID:    "KS",
		NodeTable: makenodetable(nodes),

		RevNodeTable: makerevnodetable(nodes),
		View: &View{
			ID:          viewID,
			Primary:     "0",
			C_collector: "1",
			E_collector: "2",
		},
		Pairing_sigma:    nil,
		Parameters_sigma: nil,
		Generator_sigma:  nil,
		Pairing_tou:      nil,
		Parameters_tou:   nil,
		Generator_tou:    nil,
		Pairing_pi:       nil,
		Parameters_pi:    nil,
		Generator_pi:     nil,
		key_share_sigma:  make([]*pbc.Element, 0),
		groupkey_sigma:   nil,
		key_share_tou:    make([]*pbc.Element, 0),
		groupkey_tou:     nil,
		key_share_pi:     make([]*pbc.Element, 0),
		groupkey_pi:      nil,
		pvt_key:          make([]*rsa.PrivateKey, n+2),
		pub_key:          make([]*rsa.PublicKey, n+2),
		// Channels
		RequestChannel: make(chan string, 100),
		SignChannel:    make(chan interface{}, 100),
		Alarm:          make(chan bool, 100),
		// Sig2:           make([]*pbc.Element, 0),

		ProposedViewMap: make(map[int]int),
	}

	// Start Req dispatcher
	go node.dispatchReq()
	// go node.alarmTokeyDispatcher()

	node.createkey_sigma(n, 3*f)

	node.createkey_tou(n, f)

	node.createkey_pi(n, f)

	node.createkey(n)

	return node

}

/*--------------------------------------------------------------------*/

func (node *keystoreNode) dispatchReq() {
	for {
		select {
		case msg := <-node.RequestChannel:

			fmt.Println("******HERE2222*****")
			err := node.sendkey(msg)
			if err != nil {
				fmt.Println(err)
			}

		case msg := <-node.SignChannel:
			// fmt.Println("******HERE2222*****")
			err := node.routeMsg1(msg)
			if err != nil {
				fmt.Println(err)
			}

		}
	}
}

/*--------------------------------------------------------------------*/

func (node *keystoreNode) routeMsg1(msg interface{}) []error {
	switch msg.(type) {
	case *consensus.SignPacket:
		err := node.sendSign(msg.(*consensus.SignPacket))
		if err != nil {
			fmt.Println(err)
		}
	case *consensus.AccumulatePacket:
		fmt.Println("Accumulate request rcvd at channel")
		err := node.sendFullSign(msg.(*consensus.AccumulatePacket))
		if err != nil {
			fmt.Println(err)
		}
	case *consensus.ViewChangeInitMsg:
		fmt.Println("ViewChange request rcvd at channel")
		err := node.getViewChangeInit(msg.(*consensus.ViewChangeInitMsg))
		if err != nil {
			fmt.Println(err)
		}
	case *consensus.ViewChangeExecuteMsg:
		fmt.Println("ViewChange request rcvd at channel")
		err := node.getViewChangeExecute(msg.(*consensus.ViewChangeExecuteMsg))
		if err != nil {
			fmt.Println(err)
		}

	}
	return nil
}

/*--------------------------------------------------------------------*/

/*--------------------------------------------------------------------*/

func (node *keystoreNode) sendkey(msg string) error {
	KeyShareMsg, err := node.StoreKey(msg)
	if err != nil {
		return err
	}
	jsonMsg, err := json.Marshal(KeyShareMsg)
	if err != nil {
		return err
	}
	// fmt.Println(KeyShareMsg)
	// fmt.Println("*******msg********")

	send(node.NodeTable[msg]+"/ReceiveKey", jsonMsg)
	return nil
}

/*---------------------------------------------------------------------------------*/
func (node *keystoreNode) getViewChangeInit(Msg *consensus.ViewChangeInitMsg) error {
	fmt.Println("View change Init msg received at KS node")
	viewchangeexecute := node.ViewChangeExecute(Msg)
	node.Broadcast(viewchangeexecute, "/ViewChangeExecute") //Todo: Changing this broadcast status
	return nil
}

/*---------------------------------------------------------------------------------*/
func (node *keystoreNode) getViewChangeExecute(Msg *consensus.ViewChangeExecuteMsg) error {

	fmt.Println("+++++++++++View change execute")

	node.View.ID = Msg.ProposedViewID
	node.View.Primary = strconv.Itoa((node.View.ID * 3) % n)
	node.View.C_collector = strconv.Itoa((node.View.ID*3 + 1) % n)
	node.View.E_collector = strconv.Itoa((node.View.ID*3 + 2) % n)

	//Creating new keys for new view
	node.createkey_sigma(n, 3*f)

	node.createkey_tou(n, f)

	node.createkey_pi(n, f)

	node.createkey(n)

	//Loading keys for all the nodes in the node table

	for nodeID := range node.NodeTable {
		if nodeID == node.NodeID { //remove collector nodes from broadcast
			continue //Need to make it generic for views
		}

		node.sendkey(nodeID)
	}

	jsonMsg, err := json.Marshal("Keys loaded for the new view")
	if err != nil {
		return err
	}

	send(node.NodeTable[node.View.Primary]+"/KeyLoadedSignal", jsonMsg)

	fmt.Println("+++++++++++New View Loaded with Primary:")
	fmt.Println(node.View.Primary)
	fmt.Println(node.View.C_collector)
	fmt.Println(node.View.E_collector)

	return nil
}

/*---------------------------------------------------------------------------------*/

func (node *keystoreNode) ViewChangeExecute(Msg *consensus.ViewChangeInitMsg) *consensus.ViewChangeExecuteMsg {

	return &consensus.ViewChangeExecuteMsg{
		CurrViewID:     Msg.CurrViewID,
		ProposedViewID: Msg.ProposedViewID,
		NodeID:         Msg.NodeID,
	}
}

//-----------------------------------------------------

func (node *keystoreNode) StoreKey(msg string) (consensus.Keys, error) {
	if msg == node.View.Primary {
		fmt.Println("Primary loading")
		x, _ := strconv.Atoi(msg)
		return consensus.Keys{
			PvtKey:         ExportRsaPrivateKeyAsPemStr(node.pvt_key[x]), //0 index at start
			KeyShare_Sigma: node.key_share_sigma[x].String(),
			KeyShare_Tou:   node.key_share_tou[x].String(),
			KeyShare_Pi:    node.key_share_pi[x].String(),
			TotalNodes:     n,
			ViewID:         node.View.ID,
		}, nil
	}
	if msg == node.View.C_collector {
		fmt.Println("CC loading")
		x, _ := strconv.Atoi(msg)
		return consensus.Keys{
			PvtKey:         ExportRsaPrivateKeyAsPemStr(node.pvt_key[x]), //1 index at start
			GroupKey_Sigma: node.groupkey_sigma.String(),
			GroupKey_Tou:   node.groupkey_tou.String(),
			TotalNodes:     n,
			ViewID:         node.View.ID,
		}, nil
	}

	if msg == node.View.E_collector {
		fmt.Println("EC loading")
		x, _ := strconv.Atoi(msg)
		return consensus.Keys{
			PvtKey:      ExportRsaPrivateKeyAsPemStr(node.pvt_key[x]),
			GroupKey_Pi: node.groupkey_pi.String(),
			TotalNodes:  n,
			ViewID:      node.View.ID,
		}, nil
	}

	fmt.Println("Rest loading")

	x, _ := strconv.Atoi(msg)
	return consensus.Keys{
		PvtKey:         ExportRsaPrivateKeyAsPemStr(node.pvt_key[x]), //0 index at start
		KeyShare_Sigma: node.key_share_sigma[x].String(),
		KeyShare_Tou:   node.key_share_tou[x].String(),
		KeyShare_Pi:    node.key_share_pi[x].String(),
		TotalNodes:     n,
		ViewID:         node.View.ID,
	}, nil

}

//-----------------------------------------------------

func (node *keystoreNode) sendSign(msg *consensus.SignPacket) error {

	s := node.CreateSign(msg) //current state has been changed to new state

	jsonMsg, err := json.Marshal(s)
	if err != nil {
		return err
	}
	// fmt.Println(s)
	// fmt.Println("*******Signmsg********")

	send(node.NodeTable[msg.NodeID]+"/ReceiveSign", jsonMsg)
	return nil
}

/*--------------------------------------------------------------------*/

func (node *keystoreNode) CreateSign(msg *consensus.SignPacket) string {

	fmt.Println("Msg at KS for create sign:")
	fmt.Println(msg.Request)

	var key *pbc.Element

	if msg.Type == "Sigma" {
		key = node.Pairing_sigma.NewZr()

		// key.Set(node.key_share_sigma[0])

		_, _ = key.SetString(msg.Key, 10)
		sign = threshold.Sign(node.Pairing_sigma, []byte(msg.Request), key)
	}

	if msg.Type == "Tou" {
		key = node.Pairing_tou.NewZr()

		// key.Set(node.key_share_sigma[0])

		_, _ = key.SetString(msg.Key, 10)
		sign = threshold.Sign(node.Pairing_tou, []byte(msg.Request), key)
	}

	if msg.Type == "Pi" {
		key = node.Pairing_tou.NewZr()

		// key.Set(node.key_share_sigma[0])

		_, _ = key.SetString(msg.Key, 10)
		sign = threshold.Sign(node.Pairing_pi, []byte(msg.Request), key)
	}

	return sign.String()
}

//-----------------------------------------------------

func (node *keystoreNode) sendFullSign(msg *consensus.AccumulatePacket) error {
	fmt.Println("Accumulate request called FULLSIGN")

	var s string

	if msg.Path == "FP" {
		s = node.CreateFullSignSigma(msg) //current state has been changed to new state
	} else if msg.Path == "SP" {
		s = node.CreateFullSignTou(msg) //current state has been changed to new state
	} else {
		s = node.CreateFullSignPi(msg) //current state has been changed to new state
	}

	// fmt.Println("*******Full Sign Created********")

	jsonMsg, err := json.Marshal(s)
	if err != nil {
		return err
	}
	// fmt.Println(s)
	// fmt.Println("*******Signmsg********")

	send(node.NodeTable[msg.NodeID]+"/ReceiveSign", jsonMsg)
	return nil
}

/*--------------------------------------------------------------------*/

func (node *keystoreNode) CreateFullSignSigma(msg *consensus.AccumulatePacket) string {

	fmt.Println("*******1********")

	// fmt.Println("Msg at KS for create sign:")
	// fmt.Println(msg.Request)

	// fmt.Println(msg.KeyShares)

	// Select group members.
	t := 3 * f
	memberIds := make([]int, len(msg.MemberID))
	for i := 0; i < len(msg.MemberID); i++ {
		n, _ := strconv.Atoi(msg.MemberID[i])
		memberIds[i] = n // if msg.MemberID[i] == "A" {

	}
	// fmt.Println(len(msg.KeyShares))

	// fmt.Println("MemberIds")
	// fmt.Println(memberIds)

	fmt.Println("*******2********")

	memberIds = memberIds[:t]
	fmt.Println(memberIds)

	//hash := sha256.Sum256([]byte(message))
	// fmt.Println("*******KeyShares********")
	// fmt.Println(msg.KeyShares)

	shares := make([]*pbc.Element, t)

	for i := 0; i < t; i++ {

		// fmt.Println(key.SetString(msg.KeyShares[memberIds[i]], 10))
		shares[i] = node.Pairing_sigma.NewG1()
		// shares[i].Set(key)
		// fmt.Println(shares[i])
	}

	for i := 0; i < t; i++ {

		// fmt.Println(key.SetString(msg.KeyShares[memberIds[i]], 10))
		_, _ = shares[i].SetString(msg.KeyShares[i], 10)
		// shares[i].Set(key)
		// fmt.Println(shares[i])
	}
	// fmt.Println("*******Shares********")
	// fmt.Println(shares)

	// fmt.Println("*******3********")

	signature := threshold.Threshold_Accumulator(shares, memberIds, node.Pairing_sigma, node.Parameters_sigma)

	// fmt.Println("*******4********")

	fmt.Println(signature)

	// fmt.Println(signature2)
	// Verify the threshold signature.

	var key *pbc.Element

	key = node.Pairing_sigma.NewG2()
	_, _ = key.SetString(msg.GroupKey, 10)

	IsVerified := threshold.Verify(node.Pairing_sigma, signature, []byte(msg.Request), node.Generator_sigma, key)
	if !IsVerified {
		fmt.Printf("Verification Failed !!!!! \n")
	}
	fmt.Println("*******Full Sigma Sign Created********")

	fmt.Printf("FP Verified !!!!! \n")

	return signature.String()
}

/*--------------------------------------------------------------------*/

func (node *keystoreNode) CreateFullSignTou(msg *consensus.AccumulatePacket) string {

	fmt.Println("*******1********")

	// Select group members.
	t := f
	memberIds := make([]int, len(msg.MemberID))
	for i := 0; i < len(msg.MemberID); i++ {
		n, _ := strconv.Atoi(msg.MemberID[i])
		memberIds[i] = n // if msg.MemberID[i] == "A" {

	}
	fmt.Println(len(msg.KeyShares))

	fmt.Println("*******2********")

	memberIds = memberIds[:t]
	fmt.Println(memberIds)

	fmt.Println("*******KeyShares********")
	fmt.Println(msg.KeyShares)

	shares := make([]*pbc.Element, t)

	for i := 0; i < t; i++ {

		shares[i] = node.Pairing_tou.NewG1()

	}

	for i := 0; i < t; i++ {

		_, _ = shares[i].SetString(msg.KeyShares[i], 10)

	}

	signature := threshold.Threshold_Accumulator(shares, memberIds, node.Pairing_tou, node.Parameters_tou)

	var key *pbc.Element

	key = node.Pairing_tou.NewG2()
	_, _ = key.SetString(msg.GroupKey, 10)

	IsVerified := threshold.Verify(node.Pairing_tou, signature, []byte(msg.Request), node.Generator_tou, key)
	if !IsVerified {
		fmt.Printf("Verification Failed !!!!! \n")
	}

	fmt.Println("*******Full Tou Sign Created********")

	fmt.Printf("SP Verified !!!!! \n")

	return signature.String()
}

/*--------------------------------------------------------------------*/

func (node *keystoreNode) CreateFullSignPi(msg *consensus.AccumulatePacket) string {

	fmt.Println("*******1********")

	t := f
	memberIds := make([]int, len(msg.MemberID))
	for i := 0; i < len(msg.MemberID); i++ {
		n, _ := strconv.Atoi(msg.MemberID[i])
		memberIds[i] = n

	}
	fmt.Println(len(msg.KeyShares))

	fmt.Println("*******2********")

	memberIds = memberIds[:t]
	fmt.Println(memberIds)

	fmt.Println("*******KeyShares********")
	fmt.Println(msg.KeyShares)

	shares := make([]*pbc.Element, t)

	for i := 0; i < t; i++ {

		shares[i] = node.Pairing_pi.NewG1()

	}

	for i := 0; i < t; i++ {

		_, _ = shares[i].SetString(msg.KeyShares[i], 10)

	}

	signature := threshold.Threshold_Accumulator(shares, memberIds, node.Pairing_pi, node.Parameters_pi)

	fmt.Println("*******4********")

	fmt.Println(signature)
	var key *pbc.Element

	key = node.Pairing_pi.NewG2()
	_, _ = key.SetString(msg.GroupKey, 10)

	IsVerified := threshold.Verify(node.Pairing_pi, signature, []byte(msg.Request), node.Generator_pi, key)
	if !IsVerified {
		fmt.Printf("Verification Failed !!!!! \n")
	}

	fmt.Println("*******Full PI Sign Created********")

	fmt.Printf("Client Verified !!!!! \n")

	return signature.String()
}

/*--------------------------------------------------------------------*/

func (node *keystoreNode) Broadcast(msg interface{}, path string) map[string]error {

	errorMap := make(map[string]error)

	for nodeID, url := range node.NodeTable {
		if nodeID == node.NodeID { //remove collector nodes from broadcast
			continue //Need to make it generic for views
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

func ExportRsaPrivateKeyAsPemStr(privkey *rsa.PrivateKey) string {
	privkey_bytes := x509.MarshalPKCS1PrivateKey(privkey)
	privkey_pem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: privkey_bytes,
		},
	)
	return string(privkey_pem)
}

/*--------------------------------------------------------------------*/

func send(url string, msg []byte) {
	//fmt.Println("at last")
	buff := bytes.NewBuffer(msg)
	_, err := http.Post("http://"+url, "application/json", buff)
	if err != nil {
		fmt.Printf("%v", err)
	}
}

/*--------------------------------------------------------------------*/

func makenodetable(n int) map[string]string {
	m := make(map[string]string)
	m["KS"] = "localhost:1109"
	for i := 0; i < n; i++ {
		m[strconv.Itoa(i)] = "localhost:" + strconv.Itoa(1110+i)
		// fmt.Println(strconv.Itoa(i))
		// fmt.Println("localhost:" + strconv.Itoa(1110+i))
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
