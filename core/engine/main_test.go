package engine

import (
	"fmt"
	"os"
	"testing"

	"PorosCore/internal/marker"
)

// The markers of the tests are their own: a Poros running on this computer neither sees them nor gets in their way.
func TestMain(m *testing.M) {
	marker.Namespace = fmt.Sprintf("PorosTest%d", os.Getpid())
	os.Exit(m.Run())
}
