package testdata

type NestedStruct struct {
	Field1 int32
	Field2 string
	Field3 bool
	Field4 []int64
}

type MyStruct struct {
	Field1 int32
	Field2 string
	Field3 bool
	Field4 []int64
	Nested *NestedStruct
}

type MyStructRecursive struct {
	Field1 int32
	Field2 string
	Field3 bool
	Field4 []int64
	Nested *MyStructRecursive
}
