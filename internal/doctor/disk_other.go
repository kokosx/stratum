//go:build !unix

package doctor

import "errors"

func diskFreeBytes(path string) (uint64, error) {
	return 0, errors.New("disk free not available on this platform")
}
