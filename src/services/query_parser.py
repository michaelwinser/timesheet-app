"""
Query parser for Rules v2.

Parses Gmail-style query syntax:
    domain:foo.com title:"weekly meeting" (domain:a.com OR domain:b.com)

Grammar:
    query      := expression*
    expression := group | term
    group      := '(' expression ('OR' expression)* ')'
    term       := property ':' value
    property   := [a-z-]+
    value      := quoted_string | unquoted_string
    quoted_string := '"' [^"]* '"'
    unquoted_string := [^\s()]+
"""

import re
from dataclasses import dataclass
from typing import Any


@dataclass
class Term:
    """A single property:value condition."""
    property: str
    value: str

    def __repr__(self):
        if ' ' in self.value:
            return f'{self.property}:"{self.value}"'
        return f'{self.property}:{self.value}'


@dataclass
class OrGroup:
    """A group of terms joined by OR."""
    terms: list['Term | OrGroup']

    def __repr__(self):
        return '(' + ' OR '.join(str(t) for t in self.terms) + ')'


@dataclass
class AndGroup:
    """Top-level AND of terms and groups."""
    items: list[Term | OrGroup]

    def __repr__(self):
        return ' '.join(str(item) for item in self.items)


class ParseError(Exception):
    """Raised when query syntax is invalid."""
    pass


class QueryParser:
    """Parser for query syntax."""

    # Known properties and their types
    PROPERTIES = {
        # String properties (contains matching)
        'title': 'string',
        'description': 'string',

        # Smart attendee matching
        'attendees': 'attendees',  # matches name or email
        'domain': 'domain',        # extracts domain from attendee emails
        'email': 'email',          # exact email match in attendees

        # Enum properties (exact matching)
        'response': 'enum',        # accepted, declined, tentative, needsAction
        'transparency': 'enum',    # opaque, transparent
        'visibility': 'enum',      # default, public, private, confidential
        'day-of-week': 'enum',     # mon, tue, wed, thu, fri, sat, sun
        'color': 'enum',           # calendar color ID

        # Boolean properties
        'recurring': 'boolean',
        'is-all-day': 'boolean',
        'has-attendees': 'boolean',

        # Other
        'time-of-day': 'time',     # HH:MM with optional > or < prefix
        'recurrence-id': 'string',
    }

    def __init__(self, query: str):
        self.query = query
        self.pos = 0
        self.length = len(query)

    def parse(self) -> AndGroup:
        """Parse the query string into an AST."""
        items = []

        while self.pos < self.length:
            self._skip_whitespace()
            if self.pos >= self.length:
                break

            if self._peek() == '(':
                items.append(self._parse_group())
            else:
                term = self._parse_term()
                if term:
                    items.append(term)

        return AndGroup(items=items)

    def _skip_whitespace(self):
        """Skip whitespace characters."""
        while self.pos < self.length and self.query[self.pos] in ' \t\n\r':
            self.pos += 1

    def _peek(self, n: int = 1) -> str:
        """Peek at the next n characters without advancing."""
        return self.query[self.pos:self.pos + n]

    def _advance(self, n: int = 1) -> str:
        """Advance position and return consumed characters."""
        result = self.query[self.pos:self.pos + n]
        self.pos += n
        return result

    def _parse_group(self) -> OrGroup:
        """Parse a parenthesized group: (term OR term OR ...)"""
        self._advance()  # consume '('
        self._skip_whitespace()

        terms = []

        while self.pos < self.length and self._peek() != ')':
            self._skip_whitespace()

            # Check for nested group
            if self._peek() == '(':
                terms.append(self._parse_group())
            else:
                # Check for OR keyword
                if self._peek(2).upper() == 'OR' and (
                    self.pos + 2 >= self.length or
                    self.query[self.pos + 2] in ' \t\n\r)'
                ):
                    self._advance(2)  # consume 'OR'
                    self._skip_whitespace()
                    continue

                term = self._parse_term()
                if term:
                    terms.append(term)

        if self.pos < self.length and self._peek() == ')':
            self._advance()  # consume ')'
        else:
            raise ParseError("Unclosed parenthesis")

        return OrGroup(terms=terms)

    def _parse_term(self) -> Term | None:
        """Parse a single property:value term."""
        self._skip_whitespace()

        # Parse property name
        prop_match = re.match(r'([a-z][a-z0-9-]*)', self.query[self.pos:], re.IGNORECASE)
        if not prop_match:
            return None

        prop = prop_match.group(1).lower()
        self.pos += len(prop)

        # Expect colon
        if self.pos >= self.length or self._peek() != ':':
            raise ParseError(f"Expected ':' after property '{prop}'")
        self._advance()  # consume ':'

        # Parse value
        value = self._parse_value()

        # Validate property
        if prop not in self.PROPERTIES:
            # Allow unknown properties (for extensibility)
            pass

        return Term(property=prop, value=value)

    def _parse_value(self) -> str:
        """Parse a value (quoted or unquoted)."""
        if self._peek() == '"':
            return self._parse_quoted_string()
        else:
            return self._parse_unquoted_string()

    def _parse_quoted_string(self) -> str:
        """Parse a double-quoted string."""
        self._advance()  # consume opening quote

        result = []
        while self.pos < self.length:
            char = self._peek()
            if char == '"':
                self._advance()  # consume closing quote
                return ''.join(result)
            elif char == '\\' and self.pos + 1 < self.length:
                self._advance()  # consume backslash
                result.append(self._advance())  # consume escaped char
            else:
                result.append(self._advance())

        raise ParseError("Unclosed quoted string")

    def _parse_unquoted_string(self) -> str:
        """Parse an unquoted value (until whitespace or special char)."""
        result = []
        while self.pos < self.length:
            char = self._peek()
            if char in ' \t\n\r()':
                break
            result.append(self._advance())

        if not result:
            raise ParseError("Expected value")

        return ''.join(result)


def parse_query(query: str) -> AndGroup:
    """Parse a query string into an AST.

    Args:
        query: Query string like 'domain:foo.com title:"meeting"'

    Returns:
        AndGroup containing parsed terms and groups

    Raises:
        ParseError: If query syntax is invalid
    """
    if not query or not query.strip():
        return AndGroup(items=[])

    parser = QueryParser(query.strip())
    return parser.parse()


def query_to_string(ast: AndGroup) -> str:
    """Convert AST back to query string (for normalization/display)."""
    return str(ast)
