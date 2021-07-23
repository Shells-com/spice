package spice

import "fmt"

type SpiceError uint32

const (
	ErrSpiceLinkOk SpiceError = iota
	ErrSpiceLinkError
	ErrSpiceLinkInvalidMagic
	ErrSpiceLinkInvalidData
	ErrSpiceLinkVersionMismatch
	ErrSpiceLinkNeedSecured
	ErrSpiceLinkNeedUnsecured
	ErrSpiceLinkPermissionDenied
	ErrSpiceLinkBadConnectionId
	ErrSpiceLinkChannelNotAvailable
)

func (e SpiceError) Error() string {
	switch e {
	case ErrSpiceLinkOk:
		return "spice: OK"
	case ErrSpiceLinkError:
		return "spice: Error"
	case ErrSpiceLinkInvalidMagic:
		return "spice: Invalid magic"
	case ErrSpiceLinkInvalidData:
		return "spice: Invalid data"
	case ErrSpiceLinkVersionMismatch:
		return "spice: Version mismatch"
	case ErrSpiceLinkNeedSecured:
		return "spice: Need secured"
	case ErrSpiceLinkNeedUnsecured:
		return "spice: Need unsecured"
	case ErrSpiceLinkPermissionDenied:
		return "spice: Permission denied"
	case ErrSpiceLinkBadConnectionId:
		return "spice: bad connection id"
	case ErrSpiceLinkChannelNotAvailable:
		return "spice: channel not available"
	default:
		return fmt.Sprintf("SpiceError(%d)", e)
	}
}
