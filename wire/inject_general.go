package wire

import (
	"github.com/goforj/goforj/internal/forj"
	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/wire"
)

var generalSet = wire.NewSet(
	wire.Bind(new(makeapp.Renderer), new(*forj.ProjectRenderer)),
	forj.NewProjectRenderer,
)
