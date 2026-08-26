package sampleplugin

import (
	"testing"

	"github.com/dokosoko/dokosoko-service/nativeplugin/plugintest"
)

func TestConformance(t *testing.T) {
	plugintest.TestPlugin(t, New())
}
