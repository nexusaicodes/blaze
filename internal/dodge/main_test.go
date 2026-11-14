package dodge

import (
	"os"
	"testing"

	"github.com/sakshamsaxena/blaze/internal/logger"
)

func TestMain(m *testing.M) {
	logger.InitNop()
	os.Exit(m.Run())
}
