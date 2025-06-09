package main

type ast struct {
	packageStmt *packageStmt
	imports     []*importStmt
	funcs       []*funcStmt
	structs     []*structStmt
	enums       []*EnumStmt
}
