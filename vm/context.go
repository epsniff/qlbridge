package vm

import (
	"fmt"
	u "github.com/araddon/gou"
	"net/url"
	"time"
)

var (
	_ ContextWriter = (*ContextSimple)(nil)
	_ ContextReader = (*ContextSimple)(nil)
	_ ContextWriter = (*ContextUrlValues)(nil)
	_ ContextReader = (*ContextUrlValues)(nil)
	_               = u.EMPTY
)

// Context Reader is interface to read the context of message/row/command
//  being evaluated
type ContextReader interface {
	Get(key string) (Value, bool)
	Row() map[string]Value
	Ts() time.Time
}

type ContextWriter interface {
	Put(col SchemaInfo, readCtx ContextReader, v Value) error
	Delete(row map[string]Value) error
}

// for commiting row ops (insert, update)
type RowWriter interface {
	Commit(rowInfo []SchemaInfo, row RowWriter) error
	Put(col SchemaInfo, readCtx ContextReader, v Value) error
	//Rows() []map[string]Value
}
type RowScanner interface {
	Next() map[string]Value
}
type ContextSimple struct {
	Data   map[string]Value
	Rows   []map[string]Value
	ts     time.Time
	cursor int
}

func NewContextSimple() *ContextSimple {
	return &ContextSimple{Data: make(map[string]Value), ts: time.Now(), cursor: 0}
}
func NewContextSimpleData(data map[string]Value) *ContextSimple {
	return &ContextSimple{Data: data, ts: time.Now(), cursor: 0}
}
func NewContextSimpleTs(data map[string]Value, ts time.Time) *ContextSimple {
	return &ContextSimple{Data: data, ts: ts, cursor: 0}
}

func (m ContextSimple) All() map[string]Value {
	return m.Data
}
func (m ContextSimple) Row() map[string]Value {
	return m.Data
}

func (m ContextSimple) Get(key string) (Value, bool) {
	val, ok := m.Data[key]
	return val, ok
}
func (m ContextSimple) Ts() time.Time {
	return m.ts
}

func (m *ContextSimple) Put(col SchemaInfo, rctx ContextReader, v Value) error {
	//u.Infof("put context:  %v %T:%v", col.Key(), v, v)
	m.Data[col.Key()] = v
	return nil
}
func (m *ContextSimple) Commit(rowInfo []SchemaInfo, row RowWriter) error {
	m.Rows = append(m.Rows, m.Data)
	m.Data = make(map[string]Value)
	return nil
}
func (m *ContextSimple) Insert(row map[string]Value) {
	m.Rows = append(m.Rows, row)
}
func (m *ContextSimple) Delete(delRow map[string]Value) error {
	for i, row := range m.Rows {
		foundMatch := true
		for delName, delVal := range delRow {
			if val, ok := row[delName]; !ok {
				// can't match so not in this row
				foundMatch = false
				break
			} else if val.Value() != delVal.Value() {
				foundMatch = false
				break
			} else {
				// nice, match
			}
		}
		if foundMatch {
			// we need to delete
			// a = append(a[:i], a[j:]...)
			//u.Infof("len=%d i=%d >?%v", len(m.Rows), i, len(m.Rows) > i+1)
			if i == 0 {
				m.Rows = m.Rows[1:]
			} else if len(m.Rows) > i+1 {
				m.Rows = append(m.Rows[:i-1], m.Rows[i+1:]...)
			} else {
				m.Rows = m.Rows[:i-1]
			}

			return nil
		}
	}
	return nil
}
func (m *ContextSimple) DeleteMatch(delRow map[string]Value) error {
	rowsToDelete := make(map[int]struct{})
	for i, row := range m.Rows {
		foundMatch := true
		for delName, delVal := range delRow {
			if val, ok := row[delName]; !ok {
				// can't match so not in this row
				foundMatch = false
				break
			} else if val.Value() != delVal.Value() {
				foundMatch = false
				break
			} else {
				// nice, match
			}
		}
		if foundMatch {
			// we need to delete
			rowsToDelete[i] = struct{}{}
		}
	}
	if len(rowsToDelete) > 0 {
		newRows := make([]map[string]Value, 0)
		for i, row := range m.Rows {
			if _, ok := rowsToDelete[i]; !ok {
				newRows = append(newRows, row)
			}
		}
		m.Rows = newRows
	}
	return nil
}
func (m *ContextSimple) Next() map[string]Value {
	if len(m.Rows) <= m.cursor {
		return nil
	}
	m.Data = m.Rows[m.cursor]
	m.cursor++
	return m.Data
}

type ContextUrlValues struct {
	Data url.Values
	ts   time.Time
}

func NewContextUrlValues(uv url.Values) *ContextUrlValues {
	return &ContextUrlValues{uv, time.Now()}
}
func NewContextUrlValuesTs(uv url.Values, ts time.Time) *ContextUrlValues {
	return &ContextUrlValues{uv, ts}
}
func (m ContextUrlValues) Get(key string) (Value, bool) {
	vals, ok := m.Data[key]
	if ok {
		if len(vals) == 1 {
			return NewStringValue(vals[0]), true
		}
		return NewStringsValue(vals), true
	}
	return EmptyStringValue, false
}
func (m ContextUrlValues) Row() map[string]Value {
	mi := make(map[string]Value)
	for k, v := range m.Data {
		if len(v) == 1 {
			mi[k] = NewStringValue(v[0])
		} else if len(v) > 1 {
			mi[k] = NewStringsValue(v)
		}
	}
	return mi
}
func (m *ContextUrlValues) Delete(delRow map[string]Value) error {
	return fmt.Errorf("Not implemented")
}
func (m ContextUrlValues) Ts() time.Time {
	return m.ts
}

func (m ContextUrlValues) Put(col SchemaInfo, rctx ContextReader, v Value) error {
	key := col.Key()
	switch typedValue := v.(type) {
	case StringValue:
		m.Data.Set(key, typedValue.v)
	case NumberValue:
		m.Data.Set(key, typedValue.ToString())
	}
	return nil
}
