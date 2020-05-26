package threshold

import (
	cr "crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
	mr "math/rand"
	"os"
	"strings"
	"time"

	pbc "github.com/Nik-U/pbc"
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

func compress_sign(sig *pbc.Element) (bool, *pbc.Element) {
	sigBefore := sig.NewFieldElement().Set(sig)
	data := sig.CompressedBytes()
	sig.SetCompressedBytes(data)
	return sig.Equals(sigBefore), sig
}

func Sign(pairing *pbc.Pairing, message []byte, private_key *pbc.Element) *pbc.Element {
	// fmt.Println(private_key)

	// fmt.Println(pairing)
	// fmt.Println(message)
	int_hash := sha256.Sum256(message)
	hash := pairing.NewG1()
	hash.SetFromHash(int_hash[:])
	// fmt.Printf("Hash in sign: %v\n", hash)

	signature := pairing.NewG1()
	signature.PowZn(hash, private_key)
	// fmt.Printf("Signature: \n%v\n", sig)

	isValid, signature := compress_sign(signature)
	if !isValid {
		fmt.Printf("Compression unsuccesful")
		os.Exit(0)
	}
	return signature
}

func Verify_Compressed(pairing *pbc.Pairing, signature *pbc.Element, generator *pbc.Element, e_h_pk *pbc.Element) bool {

	data := signature.XBytes()
	signature.SetXBytes(data)

	e_sg_g := pairing.NewGT()
	e_sg_g.Pair(signature, generator)

	if e_sg_g.Equals(e_h_pk) {
		return true
	} else {
		e_sg_g.Invert(e_sg_g)
		if e_sg_g.Equals(e_h_pk) {
			return true
		} else {
			return false
		}
	}
}

func Verify_Random(pairing *pbc.Pairing, signature *pbc.Element, generator *pbc.Element, e_h_pk *pbc.Element) bool {
	// A random signature shouldn't verify
	signature.Rand()
	e_sg_g := pairing.NewGT()
	e_sg_g.Pair(signature, generator)
	if e_sg_g.Equals(e_h_pk) {
		fmt.Printf("random signature verifies")
		return false
	}
	return true
}

func Verify(pairing *pbc.Pairing, signature *pbc.Element, message []byte, generator *pbc.Element, public_key *pbc.Element) bool {
	e_sg_g, e_h_pk := pairing.NewGT(), pairing.NewGT()

	int_hash := sha256.Sum256(message)
	hash := pairing.NewG1()
	hash.SetFromHash(int_hash[:])

	e_sg_g.Pair(signature, generator)
	// fmt.Printf("temp1 %v\n", temp1)

	e_h_pk.Pair(hash, public_key)
	// fmt.Printf("f(hash, pub) %v\n", temp2)

	if !e_sg_g.Equals(e_h_pk) {
		fmt.Printf("Signature does not verify\n")
		os.Exit(0)
	}

	return Verify_Compressed(pairing, signature, generator, e_h_pk) && Verify_Random(pairing, signature, generator, e_h_pk)
}

func Aggregate(pairing *pbc.Pairing, signature []*pbc.Element, generator *pbc.Element) (*pbc.Element, bool) {
	sigma := pairing.NewG1()
	err := false
	if len(signature) != 0 {
		err = true
		sigma.Set(signature[0])

		for i := 1; i < len(signature); i++ {
			sigma.Mul(sigma, signature[i])
		}
	}
	return sigma, err
}

func Aggregate_Verify(pairing *pbc.Pairing, aggregate *pbc.Element, message []string, pub_key []*pbc.Element, generator *pbc.Element) bool {
	n := len(message)
	hash_arr := make([][sha256.Size]byte, n)

	for i := 0; i < n; i++ {
		hash_arr[i] = sha256.Sum256([]byte(message[i]))
	}

	//Checking Error left

	e_sg_g := pairing.NewGT()
	e_sg_g.Pair(aggregate, generator)

	hash := pairing.NewG1()
	hash.SetFromHash(hash_arr[0][:])

	e_h_pk := pairing.NewGT()
	e_h_pk.Pair(hash, pub_key[0])

	int_pairing := pairing.NewGT()
	for i := 1; i < len(hash_arr); i++ {
		hash.SetFromHash(hash_arr[i][:])
		int_pairing.Pair(hash, pub_key[i])
		e_h_pk.Mul(e_h_pk, int_pairing)
	}

	if !e_sg_g.Equals(e_h_pk) {
		fmt.Printf("signature does not verify")
		os.Exit(0)
	}

	return true
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

//Accumulates Signature
func Threshold_Accumulator(signatures []*pbc.Element, members []int, pairing *pbc.Pairing, parameters *pbc.Params) *pbc.Element {
	//checking left
	param_string := parameters.String()
	param_array := strings.Split(param_string, "\n")
	r := string(strings.Split(param_array[2], " ")[1])
	r_BI := new(big.Int)
	r_BI.SetString(r, 10)
	n := (len(r_BI.Text(2)) + 7) / 8
	bytes := make([]byte, n)
	bytes = r_BI.Bytes()

	fmt.Println("*******3.1********")

	sigma := pairing.NewG1()
	sigma.Set1()
	var p *big.Int
	var q *big.Int
	u := big.NewInt(0)
	v := big.NewInt(0)

	fmt.Println("*******3.2********")

	var lambda *big.Int
	lambda = big.NewInt(0)
	s := pairing.NewG1()
	for i := range members {

		p = big.NewInt(1)
		q = big.NewInt(1)
		for j := range members {
			if members[i] != members[j] {
				p.Mul(p, u.Neg(big.NewInt(int64(members[j]+1))))
				q.Mul(q, v.Sub(big.NewInt(int64(members[i]+1)), big.NewInt(int64(members[j]+1))))
			}
		}
		bytes = u.Mod(u.Mul(u.Mod(p, r_BI), v.Mod(v.ModInverse(q, r_BI), r_BI)), r_BI).Bytes()
		if len(bytes) == 0 {
			lambda = big.NewInt(0)
		} else {
			lambda.SetBytes(bytes)
		}
		s.PowBig(signatures[i], lambda)
		sigma.Mul(sigma, s)
	}

	fmt.Println("*******3.3********")

	return sigma

}

func main() {
	parameters := generate_parametersF(256)
	// fmt.Printf("Parameters: \n%v\n", parameters) // Print in PBC format

	pairing := generate_pairing(parameters)
	// fmt.Printf("Pairing: \n%v\n", pairing)

	generator := generate_Generator(pairing)
	// fmt.Printf("Generator: \n%v\n", generator)

	message := "message"
	// var groupKey *pbc.Element

	mr.Seed(time.Now().UnixNano())
	n := mr.Intn(20) + 1
	t := mr.Intn(n) + 1
	groupKey, _, _, memberSecrets := Generate_Key_Parts(t, n, pairing, generator)

	// Select group members.
	memberIds := mr.Perm(n)[:t]

	// Sign the message.
	//hash := sha256.Sum256([]byte(message))
	shares := make([]*pbc.Element, t)
	for i := 0; i < t; i++ {
		shares[i] = Sign(pairing, []byte(message), memberSecrets[memberIds[i]])
	}

	// Recover the threshold signature.
	signature := Threshold_Accumulator(shares, memberIds, pairing, parameters)

	// Verify the threshold signature.
	IsVerified := Verify(pairing, signature, []byte(message), generator, groupKey)
	if !IsVerified {
		fmt.Printf("Verification Failed !!!!! \n")
	}

	s := memberSecrets[0].String()

	fmt.Printf(s)

	// fmt.Printf(memberSecrets[0].String())

	key := memberSecrets[1]
	// var key *pbc.Element

	_, _ = key.SetString(s, 10)
	if key.Equals(memberSecrets[0]) {
		fmt.Printf("Can be sent as string !!!!! \n")
	}

}
