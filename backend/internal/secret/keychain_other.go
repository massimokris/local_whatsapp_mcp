//go:build !darwin || !cgo

package secret

import "errors"

func Set(service, account, value string) error {
	return errors.New("macOS Keychain requires a macOS build with CGO enabled")
}
func Get(service, account string) (string, error) {
	return "", errors.New("macOS Keychain requires a macOS build with CGO enabled")
}
func Delete(service, account string) error {
	return errors.New("macOS Keychain requires a macOS build with CGO enabled")
}
