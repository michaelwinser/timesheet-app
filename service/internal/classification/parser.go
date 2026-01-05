package classification

import (
	"fmt"
	"strings"
	"unicode"
)

// QueryNode represents a node in the query AST
type QueryNode interface {
	isQueryNode()
}

// ConditionNode represents a single condition (property:value)
type ConditionNode struct {
	Property string
	Value    string
	Negated  bool // For future -property:value support
}

func (ConditionNode) isQueryNode() {}

// AndNode represents an AND of multiple conditions (implicit)
type AndNode struct {
	Children []QueryNode
}

func (AndNode) isQueryNode() {}

// OrNode represents an OR of conditions (explicit with OR keyword)
type OrNode struct {
	Children []QueryNode
}

func (OrNode) isQueryNode() {}

// ParseError represents a parsing error
type ParseError struct {
	Message  string
	Position int
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error at position %d: %s", e.Position, e.Message)
}

// Parser parses Gmail-style query strings
type Parser struct {
	input   string
	pos     int
	tokens  []token
	current int
}

type tokenType int

const (
	tokenProperty tokenType = iota
	tokenValue
	tokenColon
	tokenQuote
	tokenLParen
	tokenRParen
	tokenOR
	tokenNegate
	tokenEOF
)

type token struct {
	typ   tokenType
	value string
	pos   int
}

// Parse parses a query string into an AST
func Parse(query string) (QueryNode, error) {
	p := &Parser{input: strings.TrimSpace(query)}
	if err := p.tokenize(); err != nil {
		return nil, err
	}
	return p.parse()
}

func (p *Parser) tokenize() error {
	p.tokens = nil

	for p.pos < len(p.input) {
		ch := p.input[p.pos]

		switch {
		case unicode.IsSpace(rune(ch)):
			p.pos++

		case ch == '(':
			p.tokens = append(p.tokens, token{tokenLParen, "(", p.pos})
			p.pos++

		case ch == ')':
			p.tokens = append(p.tokens, token{tokenRParen, ")", p.pos})
			p.pos++

		case ch == '-':
			p.tokens = append(p.tokens, token{tokenNegate, "-", p.pos})
			p.pos++

		case ch == ':':
			p.tokens = append(p.tokens, token{tokenColon, ":", p.pos})
			p.pos++

		case ch == '"':
			// Quoted string
			start := p.pos
			p.pos++ // skip opening quote
			valueStart := p.pos
			for p.pos < len(p.input) && p.input[p.pos] != '"' {
				p.pos++
			}
			if p.pos >= len(p.input) {
				return &ParseError{Message: "unclosed quote", Position: start}
			}
			p.tokens = append(p.tokens, token{tokenValue, p.input[valueStart:p.pos], start})
			p.pos++ // skip closing quote

		default:
			// Word (property or value or OR)
			start := p.pos
			for p.pos < len(p.input) {
				ch := p.input[p.pos]
				if unicode.IsSpace(rune(ch)) || ch == ':' || ch == '(' || ch == ')' || ch == '"' {
					break
				}
				p.pos++
			}
			word := p.input[start:p.pos]
			if strings.ToUpper(word) == "OR" {
				p.tokens = append(p.tokens, token{tokenOR, "OR", start})
			} else {
				p.tokens = append(p.tokens, token{tokenProperty, word, start})
			}
		}
	}

	p.tokens = append(p.tokens, token{tokenEOF, "", p.pos})
	return nil
}

func (p *Parser) parse() (QueryNode, error) {
	p.current = 0
	return p.parseOr()
}

func (p *Parser) parseOr() (QueryNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}

	var children []QueryNode
	children = append(children, left)

	for p.peek().typ == tokenOR {
		p.advance() // consume OR
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		children = append(children, right)
	}

	if len(children) == 1 {
		return children[0], nil
	}

	return &OrNode{Children: children}, nil
}

func (p *Parser) parseAnd() (QueryNode, error) {
	var children []QueryNode

	for {
		tok := p.peek()
		if tok.typ == tokenEOF || tok.typ == tokenRParen || tok.typ == tokenOR {
			break
		}

		node, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		if node != nil {
			children = append(children, node)
		}
	}

	if len(children) == 0 {
		return nil, &ParseError{Message: "empty expression", Position: p.peek().pos}
	}

	if len(children) == 1 {
		return children[0], nil
	}

	return &AndNode{Children: children}, nil
}

func (p *Parser) parsePrimary() (QueryNode, error) {
	tok := p.peek()

	switch tok.typ {
	case tokenLParen:
		p.advance() // consume (
		node, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek().typ != tokenRParen {
			return nil, &ParseError{Message: "expected )", Position: p.peek().pos}
		}
		p.advance() // consume )
		return node, nil

	case tokenNegate:
		p.advance() // consume -
		node, err := p.parseCondition(true)
		if err != nil {
			return nil, err
		}
		return node, nil

	case tokenProperty:
		return p.parseCondition(false)

	default:
		return nil, &ParseError{Message: fmt.Sprintf("unexpected token: %s", tok.value), Position: tok.pos}
	}
}

func (p *Parser) parseCondition(negated bool) (QueryNode, error) {
	propTok := p.peek()
	if propTok.typ != tokenProperty {
		return nil, &ParseError{Message: "expected property name", Position: propTok.pos}
	}
	p.advance()

	colonTok := p.peek()
	if colonTok.typ != tokenColon {
		// No colon - treat as text search (unqualified term)
		return &ConditionNode{
			Property: "text",
			Value:    propTok.value,
			Negated:  negated,
		}, nil
	}
	p.advance()

	valueTok := p.peek()
	if valueTok.typ != tokenProperty && valueTok.typ != tokenValue {
		return nil, &ParseError{Message: "expected value", Position: valueTok.pos}
	}
	p.advance()

	return &ConditionNode{
		Property: strings.ToLower(propTok.value),
		Value:    valueTok.value,
		Negated:  negated,
	}, nil
}

func (p *Parser) peek() token {
	if p.current >= len(p.tokens) {
		return token{tokenEOF, "", len(p.input)}
	}
	return p.tokens[p.current]
}

func (p *Parser) advance() token {
	tok := p.peek()
	p.current++
	return tok
}

// String returns a string representation of the query node (for debugging)
func (n *ConditionNode) String() string {
	prefix := ""
	if n.Negated {
		prefix = "-"
	}
	if strings.Contains(n.Value, " ") {
		return fmt.Sprintf("%s%s:\"%s\"", prefix, n.Property, n.Value)
	}
	return fmt.Sprintf("%s%s:%s", prefix, n.Property, n.Value)
}

func (n *AndNode) String() string {
	parts := make([]string, len(n.Children))
	for i, child := range n.Children {
		parts[i] = nodeToString(child)
	}
	return strings.Join(parts, " ")
}

func (n *OrNode) String() string {
	parts := make([]string, len(n.Children))
	for i, child := range n.Children {
		parts[i] = nodeToString(child)
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

func nodeToString(n QueryNode) string {
	switch node := n.(type) {
	case *ConditionNode:
		return node.String()
	case *AndNode:
		return node.String()
	case *OrNode:
		return node.String()
	default:
		return ""
	}
}
