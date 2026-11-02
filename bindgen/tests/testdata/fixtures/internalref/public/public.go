package public

import (
	"github.com/ivov/lisette/bindgen/tests/testdata/fixtures/internalref/internal/inner"
	"github.com/ivov/lisette/bindgen/tests/testdata/fixtures/internalref/internal/other"
)

type Content struct{ Value int }

func (c *Content) AttachContent(v *inner.Thing) { c.Value = v.Value }
func (c *Content) Current() *inner.Thing        { return nil }
func (c *Content) Label() string                { return "public" }

type WrongContent struct{}

func (WrongContent) AttachContent(*other.Thing) {}
