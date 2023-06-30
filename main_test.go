package main

import (
	"github.com/stretchr/testify/assert"
	"math"
	"testing"
)

func TestGenerateProtobufDefinition(t *testing.T) {
	filePath := "testdata/example.go"
	structName := "MyStruct"

	expected := `message MyStruct {
  int32 field1 = 1;
  string field2 = 2;
  bool field3 = 3;
  repeated int64 field4 = 4;
  message NestedStruct {
    int32 field1 = 1;
    string field2 = 2;
    bool field3 = 3;
    repeated int64 field4 = 4;
  }
  NestedStruct nested = 5;
}`

	result, err := generateProtobufDefinition(filePath, structName, 0, math.MaxInt)
	if err != nil {
		t.Fatalf("Error generating protobuf definition: %s", err)
	}

	if result != expected {
		t.Errorf("Generated protobuf definition does not match.\nExpected:\n%s\nActual:\n%s", expected, result)
	}
}

func TestGenerateProtobufDefinitionPanic(t *testing.T) {
	filePath := "testdata/example.go"
	structName := "MyStructRecursive"

	assert.Panics(t, func() {
		_, _ = generateProtobufDefinition(filePath, structName, 0, math.MaxInt)
	})
}
