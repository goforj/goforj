//go:build !unix

package projectupgrade

import "os"

// openInspectionFile preserves native rooted read-only opens on platforms without Unix FIFO semantics.
func openInspectionFile(root *os.Root, name string) (*os.File, error) {
	return root.Open(name)
}
