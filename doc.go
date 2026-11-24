// Package waymark is a private DNS server.
// A name resolves to the addresses that are still inside their TTL.
package waymark

import "errors"

var ErrInvalid = errors.New("name is not valid")
