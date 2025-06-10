package main

type ast struct {
	packageStmt *packageStmt
	imports     []*importStmt
	funcs       []*funcStmt
	structs     []*structStmt
	interfaces  []*InterfaceStmt
	enums       []*EnumStmt
}
