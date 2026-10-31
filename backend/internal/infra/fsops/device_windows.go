package fsops

import "os"

// os.FileInfo.Sys on Windows only exposes attributes and timestamps, not a
// stable file ID. Never invent an identity from those mutable values: the
// organizer must fail closed rather than remove an unrelated replacement.
func Identity(os.FileInfo) string { return "" }

// Without the volume serial in os.FileInfo, assume different devices so that
// automatic moves stage a copy rather than an unsafe hardlink.
func Device(os.FileInfo) uint64 { return 0 }
