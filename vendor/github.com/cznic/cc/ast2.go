// Copyright 2016 The CC Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cc

import (
	"fmt"
	"go/token"

	"github.com/cznic/mathutil"
	"github.com/cznic/xc"
)

type operand interface {
	eval(*lexer) (value interface{}, typ Type)
	Node
}

// Node represents an AST node.
type Node interface {
	Pos() token.Pos
}

// ---------------------------------------------------------- CompoundStatement

// Scope returns n's scope.
func (n *CompoundStatement) Scope() *Bindings { return n.scope }

// ---------------------------------------------------------------- Declaration

// Declarator returns a synthetic Declarator when n.InitDeclaratorListOpt is
// nil.
func (n *Declaration) Declarator() *Declarator { return n.declarator }

// ------------------------------------------------------ DeclarationSpecifiers

// IsInline implements specifier.
func (n *DeclarationSpecifiers) IsInline() bool {
	return n.attr&saInline != 0
}

// IsTypedef implements specifier.
func (n *DeclarationSpecifiers) IsTypedef() bool {
	return n.attr&saTypedef != 0
}

// IsExtern implements specifier.
func (n *DeclarationSpecifiers) IsExtern() bool {
	return n.attr&saExtern != 0
}

// IsStatic implements specifier.
func (n *DeclarationSpecifiers) IsStatic() bool {
	return n.attr&saStatic != 0
}

// IsAuto implements specifier.
func (n *DeclarationSpecifiers) IsAuto() bool {
	return n.attr&saAuto != 0
}

// IsRegister implements specifier.
func (n *DeclarationSpecifiers) IsRegister() bool {
	return n.attr&saRegister != 0
}

// IsConst returns whether n includes the 'const' type qualifier.
func (n *DeclarationSpecifiers) IsConst() bool {
	return n.attr&saConst != 0
}

// IsRestrict implements specifier.
func (n *DeclarationSpecifiers) IsRestrict() bool {
	return n.attr&saRestrict != 0
}

// IsVolatile implements specifier.
func (n *DeclarationSpecifiers) IsVolatile() bool {
	return n.attr&saVolatile != 0
}

// kind implements specifier.
func (n *DeclarationSpecifiers) kind() Kind { return tsValid[n.typeSpecifiers()] }

// typeSpecifiers implements specifier.
func (n *DeclarationSpecifiers) typeSpecifiers() int {
	return n.typeSpecifier
}

// firstTypeSpecifier implements specifier.
func (n *DeclarationSpecifiers) firstTypeSpecifier() *TypeSpecifier {
	for n.Case != 1 { // TypeSpecifier DeclarationSpecifiersOpt
		o := n.DeclarationSpecifiersOpt
		if o == nil {
			return nil
		}

		n = o.DeclarationSpecifiers
	}
	return n.TypeSpecifier
}

// attrs implements specifier.
func (n *DeclarationSpecifiers) attrs() int { return n.attr }

// member implements specifier.
func (n *DeclarationSpecifiers) member(nm int) (*Member, error) {
	return n.firstTypeSpecifier().member(nm)
}

// str implements specifier.
func (n *DeclarationSpecifiers) str() string {
	return specifierString(n)
}

// TypedefName implements Specifier.
func (n *DeclarationSpecifiers) TypedefName() int {
	if n.kind() == TypedefName {
		return n.firstTypeSpecifier().Token.Val
	}
	return 0
}

// ----------------------------------------------------------------- Declarator

// Identifier returns the ID of the name declared by n and the scope the name
// is declared in.
func (n *Declarator) Identifier() (int, *Bindings) {
	dd := n.DirectDeclarator.bottom()
	if dd != nil {
		return dd.Token.Val, dd.DeclarationScope()
	}

	return 0, nil
}

// RawSpecifier returns the raw Specifier associated with n before expanding
// typedefs. The effective Specifier is accessible via the Type field of n.
func (n *Declarator) RawSpecifier() Specifier { return n.specifier }

func (n *Declarator) clone() *Declarator {
	m := *n
	return &m
}

func (n *Declarator) stars() int { return n.PointerOpt.stars() }

func (n *Declarator) isCompatible(m *Declarator) (r bool) {
	return n == m || n.Type.(*ctype).isCompatible(m.Type.(*ctype))
}

func (n *Declarator) setFull(lx *lexer) Type {
	//dbg("==== setFull(%p) %v", lx, position(n.Pos()))
	d := n
	var dds0, dds []*DirectDeclarator
	for dd := d.DirectDeclarator; dd != nil; dd = dd.directDeclarator() {
		dds = append(dds, dd)
	}
	for i, j := 0, len(dds)-1; i < j; i, j = i+1, j-1 { // reverse
		dds[i], dds[j] = dds[j], dds[i]
	}

	resultAttr := 0
	mask := 0
	if d.specifier != nil && d.specifier.IsTypedef() {
		dds0 = append([]*DirectDeclarator(nil), dds...)
	}
loop0:
	for d.specifier != nil {
		switch d.specifier.kind() {
		case TypedefName:
			resultAttr |= d.specifier.attrs()
			ts := d.specifier.firstTypeSpecifier()
			dd := ts.scope.Lookup(NSIdentifiers, ts.Token.Val).Node.(*DirectDeclarator) // eg. typedef T dd, (*dd), dd(int), ...
			if dd.Case != 0 {                                                           // IDENTIFIER
				panic("internal error")
			}

			nd := dd.top().declarator
			mask = saTypedef // nd.specifier.IsTypedef() == true
			dds2 := nd.Type.(*ctype).dds0
			d2 := d.clone()
			d2.specifier = nil
			dd2 := &DirectDeclarator{
				Case:       1, //  '(' Declarator ')'
				Declarator: d2,
			}
			dds = append(dds, dd2)
			dds = append(dds, dds2[1:]...)
			d = nd
		case typeof:
			resultAttr |= d.specifier.attrs()
			ts := d.specifier.firstTypeSpecifier()
			nd := ts.Type.Declarator()
			dds2 := ts.Type.(*ctype).dds0
			d2 := d.clone()
			d2.specifier = nil
			dd2 := &DirectDeclarator{
				Case:       1, //  '(' Declarator ')'
				Declarator: d2,
			}
			dds = append(dds, dd2)
			dds = append(dds, dds2...)
			d = nd
		default:
			break loop0
		}
	}

	// Inner ((...)) -> (...)
	for {
		changed := false
		w := 0
		for r := 0; r < len(dds); {
			dd := dds[r]
			if r == len(dds)-1 || dd.Case != 1 { // '(' Declarator ')'
				dds[w] = dd
				w++
				r++
				continue
			}

			dd2 := dds[r+1]
			if dd2.Case != 1 {
				dds[w] = dd
				w++
				r++
				continue
			}

			d := dd.Declarator
			d2 := dd2.Declarator
			switch s, s2 := d.stars(), d2.stars(); {
			case s == 0 && s2 == 0:
				dds[w] = dd
				w++
				r += 2
				changed = true
			case s == 0 && s2 != 0:
				dds[w] = dd2
				w++
				r += 2
				changed = true
			case s != 0 && s2 == 0:
				dds[w] = dd
				w++
				r += 2
				changed = true
			case s != 0 && s2 != 0:
				d2 := d2.clone()
				var p *Pointer
				for i := 0; i < s+s2; i++ {
					p = &Pointer{Pointer: p}
				}
				d2.PointerOpt = &PointerOpt{Pointer: p}
				dd2 := dd2.clone()
				dd2.Declarator = d2
				dds[w] = dd2
				w++
				r += 2
				changed = true
			}

		}
		dds = dds[:w]
		if !changed {
			break
		}
	}

	// Outer (...) -> ...
	for {
		i := len(dds) - 1
		if dd := dds[i]; dd.Case == 1 /* '(' Declarator ')' */ && dd.Declarator.stars() == 0 {
			dds = dds[:i:i]
			continue
		}

		break
	}
	resultStars := 0
	i := len(dds) - 1
	if dd := dds[i]; dd.Case == 1 /* '(' Declarator ')' */ {
		resultStars = dd.Declarator.stars()
		dds = dds[:i:i]
	}

	stars := 0
	resultStars += d.stars()
	switch {
	case len(dds) == 1:
		if dds[0].Case != 0 { // IDENTIFIER
			panic("internal error")
		}

		stars, resultStars = resultStars, 0
	default:
	again:
		i := 1
	loop:
		for {
			switch dd := dds[i]; dd.Case {
			case 1: // '(' Declarator ')'
				if dds[i-1].Case == 0 { // IDENTIFIER
					stars = dd.Declarator.stars()
					if stars == 0 {
						copy(dds[i:], dds[i+1:])
						dds = dds[:len(dds)-1 : len(dds)-1]
						goto again
					}
				} else {
					//dbg("", resultStars, stars, d.specifier.str(), ddsStr(dds))
					panic("TODO")
				}
				i++
			case
				2, // DirectDeclarator '[' TypeQualifierListOpt ExpressionOpt ']'
				6, // DirectDeclarator '(' ParameterTypeList ')'
				7: // DirectDeclarator '(' IdentifierListOpt ')'
				break loop
			default:
				//dbg("", position(n.Pos()), resultStars, stars, d.specifier.str(), ddsStr(dds))
				panic(dd.Case)
			}
		}
	}

	resultSpecifier := d.specifier
	resultAttr |= resultSpecifier.attrs()
	resultAttr &^= mask
	t := &ctype{
		dds0:            dds0,
		dds:             dds,
		model:           lx.model,
		resultAttr:      resultAttr,
		resultSpecifier: resultSpecifier,
		resultStars:     resultStars,
		stars:           stars,
	}
	n.Type = t
	//dbg("@@@@ %v: %s", position(n.Pos()), t.dds[0].Token.S())
	//dbg("setFull %v: %v, %v %v", t, t.Kind(), t.resultStars, t.stars)
	//dbg("", t.str())
	//dbg("----> %v", t)

	if lx.scope == nil {
		return t
	}

	// Determine linkage

	dd := dds[0]
	scs := resultAttr & (saTypedef | saExtern | saStatic | saAuto | saRegister)
	sk := lx.scope.kind
	var prev, prevVisible *Declarator
	var prevVisibleBinding *Binding
	id := dd.Token.Val
	if p := lx.scope.Parent; p != nil {
		b := p.Lookup(NSIdentifiers, id)
		if dd, ok := b.Node.(*DirectDeclarator); ok {
			prevVisible = dd.TopDeclarator()
			prevVisibleBinding = &b
		}
	}
	if b := dd.prev; b != nil {
		prev = b.Node.(*DirectDeclarator).TopDeclarator()
	}

	switch {
	case
		// [0]6.2.2, 6: The following identifiers have no linkage: an
		// identifier declared to be anything other than an object or a
		// function; an identifier declared to be a function parameter;
		// a block scope identifier for an object declared without the
		// storage-class specifier extern.
		resultAttr&saTypedef != 0,
		sk == ScopeParams,
		(sk == ScopeBlock || sk == ScopeMembers) && resultAttr&saExtern == 0:

		n.Linkage = None
	case
		// [0]6.2.2, 3: If the declaration of a file scope identifier
		// for an object or a function contains the storage-class
		// specifier static, the identifier has internal linkage.
		sk == ScopeFile && resultAttr&saStatic != 0:

		n.Linkage = Internal
	case
		// [0]6.2.2, 4: For an identifier declared with the
		// storage-class specifier extern in a scope in which a prior
		// declaration of that identifier is visible, if the prior
		// declaration specifies internal or external linkage, the
		// linkage of the identifier at the later declaration is the
		// same as the linkage specified at the prior declaration.

		resultAttr&saExtern != 0 &&
			(prev != nil && (prev.Linkage == Internal || prev.Linkage == External) ||
				prevVisible != nil && (prevVisible.Linkage == Internal || prevVisible.Linkage == External)):
		switch {
		case prev != nil && (prev.Linkage == Internal || prev.Linkage == External):
			n.Linkage = prev.Linkage
		default:
			n.Linkage = prevVisible.Linkage
			dd.visible = prevVisibleBinding
		}
	case
		// [0]6.2.2, 4: If no prior declaration is visible, or if the
		// prior declaration specifies no linkage, then the identifier
		// has external linkage.
		resultAttr&saExtern != 0 && (prev == nil || prev.Linkage == None):

		n.Linkage = External
	case
		// [0]6.2.2, 5: If the declaration of an identifier for a
		// function has no storage-class specifier, its linkage is
		// determined exactly as if it were declared with the
		// storage-class specifier extern.
		t.Kind() == Function && scs == 0,
		// [0]6.2.2, 5: If the declaration of an identifier for an
		// object has file scope and no storage-class specifier, its
		// linkage is external.
		t.Kind() != Function && sk == ScopeFile && scs == 0:

		n.Linkage = External
	}

	if isGenerating || id == 0 {
		//dbg("setFull done (A)(%p): %s: %s\n%v", lx.scope, position(n.Pos()), n, resultSpecifier)
		return t
	}

	if prev != nil && prev.specifier.IsTypedef() != n.specifier.IsTypedef() {
		lx.report.Err(n.Pos(),
			"redeclaration of %s as different kind of symbol, previous declaration at %v",
			xc.Dict.S(id), position(prev.Pos()))
		return t
	}

	switch n.Linkage {
	case External:
		// [0]6.2.2, 2: In the set of translation units and libraries
		// that constitutes an entire program, each declaration of a
		// particular identifier with external linkage denotes the same
		// object or function.
		if prev, ok := lx.externs[id]; ok && !n.isCompatible(prev) {
			t, isA := compositeType(prev.Type, n.Type)
			if t == nil {
				lx.report.Err(n.Pos(),
					"conflicting types for %s '%s' with external linkage, previous declaration at %s '%s'",
					xc.Dict.S(id), n.Type, position(prev.Pos()), prev.Type)
				break
			}

			if !isA {
				dd.prev.Node = n.DirectDeclarator.bottom()
			}
		}

		lx.externs[id] = n
	case Internal:
		// [0]6.2.2, 2: Within one translation unit, each declaration
		// of an identifier with internal linkage denotes the same
		// object or function.
		if prev != nil && !n.isCompatible(prev) {
			t, isA := compositeType(prev.Type, n.Type)
			if t == nil {
				lx.report.Err(n.Pos(),
					"conflicting types for %s '%s' with internal linkage, previous declaration at %s '%s'",
					xc.Dict.S(id), n.Type, position(prev.Pos()), prev.Type)
				break
			}

			if !isA {
				dd.prev.Node = n.DirectDeclarator.bottom()
			}
		}
	case None:
		// [0]6.2.2, 2: Each declaration of an identifier with no
		// linkage denotes a unique entity.
		if prev != nil {
			lx.report.Err(n.Pos(),
				"redeclaration of %s '%s' with no linkage, previous declaration at %v '%s'",
				xc.Dict.S(id), n.Type, position(prev.Pos()), prev.Type)
		}
	default:
		panic("internal error")
	}

	//dbg("setFull done: %s: %s", position(n.Pos()), n)
	return t
}

// ----------------------------------------------------------- DirectDeclarator

// DeclarationScope returns the scope a name declared by n is in. If n does not
// declare a name or n declares a name of a built in type, DeclarationScope
// returns nil.
func (n *DirectDeclarator) DeclarationScope() *Bindings {
	return n.idScope
}

// TopDeclarator returns the top level Declarator associated with n.
func (n *DirectDeclarator) TopDeclarator() *Declarator {
	return n.top().declarator
}

func (n *DirectDeclarator) top() *DirectDeclarator {
	for n.parent != nil {
		n = n.parent
	}
	return n
}

func (n *DirectDeclarator) bottom() *DirectDeclarator {
	for n.Case != 0 { // IDENTIFIER
		n = n.directDeclarator()
	}
	return n
}

func (n *DirectDeclarator) clone() *DirectDeclarator {
	m := *n
	return &m
}

func (n *DirectDeclarator) isCompatible(m *DirectDeclarator) (r bool) {
	if n == m {
		return true
	}

	if n.Case > m.Case {
		n, m = m, n
	}

	if n.Case != m.Case {
		if n.Case == 6 && m.Case == 7 {
			var b []Parameter
			if o := m.IdentifierListOpt; o != nil {
				b = o.params
			}
			return isCompatibleParameters(
				n.ParameterTypeList.params,
				b,
				n.ParameterTypeList.Case == 1, // ParameterList ',' "..."
				false,
			)
		}
	}

	switch n.Case {
	case 0: // IDENTIFIER
		return true
	case 1: // '(' Declarator ')'
		return true // Declarator checked before
	case 2: // DirectDeclarator '[' TypeQualifierListOpt ExpressionOpt ']'
		// [0]6.7.5.3 6: For two array types to be compatible, both
		// shall have compatible element types, and if both size
		// specifiers are present, and are integer constant
		// expressions, then both size specifiers shall have the same
		// constant value. If the two array types are used in a context
		// which requires them to be compatible, it is undefined
		// behavior if the two size specifiers evaluate to unequal
		// values.
		var nv, mv interface{}
		if o := n.ExpressionOpt; o != nil {
			nv = o.Expression.Value
		}
		if o := m.ExpressionOpt; o != nil {
			mv = o.Expression.Value
		}
		if nv != nil && mv != nil && nv != mv {
			return false
		}

		return true
	case 6: // DirectDeclarator '(' ParameterTypeList ')'
		return isCompatibleParameters(
			n.ParameterTypeList.params,
			m.ParameterTypeList.params,
			n.ParameterTypeList.Case == 1, // ParameterList ',' "..."
			m.ParameterTypeList.Case == 1, // ParameterList ',' "..."
		)
	case 7: // DirectDeclarator '(' IdentifierListOpt ')'
		var a, b []Parameter
		if o := n.IdentifierListOpt; o != nil {
			a = o.params
		}
		if o := m.IdentifierListOpt; o != nil {
			b = o.params
		}

		return isCompatibleParameters(a, b, false, false)
	default:
		panic(n.Case)
	}
}

func (n *DirectDeclarator) directDeclarator() *DirectDeclarator {
	switch n.Case {
	case 0: // IDENTIFIER
		return nil
	case 1: // '(' Declarator ')'
		return n.Declarator.DirectDeclarator
	case
		2, // DirectDeclarator '[' TypeQualifierListOpt ExpressionOpt ']'
		3, // DirectDeclarator '[' "static" TypeQualifierListOpt Expression ']'
		4, // DirectDeclarator '[' TypeQualifierList "static" Expression ']'
		5, // DirectDeclarator '[' TypeQualifierListOpt '*' ']'
		6, // DirectDeclarator '(' ParameterTypeList ')'
		7: // DirectDeclarator '(' IdentifierListOpt ')'
		return n.DirectDeclarator
	default:
		panic(n.Case)
	}
}

func (n *DirectDeclarator) isArray() bool {
	switch n.Case {
	case
		0, // IDENTIFIER
		1, // '(' Declarator ')'                                                 // Case 1
		6, // DirectDeclarator '(' ParameterTypeList ')'                         // Case 6
		7: // DirectDeclarator '(' IdentifierListOpt ')'                         // Case 7
		return false
	case
		2, // DirectDeclarator '[' TypeQualifierListOpt ExpressionOpt ']'        // Case 2
		3, // DirectDeclarator '[' "static" TypeQualifierListOpt Expression ']'  // Case 3
		4, // DirectDeclarator '[' TypeQualifierList "static" Expression ']'     // Case 4
		5: // DirectDeclarator '[' TypeQualifierListOpt '*' ']'                  // Case 5
		return true
	default:
		panic(n.Case)
	}
}

func (n *DirectDeclarator) isVLA() *Expression {
	switch dd := n.DirectDeclarator; dd.Case {
	case 0: // IDENTIFIER
		return nil
	case 1: // '(' Declarator ')'                                                 // Case 1
		//dbg("", n.TopDeclarator().Type, n.TopDeclarator().Type.Element(), n.TopDeclarator().Type.Element().Elements())
		d := n.TopDeclarator()
		if d.Type.Kind() == Ptr && d.Type.Element().Elements() >= 0 {
			return nil
		}

		panic("TODO")
	case 2: // DirectDeclarator '[' TypeQualifierListOpt ExpressionOpt ']'        // Case 2
		o := n.ExpressionOpt
		if o == nil || o.Expression.Value != nil {
			return nil
		}

		return o.Expression
	case 3: // DirectDeclarator '[' "static" TypeQualifierListOpt Expression ']'  // Case 3
		panic("TODO")
	case 4: // DirectDeclarator '[' TypeQualifierList "static" Expression ']'     // Case 4
		panic("TODO")
	case 5: // DirectDeclarator '[' TypeQualifierListOpt '*' ']'                  // Case 5
		panic("TODO")
	case 6: // DirectDeclarator '(' ParameterTypeList ')'                         // Case 6
		return nil
	case 7: // DirectDeclarator '(' IdentifierListOpt ')'                         // Case 7
		panic("TODO")
	default:
		panic("internal error")
	}
	panic("unreachable")
}

// ----------------------------------------------------------------- Expression

func (n *Expression) eval(lx *lexer) (interface{}, Type) {
	m := lx.model
	if n.Type != nil {
		return n.Value, n.Type
	}

	n.Type = undefined
	switch n.Case {
	case 0: // IDENTIFIER
		b := n.scope.Lookup(NSIdentifiers, n.Token.Val)
		if b.Node == nil {
			lx.report.ErrTok(n.Token, "undefined: %s", n.Token.S())
			break
		}

		dd := b.Node.(*DirectDeclarator)
		t := dd.top().declarator.Type
		if (t.Kind() == Ptr || t.Kind() == Array) && n.Type.Elements() == -1 {
			found := false
			dd := dd
		more:
			for dd.prev != nil {
				dd := dd.prev.Node.(*DirectDeclarator)
				if t2 := dd.TopDeclarator().Type; t2.Elements() >= 0 {
					t = t2
					found = true
					break
				}
			}
			if !found && dd.visible != nil {
				dd = dd.visible.Node.(*DirectDeclarator)
				if t2 := dd.TopDeclarator().Type; t2.Elements() >= 0 {
					t = t2
				} else {
					goto more
				}
			}
		}
		n.Type = t
		if v := dd.EnumVal; v != nil {
			n.Value = v
		}
	case 1: // CHARCONST
		n.Value, n.Type = m.charConst(n.Token)
	case 2: // FLOATCONST
		n.Value, n.Type = m.floatConst(lx, n.Token)
	case 3: // INTCONST
		n.Value, n.Type = m.intConst(lx, n.Token)
	case 4: // LONGCHARCONST
		n.Value, n.Type = m.charConst(n.Token)
	case 5: // LONGSTRINGLITERAL
		n.Value, n.Type = m.strConst(lx, n.Token)
	case 6: // STRINGLITERAL
		n.Value, n.Type = m.strConst(lx, n.Token)
	case 7: //  '(' ExpressionList ')'
		n.Value, n.Type = n.ExpressionList.eval(lx)
	case 8: // Expression '[' ExpressionList ']'
		_, t := n.Expression.eval(lx)
		if k := t.Kind(); k != Ptr && k != Array {
			lx.report.ErrTok(n.Token, "subscripted value is not a pointer (have '%s')", t)
			break
		}

		_, t2 := n.ExpressionList.eval(lx)
		n.Type = t.Element()
		if !IsIntType(t2) && t2.Kind() != Bool {
			lx.report.Err(n.ExpressionList.Pos(), "array subscript is not an integer or bool (have '%s')", t2)
			break
		}

		if p, x := n.Expression.Value, n.ExpressionList.Value; p != nil && x != nil {
			sz := uintptr(n.Type.SizeOf())
			switch pv := p.(type) {
			case uintptr:
				switch xv := x.(type) {
				case int32:
					pv += sz * uintptr(xv)
				case uint32:
					pv += sz * uintptr(xv)
				case int64:
					pv += sz * uintptr(xv)
				case uint64:
					pv += sz * uintptr(xv)
				case uintptr:
					pv += sz * xv
				default:
					panic("TODO")
				}
				n.Value = pv
			case StringLitID, LongStringLitID:
				// ok, but not a constant expression.
			default:
				panic("internal error")
			}
		}
	case 9: // Expression '(' ArgumentExpressionListOpt ')'
		_, t := n.Expression.eval(lx)
		if t.Kind() == Ptr {
			t = t.Element()
		}
		if t.Kind() != Function {
			lx.report.Err(n.Expression.Pos(), "called object is not a function or function pointer (have '%s')", t)
			break
		}

		n.Type = t.Result()
		params, isVariadic := t.Parameters()
		if params == nil {
			break // [0], 6.5.2.2/8
		}

		var args []*Expression
		var types []Type
		if o := n.ArgumentExpressionListOpt; o != nil {
			for l := o.ArgumentExpressionList; l != nil; l = l.ArgumentExpressionList {
				ex := l.Expression
				args = append(args, ex)
				_, t := ex.eval(lx)
				types = append(types, t)
			}
		}

		if g, e := len(args), len(params); g < e {
			lx.report.ErrTok(n.Token, "too few arguments to function (have %v, want %v)", g, e)
			break
		}

		if !isVariadic {
			if len(args) > len(params) && len(params) != 0 /* composite type */ {
				lx.report.Err(n.ArgumentExpressionListOpt.Pos(), "too many arguments to function")
				break
			}
		}

		for i, param := range params {
			pt := param.Type
			if pt.Kind() == Array {
				pt = pt.(*ctype).arrayDecay()
			}
			typ := types[i]
			if pt.Kind() == Function && typ.Kind() == Ptr && typ.Element().Kind() == Function {
				typ = typ.Element()
			}
			if !typ.CanAssignTo(pt) {
				lx.report.Err(args[i].Pos(), "expected '%s' but argument is of type '%s'", pt, typ)
			}
		}
	case 10: // Expression '.' IDENTIFIER
		_, t := n.Expression.eval(lx)
		mb, err := t.Member(n.Token2.Val)
		if err != nil {
			lx.report.Err(n.Token2.Pos(), "%v", err)
			break
		}

		n.Type = memberType(mb, lx.model)
	case 11: // Expression "->" IDENTIFIER
		v, t := n.Expression.eval(lx)
		if t.Kind() != Ptr && t.Kind() != Array {
			lx.report.ErrTok(n.Token2, "invalid type argument of -> (have '%v')", t)
			break
		}

		t = t.Element()
		mb, err := t.Member(n.Token2.Val)
		if err != nil {
			lx.report.Err(n.Token2.Pos(), "%v", err)
			break
		}

		n.Type = memberType(mb, lx.model)
		switch x := v.(type) {
		case nil:
			// nop
		case uintptr:
			n.Value = x + uintptr(mb.OffsetOf)
		default:
			panic("internal error")
		}
	case 12: // Expression "++"
		n.Value, n.Type = n.Expression.eval(lx)
	case 13: // Expression "--"
		n.Value, n.Type = n.Expression.eval(lx)
	case 14: // '(' TypeName ')' '{' InitializerList CommaOpt '}'
		n.Type = n.TypeName.Type
		n.InitializerList.typeCheck(&n.Type, n.Type, false, lx)
	case 15: // "++" Expression
		n.Value, n.Type = n.Expression.eval(lx)
	case 16: // "--" Expression
		n.Value, n.Type = n.Expression.eval(lx)
	case 17: // '&' Expression
		var t Type
		n.Value, t = n.Expression.eval(lx)
		n.Type = t.Pointer()
	case 18: // '*' Expression
		_, t := n.Expression.eval(lx)
		if t.Kind() == Function {
			n.Type = t
			break
		}

		if k := t.Kind(); k != Ptr && k != Array {
			lx.report.ErrTok(n.Token, "invalid argument type of unary * (have '%v')", t)
			break
		}

		n.Type = t.Element()
	case 19: // '+' Expression
		n.Value, n.Type = n.Expression.eval(lx)
	case 20: // '-' Expression
		v, t := n.Expression.eval(lx)
		n.Type = t
		switch x := v.(type) {
		case nil:
			// nop
		case int16:
			n.Value = -x
		case uint16:
			n.Value = -x
		case int32:
			n.Value = -x
		case uint32:
			n.Value = -x
		case uint64:
			n.Value = -x
		case int64:
			n.Value = -x
		case float32:
			n.Value = -x
		case float64:
			n.Value = -x
		default:
			panic(fmt.Errorf("internal error: %T", x))
		}
	case 21: // '~' Expression
		v, t := n.Expression.eval(lx)
		n.Type = t
		switch x := v.(type) {
		case nil:
			// nop
		case int32:
			n.Value = ^x
		case uint32:
			n.Value = ^x
		case int64:
			n.Value = ^x
		case uint64:
			n.Value = ^x
		default:
			panic(fmt.Errorf("internal error: %T", x))
		}
	case 22: // '!' Expression
		v, _ := n.Expression.eval(lx)
		n.Type = m.IntType
		if v == nil {
			break
		}

		n.Value = m.cBool(isZero(v))
	case 23: // "sizeof" Expression
		n.Type = m.getSizeType(lx)
		_, t := n.Expression.eval(lx)
		n.Value = m.MustConvert(int32(t.SizeOf()), n.Type)
	case 24: // "sizeof" '(' TypeName ')'
		n.Type = m.getSizeType(lx)
		n.Value = m.MustConvert(int32(n.TypeName.declarator.Type.SizeOf()), n.Type)
	case 25: // '(' TypeName ')' Expression
		v, _ := n.Expression.eval(lx)
		n.Type = n.TypeName.declarator.Type
		n.Value = v
		if v != nil && n.Type.Kind() != Struct && n.Type.Kind() != Union && !isStrLitID(v) {
			n.Value = m.MustConvert(v, n.Type)
		}
	case 26: // Expression '*' Expression
		var a, b interface{}
		a, b, n.Type = m.binOp(lx, n.Expression, n.Expression2)
		n.BinOpType = n.Type

		switch x := a.(type) {
		case nil:
			// nop
		case int32:
			n.Value = x * b.(int32)
		case uint32:
			n.Value = x * b.(uint32)
		case int64:
			n.Value = x * b.(int64)
		case uint64:
			n.Value = x * b.(uint64)
		case float32:
			n.Value = x * b.(float32)
		case float64:
			n.Value = x * b.(float64)
		default:
			panic(fmt.Errorf("internal error: %T", x))
		}
	case 27: // Expression '/' Expression
		var a, b interface{}
		a, b, n.Type = m.binOp(lx, n.Expression, n.Expression2)
		n.BinOpType = n.Type
		if b != nil && isZero(b) && IsIntType(n.Type) {
			lx.report.Err(n.Expression2.Pos(), "division by zero")
			break
		}

		switch x := a.(type) {
		case nil:
			// nop
		case int32:
			n.Value = x / b.(int32)
		case uint32:
			n.Value = x / b.(uint32)
		case int64:
			n.Value = x / b.(int64)
		case uint64:
			n.Value = x / b.(uint64)
		case float32:
			n.Value = x / b.(float32)
		case float64:
			n.Value = x / b.(float64)
		default:
			panic(fmt.Errorf("internal error: %T", x))
		}
	case 28: // Expression '%' Expression
		var a, b interface{}
		a, b, n.Type = m.binOp(lx, n.Expression, n.Expression2)
		n.BinOpType = n.Type
		if b != nil && isZero(b) && IsIntType(n.Type) {
			lx.report.Err(n.Expression2.Pos(), "division by zero")
			break
		}

		switch x := a.(type) {
		case nil:
			// nop
		case int32:
			n.Value = x % b.(int32)
		case uint32:
			n.Value = x % b.(uint32)
		case int64:
			n.Value = x % b.(int64)
		case uint64:
			n.Value = x % b.(uint64)
		default:
			panic(fmt.Errorf("internal error: %T", x))
		}
	case 29: // Expression '+' Expression
		_, at := n.Expression.eval(lx)
		_, bt := n.Expression2.eval(lx)
		if at.Kind() == Array {
			at = at.Element().Pointer()
		}
		if bt.Kind() == Array {
			bt = bt.Element().Pointer()
		}
		if at.Kind() > bt.Kind() {
			at, bt = bt, at
		}
		switch {
		case at.Kind() == Ptr:
			if IsIntType(bt) || bt.Kind() == Bool {
				n.Type = at
				break
			}

			lx.report.ErrTok(n.Token, "incompatible types ('%s' + '%s')", at, bt)
		case IsArithmeticType(at):
			fallthrough
		default:
			var a, b interface{}
			a, b, n.Type = m.binOp(lx, n.Expression, n.Expression2)
			n.BinOpType = n.Type
			switch x := a.(type) {
			case nil:
				// nop
			case int32:
				n.Value = x + b.(int32)
			case uint32:
				n.Value = x + b.(uint32)
			case int64:
				n.Value = x + b.(int64)
			case uint64:
				n.Value = x + b.(uint64)
			case float32:
				n.Value = x + b.(float32)
			case float64:
				n.Value = x + b.(float64)
			default:
				panic(fmt.Errorf("internal error: %T", x))
			}
		}
	case 30: // Expression '-' Expression
		_, at := n.Expression.eval(lx)
		_, bt := n.Expression2.eval(lx)
		if at.Kind() == Array {
			at = at.Element().Pointer()
		}
		if bt.Kind() == Array {
			bt = bt.Element().Pointer()
		}
		if at.Kind() == Ptr && bt.Kind() == Ptr {
			if !at.CanAssignTo(bt) {
				n.Type = undefined
				lx.report.Err(n.Expression2.Pos(), "incompatible types ('%s' - '%s')", at, bt)
				break
			}

			n.Type = m.getPtrDiffType(lx)
			break
		}

		if at.Kind() == Ptr && IsIntType(bt) {
			n.Type = at
			break
		}

		var a, b interface{}
		a, b, n.Type = m.binOp(lx, n.Expression, n.Expression2)
		n.BinOpType = n.Type
		switch x := a.(type) {
		case nil:
			// nop
		case int32:
			n.Value = x - b.(int32)
		case uint32:
			n.Value = x - b.(uint32)
		case int64:
			n.Value = x - b.(int64)
		case uint64:
			n.Value = x - b.(uint64)
		case float32:
			n.Value = x - b.(float32)
		case float64:
			n.Value = x - b.(float64)
		default:
			panic(fmt.Errorf("internal error: %T", x))
		}
	case 31: // Expression "<<" Expression
		av, at := n.Expression.eval(lx)
		bv, _ := n.Expression2.eval(lx)
		n.Type = at
		if av == nil || bv == nil {
			break
		}

		switch x := av.(type) {
		case int8:
			switch y := bv.(type) {
			case int32:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case int64:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			default:
				panic(fmt.Errorf("internal error: %T", y))
			}
		case uint8:
			switch y := bv.(type) {
			case int32:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint32:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case int64:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint64:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			default:
				panic(fmt.Errorf("internal error: %T", y))
			}
		case int16:
			switch y := bv.(type) {
			case int16:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint16:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case int32:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint32:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case int64:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint64:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			default:
				panic(fmt.Errorf("internal error: %T", y))
			}
		case uint16:
			switch y := bv.(type) {
			case int16:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint16:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case int32:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint32:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case int64:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint64:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			default:
				panic(fmt.Errorf("internal error: %T", y))
			}
		case int32:
			switch y := bv.(type) {
			case int16:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint16:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case int32:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint32:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case int64:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint64:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			default:
				panic(fmt.Errorf("internal error: %T", y))
			}
		case uint32:
			switch y := bv.(type) {
			case int16:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint16:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case int32:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint32:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case int64:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint64:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			default:
				panic(fmt.Errorf("internal error: %T", y))
			}
		case int64:
			switch y := bv.(type) {
			case int16:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint16:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case int32:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint32:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case int64:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint64:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			default:
				panic(fmt.Errorf("internal error: %T", y))
			}
		case uint64:
			switch y := bv.(type) {
			case int16:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint16:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case int32:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint32:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case int64:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			case uint64:
				switch {
				case y > 0:
					n.Value = x << uint(y)
				case y < 0:
					n.Value = x >> uint(-y)
				default:
					n.Value = x
				}
			default:
				panic(fmt.Errorf("internal error: %T", y))
			}
		default:
			panic(fmt.Errorf("internal error: %T", x))
		}
	case 32: // Expression ">>" Expression
		av, at := n.Expression.eval(lx)
		bv, _ := n.Expression2.eval(lx)
		n.Type = at
		if av == nil || bv == nil {
			break
		}

		switch x := av.(type) {
		case int8:
			switch y := bv.(type) {
			case int32:
				switch {
				case y > 0:
					n.Value = x >> uint(y)
				case y < 0:
					n.Value = x << uint(-y)
				default:
					n.Value = x
				}
			case int64:
				switch {
				case y > 0:
					n.Value = x >> uint(y)
				case y < 0:
					n.Value = x << uint(-y)
				default:
					n.Value = x
				}
			default:
				panic(fmt.Errorf("internal error: %T", y))
			}
		case uint8:
			switch y := bv.(type) {
			case int32:
				switch {
				case y > 0:
					n.Value = x >> uint(y)
				case y < 0:
					n.Value = x << uint(-y)
				default:
					n.Value = x
				}
			case int64:
				switch {
				case y > 0:
					n.Value = x >> uint(y)
				case y < 0:
					n.Value = x << uint(-y)
				default:
					n.Value = x
				}
			default:
				panic(fmt.Errorf("internal error: %T", y))
			}
		case int16:
			switch y := bv.(type) {
			case int32:
				switch {
				case y > 0:
					n.Value = x >> uint(y)
				case y < 0:
					n.Value = x << uint(-y)
				default:
					n.Value = x
				}
			case int64:
				switch {
				case y > 0:
					n.Value = x >> uint(y)
				case y < 0:
					n.Value = x << uint(-y)
				default:
					n.Value = x
				}
			default:
				panic(fmt.Errorf("internal error: %T", y))
			}
		case uint16:
			switch y := bv.(type) {
			case int32:
				switch {
				case y > 0:
					n.Value = x >> uint(y)
				case y < 0:
					n.Value = x << uint(-y)
				default:
					n.Value = x
				}
			case int64:
				switch {
				case y > 0:
					n.Value = x >> uint(y)
				case y < 0:
					n.Value = x << uint(-y)
				default:
					n.Value = x
				}
			default:
				panic(fmt.Errorf("internal error: %T", y))
			}
		case int32:
			switch y := bv.(type) {
			case int32:
				switch {
				case y > 0:
					n.Value = x >> uint(y)
				case y < 0:
					n.Value = x << uint(-y)
				default:
					n.Value = x
				}
			case int64:
				switch {
				case y > 0:
					n.Value = x >> uint(y)
				case y < 0:
					n.Value = x << uint(-y)
				default:
					n.Value = x
				}
			default:
				panic(fmt.Errorf("internal error: %T", y))
			}
		case uint32:
			switch y := bv.(type) {
			case int32:
				switch {
				case y > 0:
					n.Value = x >> uint(y)
				case y < 0:
					n.Value = x << uint(-y)
				default:
					n.Value = x
				}
			case int64:
				switch {
				case y > 0:
					n.Value = x >> uint(y)
				case y < 0:
					n.Value = x << uint(-y)
				default:
					n.Value = x
				}
			default:
				panic(fmt.Errorf("internal error: %T", y))
			}
		case int64:
			switch y := bv.(type) {
			case int32:
				switch {
				case y > 0:
					n.Value = x >> uint(y)
				case y < 0:
					n.Value = x << uint(-y)
				default:
					n.Value = x
				}
			case int64:
				switch {
				case y > 0:
					n.Value = x >> uint(y)
				case y < 0:
					n.Value = x << uint(-y)
				default:
					n.Value = x
				}
			default:
				panic(fmt.Errorf("internal error: %T", y))
			}
		case uint64:
			switch y := bv.(type) {
			case int32:
				switch {
				case y > 0:
					n.Value = x >> uint(y)
				case y < 0:
					n.Value = x << uint(-y)
				default:
					n.Value = x
				}
			case int64:
				switch {
				case y > 0:
					n.Value = x >> uint(y)
				case y < 0:
					n.Value = x << uint(-y)
				default:
					n.Value = x
				}
			default:
				panic(fmt.Errorf("internal error: %T", y))
			}
		default:
			panic(fmt.Errorf("internal error: %T", x))
		}
	case 33: // Expression '<' Expression
		n.Type = m.IntType
		_, at := n.Expression.eval(lx)
		_, bt := n.Expression2.eval(lx)
		a0, b0 := at, bt
		if at.Kind() > bt.Kind() {
			at, bt = bt, at
		}
		switch {
		case at.Kind() == Ptr:
			if bt.Kind() == Array {
				bt = bt.Element().Pointer()
			}
			if !at.CanAssignTo(bt) {
				lx.report.ErrTok(n.Token, "incompatible types ('%s' < '%s')", a0, b0)
			}
			break
		case IsArithmeticType(at):
			fallthrough
		default:
			n.Type = m.IntType
			var a, b interface{}
			a, b, n.BinOpType = m.binOp(lx, n.Expression, n.Expression2)
			switch x := a.(type) {
			case nil:
				// nop
			case int32:
				n.Value = m.cBool(x < b.(int32))
			case uint32:
				n.Value = m.cBool(x < b.(uint32))
			case int64:
				n.Value = m.cBool(x < b.(int64))
			case uint64:
				n.Value = m.cBool(x < b.(uint64))
			case float32:
				n.Value = m.cBool(x < b.(float32))
			case float64:
				n.Value = m.cBool(x < b.(float64))
			default:
				panic(fmt.Errorf("internal error: %T", x))
			}
		}
	case 34: // Expression '>' Expression
		n.Type = m.IntType
		_, at := n.Expression.eval(lx)
		_, bt := n.Expression2.eval(lx)
		a0, b0 := at, bt
		if at.Kind() > bt.Kind() {
			at, bt = bt, at
		}
		switch {
		case at.Kind() == Ptr:
			if bt.Kind() == Array {
				bt = bt.Element().Pointer()
			}
			if !at.CanAssignTo(bt) {
				lx.report.ErrTok(n.Token, "incompatible types ('%s' > '%s')", a0, b0)
			}
			break
		case IsArithmeticType(at):
			fallthrough
		default:
			n.Type = m.IntType
			var a, b interface{}
			a, b, n.BinOpType = m.binOp(lx, n.Expression, n.Expression2)
			switch x := a.(type) {
			case nil:
				// nop
			case int32:
				n.Value = m.cBool(x > b.(int32))
			case int64:
				n.Value = m.cBool(x > b.(int64))
			case uint32:
				n.Value = m.cBool(x > b.(uint32))
			case uint64:
				n.Value = m.cBool(x > b.(uint64))
			case float32:
				n.Value = m.cBool(x > b.(float32))
			case float64:
				n.Value = m.cBool(x > b.(float64))
			default:
				panic(fmt.Errorf("internal error: %T", x))
			}
		}
	case 35: // Expression "<=" Expression
		n.Type = m.IntType
		_, at := n.Expression.eval(lx)
		_, bt := n.Expression2.eval(lx)
		a0, b0 := at, bt
		if at.Kind() > bt.Kind() {
			at, bt = bt, at
		}
		switch {
		case at.Kind() == Ptr:
			if !at.CanAssignTo(bt) {
				lx.report.ErrTok(n.Token, "incompatible types ('%s' <= '%s')", a0, b0)
			}
			break
		case IsArithmeticType(at):
			fallthrough
		default:
			n.Type = m.IntType
			var a, b interface{}
			a, b, n.BinOpType = m.binOp(lx, n.Expression, n.Expression2)
			switch x := a.(type) {
			case nil:
				// nop
			case int32:
				n.Value = m.cBool(x <= b.(int32))
			case uint32:
				n.Value = m.cBool(x <= b.(uint32))
			case int64:
				n.Value = m.cBool(x <= b.(int64))
			case uint64:
				n.Value = m.cBool(x <= b.(uint64))
			case float32:
				n.Value = m.cBool(x <= b.(float32))
			case float64:
				n.Value = m.cBool(x <= b.(float64))
			default:
				panic(fmt.Errorf("internal error: %T", x))
			}
		}
	case 36: // Expression ">=" Expression
		n.Type = m.IntType
		_, at := n.Expression.eval(lx)
		_, bt := n.Expression2.eval(lx)
		a0, b0 := at, bt
		if at.Kind() > bt.Kind() {
			at, bt = bt, at
		}
		switch {
		case at.Kind() == Ptr:
			if bt.Kind() == Array {
				bt = bt.Element().Pointer()
			}
			if !at.CanAssignTo(bt) {
				lx.report.ErrTok(n.Token, "incompatible types ('%s' >= '%s')", a0, b0)
			}
			break
		case IsArithmeticType(at):
			fallthrough
		default:
			var a, b interface{}
			a, b, n.BinOpType = m.binOp(lx, n.Expression, n.Expression2)
			switch x := a.(type) {
			case nil:
				// nop
			case int32:
				n.Value = m.cBool(x >= b.(int32))
			case uint32:
				n.Value = m.cBool(x >= b.(uint32))
			case int64:
				n.Value = m.cBool(x >= b.(int64))
			case uint64:
				n.Value = m.cBool(x >= b.(uint64))
			case float32:
				n.Value = m.cBool(x >= b.(float32))
			case float64:
				n.Value = m.cBool(x >= b.(float64))
			default:
				panic(fmt.Errorf("internal error: %T", x))
			}
		}
	case 37: // Expression "==" Expression
		n.Type = m.IntType
		_, at := n.Expression.eval(lx)
		_, bt := n.Expression2.eval(lx)
		a0, b0 := at, bt
		if at.Kind() > bt.Kind() {
			at, bt = bt, at
		}
		switch {
		case at.Kind() == Ptr:
			if IsIntType(bt) {
				break
			}

			if bt.Kind() == Array {
				bt = bt.(*ctype).arrayDecay()
			}
			if bt.Kind() == Function && at.Element().Kind() == Function {
				bt = bt.Pointer()
			}
			if !at.CanAssignTo(bt) {
				lx.report.ErrTok(n.Token, "incompatible types ('%s' == '%s')", a0, b0)
			}
			break
		case IsArithmeticType(at):
			fallthrough
		default:
			var a, b interface{}
			a, b, n.BinOpType = m.binOp(lx, n.Expression, n.Expression2)
			if a == nil {
				break
			}

			n.Value = m.cBool(a == b)
		}
	case 38: // Expression "!=" Expression
		n.Type = m.IntType
		_, at := n.Expression.eval(lx)
		_, bt := n.Expression2.eval(lx)
		if at.Kind() > bt.Kind() {
			at, bt = bt, at
		}
		switch {
		case at.Kind() == Ptr:
			if IsIntType(bt) {
				break
			}

			if bt.Kind() == Function && at.Element().Kind() == Function {
				bt = bt.Pointer()
			}
			if bt.Kind() == Array {
				bt = bt.(*ctype).arrayDecay()
			}
			if !at.CanAssignTo(bt) {
				lx.report.ErrTok(n.Token, "incompatible types ('%s' != '%s')", at, bt)
			}
			break
		case IsArithmeticType(at):
			fallthrough
		default:
			var a, b interface{}
			a, b, n.BinOpType = m.binOp(lx, n.Expression, n.Expression2)
			if a == nil {
				break
			}

			n.Value = m.cBool(a != b)
		}
	case 39: // Expression '&' Expression
		var a, b interface{}
		a, b, n.Type = m.binOp(lx, n.Expression, n.Expression2)
		n.BinOpType = n.Type
		switch x := a.(type) {
		case nil:
			// nop
		case int32:
			n.Value = x & b.(int32)
		case uint32:
			n.Value = x & b.(uint32)
		case int64:
			n.Value = x & b.(int64)
		case uint64:
			n.Value = x & b.(uint64)
		default:
			panic(fmt.Errorf("internal error: %T", x))
		}
	case 40: // Expression '^' Expression
		var a, b interface{}
		a, b, n.Type = m.binOp(lx, n.Expression, n.Expression2)
		n.BinOpType = n.Type
		switch x := a.(type) {
		case nil:
			// nop
		case int32:
			n.Value = x ^ b.(int32)
		case uint32:
			n.Value = x ^ b.(uint32)
		default:
			panic(fmt.Errorf("internal error: %T", x))
		}
	case 41: // Expression '|' Expression
		var a, b interface{}
		a, b, n.Type = m.binOp(lx, n.Expression, n.Expression2)
		n.BinOpType = n.Type
		switch x := a.(type) {
		case nil:
			// nop
		case int32:
			n.Value = x | b.(int32)
		case uint32:
			n.Value = x | b.(uint32)
		case int64:
			n.Value = x | b.(int64)
		case uint64:
			n.Value = x | b.(uint64)
		default:
			panic(fmt.Sprintf("internal error: %T", x))
		}
	case 42: // Expression "&&" Expression
		n.Type = m.IntType
		a, _ := n.Expression.eval(lx)
		if a != nil && isZero(a) {
			n.Value = m.cBool(false)
			break
		}

		b, _ := n.Expression2.eval(lx)
		if a != nil && b != nil {
			if isZero(b) {
				n.Value = m.cBool(false)
				break
			}

			n.Value = m.cBool(true)
			break
		}
	case 43: // Expression "||" Expression
		n.Type = m.IntType
		av, _ := n.Expression.eval(lx)
		if av != nil && isNonZero(av) {
			n.Value = m.cBool(true)
			break
		}

		bv, _ := n.Expression2.eval(lx)
		if av != nil && bv != nil {
			n.Value = m.cBool(isNonZero(bv))
			break
		}
	case 44: // Expression '?' ExpressionList ':' Expression
		lv, _ := n.Expression.eval(lx)
		if lv == nil {
			_, at := n.ExpressionList.eval(lx)
			_, bt := n.Expression2.eval(lx)
			if eqTypes(at, bt) {
				n.Type = at
				break
			}

			if IsArithmeticType(at) && IsArithmeticType(bt) {
				n.Type = m.BinOpType(at, bt)
				break
			}

			ak := at.Kind()
			bk := bt.Kind()

			if ak == Function && bk == Ptr {
				if e := bt.Element(); e.Kind() == Function && eqTypes(at, e) {
					n.Type = bt
					break
				}
			}

			if bk == Function && ak == Ptr {
				if e := at.Element(); e.Kind() == Function && eqTypes(bt, e) {
					n.Type = at
					break
				}
			}

			if (ak == Enum || ak == Bool) && IsIntType(bt) {
				n.Type = at
				break
			}

			if (bk == Enum || bk == Bool) && IsIntType(at) {
				n.Type = bt
				break
			}

			if ak == Struct && bk == Struct ||
				ak == Union && bk == Union {
				if at.CanAssignTo(bt) {
					n.Type = at
					break
				}
			}

			if ak == Void && bk == Void {
				n.Type = at
				break
			}

			if ak == Array && bk == Array {
				if at.(*ctype).isCompatible(bt.(*ctype)) {
					n.Type = at
					break
				}

				at = at.(*ctype).arrayDecay()
				ak = at.Kind()
				bt = bt.(*ctype).arrayDecay()
				bk = bt.Kind()
			}

			if ak == Array && bk == Ptr && at.CanAssignTo(bt) {
				n.Type = bt
				break
			}

			if ak == Ptr && bk == Array && bt.CanAssignTo(at) {
				n.Type = at
				break
			}

			if ak == Ptr && bk == Ptr {
				if at.CanAssignTo(bt) {
					n.Type = at
					break
				}
			}

			if (ak == Ptr || ak == Array || ak == Function) && IsIntType(bt) {
				n.Type = at
				break
			}

			if (bk == Ptr || bk == Array || bk == Function) && IsIntType(at) {
				n.Type = bt
				break
			}

			if ak == Ptr && at.Element().Kind() == Void && bk == Ptr {
				n.Type = bt
				break
			}

			if bk == Ptr && bt.Element().Kind() == Void && ak == Ptr {
				n.Type = at
				break
			}

			lx.report.ErrTok(n.Token2, "'%s'/'%s' mismatch in conditional expression", at, bt)
			break
		}

		if isNonZero(lv) {
			n.Value, n.Type = n.ExpressionList.eval(lx)
			break
		}

		n.Value, n.Type = n.Expression2.eval(lx)
	case 45: // Expression '=' Expression
		_, at := n.Expression.eval(lx)
		_, bt := n.Expression2.eval(lx)
		if bt.Kind() == Function {
			bt = bt.Pointer()
		}
		if !bt.CanAssignTo(at) {
			lx.report.Err(n.Expression2.Pos(), "assignment from incompatible type ('%s' = '%s')", at, bt)
			break
		}

		n.Type = at
	case 46: // Expression "*=" Expression
		_, n.Type = n.Expression.eval(lx)
		if _, _, n.BinOpType = m.binOp(lx, n.Expression, n.Expression2); n.BinOpType.Kind() == Undefined {
			lx.report.ErrTok(n.Token, "incompatible types") //TODO have ...
		}
	case
		47, // Expression "/=" Expression
		48: // Expression "%=" Expression
		m.checkArithmeticType(lx, n.Expression, n.Expression2)
		n.Type = n.Expression.Type
		if v := n.Expression2.Value; v != nil && isZero(v) && IsIntType(n.Type) {
			lx.report.Err(n.Expression2.Pos(), "division by zero")
			break
		}

		if _, _, n.BinOpType = m.binOp(lx, n.Expression, n.Expression2); n.BinOpType.Kind() == Undefined {
			lx.report.ErrTok(n.Token, "incompatible types") //TODO have ...
		}
	case
		49, // Expression "+=" Expression
		50: // Expression "-=" Expression
		_, at := n.Expression.eval(lx)
		_, bt := n.Expression2.eval(lx)
		n.Type = at
		switch {
		case at.Kind() == Ptr:
			if IsIntType(bt) || bt.Kind() == Bool {
				break
			}

			lx.report.ErrTok(n.Token, "incompatible types") //TODO have ...
		case IsArithmeticType(at):
			fallthrough
		default:
			if _, _, n.BinOpType = m.binOp(lx, n.Expression, n.Expression2); n.BinOpType.Kind() == Undefined {
				lx.report.ErrTok(n.Token, "incompatible types") //TODO have ...
			}
		}
	case
		51, // Expression "<<=" Expression
		52: // Expression ">>=" Expression
		m.checkIntegerOrBoolType(lx, n.Expression, n.Expression2)
		n.Type = n.Expression.Type
	case
		53, // Expression "&=" Expression
		54, // Expression "^=" Expression
		55: // Expression "|=" Expression
		m.checkIntegerOrBoolType(lx, n.Expression, n.Expression2)
		if _, _, n.BinOpType = m.binOp(lx, n.Expression, n.Expression2); n.BinOpType.Kind() == Undefined {
			lx.report.ErrTok(n.Token, "incompatible types") //TODO have ...
		}
		n.Type = n.BinOpType
	case 56: // "_Alignof" '(' TypeName ')'
		n.Type = lx.model.getSizeType(lx)
		t := n.TypeName.Type
		el := true
	again:
		switch t.Kind() {
		case Undefined, Function:
			t = nil
		case Struct, Union:
			if _, isIncomplete := t.Members(); isIncomplete {
				t = nil
				break
			}
		case Array:
			if el {
				el = false
				t = t.Element()
				goto again
			}
		}
		if t == nil {
			lx.report.Err(n.TypeName.Pos(), "invalid argument of _Alignof")
			n.Value = lx.model.MustConvert(1, n.Type)
			break
		}

		al := t.AlignOf()
		if al < 0 {
			lx.report.Err(n.TypeName.Pos(), "invalid argument of _Alignof")
			al = 1
		}
		n.Value = lx.model.MustConvert(int32(al), n.Type)
	default:
		//dbg("", PrettyString(n))
		panic(n.Case)
	}
	//ct := n.Type.(*ctype)
	//s := ""
	//if n.Value != nil {
	//	s = fmt.Sprintf("value: %T(%#v)", n.Value, n.Value)
	//}
	//dbg("tc %v %v %v %v %v: %v %v", position(n.Pos()), n.Case, ct.resultStars, ct.stars, ct, ct.Kind(), s)
	return n.Value, n.Type
}

// IdentResolutionScope returns the scope an identifier is resolved in. If n is
// not an identifier (n.Case == 0), IdentResolutionScope returns nil.
func (n *Expression) IdentResolutionScope() *Bindings {
	if n.Case == 0 { // IDENTIFIER
		return n.scope
	}

	return nil
}

// ------------------------------------------------------------- ExpressionList

func (n *ExpressionList) eval(lx *lexer) (interface{}, Type) {
	if n.Type != nil {
		return n.Value, n.Type
	}

	n0 := n
	for ; n != nil; n = n.ExpressionList {
		n.Value, n.Type = n.Expression.eval(lx)
		n0.Value, n0.Type = n.Value, n.Type
	}
	return n0.Value, n0.Type
}

// Len returns the number of items in n.
func (n *ExpressionList) Len() (r int) {
	for ; n != nil; n = n.ExpressionList {
		r++
	}
	return r
}

// ---------------------------------------------------------- IdentifierListOpt

func (n *IdentifierListOpt) post(report *xc.Report, dl *DeclarationList) {
	type r struct {
		pos token.Pos
		i   int
	}
	var a []int
	ilm := map[int]r{}
	i := 0
	for il := n.IdentifierList; il != nil; il, i = il.IdentifierList, i+1 {
		t := il.Token
		if il.Case == 1 {
			t = il.Token2
		}
		nm := t.Val
		if r, ok := ilm[nm]; ok {
			report.ErrTok(t, "duplicate parameter name declaration, previous at %s", r.pos)
			continue
		}

		v := r{t.Pos(), i}
		ilm[nm] = v
		a = append(a, nm)
	}
	params := make([]Parameter, len(ilm))
	for ; dl != nil; dl = dl.DeclarationList {
		decl := dl.Declaration
		o := decl.InitDeclaratorListOpt
		if o == nil {
			report.Err(decl.Pos(), "invalid parameter declaration")
			continue
		}

		for l := o.InitDeclaratorList; l != nil; l = l.InitDeclaratorList {
			id := l.InitDeclarator
			if id.Case == 1 { // Declarator '=' Initializer
				report.Err(id.Pos(), "invalid parameter declarator")
			}

			d := id.Declarator
			nm, _ := d.Identifier()
			r, ok := ilm[nm]
			if !ok {
				report.Err(d.Pos(), "parameter name not declared")
				continue
			}

			params[r.i] = Parameter{d, nm, d.Type}
		}
	}
	for i, v := range params {
		if v.Declarator == nil {
			params[i] = Parameter{nil, a[i], undefined}
		}
	}
	n.params = params
}

// ---------------------------------------------------------------- Initializer

func (n *Initializer) typeCheck(pt *Type, dt Type, static bool, lx *lexer) {
	static = static && !lx.tweaks.enableNonConstStaticInitExpressions
	if dt == nil {
		return
	}

	k := dt.Kind()
	d := dt.Declarator()
	dd := d.DirectDeclarator
	if dd.isArray() && dd.isVLA() != nil {
		lx.report.Err(n.Pos(), "variable length array cannot have initializers")
		return
	}

	switch n.Case {
	case 0: // Expression
		x := n.Expression
		xt := n.Expression.Type
		switch v := x.Value.(type) {
		case StringLitID:
			switch k {
			case Array, Ptr:
				switch dt.Element().Kind() {
				case Char, SChar, UChar:
					if pt != nil && dd.isArray() && dt.Elements() < 0 {
						*pt = dt.(*ctype).setElements(len(xc.Dict.S(int(v))) + 1)
					}
				default:
					if !xt.CanAssignTo(dt) {
						lx.report.Err(x.Pos(), "cannot initialize type '%v' using expression of type '%v'", dt, xt)
					}
				}
			default:
				if !xt.CanAssignTo(dt) {
					lx.report.Err(x.Pos(), "cannot initialize type '%v' using expression of type '%v'", dt, xt)
				}
			}
			return
		case LongStringLitID:
			switch k {
			case Array, Ptr:
				switch dt.Element().Kind() {
				case Short, UShort, Int, UInt, Long, ULong:
					if pt != nil && dd.isArray() && dt.Elements() < 0 {
						*pt = dt.(*ctype).setElements(len(xc.Dict.S(int(v))) + 1)
					}
				default:
					if !xt.CanAssignTo(dt) {
						lx.report.Err(x.Pos(), "cannot initialize type '%v' using expression of type '%v'", dt, xt)
					}
				}
			default:
				if !xt.CanAssignTo(dt) {
					lx.report.Err(x.Pos(), "cannot initialize type '%v' using expression of type '%v'", dt, xt)
				}
			}
			return
		case nil:
			if static {
				lx.report.Err(x.Pos(), "expressions in an initializer for an object that has static storage duration shall be constant expressions or string literals.")
				return
			}

		}

		if !xt.CanAssignTo(dt) {
			//dbg("", dt, xt)
			if dt.Kind() == Struct || dt.Kind() == Union {
				if ma, _ := dt.Members(); len(ma) == 1 {
					//dbg("")
					n.typeCheck(nil, ma[0].Type, static, lx)
					//dbg("")
					return
				}
			}

			if dt.Kind() == Array {
				n.typeCheck(nil, dt.Element(), static, lx)
				return
			}

			lx.report.Err(x.Pos(), "cannot initialize type '%v' using expression of type '%v'", dt, xt)
			return
		}
	case 1: // '{' InitializerList CommaOpt '}'  // Case 1
		n.InitializerList.typeCheck(pt, dt, static, lx)
	default:
		panic("internal error")
	}
}

// ------------------------------------------------------------ InitializerList

// Len returns the number of items in n.
func (n *InitializerList) Len() (r int) {
	for ; n != nil; n = n.InitializerList {
		r++
	}
	return r
}

func (n *InitializerList) typeCheck(pt *Type, dt Type, static bool, lx *lexer) {
	if dt == nil {
		return
	}

	d := dt.Declarator()
	dd := d.DirectDeclarator
	switch dt.Kind() {
	case Struct, Union:
		ma, incomplete := dt.Members()
		if incomplete {
			lx.report.Err(n.Pos(), "cannot initialize incomplete type")
			return
		}

		if len(ma) == 1 {
			//dbg("%s: %v -> %v", position(n.Pos()), dt, ma[0].Type)
			n.InitializerList.typeCheck(nil, ma[0].Type, static, lx)
			return
		}

		i := 0
		var stack []int
		for l := n; l != nil; l = l.InitializerList {
			var m Member
			switch o := l.DesignationOpt; {
			case o != nil:
				ma := ma
				j := 0
				for l := o.Designation.DesignatorList; l != nil; l = l.DesignatorList {
					switch d := l.Designator; d.Case {
					case 0: // '[' ConstantExpression ']'
						panic("TODO")
					case 1: // '.' IDENTIFIER              // Case 1
						if j != 0 {
							ma, _ = m.Type.Members()
						}
						found := false
						for k, v := range ma {
							if d.Token2.Val == v.Name {
								found = true
								m = v
								if j == 0 {
									i = k
								}
								break
							}
						}
						if !found {
							panic("TODO")
						}

						j++
					default:
						panic("internal error")
					}
				}
			default:
				if i >= len(ma) {
					//dbg("", i, len(ma))
					panic("TODO")
				}

				switch l := len(stack); {
				case l != 0:
					stack[l-1]--
					if stack[l-1] != 0 {
						i--
						break
					}

					stack = stack[:l-1]
					i++
					fallthrough
				default:
					m = ma[i]
					if mt := m.Type; mt.Kind() == Array && mt.Elements() >= 0 {
						stack = append(stack, mt.Elements())
						i--
					}
				}
			}
			l.Initializer.typeCheck(nil, m.Type, static, lx)
			i++
		}
	case Array, Ptr:
		elems := dt.Elements()
		elem := dt.Element()
		elem0 := elem
		for elem0.Kind() == Array {
			n := elem0.Elements()
			if n >= 0 {
				if elems < 0 {
					elems = 1
				}
				elems *= n
			}
			elem0 = elem0.Element()
		}
		i := 0
		for l := n; l != nil; l = l.InitializerList {
			if o := l.DesignationOpt; o != nil {
				elem := elem
				j := 0
				m := lx.model
				for l := o.Designation.DesignatorList; l != nil; l = l.DesignatorList {
					switch d := l.Designator; d.Case {
					case 0: // '[' ConstantExpression ']'
						if !IsIntType(d.ConstantExpression.Type) {
							panic("TODO")
						}

						switch {
						case j == 0:
							i = int(m.MustConvert(d.ConstantExpression.Value, m.IntType).(int32))
						default:
							elem = elem.Element()
						}
						j++
					case 1: // '.' IDENTIFIER              // Case 1
						panic("TODO")
					default:
						panic("internal error")
					}
				}
			}

			if elems >= 0 && i >= elems {
				panic("TODO")
			}

			switch in := l.Initializer; in.Case {
			case 0: // Expression
				in.typeCheck(nil, elem0, static, lx)
				i++
			case 1: // '{' InitializerList CommaOpt '}'  // Case 1
				if !elem.Declarator().DirectDeclarator.isArray() {
					panic("TODO")
				}

				in.InitializerList.typeCheck(nil, elem, static, lx)
				i++
			default:
				panic("internal error")
			}
		}
		if pt != nil && dd.isArray() && elems < 0 {
			//dbg("", position(n.Pos()), elem, elems)
			*pt = dt.(*ctype).setElements(i)
		}
	default:
		i := 0
		for l := n; l != nil; l = l.InitializerList {
			if i != 0 {
				//dbg("%s: %v", position(n.Pos()), dt)
				panic("TODO")
			}

			l.Initializer.typeCheck(nil, dt, static, lx)
			i++
		}
	}
}

// ---------------------------------------------------------- ParameterTypeList

func (n *ParameterTypeList) post() {
	for l := n.ParameterList; l != nil; l = l.ParameterList {
		d := l.ParameterDeclaration.declarator
		nm, _ := d.Identifier()
		t := d.Type
		n.params = append(n.params, Parameter{
			Declarator: d,
			Name:       nm,
			Type:       t,
		})
	}
	if len(n.params) == 1 && n.params[0].Type.Kind() == Void {
		n.params = make([]Parameter, 0) // Must be non nil.
	}
}

// -------------------------------------------------------------------- Pointer

func (n *Pointer) stars() (r int) {
	for ; n != nil; n = n.Pointer {
		r++
	}
	return r
}

// ----------------------------------------------------------------- PointerOpt

func (n *PointerOpt) stars() int {
	if n == nil {
		return 0
	}

	return n.Pointer.stars()
}

// ----------------------------------------------------- SpecifierQualifierList

// IsInline implements specifier.
func (n *SpecifierQualifierList) IsInline() bool {
	return n.attr&saInline != 0
}

// IsTypedef implements specifier.
func (n *SpecifierQualifierList) IsTypedef() bool {
	return n.attr&saTypedef != 0
}

// IsExtern implements specifier.
func (n *SpecifierQualifierList) IsExtern() bool {
	return n.attr&saExtern != 0
}

// IsStatic implements specifier.
func (n *SpecifierQualifierList) IsStatic() bool {
	return n.attr&saStatic != 0
}

// IsAuto implements specifier.
func (n *SpecifierQualifierList) IsAuto() bool {
	return n.attr&saAuto != 0
}

// IsRegister implements specifier.
func (n *SpecifierQualifierList) IsRegister() bool {
	return n.attr&saRegister != 0
}

// IsConst returns whether n includes the 'const' type qualifier.
func (n *SpecifierQualifierList) IsConst() bool {
	return n.attr&saConst != 0
}

// IsRestrict implements specifier.
func (n *SpecifierQualifierList) IsRestrict() bool {
	return n.attr&saRestrict != 0
}

// IsVolatile implements specifier.
func (n *SpecifierQualifierList) IsVolatile() bool {
	return n.attr&saVolatile != 0
}

// kind implements specifier.
func (n *SpecifierQualifierList) kind() Kind { return tsValid[n.typeSpecifiers()] }

// typeSpecifiers implements specifier.
func (n *SpecifierQualifierList) typeSpecifiers() int {
	return n.typeSpecifier
}

// firstTypeSpecifier implements specifier.
func (n *SpecifierQualifierList) firstTypeSpecifier() *TypeSpecifier {
	for n.Case != 0 { // TypeSpecifier SpecifierQualifierListOpt
		o := n.SpecifierQualifierListOpt
		if o == nil {
			return nil
		}

		n = o.SpecifierQualifierList
	}
	return n.TypeSpecifier
}

// attrs implements specifier.
func (n *SpecifierQualifierList) attrs() int { return n.attr }

// member implements specifier.
func (n *SpecifierQualifierList) member(nm int) (*Member, error) {
	return n.firstTypeSpecifier().member(nm)
}

// str implements specifier.
func (n *SpecifierQualifierList) str() string {
	return specifierString(n)
}

// TypedefName implements Specifier.
func (n *SpecifierQualifierList) TypedefName() int {
	if n.kind() == TypedefName {
		return n.firstTypeSpecifier().Token.Val
	}
	return 0
}

// ----------------------------------------------------------- StructDeclarator

func (n *StructDeclarator) post(lx *lexer) {
	sc := lx.scope
	switch n.Case {
	case 0: // Declarator
		if sc.bitOffset != 0 {
			finishBitField(lx)
		}

		t := n.Declarator.Type
		sz := t.SizeOf()
		al := t.StructAlignOf()
		switch {
		case sc.isUnion:
			// Track union size.
			sc.maxSize = mathutil.Max(sc.maxSize, sz)
		default:
			off := sc.offset
			sc.offset = align(sc.offset, al) // Bump offset if necessary.
			if pd := sc.prevStructDeclarator; pd != nil {
				pd.padding = sc.offset - off
			}
			n.Declarator.offsetOf = sc.offset
			sc.offset += sz // Allocate sz.
		}
		sc.maxAlign = mathutil.Max(sc.maxAlign, al)
		sc.prevStructDeclarator = n.Declarator
	case 1: // DeclaratorOpt ':' ConstantExpression
		t := lx.model.IntType
		if o := n.DeclaratorOpt; o != nil {
			t = o.Declarator.Type
		}

		var w int
		switch x := n.ConstantExpression.Value.(type) {
		case int32:
			w = int(x)
		case int64:
			w = int(x)
			if m := t.SizeOf() * 8; x > int64(m) {
				lx.report.Err(n.ConstantExpression.Pos(), "width of bit field exceeds its type")
				w = m
			}
		case uint64:
			w = int(x)
			m := t.SizeOf() * 8
			if x > uint64(m) {
				lx.report.Err(n.ConstantExpression.Pos(), "width of bit field exceeds its type")
				w = m
				break
			}

			if x > uint64(lx.model.Items[Int].Size*8) {
				lx.report.Err(n.ConstantExpression.Pos(), "width of bit field exceeds int bits")
				w = m
				break
			}
		default:
			panic("internal error")
		}
		if m := t.SizeOf() * 8; w > m {
			lx.report.Err(n.ConstantExpression.Pos(), "width of bit field exceeds its type")
			w = m
		}
		maxBits := lx.model.LongType.SizeOf() * 8
		if sc.bitOffset+w > maxBits {
			finishBitField(lx)
		}
		if o := n.DeclaratorOpt; o != nil {
			d := o.Declarator
			d.offsetOf = sc.offset
			d.bitOffset = sc.bitOffset
			d.bitFieldGroup = sc.bitFieldGroup
			sc.prevStructDeclarator = o.Declarator
			t = d.Type
			switch t.Kind() {
			case Char, SChar, UChar, Int, UInt, Long, ULong, Short, UShort, Enum, Bool:
				// ok
			case LongLong, ULongLong:
				if lx.tweaks.enableWideBitFieldTypes {
					// Non-standard, but enabled.
					break
				}
				lx.report.Err(n.ConstantExpression.Pos(), "bit field has invalid type (have %s)", t)
				t = lx.model.IntType
			default:
				lx.report.Err(n.ConstantExpression.Pos(), "bit field has invalid type (have %s)", t)
				t = lx.model.IntType
			}
		}
		sc.bitOffset += w
	default:
		panic(n.Case)
	}
}

// -------------------------------------------------------------- StructOrUnion

func (n *StructOrUnion) typeSpecifiers() int {
	switch n.Token.Rune {
	case STRUCT:
		return tsStructSpecifier
	case UNION:
		return tsUnionSpecifier
	default:
		panic("internal error")
	}
}

func (n *StructOrUnion) isCompatible(m *StructOrUnion) (r bool) {
	return n == m || n.Case == m.Case
}

func (n *StructOrUnion) str() string {
	switch n.Token.Rune {
	case STRUCT:
		return "struct"
	case UNION:
		return "union"
	default:
		panic("internal error")
	}
}

// ----------------------------------------------------- StructOrUnionSpecifier

// Declarator returns a synthetic Declarator when a tagged struc/union type is
// defined inline a declaration.
func (n *StructOrUnionSpecifier) Declarator() *Declarator { return n.declarator }

func (n *StructOrUnionSpecifier) typeSpecifiers() int { return n.StructOrUnion.typeSpecifiers() }

func (n *StructOrUnionSpecifier) isCompatible(m *StructOrUnionSpecifier) (r bool) {
	if n == m {
		return true
	}

	if !n.StructOrUnion.isCompatible(m.StructOrUnion) {
		return false
	}

	if n.Case > m.Case {
		n, m = m, n
	}
	switch n.Case {
	case 0: // StructOrUnion IdentifierOpt '{' StructDeclarationList '}'
		switch m.Case {
		case 1: // StructOrUnion IDENTIFIER
			if o := n.IdentifierOpt; o != nil {
				return o.Token.Val == m.Token.Val
			}

			panic("TODO")
		default:
			panic(m.Case)
		}
	case 1: // StructOrUnion IDENTIFIER
		switch m.Case {
		case 1: // StructOrUnion IDENTIFIER
			return n.Token.Val == m.Token.Val
		default:
			panic(m.Case)
		}
	default:
		panic(n.Case)
	}
}

func (n *StructOrUnionSpecifier) member(nm int) (*Member, error) {
	switch n.Case {
	case 0: // StructOrUnion IdentifierOpt '{' StructDeclarationList '}'
		b, s := n.scope.Lookup2(NSIdentifiers, nm)
		if s != n.scope {
			var t []byte
			if o := n.IdentifierOpt; o != nil {
				t = o.Token.S()
			}
			return nil, fmt.Errorf("%s %s has no member named %s", n.StructOrUnion.str(), t, xc.Dict.S(nm))
		}

		d := b.Node.(*DirectDeclarator).top().declarator
		return &Member{
			Bits:       d.bits,
			Declarator: d,
			Name:       nm,
			OffsetOf:   d.offsetOf,
			Type:       d.Type,
		}, nil
	case 1: // StructOrUnion IDENTIFIER
		b := n.scope.Lookup(NSTags, n.Token.Val)
		n2, def := b.Node.(*StructOrUnionSpecifier)
		if !def {
			return nil, fmt.Errorf("invalid use of undefined type '%s %s'", n.StructOrUnion.str(), n.Token.S())
		}

		return n2.member(nm)
	default:
		panic(n.Case)
	}
}

// -------------------------------------------------------------- TypeSpecifier

func (n *TypeSpecifier) member(nm int) (*Member, error) {
	switch n.Case {
	case 11: // StructOrUnionSpecifier
		return n.StructOrUnionSpecifier.member(nm)
	default:
		panic("internal error")
	}
}
