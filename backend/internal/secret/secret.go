package secret

import "errors"

var ErrNotFound = errors.New("credential was not found in macOS Keychain")
