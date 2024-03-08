package parser

import (
	"encoding/json"
	"fmt"
	"golang.org/x/exp/maps"
	"slices"
	"strings"
)

type Scope struct {
	Region int
	Nested int

	parent        *Scope
	Children      *[]*Scope
	Table         map[string][]Symbol
	regionCounter *int
}

type Type int

const (
	Int Type = iota
	Char
	Bool
	Float
	Rec     = "rec"
	Func    = "func"
	Proc    = "proc"
	Unknown = "unknown"
)

type Symbol interface {
	// Name returns the name of the symbol
	Name() string
	// Type returns the type of the symbol
	Type() string
}

type Variable struct {
	VName      string
	SType      string
	IsParamIn  bool
	IsParamOut bool
	IsLoop     bool
	Offset     int
}

type Function struct {
	FName      string
	SType      string
	ParamCount int
	Params     map[int]*Variable
	ReturnType string
	children   []int
}

type Procedure struct {
	PName      string
	PType      string
	ParamCount int
	Params     map[int]*Variable
	children   []int
}

type Record struct {
	RName  string
	SType  string
	Fields map[string]string
}

func (v Variable) Name() string {
	return v.VName
}

func (v Variable) Type() string {
	return v.SType
}

func (f Function) Name() string {
	return f.FName
}

func (f Function) Type() string {
	return f.SType
}

func (p Procedure) Name() string {
	return p.PName
}

func (p Procedure) Type() string {
	return p.PType
}

func (r Record) Name() string {
	return r.RName
}

func (r Record) Type() string {
	return r.SType
}

func (r Record) Offset() int {
	return 0
}

func getSymbolType(symbol string) string {
	return strings.ToLower(symbol)
}

func newScope(parent *Scope) *Scope {
	var regionCounter *int
	var scope *Scope
	if parent == nil {
		regionCounter = new(int)
		*regionCounter = 0

		scope = &Scope{parent: nil, Table: make(map[string][]Symbol), Children: &[]*Scope{}, regionCounter: regionCounter, Region: 0, Nested: 0}
	} else {
		*parent.regionCounter++
		scope = &Scope{parent: parent, Table: make(map[string][]Symbol), Children: &[]*Scope{}, regionCounter: parent.regionCounter, Region: *parent.regionCounter, Nested: parent.Nested + 1}
		*parent.Children = append(*parent.Children, scope)
		fmt.Println("new scope", scope.Region, len(*parent.Children))
	}

	return scope
}

func (scope *Scope) getCurrentOffset() int {
	maxOffset := 0
	for _, symbols := range scope.Table {
		for _, symbol := range symbols {
			if symbol, ok := symbol.(Variable); ok {
				if symbol.Offset > maxOffset {
					maxOffset = symbol.Offset
				}
			}
		}
	}
	return maxOffset
}

func (scope *Scope) String() string {
	return fmt.Sprintf("Region: %d, Nested: %d, Table: %v", scope.Region, scope.Nested, scope.Table)
}

func (scope *Scope) addSymbol(symbol Symbol) {
	name := symbol.Name()
	if existingSymbols, ok := scope.Table[name]; ok {
		// Array already exists, append the symbol to it
		scope.Table[name] = append(existingSymbols, symbol)
	} else {
		// Array doesn't exist, create a new array with the symbol
		scope.Table[name] = []Symbol{symbol}
	}
}

func ReadAST(graph Graph) (*Scope, error) {
	fileScope := newScope(nil)
	currentScope := *fileScope
	fileNodeIndex := 0
	currentScope.addSymbol(Procedure{PName: "Put", PType: Proc, ParamCount: 1, Params: map[int]*Variable{1: &Variable{VName: "x", SType: "character"}}, children: []int{}})
	dfsSymbols(graph, fileNodeIndex, &currentScope)

	// fileScope to json
	b, err := json.MarshalIndent(fileScope, "", "  ")
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	fmt.Println(string(b))
	return fileScope, nil
}

func handleInOut(graph Graph, children []int, name string) *Variable {
	if len(children) == 3 {
		newParam := &Variable{VName: name, SType: getSymbolType(graph.types[children[2]])}
		if graph.types[children[1]] == "out" {
			newParam.IsParamIn = false
			newParam.IsParamOut = true
		} else if graph.types[children[1]] == "in" {
			newParam.IsParamIn = true
			newParam.IsParamOut = false
		} else {
			newParam.IsParamIn = true
			newParam.IsParamOut = true
		}
		return newParam
	} else {
		newParam := &Variable{VName: name, SType: getSymbolType(graph.types[children[1]])}
		newParam.IsParamIn = true
		newParam.IsParamOut = false
		return newParam
	}
}

func addParam(graph Graph, node int, currentFunc *Function, funcScope *Scope) {
	children := maps.Keys(graph.gmap[node])
	slices.Sort(children)
	if graph.types[children[0]] == "sameType" {
		childrenchildren := maps.Keys(graph.gmap[children[0]])
		slices.Sort(childrenchildren)
		for _, child := range childrenchildren {
			currentFunc.ParamCount++
			newParam := handleInOut(graph, children, graph.types[child])
			currentFunc.Params[currentFunc.ParamCount] = newParam
			funcScope.addSymbol(*newParam)
		}
	} else {
		currentFunc.ParamCount++
		newParam := handleInOut(graph, children, graph.types[children[0]])
		currentFunc.Params[currentFunc.ParamCount] = newParam
		funcScope.addSymbol(*newParam)
	}
}

func addParamProc(graph Graph, node int, currentProc *Procedure, procScope *Scope) {
	if graph.types[node] == "param" {
		children := maps.Keys(graph.gmap[node])
		slices.Sort(children)
		if graph.types[children[0]] == "sameType" {
			for _, child := range maps.Keys(graph.gmap[children[0]]) {
				currentProc.ParamCount++
				newParam := handleInOut(graph, children, graph.types[child])
				currentProc.Params[currentProc.ParamCount] = newParam
				procScope.addSymbol(*newParam)
			}
		} else {
			currentProc.ParamCount++
			newParam := handleInOut(graph, children, graph.types[children[0]])
			currentProc.Params[currentProc.ParamCount] = newParam
			procScope.addSymbol(*newParam)
		}
	}
}

func dfsSymbols(graph Graph, node int, currentScope *Scope) {
	sorted := maps.Keys(graph.gmap[node])
	slices.Sort(sorted)
	scope := *currentScope

	switch graph.types[node] {
	case "file":
		shift := 0
		if graph.types[sorted[1]] == "decl" {
			children := maps.Keys(graph.gmap[sorted[1]])
			for _, child := range children {
				dfsSymbols(graph, child, currentScope)
			}
			shift++
		}
		dfsSymbols(graph, sorted[1+shift], currentScope)
	case "function":
		funcParam := make(map[int]*Variable)
		funcElem := Function{FName: graph.types[sorted[0]], SType: Func, children: sorted, Params: funcParam}
		funcScope := newScope(&scope)
		shift := 0
		if graph.types[sorted[1]] == "params" {
			child := maps.Keys(graph.gmap[sorted[1]])
			slices.Sort(child)
			for _, param := range child {
				addParam(graph, param, &funcElem, funcScope)
			}
			shift = 1
		}
		funcElem.ReturnType = getSymbolType(graph.types[sorted[1+shift]])
		scope.addSymbol(funcElem)
		if graph.types[sorted[2+shift]] == "decl" {

			children := maps.Keys(graph.gmap[sorted[2+shift]])
			for _, child := range children {
				dfsSymbols(graph, child, funcScope)
			}
			shift++
		}
		dfsSymbols(graph, sorted[2+shift], funcScope)
	case "procedure":
		procParam := make(map[int]*Variable)
		procElem := Procedure{PName: graph.types[sorted[0]], PType: Proc, children: sorted, Params: procParam}
		procScope := newScope(&scope)
		shift := 0
		if graph.types[sorted[1]] == "params" {
			child := maps.Keys(graph.gmap[sorted[1]])
			slices.Sort(child)
			for _, param := range child {
				addParamProc(graph, param, &procElem, procScope)
			}
			scope.addSymbol(procElem)
			shift = 1
		}
		if graph.types[sorted[1+shift]] == "decl" {
			children := maps.Keys(graph.gmap[sorted[1+shift]])
			for _, child := range children {
				dfsSymbols(graph, child, procScope)
			}
			shift++
		}
		dfsSymbols(graph, sorted[1+shift], procScope)
	case "for":
		forScope := newScope(&scope)
		forScope.addSymbol(Variable{VName: graph.types[sorted[0]], SType: "integer", IsLoop: true})
		for _, child := range sorted {
			dfsSymbols(graph, child, forScope)
		}
	case "var":
		currentOffset := scope.getCurrentOffset()
		if graph.types[sorted[0]] == "sameType" {
			for _, k := range maps.Keys(graph.gmap[sorted[0]]) {
				currentOffset += getTypeSize(getSymbolType(graph.types[sorted[1]]), scope)
				scope.addSymbol(Variable{VName: graph.types[k], SType: getSymbolType(graph.types[sorted[1]]), Offset: currentOffset})
			}
		} else {
			currentOffset += getTypeSize(getSymbolType(graph.types[sorted[1]]), scope)
			scope.addSymbol(Variable{VName: graph.types[sorted[0]], SType: getSymbolType(graph.types[sorted[1]]), Offset: currentOffset})
		}
	case "type":
		recordElem := Record{RName: getSymbolType(graph.types[sorted[0]]), SType: Rec, Fields: make(map[string]string)}
		for _, child := range maps.Keys(graph.gmap[sorted[1]]) {
			childChild := maps.Keys(graph.gmap[child])
			slices.Sort(childChild)
			recordElem.Fields[graph.types[childChild[0]]] = getSymbolType(graph.types[childChild[1]])
		}
		scope.addSymbol(recordElem)
	default:
		for _, child := range sorted {
			dfsSymbols(graph, child, currentScope)
		}
	}
	graph.scopes[node] = &scope
}
