// +build go1.13

package blackfire

import (
	"reflect"
)

func valueIsZero(v reflect.Value) bool {
	return v.IsZero()
}
