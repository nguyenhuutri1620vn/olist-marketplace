// Package password hashes passwords with bcrypt; clear-text passwords are
// never stored.
package password

import "golang.org/x/crypto/bcrypt"

// Bcrypt implements the service's Hasher. Zero Cost means bcrypt.DefaultCost.
type Bcrypt struct {
	Cost int
}

// Hash returns the bcrypt hash of pw.
func (b Bcrypt) Hash(pw string) (string, error) {
	cost := b.Cost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), cost)
	return string(h), err
}

// Compare returns nil when pw matches hash.
func (Bcrypt) Compare(hash, pw string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw))
}
