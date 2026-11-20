//go:build windows

package cua

func ownedBy(string, int) bool { return true }
