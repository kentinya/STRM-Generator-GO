//go:build !unix

package reconciler

import "os"

func flockExclusive(*os.File) error { return ErrBusy }

func flockUnlock(*os.File) error { return nil }
