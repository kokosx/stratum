// Package datalock coordinates exclusive operations on a Stratum data directory.
package datalock

import "os"

// Lock is held for the lifetime of an exclusive data-directory operation.
type Lock struct {
	file *os.File
}
