package wire

import (
	"github.com/goforj/goforj/internal/forj"
	"github.com/goforj/wire"
)

var generalSet = wire.NewSet(
	forj.NewProjectRenderer,
)
