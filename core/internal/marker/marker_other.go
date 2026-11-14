//go:build !windows

package marker

// Marker holds nothing until Linux and Android get their own: a Poros there runs alone for now.
type Marker struct{}

// Hold always succeeds.
func Hold(string) (*Marker, error) {
	return &Marker{}, nil
}

// Release has nothing to let go of.
func (m *Marker) Release() {}

// Taken is always false.
func Taken(string) bool {
	return false
}
