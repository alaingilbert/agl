package main

type ast struct {
	packageStmt *packageStmt
	imports     []*importStmt
	funcs       []*FuncExpr
	structs     []*structStmt
	interfaces  []*InterfaceStmt
	enums       []*EnumStmt
}
