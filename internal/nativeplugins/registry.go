package nativeplugins

import "github.com/dokosoko/dokosoko-service/nativeplugin"

// Registered is the explicit source-level composition point for trusted native
// plugins. Add imports and constructors here; never register plugins via init.
func Registered() []nativeplugin.Plugin {
	return nil
}
