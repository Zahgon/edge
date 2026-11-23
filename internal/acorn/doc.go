// Package acorn is a Go port of acorn 8.19.0, the JavaScript parser edge-parser uses
// to read the expressions of Edge templates.
//
// It parses scripts with acorn's grammar up to ECMAScript 2020 into ESTree nodes
// (see Node), with acorn's locations, its onToken stream, its scope and strict-mode
// checks, its regular expression validator, and its error messages and positions.
// Syntax of later editions (numeric separators, logical assignment, class fields,
// private names, ...) fails exactly as it fails in acorn with ecmaVersion 2020.
// Positions and columns count UTF-16 code units as JavaScript does.
//
// Under V8, acorn turns a stack overflow on deeply nested input into the error
// "Not enough stack space to parse input"; where V8 runs out of stack depends on
// its frame sizes, and Go's stacks grow, so this port parses any nesting depth.
//
// acorn is MIT licensed: Copyright (C) 2012-2022 by various contributors (see
// AUTHORS in the acorn distribution). The permission notice of the MIT License
// applies to this port as to the original.
package acorn
