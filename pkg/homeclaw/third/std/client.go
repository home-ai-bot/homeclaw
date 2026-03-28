// Package std defines the standard interface that all third-party smart-home
// platform clients (e.g. Xiaomi/Mi Home, Tuya) must implement.
//
// Every platform adapter must:
//  1. Implement [Client] and return it from a [Factory] function.
//  2. Report its brand name via [Client.Brand].
//  3. Provide the four data-query methods (Home / Room / Device / Spec).
//  4. Provide two single-device control methods (Execute / GetProp).
//  5. Provide two event-subscription lifecycle methods (EnableEvent / DisableEvent).
package std

import (
	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
)

// ─────────────────────────────────────────────────────────────────────────────
// Data types returned by the query methods
// ─────────────────────────────────────────────────────────────────────────────

// HomeInfo describes a single home (家庭) on the platform.
type HomeInfo struct {
	// ID is the platform-specific home identifier.
	ID string `json:"id"`

	// Name is the human-readable home name.
	Name string `json:"name"`
}

// SpecInfo holds the capability specification of a device model.
// For Xiaomi this is the MIoT Spec JSON; for Tuya it would be the
// equivalent schema.  The raw document is stored as a string so that
// callers can unmarshal it according to platform-specific types.
type SpecInfo struct {
	// DeviceID is the platform-specific device identifier the spec belongs to.
	DeviceID string `json:"device_id"`

	// Model is the device model / URN used to fetch the spec.
	Model string `json:"model"`

	// Raw is the platform-native spec document (JSON string).
	Raw string `json:"raw"`

	// Extra carries any additional metadata.
	Extra map[string]any `json:"extra,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Client interface
// ─────────────────────────────────────────────────────────────────────────────

// Client is the unified interface that every third-party platform adapter must
// implement.
//
// Method groups:
//
//   - Identity  – Brand()
//   - Query     – GetHomes / GetRooms / GetDevices / GetSpec
//   - Control   – Execute / GetProp
//   - Events    – EnableEvent / DisableEvent
type Client interface {
	// ── 1. Identity ──────────────────────────────────────────────────────────

	// Brand returns the human-readable brand name for this adapter.
	// Use the package-level constants [BrandXiaomi] / [BrandTuya] (or define
	// your own) so that callers can distinguish implementations at runtime.
	//
	// Example return values: "xiaomi", "tuya"
	Brand() string

	// ── 2. Query methods ─────────────────────────────────────────────────────

	// GetHomes returns all homes (家庭) visible to the authenticated user.
	// The returned slice must be non-nil; return an empty slice when there are
	// no homes rather than nil.
	GetHomes() ([]*HomeInfo, error)

	// GetRooms returns all rooms for the given homeID.
	// Passing an empty homeID should return rooms across all homes, or an
	// error if the platform does not support that operation.
	GetRooms(homeID string) ([]*data.Space, error)

	// GetDevices returns all devices for the given homeID.
	// Passing an empty homeID should return devices across all homes, or an
	// error if the platform does not support that operation.
	GetDevices(homeID string) ([]*data.Device, error)

	// GetSpec fetches the capability specification for deviceID.
	// deviceID is the platform-native device identifier returned by
	// [GetDevices].
	GetSpec(deviceID string) (*SpecInfo, error)

	// ── 3. Single-device control ─────────────────────────────────────────────

	// Execute sends an action command to a device.
	//
	// Parameters:
	//   - deviceID: platform-native device identifier.
	//   - action:   action name or identifier (platform-specific).
	//   - params:   key-value pairs passed to the action.
	//
	// Returns the platform response as an opaque map, or an error.
	Execute(params map[string]any) (map[string]any, error)

	// GetProp reads a single property value from a device.
	//
	// Parameters:
	//   - deviceID: platform-native device identifier.
	//   - prop:     property name or identifier (platform-specific).
	//
	// Returns the property value (any JSON-compatible type) or an error.
	GetProps(params map[string]any) (any, error)
	SetProps(params map[string]any) (any, error)
	// ── 4. Event lifecycle ───────────────────────────────────────────────────

	// EnableEvent starts the adapter's event subscription for the given
	// deviceID and registers handler as the callback.  Calls to handler are
	// dispatched with a populated [event.Event] whenever the platform reports
	// a state change or push notification.
	//
	// Implementations must be idempotent: calling EnableEvent on a deviceID
	// that is already subscribed should be a no-op (or refresh the subscription).
	EnableEvent(params map[string]any) error

	// DisableEvent stops event subscription for the given deviceID and
	// deregisters any previously registered handler.
	//
	// Implementations must be idempotent: calling DisableEvent on a deviceID
	// that is not subscribed should return nil without error.
	DisableEvent(params map[string]any) error
}
