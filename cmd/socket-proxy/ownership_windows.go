//go:build windows

package main

func pathOwnership(string) (int, int, bool, error) {
	return 0, 0, false, nil
}
