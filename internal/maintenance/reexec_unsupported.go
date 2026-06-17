//go:build !unix

package maintenance

import "fmt"

func reexecSelf() error {
	return fmt.Errorf("auto restart is unsupported on this platform")
}
