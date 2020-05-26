package keystore

import (
	pbc "github.com/Nik-U/pbc"

	"crypto/rand"
	cr "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"math/big"
)

func generate_parametersA(r uint32, q uint32) *pbc.Params {
	return pbc.GenerateA(r, q)
}

func generate_parametersD(d uint32, rbits uint32, qbits uint32, bitlimit uint32) (*pbc.Params, error) {
	return pbc.GenerateD(d, rbits, qbits, bitlimit)
}

func generate_parametersF(bits uint32) *pbc.Params {
	return pbc.GenerateF(bits)
}

func generate_pairing(parameters *pbc.Params) *pbc.Pairing {
	return parameters.NewPairing()
}

func generate_Generator(pairing *pbc.Pairing) *pbc.Element {
	return pairing.NewG2().Rand()
}

func generate_private_key(pairing *pbc.Pairing) *pbc.Element {
	return pairing.NewZr().Rand()
}

func generate_public_key(pairing *pbc.Pairing, Generator *pbc.Element, private_key *pbc.Element) *pbc.Element {
	return pairing.NewG2().PowZn(Generator, private_key)
}

func generate_keys(pairing *pbc.Pairing, Generator *pbc.Element) (*pbc.Element, *pbc.Element) {
	private_key := generate_private_key(pairing)
	public_key := generate_public_key(pairing, Generator, private_key)
	return private_key, public_key
}

func Generate_Key_Parts(t int, n int, pairing *pbc.Pairing, generator *pbc.Element) (*pbc.Element, []*pbc.Element, *pbc.Element, []*pbc.Element) {

	//Error checking left

	coefficient := make([]*pbc.Element, t)
	var hash [sha256.Size]byte

	for j := range coefficient {
		cr.Read(hash[:])
		// check error here, this function returns two values so can be done by  _, err := rand.Read(hash[:])

		coefficient[j] = pairing.NewZr()
		coefficient[j].SetFromHash(hash[:])
	}

	pub_key := make([]*pbc.Element, n+1)
	pvt_key := make([]*pbc.Element, n+1)
	var bytes []byte
	var iToj *big.Int
	iToj = big.NewInt(0)
	temp := pairing.NewZr()

	for i := 0; i < n+1; i++ {

		pvt_key[i] = pairing.NewZr()
		pvt_key[i].Set0()
		for j := 0; j < t; j++ {
			bytes = big.NewInt(0).Exp(big.NewInt(int64(i)), big.NewInt(int64(j)), nil).Bytes()
			if len(bytes) > 0 {
				iToj.SetBytes(bytes)
			} else {
				iToj = big.NewInt(0)
			}
			temp.MulBig(coefficient[j], iToj)
			pvt_key[i].Add(pvt_key[i], temp)
		}
		pub_key[i] = generate_public_key(pairing, generator, pvt_key[i])
	}
	return pub_key[0], pub_key[1:], pvt_key[0], pvt_key[1:]
}

func (node *keystoreNode) createkey_sigma(n int, t int) error {
	parameters := generate_parametersF(256)
	// fmt.Printf("Parameters: \n%v\n", parameters)    // Print in PBC format

	pairing := generate_pairing(parameters)
	// fmt.Printf("Pairing: \n%v\n", pairing)

	generator := generate_Generator(pairing)
	// fmt.Printf("Generator: \n%v\n", generator)

	node.groupkey_sigma, _, _, node.key_share_sigma = Generate_Key_Parts(t, n, pairing, generator)

	node.Pairing_sigma = pairing
	node.Generator_sigma = generator
	node.Parameters_sigma = parameters

	return nil
}

func (node *keystoreNode) createkey_tou(n int, t int) error {
	parameters := generate_parametersF(256)
	// fmt.Printf("Parameters: \n%v\n", parameters)    // Print in PBC format

	pairing := generate_pairing(parameters)
	// fmt.Printf("Pairing: \n%v\n", pairing)

	generator := generate_Generator(pairing)
	// fmt.Printf("Generator: \n%v\n", generator)

	node.groupkey_tou, _, _, node.key_share_tou = Generate_Key_Parts(t, n, pairing, generator)

	node.Pairing_tou = pairing
	node.Generator_tou = generator
	node.Parameters_tou = parameters

	return nil
}

func (node *keystoreNode) createkey_pi(n int, t int) error {
	parameters := generate_parametersF(256)
	// fmt.Printf("Parameters: \n%v\n", parameters)    // Print in PBC format

	pairing := generate_pairing(parameters)
	// fmt.Printf("Pairing: \n%v\n", pairing)

	generator := generate_Generator(pairing)
	// fmt.Printf("Generator: \n%v\n", generator)

	node.groupkey_pi, _, _, node.key_share_pi = Generate_Key_Parts(t, n, pairing, generator)

	node.Pairing_pi = pairing
	node.Generator_pi = generator
	node.Parameters_pi = parameters

	return nil
}

func (node *keystoreNode) createkey(n int) error {

	for i := 0; i < n+2; i++ {
		node.pvt_key[i], _ = rsa.GenerateKey(rand.Reader, 2048)
		node.pub_key[i] = &node.pvt_key[i].PublicKey
		// fmt.Println(node.pvt_key[i])
		// fmt.Println(node.pub_key[i])
	}

	return nil
}
