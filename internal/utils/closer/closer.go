package closer

import (
	"fmt"
	"io"
	"os"
)

// Close closes the closer and logs any errors to stderr.
func Close(closable io.Closer) {
	if closable == nil {
		return
	}

	err := closable.Close()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to close closer: %v\n", err)
	}
}
