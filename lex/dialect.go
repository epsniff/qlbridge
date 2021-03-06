package lex

import (
	u "github.com/araddon/gou"
	"strings"
)

var _ = u.EMPTY

type Dialect struct {
	Name       string
	Statements []*Statement
}

func (m *Dialect) Init() {
	for _, s := range m.Statements {
		s.init()
	}
}

type Statement struct {
	Keyword TokenType
	Clauses []*Clause
}

func (m *Statement) init() {
	for _, clause := range m.Clauses {
		clause.init()
	}
}

type Clause struct {
	keyword   string
	multiWord bool
	Optional  bool
	Token     TokenType
	Lexer     StateFn
	Clauses   []*Clause
}

func (c *Clause) init() {
	c.keyword = strings.ToLower(c.Token.MatchString())
	c.multiWord = c.Token.MultiWord()
	for _, clause := range c.Clauses {
		clause.init()
	}
}
