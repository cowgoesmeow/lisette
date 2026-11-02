package convert

import (
	"go/token"
	"go/types"
	"testing"

	"github.com/ivov/lisette/bindgen/internal/extract"
)

func hiddenTestSignature(params, results []types.Type, variadic bool) *types.Signature {
	tuple := func(ts []types.Type) *types.Tuple {
		vars := make([]*types.Var, len(ts))
		for i, typ := range ts {
			vars[i] = types.NewVar(token.NoPos, nil, "", typ)
		}
		return types.NewTuple(vars...)
	}
	return types.NewSignatureType(nil, nil, nil, tuple(params), tuple(results), variadic)
}

func (c *Converter) hiddenSignatureEligible(sig *types.Signature) bool {
	_, skip := c.convertParams(sig, nil, "Content.M", "M", nil, false, nil)
	result := ReturnsToLisette(sig, c, nil, "Content.M")
	return hiddenSignatureEligible(sig, skip, result.SkipReason)
}

func TestHiddenMethodIdentity(t *testing.T) {
	pkg := types.NewPackage("example.com/a/internal/inner", "inner")
	other := types.NewPackage("example.com/b/internal/inner", "inner")
	named := func(pkg *types.Package) *types.Named {
		return types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Conn", nil), types.NewStruct(nil, nil), nil)
	}
	conn := named(pkg)
	alias := types.NewAlias(types.NewTypeName(token.NoPos, pkg, "Alias", nil), conn)
	sig := hiddenTestSignature([]types.Type{types.NewPointer(conn)}, nil, false)
	want := "AttachContent(*example.com/a/internal/inner.Conn)"
	if got := sealIdentity("example.com/api", "AttachContent", sig); got != want {
		t.Fatalf("identity = %q, want %q", got, want)
	}
	if sealIdentity("example.com/impl", "AttachContent", sig) != want {
		t.Fatal("exported method identity depends on declaring package")
	}
	if sealIdentity("example.com/api", "attachContent", sig) == sealIdentity("example.com/impl", "attachContent", sig) {
		t.Fatal("unexported method identity lost declaring package")
	}
	if got := sealIdentity("example.com/api", "attachContent", sig); got != "example.com/api.attachContent(*example.com/a/internal/inner.Conn)" {
		t.Fatalf("legacy unexported identity changed: %q", got)
	}
	for _, typ := range []types.Type{alias, types.NewPointer(alias), types.NewSlice(types.NewPointer(alias))} {
		canonical, ok := hiddenIdentityType(typ)
		if !ok {
			t.Fatalf("alias rejected: %v", typ)
		}
		if sealIdentity("a", "M", hiddenTestSignature([]types.Type{typ}, nil, false)) != sealIdentity("b", "M", hiddenTestSignature([]types.Type{canonical}, nil, false)) {
			t.Fatalf("alias identity differs: %v", typ)
		}
	}
	for _, different := range []*types.Signature{
		hiddenTestSignature([]types.Type{types.NewPointer(named(other))}, nil, false),
		hiddenTestSignature(nil, []types.Type{types.NewPointer(conn)}, false),
		hiddenTestSignature([]types.Type{conn}, nil, false),
	} {
		if sealIdentity("api", "AttachContent", different) == want {
			t.Fatalf("distinct signature matched: %v", different)
		}
	}
	slice := []types.Type{types.NewSlice(conn)}
	if sealIdentity("a", "M", hiddenTestSignature(slice, nil, true)) == sealIdentity("b", "M", hiddenTestSignature(slice, nil, false)) {
		t.Fatal("variadic identity lost")
	}
}

func TestHiddenSignatureEligibility(t *testing.T) {
	pkg := types.NewPackage("example.com/api/internal/inner", "inner")
	conn := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Conn", nil), types.NewStruct(nil, nil), nil)
	param := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "T", nil), types.NewInterfaceType(nil, nil).Complete())
	box := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Box", nil), types.NewStruct(nil, nil), nil)
	box.SetTypeParams([]*types.TypeParam{param})
	generic, err := types.Instantiate(nil, box, []types.Type{param}, false)
	if err != nil {
		t.Fatal(err)
	}
	c := NewConverter("example.com/api", nil, nil, nil, nil, nil)
	for _, sig := range []*types.Signature{
		hiddenTestSignature([]types.Type{types.NewPointer(conn)}, nil, false),
		hiddenTestSignature(nil, []types.Type{types.NewPointer(conn)}, false),
		hiddenTestSignature([]types.Type{types.NewPointer(conn)}, []types.Type{types.NewPointer(conn)}, false),
	} {
		if !c.hiddenSignatureEligible(sig) {
			t.Fatalf("internal signature rejected: %v", sig)
		}
	}
	for _, typ := range []types.Type{
		param, types.NewPointer(param), types.NewSlice(param), types.NewArray(param, 1),
		types.NewMap(types.Typ[types.String], param), types.NewChan(types.SendRecv, param), generic,
		hiddenTestSignature(nil, []types.Type{param}, false),
		types.NewStruct(nil, nil), types.NewInterfaceType(nil, nil).Complete(),
	} {
		if _, ok := hiddenIdentityType(typ); ok {
			t.Fatalf("unsafe identity accepted: %v", typ)
		}
		if c.hiddenSignatureEligible(hiddenTestSignature([]types.Type{types.NewPointer(conn), typ}, nil, false)) {
			t.Fatalf("mixed signature accepted: %v", typ)
		}
	}
	if c.hiddenSignatureEligible(hiddenTestSignature([]types.Type{types.Typ[types.Int]}, nil, false)) {
		t.Fatal("ordinary signature hidden")
	}
}

func TestHiddenInterfaceRequirement(t *testing.T) {
	pkg := types.NewPackage("example.com/api", "api")
	inner := types.NewPackage("example.com/api/internal/inner", "inner")
	conn := types.NewNamed(types.NewTypeName(token.NoPos, inner, "Conn", nil), types.NewStruct(nil, nil), nil)
	sig := hiddenTestSignature([]types.Type{types.NewPointer(conn)}, []types.Type{types.NewPointer(conn)}, false)
	iface := types.NewInterfaceType([]*types.Func{
		types.NewFunc(token.NoPos, pkg, "AttachContent", sig),
	}, nil).Complete()
	c := NewConverter(pkg.Path(), nil, nil, nil, nil, nil)
	methods, ok := c.extractInterfaceMethods(iface, "Content")
	if !ok || len(methods) != 1 {
		t.Fatalf("hidden interface conversion = %v, %v", methods, ok)
	}
	method := methods[0]
	if method.SealId != "AttachContent(*example.com/api/internal/inner.Conn) *example.com/api/internal/inner.Conn" ||
		len(method.Params) != 0 || method.ReturnType != "" || method.CommaOk {
		t.Fatalf("unexpected hidden requirement: %+v", method)
	}
	union := types.NewUnion([]*types.Term{types.NewTerm(true, types.Typ[types.Int])})
	for _, mixed := range []*types.Signature{
		hiddenTestSignature([]types.Type{types.NewPointer(conn), union}, nil, false),
		hiddenTestSignature([]types.Type{union, types.NewPointer(conn)}, nil, false),
		hiddenTestSignature([]types.Type{types.NewPointer(conn)}, []types.Type{union}, false),
		hiddenTestSignature(nil, []types.Type{types.NewPointer(conn), union}, false),
	} {
		if c.hiddenSignatureEligible(mixed) {
			t.Fatalf("unrelated conversion failure hidden: %v", mixed)
		}
	}
}

func TestHiddenConcreteMethod(t *testing.T) {
	pkg := types.NewPackage("example.com/impl", "impl")
	inner := types.NewPackage("example.com/api/internal/inner", "inner")
	conn := types.NewNamed(types.NewTypeName(token.NoPos, inner, "Conn", nil), types.NewStruct(nil, nil), nil)
	base := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Content", nil), types.NewStruct(nil, nil), nil)
	for _, pointer := range []bool{false, true} {
		var recvType types.Type = base
		if pointer {
			recvType = types.NewPointer(base)
		}
		recv := types.NewVar(token.NoPos, pkg, "c", recvType)
		for _, sig := range []*types.Signature{
			hiddenTestSignature([]types.Type{types.NewPointer(conn)}, nil, false),
			hiddenTestSignature(nil, []types.Type{types.NewPointer(conn)}, false),
			hiddenTestSignature([]types.Type{types.NewPointer(conn)}, []types.Type{types.NewPointer(conn)}, false),
		} {
			c := NewConverter(pkg.Path(), nil, nil, nil, nil, nil)
			result := ConvertResult{Name: "AttachContent"}
			c.convertMethod(&result, extract.SymbolExport{
				GoType:           sig,
				Obj:              types.NewFunc(token.NoPos, pkg, "AttachContent", sig),
				ReceiverVariable: recv,
				BaseType:         base,
				IsPromoted:       true,
			})
			if result.SkipReason != nil || result.SealId != sealIdentity(pkg.Path(), "AttachContent", sig) {
				t.Fatalf("concrete evidence missing: %+v", result)
			}
			if len(result.Params) != 0 || result.ReturnType != "" || result.CommaOk || result.SkipNote != nil {
				t.Fatalf("callable signature leaked: %+v", result)
			}
			if result.Receiver == nil || result.Receiver.IsPointer != pointer || result.Receiver.BaseTypeName != "Content" {
				t.Fatalf("receiver changed: %+v", result.Receiver)
			}
		}
	}
}

func TestHiddenCallbackIdentityIgnoresNames(t *testing.T) {
	pkg := types.NewPackage("example.com/api/internal/inner", "inner")
	conn := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Conn", nil), types.NewStruct(nil, nil), nil)
	callback := func(paramName, resultName string) *types.Signature {
		return types.NewSignatureType(nil, nil, nil,
			types.NewTuple(types.NewVar(token.NoPos, pkg, paramName, types.NewPointer(conn))),
			types.NewTuple(types.NewVar(token.NoPos, pkg, resultName, types.NewPointer(conn))), false)
	}
	left, right := callback("x", "result"), callback("y", "other")
	if !types.Identical(left, right) {
		t.Fatal("callback fixture signatures differ")
	}
	for _, result := range []bool{false, true} {
		signature := func(callback *types.Signature) *types.Signature {
			if result {
				return hiddenTestSignature(nil, []types.Type{callback}, false)
			}
			return hiddenTestSignature([]types.Type{callback}, nil, false)
		}
		if sealIdentity("api", "M", signature(left)) != sealIdentity("impl", "M", signature(right)) {
			t.Fatal("callback variable names changed hidden identity")
		}
	}
}

func TestHiddenBasicAliasIdentity(t *testing.T) {
	for _, pair := range [][2]types.Type{
		{types.Universe.Lookup("byte").Type(), types.Typ[types.Uint8]},
		{types.Universe.Lookup("rune").Type(), types.Typ[types.Int32]},
	} {
		for _, wrap := range []func(types.Type) types.Type{
			func(t types.Type) types.Type { return t },
			func(t types.Type) types.Type { return types.NewSlice(types.NewPointer(t)) },
			func(t types.Type) types.Type { return types.NewMap(t, types.NewArray(t, 2)) },
			func(t types.Type) types.Type { return types.NewChan(types.SendRecv, t) },
			func(t types.Type) types.Type { return hiddenTestSignature([]types.Type{t}, []types.Type{t}, false) },
		} {
			left := hiddenTestSignature([]types.Type{wrap(pair[0])}, []types.Type{wrap(pair[0])}, false)
			right := hiddenTestSignature([]types.Type{wrap(pair[1])}, []types.Type{wrap(pair[1])}, false)
			if !types.Identical(left, right) {
				t.Fatal("alias fixture signatures differ")
			}
			if sealIdentity("api", "M", left) != sealIdentity("impl", "M", right) {
				t.Fatalf("basic alias identity differs: %v and %v", left, right)
			}
		}
		left := hiddenTestSignature([]types.Type{pair[0]}, nil, false)
		right := hiddenTestSignature([]types.Type{pair[1]}, nil, false)
		if sealIdentity("api", "m", left) == sealIdentity("api", "m", right) {
			t.Fatal("legacy unexported basic alias spelling changed")
		}
	}
}

func TestInternalErrorReturnRemainsCallable(t *testing.T) {
	pkg := types.NewPackage("example.com/api", "api")
	inner := types.NewPackage("example.com/api/internal/inner", "inner")
	failure := types.NewNamed(types.NewTypeName(token.NoPos, inner, "Failure", nil), types.NewStruct(nil, nil), nil)
	errorSig := types.NewSignatureType(types.NewVar(token.NoPos, inner, "f", types.NewPointer(failure)), nil, nil,
		types.NewTuple(), types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.String])), false)
	failure.AddMethod(types.NewFunc(token.NoPos, inner, "Error", errorSig))
	base := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Service", nil), types.NewStruct(nil, nil), nil)
	sig := hiddenTestSignature(nil, []types.Type{types.NewPointer(failure)}, false)
	method := types.NewFunc(token.NoPos, pkg, "Err", sig)
	c := NewConverter(pkg.Path(), nil, nil, nil, nil, nil)
	result := ConvertResult{Name: "Err"}
	c.convertMethod(&result, extract.SymbolExport{
		GoType: sig, Obj: method, BaseType: base, IsPromoted: true,
		ReceiverVariable: types.NewVar(token.NoPos, pkg, "s", base),
	})
	if result.SkipReason != nil || result.SkipNote != nil || result.SealId != "" || result.ReturnType != "error" {
		t.Fatalf("concrete error result lost callable conversion: %+v", result)
	}
	iface := types.NewInterfaceType([]*types.Func{method}, nil).Complete()
	methods, ok := c.extractInterfaceMethods(iface, "Errors")
	if !ok || len(methods) != 1 || methods[0].SealId != "" || methods[0].ReturnType != "error" {
		t.Fatalf("interface error result lost callable conversion: %+v, %v", methods, ok)
	}
	conn := types.NewNamed(types.NewTypeName(token.NoPos, inner, "Conn", nil), types.NewStruct(nil, nil), nil)
	if !c.hiddenSignatureEligible(hiddenTestSignature([]types.Type{types.NewPointer(conn)}, []types.Type{types.NewPointer(failure)}, false)) {
		t.Fatal("normal error result blocked an internal parameter fallback")
	}
}

func TestHiddenFallbackRejectsMixedConversionFailures(t *testing.T) {
	pkg := types.NewPackage("example.com/api", "api")
	inner := types.NewPackage("example.com/api/internal/inner", "inner")
	conn := types.NewNamed(types.NewTypeName(token.NoPos, inner, "Conn", nil), types.NewStruct(nil, nil), nil)
	base := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Service", nil), types.NewStruct(nil, nil), nil)
	union := types.NewUnion([]*types.Term{types.NewTerm(true, types.Typ[types.Int])})
	internal := types.NewPointer(conn)
	for _, sig := range []*types.Signature{
		hiddenTestSignature([]types.Type{internal, union}, nil, false),
		hiddenTestSignature([]types.Type{union, internal}, nil, false),
		hiddenTestSignature([]types.Type{internal}, []types.Type{union}, false),
		hiddenTestSignature([]types.Type{union}, []types.Type{internal}, false),
		hiddenTestSignature(nil, []types.Type{internal, union}, false),
		hiddenTestSignature(nil, []types.Type{union, internal}, false),
	} {
		c := NewConverter(pkg.Path(), nil, nil, nil, nil, nil)
		method := types.NewFunc(token.NoPos, pkg, "M", sig)
		result := ConvertResult{Name: "M"}
		c.convertMethod(&result, extract.SymbolExport{
			GoType: sig, Obj: method, BaseType: base, IsPromoted: true,
			ReceiverVariable: types.NewVar(token.NoPos, pkg, "s", base),
		})
		if result.SealId != "" {
			t.Fatalf("mixed concrete failure hidden: %v", sig)
		}
		iface := types.NewInterfaceType([]*types.Func{method}, nil).Complete()
		if methods, ok := c.extractInterfaceMethods(iface, "Mixed"); ok {
			t.Fatalf("mixed interface failure hidden: %v, %+v", sig, methods)
		}
	}
}

func TestHiddenFallbackRejectsNestedCallbackFailures(t *testing.T) {
	pkg := types.NewPackage("example.com/api", "api")
	inner := types.NewPackage("example.com/api/internal/inner", "inner")
	conn := types.NewNamed(types.NewTypeName(token.NoPos, inner, "Conn", nil), types.NewStruct(nil, nil), nil)
	internal := types.NewPointer(conn)
	sixInts := []types.Type{types.Typ[types.Int], types.Typ[types.Int], types.Typ[types.Int], types.Typ[types.Int], types.Typ[types.Int], types.Typ[types.Int]}
	unsupported := hiddenTestSignature(nil, sixInts, false)
	box := func(owner *types.Package, args ...types.Type) types.Type {
		t.Helper()
		named := types.NewNamed(types.NewTypeName(token.NoPos, owner, "Box", nil), types.NewStruct(nil, nil), nil)
		params := make([]*types.TypeParam, len(args))
		for i := range params {
			params[i] = types.NewTypeParam(types.NewTypeName(token.NoPos, nil, string(rune('A'+i)), nil), types.NewInterfaceType(nil, nil).Complete())
		}
		named.SetTypeParams(params)
		instance, err := types.Instantiate(nil, named, args, true)
		if err != nil {
			t.Fatal(err)
		}
		return instance
	}
	for name, typ := range map[string]types.Type{
		"callback tuple":                  hiddenTestSignature([]types.Type{internal}, sixInts, false),
		"callback internal first":         hiddenTestSignature([]types.Type{internal, unsupported}, nil, false),
		"callback internal last":          hiddenTestSignature([]types.Type{unsupported, internal}, nil, false),
		"callback result":                 hiddenTestSignature([]types.Type{internal}, []types.Type{unsupported}, false),
		"callback results internal first": hiddenTestSignature(nil, []types.Type{internal, unsupported}, false),
		"callback results internal last":  hiddenTestSignature(nil, []types.Type{unsupported, internal}, false),
		"map key":                         types.NewMap(internal, unsupported),
		"map value":                       types.NewMap(types.NewPointer(unsupported), internal),
		"named args internal first":       box(pkg, internal, unsupported),
		"named args internal last":        box(pkg, unsupported, internal),
		"internal named arg":              box(inner, unsupported),
	} {
		t.Run(name, func(t *testing.T) {
			c := NewConverter(pkg.Path(), nil, nil, nil, nil, nil)
			for _, convert := range []func(types.Type, *Converter) TypeResult{ToLisette, ToLisetteNilable} {
				result := convert(typ, c)
				if result.SkipReason == nil || result.SkipReason.Code != "tuple-too-large" {
					t.Fatalf("nested failure = %+v, want tuple-too-large", result)
				}
			}
			for _, sig := range []*types.Signature{
				hiddenTestSignature([]types.Type{typ}, nil, false),
				hiddenTestSignature(nil, []types.Type{typ}, false),
			} {
				if c.hiddenSignatureEligible(sig) {
					t.Fatalf("nested failure hidden: %v", sig)
				}
				method := types.NewFunc(token.NoPos, pkg, "M", sig)
				base := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Service", nil), types.NewStruct(nil, nil), nil)
				result := ConvertResult{Name: "M"}
				c.convertMethod(&result, extract.SymbolExport{
					GoType: sig, Obj: method, BaseType: base, IsPromoted: true,
					ReceiverVariable: types.NewVar(token.NoPos, pkg, "s", base),
				})
				if result.SealId != "" {
					t.Fatalf("nested concrete failure hidden: %+v", result)
				}
				iface := types.NewInterfaceType([]*types.Func{method}, nil).Complete()
				if methods, ok := c.extractInterfaceMethods(iface, "Mixed"); ok {
					t.Fatalf("nested interface failure hidden: %+v", methods)
				}
			}
		})
	}
	c := NewConverter(pkg.Path(), nil, nil, nil, nil, nil)
	for _, callback := range []*types.Signature{
		hiddenTestSignature([]types.Type{internal}, []types.Type{types.Typ[types.Int]}, false),
		hiddenTestSignature([]types.Type{types.Typ[types.Int]}, []types.Type{internal}, false),
		hiddenTestSignature([]types.Type{internal}, []types.Type{internal}, false),
	} {
		for _, sig := range []*types.Signature{
			hiddenTestSignature([]types.Type{callback}, nil, false),
			hiddenTestSignature(nil, []types.Type{callback}, false),
		} {
			if !c.hiddenSignatureEligible(sig) {
				t.Fatalf("internal-only callback rejected: %v", sig)
			}
		}
	}
}
