package utils

import (
	"fmt"
	"math/big"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ConvertTerraformValueToJSON converts a Terraform attr.Value to its corresponding Go value
func ConvertTerraformValueToJSON(value attr.Value) any {
	// Handle dynamic values by extracting the underlying value
	if dynamicValue, ok := value.(types.Dynamic); ok {
		value = dynamicValue.UnderlyingValue()
	}

	switch v := value.(type) {
	case types.String:
		return v.ValueString()
	case types.Bool:
		return v.ValueBool()
	case types.Number:
		if intVal, accuracy := v.ValueBigFloat().Int64(); accuracy == big.Exact {
			return intVal
		} else if floatVal, accuracy := v.ValueBigFloat().Float64(); accuracy == big.Exact {
			return floatVal
		} else {
			return v.ValueBigFloat().String()
		}
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ConvertJSONValueToTerraform converts a Go interface{} value to its corresponding Terraform attr.Value
func ConvertJSONValueToTerraform(value any) attr.Value {
	switch v := value.(type) {
	case string:
		return types.StringValue(v)
	case bool:
		return types.BoolValue(v)
	case int64:
		return types.NumberValue(big.NewFloat(float64(v)))
	case float64:
		return types.NumberValue(big.NewFloat(v))
	case int:
		return types.NumberValue(big.NewFloat(float64(v)))
	case float32:
		return types.NumberValue(big.NewFloat(float64(v)))
	default:
		// Fallback to string representation
		return types.StringValue(fmt.Sprintf("%v", v))
	}
}