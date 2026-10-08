//go:build !darwin && !windows

package input

// GenericBackend is the fallback implementation of InputBackend for other OSes.
type GenericBackend struct {
	*MockBackend
}

// NewNativeBackend initializes the generic input backend.
func NewNativeBackend() (InputBackend, error) {
	return &GenericBackend{
		MockBackend: NewMockBackend(),
	}, nil
}
