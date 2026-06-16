package wire

import (
	"github.com/goforj/goforj/internal/cmd"
	"github.com/goforj/wire"
)

var cmdSet = wire.NewSet(
	cmd.NewRootCmd,
	cmd.NewHelloWorldCmd,
)
