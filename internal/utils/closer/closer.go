package closer

import (
	"fmt"
	"io"
	"os"
	"reflect"
)

// Close closes the closer and logs any errors to stderr.
func Close(c io.Closer) {
	if c == nil {
		return
	}

	v := reflect.ValueOf(c)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Interface, reflect.Slice:
		if v.IsNil() {
			return
		}
	}

	err := c.Close()
	if err != nil {
		_, err := fmt.Fprintf(os.Stderr, "failed to close closer: %v\n", err)
		if err != nil {
			return
		}
	}
}
