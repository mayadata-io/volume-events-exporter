package empty

// PublicKey will implement dummy methods to implement signer
type PublicKey struct{}

// Unsign will verify the given singature, if matches error will be nil
func (p *PublicKey) Unsign(data, signature []byte) error {
	return nil
}
