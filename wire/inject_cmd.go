package wire

import (
	"github.com/goforj/forj/internal/cmd"
	"github.com/google/wire"
)

var cmdSet = wire.NewSet(
	cmd.NewRootCmd,
	cmd.NewHelloWorldCmd,
)
