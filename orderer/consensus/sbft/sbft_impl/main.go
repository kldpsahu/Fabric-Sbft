package main

import (
	"os"
	"strconv"

	"./network"

	"./keystore"
)

// "github.com\\bigpicturelabs\\consensusPBFT\\pbft\\network"
func main() {
	nodeIP := os.Args[1] // taking node Id in cmd line

	// var ser *network.Server
	if nodeIP == "localhost:1109" {
		n, _ := strconv.Atoi(os.Args[2])
		ser := keystore.NewKeyStoreServer(nodeIP, n) //In keystoreinit.go
		ser.Startkeystore()
	} else {
		ser := network.NewServer(nodeIP) //In proxy_server.go
		ser.Start()
	}

}
