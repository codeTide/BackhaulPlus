//go:build !linux

package maintenance

func readRSS() (uint64, bool) {
	return 0, false
}
