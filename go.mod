module main

require (
	github.com/decred/dcrd/chaincfg v1.2.0
	github.com/decred/dcrd/chaincfg/chainhash v1.0.1
	github.com/decred/dcrd/dcrutil v1.1.1
	github.com/decred/dcrd/fees v0.0.0-20181204200512-bd7106912abc
	github.com/decred/dcrd/wire v1.2.0
	github.com/decred/slog v1.0.0
)

replace (
	github.com/decred/dcrd/chaincfg => ../dcrd/chaincfg
	github.com/decred/dcrd/fees => ../dcrd/fees
	github.com/decred/dcrd/wire => ../dcrd/wire
)
