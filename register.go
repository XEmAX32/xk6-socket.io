package socketio

import "go.k6.io/k6/js/modules"

const importPath = "k6/x/socketio"

func init() {
	modules.Register(importPath, new(rootModule))
}
