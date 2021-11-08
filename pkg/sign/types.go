package sign

// Signer will create a signature that can be
// used to verify against public key
type Signer interface {
	Sign(obj interface{}) ([]byte, error)
}

// Unsigner will verify signature for given data using public key
type Unsigner interface {
	Unsign(data, signature []byte) error
}
