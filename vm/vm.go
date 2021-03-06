package vm

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"runtime"

	u "github.com/araddon/gou"
	ql "github.com/araddon/qlbridge/lex"
)

var (
	ErrUnknownOp       = fmt.Errorf("expr: unknown op type")
	ErrUnknownNodeType = fmt.Errorf("expr: unknown node type")
	ErrExecute         = fmt.Errorf("Could not execute")
	_                  = u.EMPTY

	SchemaInfoEmpty = &NoSchema{}
)

type State struct {
	ExprVm // reference to the VM operating on this state
	// We make a reflect value of self (state) as we use []reflect.ValueOf often
	rv     reflect.Value
	Reader ContextReader
	Writer ContextWriter
}

func NewState(vm ExprVm, read ContextReader, write ContextWriter) *State {
	s := &State{
		ExprVm: vm,
		Reader: read,
		Writer: write,
	}
	s.rv = reflect.ValueOf(s)
	return s
}

type ExprVm interface {
	Execute(writeContext ContextWriter, readContext ContextReader) error
}

type NoSchema struct {
}

func (m *NoSchema) Key() string { return "" }

// A node vm is a vm for parsing, evaluating a single tree-node
//
type Vm struct {
	*Tree
}

func (m *Vm) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.String())
}

func NewVm(expr string) (*Vm, error) {
	t, err := ParseExpression(expr)
	if err != nil {
		return nil, err
	}
	m := &Vm{
		Tree: t,
	}
	return m, nil
}

// Execute applies a parse expression to the specified context's
func (m *Vm) Execute(writeContext ContextWriter, readContext ContextReader) (err error) {
	//defer errRecover(&err)
	s := &State{
		ExprVm: m,
		Reader: readContext,
	}
	s.rv = reflect.ValueOf(s)
	//u.Debugf("vm.Execute:  %#v", m.Tree.Root)
	v, ok := s.Walk(m.Tree.Root)
	if ok {
		// Special Vm that doesnt' have name fields
		//u.Debugf("vm.Walk val:  %v", v)
		writeContext.Put(SchemaInfoEmpty, readContext, v)
		return nil
	}
	return ErrExecute
}

// errRecover is the handler that turns panics into returns from the top
// level of
func errRecover(errp *error) {
	e := recover()
	if e != nil {
		switch err := e.(type) {
		case runtime.Error:
			panic(e)
		case error:
			*errp = err
		default:
			panic(e)
		}
	}
}

// creates a new Value with a nil group and given value.
// TODO:  convert this to an interface method on nodes called Value()
func nodeToValue(t *NumberNode) (v Value) {
	//u.Debugf("nodeToValue()  isFloat?%v", t.IsFloat)
	if t.IsInt {
		v = NewIntValue(t.Int64)
	} else if t.IsFloat {
		v = NewNumberValue(ToFloat64(reflect.ValueOf(t.Text)))
	} else {
		u.Errorf("Could not find type? %v", t.Type())
	}
	//u.Debugf("return nodeToValue()	%v  %T  arg:%T", v, v, t)
	return v
}

func (e *State) Walk(arg Node) (Value, bool) {
	//u.Debugf("Walk() node=%T  %v", arg, arg)
	switch argVal := arg.(type) {
	case *NumberNode:
		return nodeToValue(argVal), true
	case *BinaryNode:
		return e.walkBinary(argVal), true
	case *UnaryNode:
		return e.walkUnary(argVal)
	case *FuncNode:
		//return e.walkFunc(argVal)
		return e.walkFunc(argVal)
	case *IdentityNode:
		return e.walkIdentity(argVal)
	case *StringNode:
		return NewStringValue(argVal.Text), true
	default:
		u.Errorf("Unknonwn node type:  %T", argVal)
		panic(ErrUnknownNodeType)
	}
}

func (e *State) walkBinary(node *BinaryNode) Value {
	ar, aok := e.Walk(node.Args[0])
	br, bok := e.Walk(node.Args[1])
	if !aok || !bok {
		//u.Warnf("not ok: %v  l:%v  r:%v  %T  %T", node, ar, br, ar, br)
		return nil
	}
	//u.Debugf("walkBinary: %v  l:%v  r:%v  %T  %T", node, ar, br, ar, br)
	switch at := ar.(type) {
	case IntValue:
		switch bt := br.(type) {
		case IntValue:
			//u.Debugf("doing operate ints  %v %v  %v", at, node.Operator.V, bt)
			n := operateInts(node.Operator, at, bt)
			return n
		case NumberValue:
			//u.Debugf("doing operate ints/numbers  %v %v  %v", at, node.Operator.V, bt)
			n := operateNumbers(node.Operator, at.NumberValue(), bt)
			return n
		default:
			u.Errorf("unknown type:  %T %v", bt, bt)
			panic(ErrUnknownOp)
		}
	case NumberValue:
		switch bt := br.(type) {
		case IntValue:
			n := operateNumbers(node.Operator, at, bt.NumberValue())
			return n
		case NumberValue:
			n := operateNumbers(node.Operator, at, bt)
			return n
		default:
			u.Errorf("unknown type:  %T %v", bt, bt)
			panic(ErrUnknownOp)
		}
	case BoolValue:
		switch bt := br.(type) {
		case BoolValue:
			switch node.Operator.T {
			case ql.TokenLogicAnd:
				return NewBoolValue(at.v && bt.v)
			case ql.TokenLogicOr:
				return NewBoolValue(at.v || bt.v)
			case ql.TokenEqualEqual:
				return NewBoolValue(at.v == bt.v)
			case ql.TokenNE:
				return NewBoolValue(at.v != bt.v)
			default:
				u.Infof("bool binary?:  %v  %v", at, bt)
				panic(ErrUnknownOp)
			}

		default:
			u.Errorf("at?%T  %v  coerce?%v bt? %T     %v", at, at.Value(), at.CanCoerce(stringRv), bt, bt.Value())
			panic(ErrUnknownOp)
		}
	case StringValue:
		if at.CanCoerce(int64Rv) {
			switch bt := br.(type) {
			case StringValue:
				n := operateNumbers(node.Operator, at.NumberValue(), bt.NumberValue())
				return n
			case IntValue:
				n := operateNumbers(node.Operator, at.NumberValue(), bt.NumberValue())
				return n
			case NumberValue:
				n := operateNumbers(node.Operator, at.NumberValue(), bt)
				return n
			default:
				u.Errorf("at?%T  %v  coerce?%v bt? %T     %v", at, at.Value(), at.CanCoerce(stringRv), bt, bt.Value())
				panic(ErrUnknownOp)
			}
		} else {
			u.Errorf("at?%T  %v  coerce?%v bt? %T     %v", at, at.Value(), at.CanCoerce(stringRv), br, br)
		}
		// case nil:
		// 	// TODO, remove this case?  is this valid?  used?
		// 	switch bt := br.(type) {
		// 	case StringValue:
		// 		n := operateNumbers(node.Operator, NumberNaNValue, bt.NumberValue())
		// 		return n
		// 	case IntValue:
		// 		n := operateNumbers(node.Operator, NumberNaNValue, bt.NumberValue())
		// 		return n
		// 	case NumberValue:
		// 		n := operateNumbers(node.Operator, NumberNaNValue, bt)
		// 		return n
		// 	case nil:
		// 		u.Errorf("a && b nil? at?%v  %v    %v", at, bt, node.Operator)
		// 	default:
		// 		u.Errorf("nil at?%v  %T      %v", at, bt, node.Operator)
		// 		panic(ErrUnknownOp)
		// 	}
		// default:
		u.Errorf("Unknown op?  %T  %T  %v", ar, at, ar)
		panic(ErrUnknownOp)
	}

	return nil
}

func (e *State) walkIdentity(node *IdentityNode) (Value, bool) {

	if node.IsBooleanIdentity() {
		//u.Debugf("walkIdentity() boolean: node=%T  %v Bool:%v", node, node, node.Bool())
		return NewBoolValue(node.Bool()), true
	}
	//u.Debugf("walkIdentity() node=%T  %v", node, node)
	return e.Reader.Get(node.Text)
}

func (e *State) walkUnary(node *UnaryNode) (Value, bool) {

	a, ok := e.Walk(node.Arg)
	if !ok {
		u.Infof("whoops, %#v", node)
		return a, false
	}
	switch node.Operator.T {
	case ql.TokenNegate:
		switch argVal := a.(type) {
		case BoolValue:
			//u.Infof("found urnary bool:  res=%v   expr=%v", !argVal.v, node.StringAST())
			return NewBoolValue(!argVal.v), true
		default:
			//u.Errorf("urnary type not implementedUnknonwn node type:  %T", argVal)
			panic(ErrUnknownNodeType)
		}
	case ql.TokenMinus:
		if an, aok := a.(NumericValue); aok {
			return NewNumberValue(-an.Float()), true
		}
	default:
		u.Warnf("urnary not implemented:   %#v", node)
	}

	return NewNilValue(), false
}

func (e *State) walkFunc(node *FuncNode) (Value, bool) {

	//u.Debugf("walk node --- %v   ", node.StringAST())

	//we create a set of arguments to pass to the function, first arg
	// is this *State
	var ok bool
	funcArgs := []reflect.Value{e.rv}
	for _, a := range node.Args {

		//u.Debugf("arg %v  %T %v", a, a, a.Type().Kind())

		var v interface{}

		switch t := a.(type) {
		case *StringNode: // String Literal
			v = NewStringValue(t.Text)
		case *IdentityNode: // Identity node = lookup in context

			if t.IsBooleanIdentity() {
				v = NewBoolValue(t.Bool())
			} else {
				v, ok = e.Reader.Get(t.Text)
				if !ok {
					// nil arguments are valid
					v = NewNilValue()
				}
			}

		case *NumberNode:
			v = nodeToValue(t)
		case *FuncNode:
			//u.Debugf("descending to %v()", t.Name)
			v, ok = e.walkFunc(t)
			if !ok {
				return NewNilValue(), false
			}
			//u.Debugf("result of %v() = %v, %T", t.Name, v, v)
			//v = extractScalar()
		case *UnaryNode:
			//v = extractScalar(e.walkUnary(t))
			v, ok = e.walkUnary(t)
			if !ok {
				return NewNilValue(), false
			}
		case *BinaryNode:
			//v = extractScalar(e.walkBinary(t))
			v = e.walkBinary(t)
		default:
			panic(fmt.Errorf("expr: unknown func arg type"))
		}

		if v == nil {
			//u.Warnf("Nil vals?  %v  %T  arg:%T", v, v, a)
			// What do we do with Nil Values?
			switch a.(type) {
			case *StringNode: // String Literal
				u.Warnf("NOT IMPLEMENTED T:%T v:%v", a, a)
			case *IdentityNode: // Identity node = lookup in context
				v = NewStringValue("")
			default:
				u.Warnf("unknown type:  %v  %T", v, v)
			}

			funcArgs = append(funcArgs, reflect.ValueOf(v))
		} else {
			//u.Debugf(`found func arg:  key="%v"  %T  arg:%T`, v, v, a)
			funcArgs = append(funcArgs, reflect.ValueOf(v))
		}

	}
	// Get the result of calling our Function (Value,bool)
	//u.Debugf("Calling func:%v(%v)", node.F.Name, funcArgs)
	fnRet := node.F.F.Call(funcArgs)
	// check if has an error response?
	if len(fnRet) > 1 && !fnRet[1].Bool() {
		// What do we do if not ok?
		return EmptyStringValue, false
	}
	//u.Debugf("response %v %v  %T", node.F.Name, fnRet[0].Interface(), fnRet[0].Interface())
	return fnRet[0].Interface().(Value), true
}

func operateNumbers(op ql.Token, av, bv NumberValue) Value {
	switch op.T {
	case ql.TokenPlus, ql.TokenStar, ql.TokenMultiply, ql.TokenDivide, ql.TokenMinus,
		ql.TokenModulus:
		if math.IsNaN(av.v) || math.IsNaN(bv.v) {
			return NewNumberValue(math.NaN())
		}
	}

	//
	a, b := av.v, bv.v
	switch op.T {
	case ql.TokenPlus: // +
		return NewNumberValue(a + b)
	case ql.TokenStar, ql.TokenMultiply: // *
		return NewNumberValue(a * b)
	case ql.TokenMinus: // -
		return NewNumberValue(a - b)
	case ql.TokenDivide: //    /
		return NewNumberValue(a / b)
	case ql.TokenModulus: //    %
		// is this even valid?   modulus on floats?
		return NewNumberValue(float64(int64(a) % int64(b)))

	// Below here are Boolean Returns
	case ql.TokenEqualEqual: //  ==
		//u.Infof("==?  %v  %v", av, bv)
		if a == b {
			return BoolValueTrue
		} else {
			return BoolValueFalse
		}
	case ql.TokenGT: //  >
		if a > b {
			//r = 1
			return BoolValueTrue
		} else {
			//r = 0
			return BoolValueFalse
		}
	case ql.TokenNE: //  !=    or <>
		if a != b {
			return BoolValueTrue
		} else {
			return BoolValueFalse
		}
	case ql.TokenLT: // <
		if a < b {
			return BoolValueTrue
		} else {
			return BoolValueFalse
		}
	case ql.TokenGE: // >=
		if a >= b {
			return BoolValueTrue
		} else {
			return BoolValueFalse
		}
	case ql.TokenLE: // <=
		if a <= b {
			return BoolValueTrue
		} else {
			return BoolValueFalse
		}
	case ql.TokenLogicOr, ql.TokenOr: //  ||
		if a != 0 || b != 0 {
			return BoolValueTrue
		} else {
			return BoolValueFalse
		}
	case ql.TokenLogicAnd: //  &&
		if a != 0 && b != 0 {
			return BoolValueTrue
		} else {
			return BoolValueFalse
		}
	}
	panic(fmt.Errorf("expr: unknown operator %s", op))
}

func operateInts(op ql.Token, av, bv IntValue) Value {
	//if math.IsNaN(a) || math.IsNaN(b) {
	//	return math.NaN()
	//}
	a, b := av.v, bv.v
	//u.Infof("a op b:   %v %v %v", a, op.V, b)
	switch op.T {
	case ql.TokenPlus: // +
		//r = a + b
		return NewIntValue(a + b)
	case ql.TokenStar, ql.TokenMultiply: // *
		//r = a * b
		return NewIntValue(a * b)
	case ql.TokenMinus: // -
		//r = a - b
		return NewIntValue(a - b)
	case ql.TokenDivide: //    /
		//r = a / b
		//u.Debugf("divide:   %v / %v = %v", a, b, a/b)
		return NewIntValue(a / b)
	case ql.TokenModulus: //    %
		//r = a / b
		//u.Debugf("modulus:   %v / %v = %v", a, b, a/b)
		return NewIntValue(a % b)

	// Below here are Boolean Returns
	case ql.TokenEqualEqual: //  ==
		if a == b {
			return BoolValueTrue
		} else {
			return BoolValueFalse
		}
	case ql.TokenGT: //  >
		if a > b {
			return BoolValueTrue
		} else {
			return BoolValueFalse
		}
	case ql.TokenNE: //  !=    or <>
		if a != b {
			return BoolValueTrue
		} else {
			return BoolValueFalse
		}
	case ql.TokenLT: // <
		if a < b {
			return BoolValueTrue
		} else {
			return BoolValueFalse
		}
	case ql.TokenGE: // >=
		if a >= b {
			return BoolValueTrue
		} else {
			return BoolValueFalse
		}
	case ql.TokenLE: // <=
		if a <= b {
			return BoolValueTrue
		} else {
			return BoolValueFalse
		}
	case ql.TokenLogicOr: //  ||
		if a != 0 || b != 0 {
			return BoolValueTrue
		} else {
			return BoolValueFalse
		}
	case ql.TokenLogicAnd: //  &&
		if a != 0 && b != 0 {
			return BoolValueTrue
		} else {
			return BoolValueFalse
		}
	}
	panic(fmt.Errorf("expr: unknown operator %s", op))
}

func uoperate(op string, a float64) (r float64) {
	switch op {
	case "!":
		if a == 0 {
			r = 1
		} else {
			r = 0
		}
	case "-":
		r = -a
	default:
		panic(fmt.Errorf("expr: unknown operator %s", op))
	}
	return
}

// extractScalar will return a float64 if res contains exactly one scalar.
func extractScalarXXX(v Value) interface{} {
	// if len(res.Results) == 1 && res.Results[0].Type() == TYPE_SCALAR {
	// 	return float64(res.Results[0].Value.Value().(Scalar))
	// }
	// return res
	return nil
}
