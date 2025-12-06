package wire

import (
	"github.com/goforj/forj/internal/forj"
	"github.com/google/wire"
)

var generalSet = wire.NewSet(
	forj.NewProjectRenderer,
)
