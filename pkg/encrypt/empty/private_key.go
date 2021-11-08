package empty

// PrivateKey will implement dummy methods to implement signer
type PrivateKey struct{}

// Sign will create a signature for given data
func (p *PrivateKey) Sign(obj interface{}) ([]byte, error) {
	return nil, nil
}
