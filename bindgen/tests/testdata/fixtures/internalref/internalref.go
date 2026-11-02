package internalref

import "github.com/ivov/lisette/bindgen/tests/testdata/fixtures/internalref/internal/inner"

// Should be skipped: function returning a type from an internal package.
func GetInner() *inner.Thing { return nil }

// Should be skipped: function whose parameter is a type from an internal package.
func ConsumeInner(t *inner.Thing) {}

// Package-level variable typed by an internal package — type collapses to
// Unknown so the symbol stays referenceable as an opaque value.
var GlobalInner inner.Thing

// Field referencing an internal package is dropped, leaving a struct
// that still emits its remaining exported fields.
type Holder struct {
	Name  string
	Inner *inner.Thing
}

// Should be unaffected by the internal-package check.
type Local struct {
	Value int
}

type Consumer interface {
	AttachContent(*inner.Thing)
}

type Producer interface {
	Current() *inner.Thing
}

type Content interface {
	Consumer
	Producer
	Label() string
}

type ValueContent struct{}

func (ValueContent) AttachContent(*inner.Thing) {}
func (ValueContent) Current() *inner.Thing      { return nil }
func (ValueContent) Label() string              { return "value" }

type PointerContent struct {
	Value int
}

func (c *PointerContent) AttachContent(v *inner.Thing) { c.Value = v.Value }
func (c *PointerContent) Current() *inner.Thing        { return nil }
func (c *PointerContent) Label() string                { return "pointer" }

type GenericConsumer[T any] interface {
	AttachContent(*inner.Thing, T)
}

type GenericContent[T any] struct{}

func (GenericContent[T]) AttachContent(*inner.Thing, T) {}
