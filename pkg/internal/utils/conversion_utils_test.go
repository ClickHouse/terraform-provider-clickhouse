package utils

import (
	"math/big"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestConvertTerraformValueToJSON(t *testing.T) {
	t.Run("String value", func(t *testing.T) {
		input := types.StringValue("hello")
		result := ConvertTerraformValueToJSON(input)
		if result != "hello" {
			t.Errorf("ConvertTerraformValueToJSON() = %v, want %v", result, "hello")
		}
	})

	t.Run("Bool value true", func(t *testing.T) {
		input := types.BoolValue(true)
		result := ConvertTerraformValueToJSON(input)
		if result != true {
			t.Errorf("ConvertTerraformValueToJSON() = %v, want %v", result, true)
		}
	})

	t.Run("Bool value false", func(t *testing.T) {
		input := types.BoolValue(false)
		result := ConvertTerraformValueToJSON(input)
		if result != false {
			t.Errorf("ConvertTerraformValueToJSON() = %v, want %v", result, false)
		}
	})

	t.Run("Number value int64", func(t *testing.T) {
		input := types.NumberValue(big.NewFloat(42))
		result := ConvertTerraformValueToJSON(input)
		if result != int64(42) {
			t.Errorf("ConvertTerraformValueToJSON() = %v, want %v", result, int64(42))
		}
	})

	t.Run("Number value float64", func(t *testing.T) {
		input := types.NumberValue(big.NewFloat(3.14))
		result := ConvertTerraformValueToJSON(input)
		if result != 3.14 {
			t.Errorf("ConvertTerraformValueToJSON() = %v, want %v", result, 3.14)
		}
	})

	t.Run("Dynamic value with string", func(t *testing.T) {
		input := types.DynamicValue(types.StringValue("dynamic"))
		result := ConvertTerraformValueToJSON(input)
		if result != "dynamic" {
			t.Errorf("ConvertTerraformValueToJSON() = %v, want %v", result, "dynamic")
		}
	})
}

func TestConvertJSONValueToTerraform(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
		checkFn  func(t *testing.T, result interface{})
	}{
		{
			name:  "String value",
			input: "hello",
			checkFn: func(t *testing.T, result interface{}) {
				strVal, ok := result.(types.String)
				if !ok {
					t.Errorf("Expected types.String, got %T", result)
					return
				}
				if strVal.ValueString() != "hello" {
					t.Errorf("Expected 'hello', got %s", strVal.ValueString())
				}
			},
		},
		{
			name:  "Bool value true",
			input: true,
			checkFn: func(t *testing.T, result interface{}) {
				boolVal, ok := result.(types.Bool)
				if !ok {
					t.Errorf("Expected types.Bool, got %T", result)
					return
				}
				if !boolVal.ValueBool() {
					t.Errorf("Expected true, got false")
				}
			},
		},
		{
			name:  "Bool value false",
			input: false,
			checkFn: func(t *testing.T, result interface{}) {
				boolVal, ok := result.(types.Bool)
				if !ok {
					t.Errorf("Expected types.Bool, got %T", result)
					return
				}
				if boolVal.ValueBool() {
					t.Errorf("Expected false, got true")
				}
			},
		},
		{
			name:  "Int64 value",
			input: int64(42),
			checkFn: func(t *testing.T, result interface{}) {
				numVal, ok := result.(types.Number)
				if !ok {
					t.Errorf("Expected types.Number, got %T", result)
					return
				}
				intVal, _ := numVal.ValueBigFloat().Int64()
				if intVal != 42 {
					t.Errorf("Expected 42, got %d", intVal)
				}
			},
		},
		{
			name:  "Float64 value",
			input: 3.14,
			checkFn: func(t *testing.T, result interface{}) {
				numVal, ok := result.(types.Number)
				if !ok {
					t.Errorf("Expected types.Number, got %T", result)
					return
				}
				floatVal, _ := numVal.ValueBigFloat().Float64()
				if floatVal != 3.14 {
					t.Errorf("Expected 3.14, got %f", floatVal)
				}
			},
		},
		{
			name:  "Int value",
			input: 100,
			checkFn: func(t *testing.T, result interface{}) {
				numVal, ok := result.(types.Number)
				if !ok {
					t.Errorf("Expected types.Number, got %T", result)
					return
				}
				intVal, _ := numVal.ValueBigFloat().Int64()
				if intVal != 100 {
					t.Errorf("Expected 100, got %d", intVal)
				}
			},
		},
		{
			name:  "Float32 value",
			input: float32(2.5),
			checkFn: func(t *testing.T, result interface{}) {
				numVal, ok := result.(types.Number)
				if !ok {
					t.Errorf("Expected types.Number, got %T", result)
					return
				}
				floatVal, _ := numVal.ValueBigFloat().Float64()
				if floatVal != 2.5 {
					t.Errorf("Expected 2.5, got %f", floatVal)
				}
			},
		},
		{
			name:  "Unknown type fallback",
			input: []int{1, 2, 3},
			checkFn: func(t *testing.T, result interface{}) {
				strVal, ok := result.(types.String)
				if !ok {
					t.Errorf("Expected types.String, got %T", result)
					return
				}
				if strVal.ValueString() != "[1 2 3]" {
					t.Errorf("Expected '[1 2 3]', got %s", strVal.ValueString())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertJSONValueToTerraform(tt.input)
			tt.checkFn(t, result)
		})
	}
}
